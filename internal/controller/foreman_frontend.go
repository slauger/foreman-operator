package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	foremanv1alpha1 "github.com/slauger/foreman-operator/api/v1alpha1"
)

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tlsroutes;httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes;routes/custom-host,verbs=get;list;watch;create;update;patch;delete

// ensureFrontend applies the certificates, the Apache configuration, the Service, and the
// exposure of the frontend. It returns the checksum of everything the frontend container
// reads, so that a change rolls the web pods.
func (r *ForemanReconciler) ensureFrontend(ctx context.Context, f *foremanv1alpha1.Foreman) (string, error) {
	if err := r.ensureFrontendCertificates(ctx, f); err != nil {
		return "", err
	}

	config, err := frontendConfig(f)
	if err != nil {
		return "", err
	}
	data := map[string]string{"foreman.conf": config}
	cm := &corev1.ConfigMap{}
	cm.Name = foremanFrontendConfigName(f)
	cm.Namespace = f.Namespace
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Labels = commonLabels(foremanAppName, f.Name, componentWeb)
		cm.Data = data
		return controllerutil.SetControllerReference(f, cm, r.Scheme)
	}); err != nil {
		return "", fmt.Errorf("frontend configuration: %w", err)
	}

	serverChecksum, err := secretChecksum(ctx, r.Client, f.Namespace, frontendServerCertName(f))
	if err != nil {
		return "", err
	}
	caChecksum, err := secretChecksum(ctx, r.Client, f.Namespace, frontendClientCAName(f))
	if err != nil {
		return "", err
	}

	if err := r.ensureService(ctx, f, foremanWebService(f, r.Capabilities)); err != nil {
		return "", fmt.Errorf("service: %w", err)
	}
	if err := r.ensureExposure(ctx, f); err != nil {
		return "", fmt.Errorf("exposure: %w", err)
	}
	return checksum(checksumMap(data), serverChecksum, caChecksum), nil
}

func (r *ForemanReconciler) ensureFrontendCertificates(ctx context.Context, f *foremanv1alpha1.Foreman) error {
	tls := f.Spec.Frontend.TLS
	if (tls.Server.IssuerRef != nil || tls.ClientCA.IssuerRef != nil) && !r.Capabilities.CertManager {
		return errMissingCapability("cert-manager")
	}
	labels := commonLabels(foremanAppName, f.Name, componentWeb)
	if issuer := tls.Server.IssuerRef; issuer != nil {
		if err := ensureCertificate(ctx, r.Client, r.Scheme, f, certificateRequest{
			name:       frontendServerCertName(f),
			namespace:  f.Namespace,
			issuer:     *issuer,
			commonName: f.Spec.FQDN,
			dnsNames:   append([]string{f.Spec.FQDN}, f.Spec.Aliases...),
			usages:     []string{"server auth", "digital signature", "key encipherment"},
			labels:     labels,
		}); err != nil {
			return fmt.Errorf("server certificate: %w", err)
		}
	}
	if issuer := tls.ClientCA.IssuerRef; issuer != nil {
		name := f.Name + "-client-ca." + f.Namespace + ".foreman-operator.invalid"
		if err := ensureCertificate(ctx, r.Client, r.Scheme, f, certificateRequest{
			name:       frontendClientCAName(f),
			namespace:  f.Namespace,
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

// ensureService creates or updates a Service owned by the Foreman.
func (r *ForemanReconciler) ensureService(ctx context.Context, f *foremanv1alpha1.Foreman, desired *corev1.Service) error {
	svc := &corev1.Service{}
	svc.Name = desired.Name
	svc.Namespace = desired.Namespace
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = desired.Labels
		svc.Annotations = desired.Annotations
		svc.Spec.Type = desired.Spec.Type
		svc.Spec.Selector = desired.Spec.Selector
		svc.Spec.Ports = mergeServicePorts(svc.Spec.Ports, desired.Spec.Ports)
		return controllerutil.SetControllerReference(f, svc, r.Scheme)
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

func (r *ForemanReconciler) ensureExposure(ctx context.Context, f *foremanv1alpha1.Foreman) error {
	switch exposureType(f, r.Capabilities) {
	case foremanv1alpha1.FrontendExposureRoute:
		if !r.Capabilities.OpenShift {
			return errMissingCapability("the OpenShift Route API")
		}
		desired := frontendRoute(f)
		route := &unstructured.Unstructured{}
		route.SetGroupVersionKind(routeGVK)
		route.SetName(desired.GetName())
		route.SetNamespace(desired.GetNamespace())
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, route, func() error {
			route.SetLabels(desired.GetLabels())
			route.SetAnnotations(desired.GetAnnotations())
			route.Object["spec"] = desired.Object["spec"]
			return controllerutil.SetControllerReference(f, route, r.Scheme)
		})
		return err
	case foremanv1alpha1.FrontendExposureTLSRoute:
		if !r.Capabilities.GatewayAPI {
			return errMissingCapability("the Gateway API")
		}
		labels := commonLabels(foremanAppName, f.Name, componentWeb)
		tlsRoute := &gwapiv1.TLSRoute{}
		tlsRoute.Name = f.Name
		tlsRoute.Namespace = f.Namespace
		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, tlsRoute, func() error {
			tlsRoute.Labels = labels
			tlsRoute.Spec = frontendTLSRouteSpec(f)
			return controllerutil.SetControllerReference(f, tlsRoute, r.Scheme)
		}); err != nil {
			return err
		}
		httpRoute := &gwapiv1.HTTPRoute{}
		httpRoute.Name = f.Name
		httpRoute.Namespace = f.Namespace
		if f.Spec.Frontend.Exposure.GatewayRef.HTTPSectionName == "" {
			return client.IgnoreNotFound(r.Delete(ctx, httpRoute))
		}
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, httpRoute, func() error {
			httpRoute.Labels = labels
			httpRoute.Spec = frontendHTTPRouteSpec(f)
			return controllerutil.SetControllerReference(f, httpRoute, r.Scheme)
		})
		return err
	}
	return nil
}
