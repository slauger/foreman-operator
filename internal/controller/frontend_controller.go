package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	foremanv1alpha1 "github.com/slauger/foreman-operator/api/v1alpha1"
)

// FrontendReconciler reconciles a Frontend object.
type FrontendReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Images       Images
	Capabilities Capabilities
}

// +kubebuilder:rbac:groups=platform.theforeman.org,resources=frontends,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=platform.theforeman.org,resources=frontends/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tlsroutes;httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes;routes/custom-host,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives a Frontend towards its spec.
func (r *FrontendReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	fe := &foremanv1alpha1.Frontend{}
	if err := r.Get(ctx, req.NamespacedName, fe); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	original := fe.DeepCopy()

	result, err := r.reconcile(ctx, fe)
	if err != nil {
		fe.Status.Phase = string(foremanv1alpha1.ForemanPhaseFailed)
		setCondition(&fe.Status.Conditions, fe.Generation, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonReconcileError, err.Error())
	}
	fe.Status.ObservedGeneration = fe.Generation
	if statusErr := r.Status().Patch(ctx, fe, client.MergeFrom(original)); statusErr != nil && !apierrors.IsNotFound(statusErr) {
		return ctrl.Result{}, statusErr
	}
	return result, err
}

func (r *FrontendReconciler) reconcile(ctx context.Context, fe *foremanv1alpha1.Frontend) (ctrl.Result, error) {
	if isPaused(fe) {
		setCondition(&fe.Status.Conditions, fe.Generation, foremanv1alpha1.ConditionPaused, metav1.ConditionTrue, reasonPaused, "reconciliation is paused by annotation")
		return ctrl.Result{}, nil
	}
	setCondition(&fe.Status.Conditions, fe.Generation, foremanv1alpha1.ConditionPaused, metav1.ConditionFalse, reasonReconciled, "")

	f := &foremanv1alpha1.Foreman{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: fe.Namespace, Name: fe.Spec.ForemanRef}, f); err != nil {
		if apierrors.IsNotFound(err) {
			fe.Status.Phase = string(foremanv1alpha1.ForemanPhasePending)
			setCondition(&fe.Status.Conditions, fe.Generation, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonWaitingForeman,
				fmt.Sprintf("Foreman %q does not exist", fe.Spec.ForemanRef))
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if err := r.ensureCertificates(ctx, fe, f); err != nil {
		return ctrl.Result{}, err
	}

	config, err := frontendConfig(f)
	if err != nil {
		return ctrl.Result{}, err
	}
	data := map[string]string{"foreman.conf": config}
	cm := &corev1.ConfigMap{}
	cm.Name = fe.Name
	cm.Namespace = fe.Namespace
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Labels = commonLabels(frontendAppName, fe.Name, componentFrontend)
		cm.Data = data
		return controllerutil.SetControllerReference(fe, cm, r.Scheme)
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("configuration: %w", err)
	}

	desired := frontendDeployment(fe, r.Images, r.Capabilities, checksumMap(data))
	d := &appsv1.Deployment{}
	d.Name = desired.Name
	d.Namespace = desired.Namespace
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, d, func() error {
		d.Labels = desired.Labels
		d.Spec.Replicas = desired.Spec.Replicas
		if d.Spec.Selector == nil {
			d.Spec.Selector = desired.Spec.Selector
		}
		d.Spec.Template = desired.Spec.Template
		return controllerutil.SetControllerReference(fe, d, r.Scheme)
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("deployment: %w", err)
	}

	if err := r.ensureService(ctx, fe); err != nil {
		return ctrl.Result{}, fmt.Errorf("service: %w", err)
	}
	if err := r.ensureExposure(ctx, fe, f); err != nil {
		return ctrl.Result{}, fmt.Errorf("exposure: %w", err)
	}

	if !deploymentAvailable(d.Status.AvailableReplicas, min(*desired.Spec.Replicas, 1), d.Status.ObservedGeneration, d.Generation) {
		fe.Status.Phase = string(foremanv1alpha1.ForemanPhaseProgressing)
		setCondition(&fe.Status.Conditions, fe.Generation, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonRollingOut, "frontend is rolling out")
		return ctrl.Result{RequeueAfter: requeueShort}, nil
	}
	fe.Status.Phase = string(foremanv1alpha1.ForemanPhaseRunning)
	setCondition(&fe.Status.Conditions, fe.Generation, foremanv1alpha1.ConditionReady, metav1.ConditionTrue, reasonReconciled, "frontend is serving")
	return ctrl.Result{}, nil
}

func (r *FrontendReconciler) ensureCertificates(ctx context.Context, fe *foremanv1alpha1.Frontend, f *foremanv1alpha1.Foreman) error {
	needsCertManager := fe.Spec.TLS.Server.IssuerRef != nil || fe.Spec.TLS.ClientCA.IssuerRef != nil
	if needsCertManager && !r.Capabilities.CertManager {
		return errMissingCapability("cert-manager")
	}
	labels := commonLabels(frontendAppName, fe.Name, componentFrontend)
	if issuer := fe.Spec.TLS.Server.IssuerRef; issuer != nil {
		if err := ensureCertificate(ctx, r.Client, r.Scheme, fe, certificateRequest{
			name:       frontendServerCertName(fe),
			namespace:  fe.Namespace,
			issuer:     *issuer,
			commonName: f.Spec.FQDN,
			dnsNames:   append([]string{f.Spec.FQDN}, f.Spec.Aliases...),
			usages:     []string{"server auth", "digital signature", "key encipherment"},
			labels:     labels,
		}); err != nil {
			return fmt.Errorf("server certificate: %w", err)
		}
	}
	if issuer := fe.Spec.TLS.ClientCA.IssuerRef; issuer != nil {
		name := fe.Name + "-client-ca." + fe.Namespace + ".foreman-operator.invalid"
		if err := ensureCertificate(ctx, r.Client, r.Scheme, fe, certificateRequest{
			name:       frontendClientCAName(fe),
			namespace:  fe.Namespace,
			issuer:     *issuer,
			commonName: name,
			dnsNames:   []string{name},
			usages:     []string{"client auth", "digital signature"},
			labels:     labels,
		}); err != nil {
			return fmt.Errorf("client CA certificate: %w", err)
		}
	}
	return nil
}

func (r *FrontendReconciler) ensureService(ctx context.Context, fe *foremanv1alpha1.Frontend) error {
	desired := frontendService(fe, r.Capabilities)
	svc := &corev1.Service{}
	svc.Name = desired.Name
	svc.Namespace = desired.Namespace
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = desired.Labels
		svc.Annotations = desired.Annotations
		svc.Spec.Type = desired.Spec.Type
		svc.Spec.Selector = desired.Spec.Selector
		svc.Spec.Ports = mergeServicePorts(svc.Spec.Ports, desired.Spec.Ports)
		return controllerutil.SetControllerReference(fe, svc, r.Scheme)
	})
	return err
}

// mergeServicePorts keeps node ports assigned by the API server for ports that still exist.
func mergeServicePorts(existing, desired []corev1.ServicePort) []corev1.ServicePort {
	for i := range desired {
		for _, e := range existing {
			if e.Name == desired[i].Name {
				desired[i].NodePort = e.NodePort
			}
		}
	}
	return desired
}

func (r *FrontendReconciler) ensureExposure(ctx context.Context, fe *foremanv1alpha1.Frontend, f *foremanv1alpha1.Foreman) error {
	switch exposureType(fe, r.Capabilities) {
	case foremanv1alpha1.FrontendExposureRoute:
		if !r.Capabilities.OpenShift {
			return errMissingCapability("the OpenShift Route API")
		}
		desired := frontendRoute(fe, f)
		route := &unstructured.Unstructured{}
		route.SetGroupVersionKind(routeGVK)
		route.SetName(desired.GetName())
		route.SetNamespace(desired.GetNamespace())
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, route, func() error {
			route.SetLabels(desired.GetLabels())
			route.SetAnnotations(desired.GetAnnotations())
			route.Object["spec"] = desired.Object["spec"]
			return controllerutil.SetControllerReference(fe, route, r.Scheme)
		})
		return err
	case foremanv1alpha1.FrontendExposureTLSRoute:
		if !r.Capabilities.GatewayAPI {
			return errMissingCapability("the Gateway API")
		}
		labels := commonLabels(frontendAppName, fe.Name, componentFrontend)
		tlsRoute := &gwapiv1.TLSRoute{}
		tlsRoute.Name = fe.Name
		tlsRoute.Namespace = fe.Namespace
		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, tlsRoute, func() error {
			tlsRoute.Labels = labels
			tlsRoute.Spec = frontendTLSRouteSpec(fe, f)
			return controllerutil.SetControllerReference(fe, tlsRoute, r.Scheme)
		}); err != nil {
			return err
		}
		httpRoute := &gwapiv1.HTTPRoute{}
		httpRoute.Name = fe.Name
		httpRoute.Namespace = fe.Namespace
		if fe.Spec.Exposure.GatewayRef.HTTPSectionName == "" {
			return client.IgnoreNotFound(r.Delete(ctx, httpRoute))
		}
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, httpRoute, func() error {
			httpRoute.Labels = labels
			httpRoute.Spec = frontendHTTPRouteSpec(fe, f)
			return controllerutil.SetControllerReference(fe, httpRoute, r.Scheme)
		})
		return err
	}
	return nil
}

// frontendsForForeman maps a Foreman to the Frontends that serve it.
func (r *FrontendReconciler) frontendsForForeman(ctx context.Context, obj client.Object) []reconcile.Request {
	list := &foremanv1alpha1.FrontendList{}
	if err := r.List(ctx, list, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}
	var requests []reconcile.Request
	for _, fe := range list.Items {
		if fe.Spec.ForemanRef == obj.GetName() {
			requests = append(requests, reconcile.Request{Namespace: fe.Namespace, Name: fe.Name})
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *FrontendReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&foremanv1alpha1.Frontend{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Watches(&foremanv1alpha1.Foreman{}, handler.EnqueueRequestsFromMapFunc(r.frontendsForForeman)).
		Named("frontend")
	if r.Capabilities.CertManager {
		b = b.Owns(newCertificateObject())
	}
	if r.Capabilities.GatewayAPI {
		b = b.Owns(&gwapiv1.TLSRoute{}).Owns(&gwapiv1.HTTPRoute{})
	}
	return b.Complete(r)
}
