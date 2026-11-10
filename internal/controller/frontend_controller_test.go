package controller

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	foremanv1alpha1 "github.com/slauger/foreman-operator/api/v1alpha1"
)

func testFrontend(exposure foremanv1alpha1.FrontendExposureSpec) *foremanv1alpha1.Frontend {
	return &foremanv1alpha1.Frontend{
		Name: "prod", Namespace: testNamespace, Generation: 1,
		Spec: foremanv1alpha1.FrontendSpec{
			ForemanRef: "prod",
			TLS: foremanv1alpha1.FrontendTLSSpec{
				Server:   foremanv1alpha1.TLSSpec{IssuerRef: &foremanv1alpha1.IssuerReference{Name: "letsencrypt", Kind: "ClusterIssuer"}},
				ClientCA: foremanv1alpha1.TLSSpec{SecretName: "internal-ca"},
			},
			Exposure: exposure,
		},
	}
}

func reconcileFrontend(t *testing.T, r *FrontendReconciler, fe *foremanv1alpha1.Frontend) *foremanv1alpha1.Frontend {
	t.Helper()
	if _, err := r.Reconcile(context.Background(), reconcileRequest(fe)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	return mustGet(t, r.Client, fe.Name, &foremanv1alpha1.Frontend{})
}

func newFrontendReconciler(c client.Client, caps Capabilities) *FrontendReconciler {
	return &FrontendReconciler{Client: c, Scheme: c.Scheme(), Images: DefaultImages("test"), Capabilities: caps}
}

func TestFrontendWaitsForForeman(t *testing.T) {
	fe := testFrontend(foremanv1alpha1.FrontendExposureSpec{})
	c := newTestClient(fe)
	got := reconcileFrontend(t, newFrontendReconciler(c, allCapabilities), fe)
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonWaitingForeman)
}

func TestFrontendLoadBalancer(t *testing.T) {
	fe := testFrontend(foremanv1alpha1.FrontendExposureSpec{
		Annotations: map[string]string{"metallb.universe.tf/loadBalancerIPs": "192.0.2.1"},
	})
	c := newTestClient(testForeman(), fe)
	r := newFrontendReconciler(c, allCapabilities)
	got := reconcileFrontend(t, r, fe)
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonRollingOut)

	conf := mustGet(t, c, "prod", &corev1.ConfigMap{}).Data["foreman.conf"]
	for _, want := range []string{
		"ServerName foreman.example.com",
		"ServerAlias puppet.example.com",
		"SSLVerifyClient optional",
		"RequestHeader unset REMOTE-USER",
		"ProxyPass / http://prod-web.foreman.svc:3000/ retry=0 timeout=900 upgrade=websocket",
		`RequestHeader set SSL-CLIENT-VERIFY ""`,
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("foreman.conf lacks %q", want)
		}
	}

	svc := mustGet(t, c, "prod", &corev1.Service{})
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer || svc.Annotations["metallb.universe.tf/loadBalancerIPs"] != "192.0.2.1" {
		t.Errorf("service type = %s, annotations = %v", svc.Spec.Type, svc.Annotations)
	}

	d := mustGet(t, c, "prod", &appsv1.Deployment{})
	vols := map[string]corev1.Volume{}
	for _, v := range d.Spec.Template.Spec.Volumes {
		vols[v.Name] = v
	}
	if vols["server"].Secret.SecretName != "prod-server" || vols["client-ca"].Secret.SecretName != "internal-ca" {
		t.Errorf("certificate volumes = %+v / %+v", vols["server"].Secret, vols["client-ca"].Secret)
	}
	cert := mustGet(t, c, "prod-server", newCertificateObject())
	names, _, _ := unstructured.NestedStringSlice(cert.Object, "spec", "dnsNames")
	if strings.Join(names, ",") != "foreman.example.com,puppet.example.com" {
		t.Errorf("dnsNames = %v", names)
	}

	markDeploymentAvailable(t, c, "prod")
	got = reconcileFrontend(t, r, got)
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionReady, metav1.ConditionTrue, reasonReconciled)
}

func TestFrontendClientCAFromIssuer(t *testing.T) {
	fe := testFrontend(foremanv1alpha1.FrontendExposureSpec{Type: foremanv1alpha1.FrontendExposureClusterIP})
	fe.Spec.TLS.ClientCA = foremanv1alpha1.TLSSpec{IssuerRef: &foremanv1alpha1.IssuerReference{Name: "internal-ca"}}
	c := newTestClient(testForeman(), fe)
	reconcileFrontend(t, newFrontendReconciler(c, allCapabilities), fe)

	cert := mustGet(t, c, "prod-client-ca", newCertificateObject())
	kind, _, _ := unstructured.NestedString(cert.Object, "spec", "issuerRef", "kind")
	if kind != "Issuer" {
		t.Errorf("issuer kind = %q, want Issuer", kind)
	}
	if svc := mustGet(t, c, "prod", &corev1.Service{}); svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("service type = %s", svc.Spec.Type)
	}
}

func TestFrontendTLSRoute(t *testing.T) {
	fe := testFrontend(foremanv1alpha1.FrontendExposureSpec{
		Type: foremanv1alpha1.FrontendExposureTLSRoute,
		GatewayRef: &foremanv1alpha1.GatewayReference{
			Name: "public", Namespace: "gateways", TLSSectionName: "tls", HTTPSectionName: "http",
		},
	})
	c := newTestClient(testForeman(), fe)
	r := newFrontendReconciler(c, allCapabilities)
	reconcileFrontend(t, r, fe)

	tlsRoute := mustGet(t, c, "prod", &gwapiv1.TLSRoute{})
	parent := tlsRoute.Spec.ParentRefs[0]
	if parent.Name != "public" || *parent.Namespace != "gateways" || *parent.SectionName != "tls" {
		t.Errorf("parent = %+v", parent)
	}
	if *tlsRoute.Spec.Rules[0].BackendRefs[0].Port != 443 || len(tlsRoute.Spec.Hostnames) != 2 {
		t.Errorf("tls route spec = %+v", tlsRoute.Spec)
	}
	httpRoute := mustGet(t, c, "prod", &gwapiv1.HTTPRoute{})
	if *httpRoute.Spec.Rules[0].BackendRefs[0].Port != 80 {
		t.Errorf("http route backend = %+v", httpRoute.Spec.Rules[0].BackendRefs[0])
	}
	if svc := mustGet(t, c, "prod", &corev1.Service{}); svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("service type = %s", svc.Spec.Type)
	}

	// Without an HTTP listener the HTTPRoute goes away.
	current := mustGet(t, c, "prod", &foremanv1alpha1.Frontend{})
	current.Spec.Exposure.GatewayRef.HTTPSectionName = ""
	if err := c.Update(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	reconcileFrontend(t, r, current)
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(httpRoute), &gwapiv1.HTTPRoute{}); err == nil {
		t.Error("HTTPRoute should have been deleted")
	}
}

func TestFrontendRouteOnOpenShift(t *testing.T) {
	fe := testFrontend(foremanv1alpha1.FrontendExposureSpec{})
	c := newTestClient(testForeman(), fe)
	reconcileFrontend(t, newFrontendReconciler(c, Capabilities{CertManager: true, OpenShift: true}), fe)

	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(routeGVK)
	mustGet(t, c, "prod", route)
	termination, _, _ := unstructured.NestedString(route.Object, "spec", "tls", "termination")
	host, _, _ := unstructured.NestedString(route.Object, "spec", "host")
	if termination != "passthrough" || host != "foreman.example.com" {
		t.Errorf("route termination = %q, host = %q", termination, host)
	}
}

func TestFrontendMissingCapabilities(t *testing.T) {
	tests := []struct {
		name     string
		exposure foremanv1alpha1.FrontendExposureSpec
		caps     Capabilities
	}{
		{"cert-manager", foremanv1alpha1.FrontendExposureSpec{}, Capabilities{}},
		{"gateway api", foremanv1alpha1.FrontendExposureSpec{
			Type: foremanv1alpha1.FrontendExposureTLSRoute, GatewayRef: &foremanv1alpha1.GatewayReference{Name: "gw"},
		}, Capabilities{CertManager: true}},
		{"route", foremanv1alpha1.FrontendExposureSpec{Type: foremanv1alpha1.FrontendExposureRoute}, Capabilities{CertManager: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fe := testFrontend(tt.exposure)
			c := newTestClient(testForeman(), fe)
			if _, err := newFrontendReconciler(c, tt.caps).Reconcile(context.Background(), reconcileRequest(fe)); err == nil {
				t.Fatal("expected an error")
			}
			got := mustGet(t, c, "prod", &foremanv1alpha1.Frontend{})
			requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonReconcileError)
		})
	}
}

func TestFrontendsForForeman(t *testing.T) {
	other := testFrontend(foremanv1alpha1.FrontendExposureSpec{})
	other.Name = "other"
	other.Spec.ForemanRef = "staging"
	c := newTestClient(testFrontend(foremanv1alpha1.FrontendExposureSpec{}), other)
	reqs := newFrontendReconciler(c, allCapabilities).frontendsForForeman(context.Background(), testForeman())
	if len(reqs) != 1 || reqs[0].Name != "prod" {
		t.Errorf("requests = %v", reqs)
	}
}
