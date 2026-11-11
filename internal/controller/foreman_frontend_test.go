package controller

import (
	"context"
	"slices"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	foremanv1alpha1 "github.com/slauger/foreman-operator/api/v1alpha1"
)

func foremanWithExposure(exposure foremanv1alpha1.FrontendExposureSpec) *foremanv1alpha1.Foreman {
	f := testForeman()
	f.Spec.Frontend.Exposure = exposure
	return f
}

func TestFrontendLoadBalancer(t *testing.T) {
	f := foremanWithExposure(foremanv1alpha1.FrontendExposureSpec{
		Annotations: map[string]string{"metallb.universe.tf/loadBalancerIPs": "192.0.2.1"},
	})
	c := newTestClient(f)
	reconcileForeman(t, newForemanReconciler(c), f)

	conf := mustGet(t, c, "prod-frontend", &corev1.ConfigMap{}).Data["foreman.conf"]
	for _, want := range []string{
		"ServerName foreman.example.com",
		"ServerAlias puppet.example.com",
		"SSLVerifyClient optional",
		"RequestHeader unset REMOTE-USER",
		"ProxyPass / unix:/run/foreman/puma.sock|http://localhost/ retry=0 timeout=900 upgrade=websocket",
		`RequestHeader set SSL-CLIENT-VERIFY ""`,
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("foreman.conf lacks %q", want)
		}
	}

	svc := mustGet(t, c, "prod-web", &corev1.Service{})
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer || svc.Annotations["metallb.universe.tf/loadBalancerIPs"] != "192.0.2.1" {
		t.Errorf("service type = %s, annotations = %v", svc.Spec.Type, svc.Annotations)
	}
	if len(svc.Spec.Ports) != 2 || svc.Spec.Ports[0].Port != 443 || svc.Spec.Ports[1].Port != 80 {
		t.Errorf("service ports = %+v", svc.Spec.Ports)
	}

	cert := mustGet(t, c, "prod-server", newCertificateObject())
	names, _, _ := unstructured.NestedStringSlice(cert.Object, "spec", "dnsNames")
	if strings.Join(names, ",") != "foreman.example.com,puppet.example.com" {
		t.Errorf("dnsNames = %v", names)
	}
}

// Puma must be reachable only through the frontend container in the same pod.
func TestWebPodRunsFrontendNextToPuma(t *testing.T) {
	d := foremanWebDeployment(testForeman(), DefaultImages("test"), Capabilities{}, 2, "")
	spec := d.Spec.Template.Spec
	if len(spec.Containers) != 2 || spec.Containers[0].Name != "foreman" || spec.Containers[1].Name != "httpd" {
		t.Fatalf("containers = %+v", spec.Containers)
	}
	puma, httpd := spec.Containers[0], spec.Containers[1]

	if len(puma.Ports) != 0 {
		t.Errorf("puma must not expose a port: %+v", puma.Ports)
	}
	env := map[string]string{}
	for _, e := range puma.Env {
		env[e.Name] = e.Value
	}
	if env["FOREMAN_BIND"] != "unix:///run/foreman/puma.sock?umask=0007" {
		t.Errorf("FOREMAN_BIND = %q", env["FOREMAN_BIND"])
	}
	for _, p := range []*corev1.Probe{puma.StartupProbe, puma.ReadinessProbe, puma.LivenessProbe} {
		if p.Exec == nil || !slices.Contains(p.Exec.Command, "--unix-socket") {
			t.Errorf("puma probe must use the socket: %+v", p.ProbeHandler)
		}
	}

	for _, ctr := range []corev1.Container{puma, httpd} {
		if !slices.ContainsFunc(ctr.VolumeMounts, func(m corev1.VolumeMount) bool {
			return m.Name == "puma-socket" && m.MountPath == pumaSocketDir
		}) {
			t.Errorf("%s does not mount the socket directory", ctr.Name)
		}
	}
	if httpd.Image != "ghcr.io/slauger/foreman-operator-frontend:test" || len(httpd.Ports) != 2 {
		t.Errorf("httpd = %s, ports %+v", httpd.Image, httpd.Ports)
	}

	vols := map[string]corev1.Volume{}
	for _, v := range spec.Volumes {
		vols[v.Name] = v
	}
	if vols["frontend-server"].Secret.SecretName != "prod-server" || vols["frontend-client-ca"].Secret.SecretName != "internal-ca" {
		t.Errorf("certificate volumes = %+v / %+v", vols["frontend-server"].Secret, vols["frontend-client-ca"].Secret)
	}
	if vols["puma-socket"].EmptyDir == nil {
		t.Error("the socket directory must be an emptyDir")
	}
}

func TestFrontendClientCAFromIssuer(t *testing.T) {
	f := foremanWithExposure(foremanv1alpha1.FrontendExposureSpec{Type: foremanv1alpha1.FrontendExposureClusterIP})
	f.Spec.Frontend.TLS.ClientCA = foremanv1alpha1.TLSSpec{IssuerRef: &foremanv1alpha1.IssuerReference{Name: "internal-ca"}}
	c := newTestClient(f)
	reconcileForeman(t, newForemanReconciler(c), f)

	cert := mustGet(t, c, "prod-client-ca", newCertificateObject())
	kind, _, _ := unstructured.NestedString(cert.Object, "spec", "issuerRef", "kind")
	if kind != "Issuer" {
		t.Errorf("issuer kind = %q, want Issuer", kind)
	}
	if svc := mustGet(t, c, "prod-web", &corev1.Service{}); svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("service type = %s", svc.Spec.Type)
	}
}

func TestFrontendTLSRoute(t *testing.T) {
	f := foremanWithExposure(foremanv1alpha1.FrontendExposureSpec{
		Type: foremanv1alpha1.FrontendExposureTLSRoute,
		GatewayRef: &foremanv1alpha1.GatewayReference{
			Name: "public", Namespace: "gateways", TLSSectionName: "tls", HTTPSectionName: "http",
		},
	})
	c := newTestClient(f)
	r := newForemanReconciler(c)
	reconcileForeman(t, r, f)

	tlsRoute := mustGet(t, c, "prod", &gwapiv1.TLSRoute{})
	parent := tlsRoute.Spec.ParentRefs[0]
	if parent.Name != "public" || *parent.Namespace != "gateways" || *parent.SectionName != "tls" {
		t.Errorf("parent = %+v", parent)
	}
	backend := tlsRoute.Spec.Rules[0].BackendRefs[0]
	if backend.Name != "prod-web" || *backend.Port != 443 || len(tlsRoute.Spec.Hostnames) != 2 {
		t.Errorf("tls route spec = %+v", tlsRoute.Spec)
	}
	httpRoute := mustGet(t, c, "prod", &gwapiv1.HTTPRoute{})
	if b := httpRoute.Spec.Rules[0].BackendRefs[0]; b.Name != "prod-web" || *b.Port != 80 {
		t.Errorf("http route backend = %+v", b)
	}
	if svc := mustGet(t, c, "prod-web", &corev1.Service{}); svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("service type = %s", svc.Spec.Type)
	}

	// Without an HTTP listener the HTTPRoute goes away.
	current := mustGet(t, c, "prod", &foremanv1alpha1.Foreman{})
	current.Spec.Frontend.Exposure.GatewayRef.HTTPSectionName = ""
	if err := c.Update(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	reconcileForeman(t, r, current)
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(httpRoute), &gwapiv1.HTTPRoute{}); err == nil {
		t.Error("HTTPRoute should have been deleted")
	}
}

func TestFrontendRouteOnOpenShift(t *testing.T) {
	f := foremanWithExposure(foremanv1alpha1.FrontendExposureSpec{})
	c := newTestClient(f)
	r := newForemanReconciler(c)
	r.Capabilities = Capabilities{CertManager: true, OpenShift: true}
	reconcileForeman(t, r, f)

	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(routeGVK)
	mustGet(t, c, "prod", route)
	termination, _, _ := unstructured.NestedString(route.Object, "spec", "tls", "termination")
	host, _, _ := unstructured.NestedString(route.Object, "spec", "host")
	target, _, _ := unstructured.NestedString(route.Object, "spec", "to", "name")
	if termination != "passthrough" || host != "foreman.example.com" || target != "prod-web" {
		t.Errorf("route termination = %q, host = %q, target = %q", termination, host, target)
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
			f := foremanWithExposure(tt.exposure)
			c := newTestClient(f)
			r := newForemanReconciler(c)
			r.Capabilities = tt.caps
			if _, err := r.Reconcile(context.Background(), reconcileRequest(f)); err == nil {
				t.Fatal("expected an error")
			}
			got := mustGet(t, c, "prod", &foremanv1alpha1.Foreman{})
			requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonReconcileError)
		})
	}
}

// A renewed frontend certificate rolls the web pods, but not the Dynflow workloads.
func TestFrontendCertificateRenewalRollsOnlyWeb(t *testing.T) {
	f := foremanWithExposure(foremanv1alpha1.FrontendExposureSpec{Type: foremanv1alpha1.FrontendExposureClusterIP})
	server := &corev1.Secret{
		Name: "prod-server", Namespace: testNamespace,
		Data: map[string][]byte{"tls.crt": []byte("first")},
	}
	c := newTestClient(f, server)
	r := newForemanReconciler(c)
	before, err := r.ensureFrontend(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}

	server.Data["tls.crt"] = []byte("renewed")
	if err := c.Update(context.Background(), server); err != nil {
		t.Fatal(err)
	}
	after, err := r.ensureFrontend(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("a renewed certificate must change the frontend checksum")
	}

	old, renewed := r.desiredDeployments(f, "config", before), r.desiredDeployments(f, "config", after)
	for i := range old {
		changed := old[i].Spec.Template.Annotations[checksumAnnotation] != renewed[i].Spec.Template.Annotations[checksumAnnotation]
		if isWeb := old[i].Labels[labelComponent] == componentWeb; changed != isWeb {
			t.Errorf("%s: pod template changed = %v", old[i].Name, changed)
		}
	}
}
