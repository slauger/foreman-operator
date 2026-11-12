package controller

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	foremanv1alpha1 "github.com/slauger/foreman-operator/api/v1alpha1"
	"github.com/slauger/foreman-operator/internal/foremanapi"
)

// ForemanAPI is the part of the Foreman API used to register Smart Proxies.
type ForemanAPI interface {
	FindSmartProxy(ctx context.Context, name string) (*foremanapi.SmartProxy, error)
	CreateSmartProxy(ctx context.Context, name, proxyURL string) (*foremanapi.SmartProxy, error)
	UpdateSmartProxy(ctx context.Context, id int64, name, proxyURL string) error
	RefreshSmartProxy(ctx context.Context, id int64) error
	DeleteSmartProxy(ctx context.Context, id int64) error
}

// ForemanAPIFactory creates a client for a Foreman. baseURL is the in-cluster Service of the
// frontend, and rootCAs verifies its certificate (nil: system roots).
type ForemanAPIFactory func(baseURL, publicURL, consumerKey, consumerSecret string, rootCAs *x509.CertPool) (ForemanAPI, error)

// NewForemanAPI is the default ForemanAPIFactory.
func NewForemanAPI(baseURL, publicURL, consumerKey, consumerSecret string, rootCAs *x509.CertPool) (ForemanAPI, error) {
	return foremanapi.New(baseURL, publicURL, consumerKey, consumerSecret, rootCAs)
}

// SmartProxyReconciler reconciles a SmartProxy object.
type SmartProxyReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Images       Images
	Capabilities Capabilities
	NewAPI       ForemanAPIFactory
}

// +kubebuilder:rbac:groups=platform.theforeman.org,resources=smartproxies,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=platform.theforeman.org,resources=smartproxies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.theforeman.org,resources=smartproxies/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch

// Reconcile drives a SmartProxy towards its spec and keeps its registration in Foreman.
func (r *SmartProxyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	p := &foremanv1alpha1.SmartProxy{}
	if err := r.Get(ctx, req.NamespacedName, p); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !p.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.finalize(ctx, p)
	}
	if controllerutil.AddFinalizer(p, smartProxyFinalizer) {
		if err := r.Update(ctx, p); err != nil {
			return ctrl.Result{}, err
		}
	}
	original := p.DeepCopy()

	result, err := r.reconcile(ctx, p)
	if err != nil {
		p.Status.Phase = string(foremanv1alpha1.ForemanPhaseFailed)
		setCondition(&p.Status.Conditions, p.Generation, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonReconcileError, err.Error())
	}
	p.Status.ObservedGeneration = p.Generation
	if statusErr := r.Status().Patch(ctx, p, client.MergeFrom(original)); statusErr != nil && !apierrors.IsNotFound(statusErr) {
		return ctrl.Result{}, statusErr
	}
	return result, err
}

func (r *SmartProxyReconciler) reconcile(ctx context.Context, p *foremanv1alpha1.SmartProxy) (ctrl.Result, error) {
	if isPaused(p) {
		setCondition(&p.Status.Conditions, p.Generation, foremanv1alpha1.ConditionPaused, metav1.ConditionTrue, reasonPaused, "reconciliation is paused by annotation")
		return ctrl.Result{}, nil
	}
	setCondition(&p.Status.Conditions, p.Generation, foremanv1alpha1.ConditionPaused, metav1.ConditionFalse, reasonReconciled, "")

	f := &foremanv1alpha1.Foreman{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: p.Namespace, Name: p.Spec.ForemanRef}, f); err != nil {
		if apierrors.IsNotFound(err) {
			r.waiting(p, fmt.Sprintf("Foreman %q does not exist", p.Spec.ForemanRef))
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if err := r.ensureCertificate(ctx, p); err != nil {
		return ctrl.Result{}, fmt.Errorf("certificate: %w", err)
	}

	config, err := smartProxyConfig(p, f)
	if err != nil {
		return ctrl.Result{}, err
	}
	cm := &corev1.ConfigMap{}
	cm.Name = p.Name
	cm.Namespace = p.Namespace
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Labels = commonLabels(smartProxyAppName, p.Name, componentSmartProxy)
		cm.Data = config
		return controllerutil.SetControllerReference(p, cm, r.Scheme)
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("configuration: %w", err)
	}

	if p.Spec.Storage != nil {
		if err := r.ensurePVC(ctx, p); err != nil {
			return ctrl.Result{}, fmt.Errorf("storage: %w", err)
		}
	}

	d, err := r.ensureDeployment(ctx, p, config)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("deployment: %w", err)
	}
	if err := r.ensureService(ctx, p); err != nil {
		return ctrl.Result{}, fmt.Errorf("service: %w", err)
	}

	if !deploymentAvailable(d.Status.AvailableReplicas, 1, d.Status.ObservedGeneration, d.Generation) {
		p.Status.Phase = string(foremanv1alpha1.ForemanPhaseProgressing)
		setCondition(&p.Status.Conditions, p.Generation, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonRollingOut, "proxy is rolling out")
		return ctrl.Result{RequeueAfter: requeueShort}, nil
	}

	if !meta.IsStatusConditionTrue(f.Status.Conditions, foremanv1alpha1.ConditionReady) {
		r.waiting(p, fmt.Sprintf("Foreman %q is not ready", f.Name))
		return ctrl.Result{RequeueAfter: requeueShort}, nil
	}

	// Foreman knows only the features of its enabled plugins, so a migration for a new plugin set
	// refreshes the features as well.
	if err := r.register(ctx, p, f, checksum(p.Spec.URL, d.Spec.Template.Annotations[checksumAnnotation], f.Status.AppliedMigrationHash)); err != nil {
		p.Status.Phase = string(foremanv1alpha1.ForemanPhaseProgressing)
		setCondition(&p.Status.Conditions, p.Generation, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonRegistrationFail, err.Error())
		// Foreman reaches the proxy only once its certificate and Service are in place.
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	p.Status.Phase = string(foremanv1alpha1.ForemanPhaseRunning)
	setCondition(&p.Status.Conditions, p.Generation, foremanv1alpha1.ConditionReady, metav1.ConditionTrue, reasonReconciled, "proxy is registered in Foreman")
	return ctrl.Result{RequeueAfter: resyncPeriod}, nil
}

func (r *SmartProxyReconciler) waiting(p *foremanv1alpha1.SmartProxy, message string) {
	p.Status.Phase = string(foremanv1alpha1.ForemanPhasePending)
	setCondition(&p.Status.Conditions, p.Generation, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonWaitingForeman, message)
}

func (r *SmartProxyReconciler) ensureCertificate(ctx context.Context, p *foremanv1alpha1.SmartProxy) error {
	if p.Spec.TLS.IssuerRef == nil {
		return nil
	}
	if !r.Capabilities.CertManager {
		return errMissingCapability("cert-manager")
	}
	host, _, err := proxyEndpoint(p)
	if err != nil {
		return err
	}
	dnsNames := []string{host, p.Name, p.Name + "." + p.Namespace + ".svc", p.Name + "." + p.Namespace + ".svc.cluster.local"}
	return ensureCertificate(ctx, r.Client, r.Scheme, p, certificateRequest{
		name:       smartProxyCertName(p),
		namespace:  p.Namespace,
		issuer:     *p.Spec.TLS.IssuerRef,
		commonName: host,
		dnsNames:   dnsNames,
		// The proxy uses the same certificate as client towards Foreman.
		usages: []string{"server auth", "client auth", "digital signature", "key encipherment"},
		labels: commonLabels(smartProxyAppName, p.Name, componentSmartProxy),
	})
}

func (r *SmartProxyReconciler) ensurePVC(ctx context.Context, p *foremanv1alpha1.SmartProxy) error {
	desired := smartProxyPVC(p)
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), pvc)
	if apierrors.IsNotFound(err) {
		// No owner reference: boot files and leases outlive a recreated SmartProxy.
		return r.Create(ctx, desired)
	}
	return err
}

func (r *SmartProxyReconciler) ensureDeployment(ctx context.Context, p *foremanv1alpha1.SmartProxy, config map[string]string) (*appsv1.Deployment, error) {
	certChecksum, err := secretChecksum(ctx, r.Client, p.Namespace, smartProxyCertName(p))
	if err != nil {
		return nil, err
	}
	desired, err := smartProxyDeployment(p, r.Images, r.Capabilities, config, certChecksum)
	if err != nil {
		return nil, err
	}
	d := &appsv1.Deployment{}
	d.Name = desired.Name
	d.Namespace = desired.Namespace
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, d, func() error {
		d.Labels = desired.Labels
		d.Spec.Replicas = desired.Spec.Replicas
		d.Spec.Strategy = desired.Spec.Strategy
		if d.Spec.Selector == nil {
			d.Spec.Selector = desired.Spec.Selector
		}
		setPodTemplate(d, desired.Spec.Template)
		return controllerutil.SetControllerReference(p, d, r.Scheme)
	})
	return d, err
}

func (r *SmartProxyReconciler) ensureService(ctx context.Context, p *foremanv1alpha1.SmartProxy) error {
	desired, err := smartProxyService(p)
	if err != nil {
		return err
	}
	svc := &corev1.Service{}
	svc.Name = desired.Name
	svc.Namespace = desired.Namespace
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = desired.Labels
		svc.Spec.Selector = desired.Spec.Selector
		svc.Spec.Ports = desired.Spec.Ports
		return controllerutil.SetControllerReference(p, svc, r.Scheme)
	})
	return err
}

// apiFor builds a Foreman API client with the operator's OAuth consumer.
func (r *SmartProxyReconciler) apiFor(ctx context.Context, f *foremanv1alpha1.Foreman) (ForemanAPI, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: f.Namespace, Name: foremanSecretsName(f)}, secret); err != nil {
		return nil, err
	}
	key, secretValue := string(secret.Data[keyOAuthConsumerKey]), string(secret.Data[keyOAuthConsumerSecret])
	if key == "" || secretValue == "" {
		return nil, errors.New("the Foreman secrets do not contain OAuth credentials yet")
	}
	rootCAs, err := r.frontendRootCAs(ctx, f)
	if err != nil {
		return nil, err
	}
	baseURL := fmt.Sprintf("https://%s.%s.svc", foremanWebServiceName(f), f.Namespace)
	return r.NewAPI(baseURL, foremanPublicURL(f), key, secretValue, rootCAs)
}

// frontendRootCAs returns the CA from ca.crt of the frontend server certificate. A public
// certificate, for example from ACME, often comes without ca.crt and uses the system roots.
func (r *SmartProxyReconciler) frontendRootCAs(ctx context.Context, f *foremanv1alpha1.Foreman) (*x509.CertPool, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: f.Namespace, Name: frontendServerCertName(f)}, secret); err != nil {
		return nil, fmt.Errorf("frontend server certificate: %w", err)
	}
	ca := secret.Data["ca.crt"]
	if len(ca) == 0 {
		return nil, nil
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("frontend server certificate %s: ca.crt contains no certificate", secret.Name)
	}
	return pool, nil
}

// register creates or updates the proxy in Foreman. Foreman reads the features only on
// registration and refresh, so every change of URL, configuration, or certificate refreshes.
func (r *SmartProxyReconciler) register(ctx context.Context, p *foremanv1alpha1.SmartProxy, f *foremanv1alpha1.Foreman, registration string) error {
	log := logf.FromContext(ctx)
	api, err := r.apiFor(ctx, f)
	if err != nil {
		return err
	}

	if p.Status.ForemanID == 0 {
		existing, err := api.FindSmartProxy(ctx, p.Name)
		switch {
		case errors.Is(err, foremanapi.ErrNotFound):
			created, err := api.CreateSmartProxy(ctx, p.Name, p.Spec.URL)
			if err != nil {
				return err
			}
			log.Info("registered smart proxy in Foreman", "id", created.ID)
			p.Status.ForemanID = created.ID
			p.Status.RegisteredChecksum = registration
			return nil
		case err != nil:
			return err
		default:
			p.Status.ForemanID = existing.ID
		}
	}

	if p.Status.RegisteredChecksum == registration {
		return nil
	}
	if err := api.UpdateSmartProxy(ctx, p.Status.ForemanID, p.Name, p.Spec.URL); err != nil {
		if errors.Is(err, foremanapi.ErrNotFound) {
			// Removed in Foreman by hand: register again on the next pass.
			p.Status.ForemanID = 0
		}
		return err
	}
	if err := api.RefreshSmartProxy(ctx, p.Status.ForemanID); err != nil {
		return err
	}
	log.Info("refreshed smart proxy features in Foreman", "id", p.Status.ForemanID)
	p.Status.RegisteredChecksum = registration
	return nil
}

// finalize removes the proxy from Foreman. Without a reachable Foreman, the registration
// cannot be cleaned up and the finalizer stays until the Foreman is ready or gone.
func (r *SmartProxyReconciler) finalize(ctx context.Context, p *foremanv1alpha1.SmartProxy) error {
	if !controllerutil.ContainsFinalizer(p, smartProxyFinalizer) {
		return nil
	}
	if p.Status.ForemanID != 0 {
		f := &foremanv1alpha1.Foreman{}
		err := r.Get(ctx, client.ObjectKey{Namespace: p.Namespace, Name: p.Spec.ForemanRef}, f)
		switch {
		case apierrors.IsNotFound(err) || !f.DeletionTimestamp.IsZero():
			// The Foreman is gone, and with it the registration.
		case err != nil:
			return err
		default:
			api, err := r.apiFor(ctx, f)
			if err != nil {
				return err
			}
			if err := api.DeleteSmartProxy(ctx, p.Status.ForemanID); err != nil {
				return fmt.Errorf("remove smart proxy from Foreman: %w", err)
			}
		}
	}
	controllerutil.RemoveFinalizer(p, smartProxyFinalizer)
	return r.Update(ctx, p)
}

// smartProxiesForForeman maps a Foreman to the Smart Proxies registered with it.
func (r *SmartProxyReconciler) smartProxiesForForeman(ctx context.Context, obj client.Object) []reconcile.Request {
	list := &foremanv1alpha1.SmartProxyList{}
	if err := r.List(ctx, list, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}
	var requests []reconcile.Request
	for _, p := range list.Items {
		if p.Spec.ForemanRef == obj.GetName() {
			requests = append(requests, reconcile.Request{Namespace: p.Namespace, Name: p.Name})
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *SmartProxyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.NewAPI == nil {
		r.NewAPI = NewForemanAPI
	}
	b := ctrl.NewControllerManagedBy(mgr).
		For(&foremanv1alpha1.SmartProxy{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Watches(&foremanv1alpha1.Foreman{}, handler.EnqueueRequestsFromMapFunc(r.smartProxiesForForeman)).
		Named("smartproxy")
	if r.Capabilities.CertManager {
		b = b.Owns(newCertificateObject())
	}
	return b.Complete(r)
}
