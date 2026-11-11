package controller

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	foremanv1alpha1 "github.com/slauger/foreman-operator/api/v1alpha1"
	"github.com/slauger/foreman-operator/internal/foremanapi"
)

// fakeForemanAPI records the calls the reconciler makes.
type fakeForemanAPI struct {
	proxies   map[string]*foremanapi.SmartProxy
	nextID    int64
	refreshed []int64
	deleted   []int64
	createErr error
	baseURL   string
	publicURL string
}

func newFakeForemanAPI() *fakeForemanAPI {
	return &fakeForemanAPI{proxies: map[string]*foremanapi.SmartProxy{}, nextID: 1}
}

func (f *fakeForemanAPI) factory(baseURL, publicURL, _, _ string) (ForemanAPI, error) {
	f.baseURL, f.publicURL = baseURL, publicURL
	return f, nil
}

func (f *fakeForemanAPI) FindSmartProxy(_ context.Context, name string) (*foremanapi.SmartProxy, error) {
	if p, ok := f.proxies[name]; ok {
		return p, nil
	}
	return nil, foremanapi.ErrNotFound
}

func (f *fakeForemanAPI) CreateSmartProxy(_ context.Context, name, proxyURL string) (*foremanapi.SmartProxy, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	p := &foremanapi.SmartProxy{ID: f.nextID, Name: name, URL: proxyURL}
	f.nextID++
	f.proxies[name] = p
	return p, nil
}

func (f *fakeForemanAPI) UpdateSmartProxy(_ context.Context, id int64, name, proxyURL string) error {
	for _, p := range f.proxies {
		if p.ID == id {
			p.Name, p.URL = name, proxyURL
			return nil
		}
	}
	return foremanapi.ErrNotFound
}

func (f *fakeForemanAPI) RefreshSmartProxy(_ context.Context, id int64) error {
	f.refreshed = append(f.refreshed, id)
	return nil
}

func (f *fakeForemanAPI) DeleteSmartProxy(_ context.Context, id int64) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func testSmartProxy() *foremanv1alpha1.SmartProxy {
	return &foremanv1alpha1.SmartProxy{
		Name: "pxe", Namespace: testNamespace, Generation: 1,
		Spec: foremanv1alpha1.SmartProxySpec{
			ForemanRef: "prod",
			Profile:    foremanv1alpha1.SmartProxyProfileProvisioning,
			URL:        "https://pxe.foreman.svc:8443",
			TLS:        foremanv1alpha1.TLSSpec{IssuerRef: &foremanv1alpha1.IssuerReference{Name: "internal-ca"}},
			Networks:   []foremanv1alpha1.SmartProxyNetwork{{Attachment: "pxe-vlan", Address: "10.99.0.10/24"}},
			Features: foremanv1alpha1.SmartProxyFeatures{
				TFTP:      &foremanv1alpha1.TFTPFeature{},
				Templates: &foremanv1alpha1.TemplatesFeature{URL: "http://10.99.0.10:8000"},
				DHCP: &foremanv1alpha1.DHCPFeature{
					Provider: foremanv1alpha1.DHCPProviderDnsmasq,
					Subnets: []foremanv1alpha1.DHCPSubnet{{
						Network: "10.99.0.0/24", Range: "10.99.0.100-10.99.0.150",
						Gateway: "10.99.0.1", DNSServers: []string{"10.99.0.2", "10.99.0.3"},
					}},
				},
				DNS: &foremanv1alpha1.DNSFeature{Server: "10.99.0.2", KeySecretRef: "tsig"},
			},
		},
	}
}

// readyForeman is a Foreman whose Ready condition is true, with its secrets.
func readyForeman() (*foremanv1alpha1.Foreman, *corev1.Secret) {
	f := testForeman()
	f.Status.Conditions = []metav1.Condition{{
		Type: foremanv1alpha1.ConditionReady, Status: metav1.ConditionTrue, Reason: reasonReconciled, LastTransitionTime: metav1.Now(),
	}}
	secrets := &corev1.Secret{
		Name: "prod-secrets", Namespace: testNamespace,
		Data: map[string][]byte{
			keyOAuthConsumerKey:    []byte("key"),
			keyOAuthConsumerSecret: []byte("secret"),
		},
	}
	return f, secrets
}

func newSmartProxyReconciler(c client.Client, api *fakeForemanAPI) *SmartProxyReconciler {
	return &SmartProxyReconciler{Client: c, Scheme: c.Scheme(), Images: DefaultImages("test"), Capabilities: allCapabilities, NewAPI: api.factory}
}

func reconcileSmartProxy(t *testing.T, r *SmartProxyReconciler) *foremanv1alpha1.SmartProxy {
	const name = "pxe"
	t.Helper()
	p := &foremanv1alpha1.SmartProxy{Name: name, Namespace: testNamespace}
	if _, err := r.Reconcile(context.Background(), reconcileRequest(p)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	return mustGet(t, r.Client, name, &foremanv1alpha1.SmartProxy{})
}

func TestSmartProxyConfig(t *testing.T) {
	f := testForeman()
	files, err := smartProxyConfig(testSmartProxy(), f)
	if err != nil {
		t.Fatal(err)
	}
	expect := map[string][]string{
		"settings.yml": {
			`:trusted_hosts:` + "\n" + `  - "foreman.example.com"`,
			`:https_port: 8443`,
			`:http_port: 8000`,
			`:foreman_url: "https://foreman.example.com"`,
			`:foreman_ssl_cert: "/etc/foreman-proxy/tls/tls.crt"`,
		},
		"dhcp.yml":          {`:use_provider: "dhcp_dnsmasq"`, `  - "10.99.0.0/255.255.255.0"`},
		"dhcp_dnsmasq.yml":  {`:reload_cmd: "touch /var/lib/foreman-proxy/dhcp/.reload"`, `:blacklist_duration_minutes: 1800`},
		"dns.yml":           {`:enabled: "https"`, `:use_provider: "dns_nsupdate"`},
		"dns_nsupdate.yml":  {`:dns_server: "10.99.0.2"`, `:dns_key: "/etc/foreman-proxy/dns/key"`},
		"tftp.yml":          {`:tftp_servername: "10.99.0.10"`},
		"templates.yml":     {`:enabled: true`, `:template_url: "http://10.99.0.10:8000"`},
		"bmc.yml":           {`:enabled: false`},
		"facts.yml":         {`:enabled: false`},
		"dnsmasq.conf":      {"dhcp-range=10.99.0.100,10.99.0.150,255.255.255.0,12h", "dhcp-option=3,10.99.0.1", "dhcp-option=6,10.99.0.2,10.99.0.3", "dhcp-hostsfile=/var/lib/foreman-proxy/dhcp/dhcphosts", "enable-tftp"},
		"dhcp_infoblox.yml": nil,
	}
	for file, wants := range expect {
		content, ok := files[file]
		if wants == nil {
			if ok {
				t.Errorf("%s should not be generated", file)
			}
			continue
		}
		if !ok {
			t.Errorf("%s missing", file)
			continue
		}
		for _, want := range wants {
			if !strings.Contains(content, want) {
				t.Errorf("%s lacks %q:\n%s", file, want, content)
			}
		}
	}
	if strings.Contains(files["dnsmasq.conf"], "dhcp-boot") {
		t.Error("dnsmasq.conf must not set a global boot file")
	}
}

func TestSmartProxyConfigWithoutDHCP(t *testing.T) {
	p := testSmartProxy()
	p.Spec.Features.DHCP = nil
	p.Spec.Features.DNS = nil
	p.Spec.Features.Templates = nil
	files, err := smartProxyConfig(p, testForeman())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(files["dhcp.yml"], `:enabled: false`) || !strings.Contains(files["dns.yml"], `:enabled: false`) {
		t.Error("dhcp and dns must be disabled")
	}
	if strings.Contains(files["settings.yml"], "http_port") {
		t.Error("http must stay off without templates or httpboot")
	}
	if strings.Contains(files["dnsmasq.conf"], "dhcp-range") || !strings.Contains(files["dnsmasq.conf"], "enable-tftp") {
		t.Errorf("TFTP-only dnsmasq.conf is wrong:\n%s", files["dnsmasq.conf"])
	}
}

func TestDnsmasqConfigWithoutGateway(t *testing.T) {
	p := testSmartProxy()
	p.Spec.Features.DHCP.Subnets[0].Gateway = ""
	conf, err := dnsmasqConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(conf, "dhcp-option=3\n") {
		t.Errorf("an empty router option is needed, or dnsmasq announces itself:\n%s", conf)
	}
}

func TestSmartProxyDeployment(t *testing.T) {
	p := testSmartProxy()
	p.Spec.Storage = &foremanv1alpha1.SmartProxyStorage{Size: resource.MustParse("1Gi")}
	files, err := smartProxyConfig(p, testForeman())
	if err != nil {
		t.Fatal(err)
	}
	d, err := smartProxyDeployment(p, DefaultImages("test"), Capabilities{}, files, "")
	if err != nil {
		t.Fatal(err)
	}
	if d.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType {
		t.Error("the proxy must use the Recreate strategy")
	}
	var nets []multusNetwork
	if err := json.Unmarshal([]byte(d.Spec.Template.Annotations[multusAnnotation]), &nets); err != nil || len(nets) != 1 || nets[0].Name != "pxe-vlan" || nets[0].Interface != "net1" {
		t.Errorf("multus annotation = %q", d.Spec.Template.Annotations[multusAnnotation])
	}

	spec := d.Spec.Template.Spec
	if len(spec.Containers) != 2 || spec.Containers[1].Name != "dnsmasq" {
		t.Fatalf("containers = %d", len(spec.Containers))
	}
	proxy, dnsmasq := spec.Containers[0], spec.Containers[1]
	if proxy.SecurityContext.RunAsNonRoot == nil || !*proxy.SecurityContext.RunAsNonRoot || *proxy.SecurityContext.AllowPrivilegeEscalation {
		t.Error("the proxy container must run unprivileged")
	}
	caps := dnsmasq.SecurityContext.Capabilities
	if !slices.Contains(caps.Drop, "ALL") || !slices.Contains(caps.Add, "NET_ADMIN") || !slices.Contains(caps.Add, "KILL") || len(caps.Add) != 6 {
		t.Errorf("dnsmasq capabilities = %+v", caps)
	}
	env := map[string]string{}
	for _, e := range dnsmasq.Env {
		env[e.Name] = e.Value
	}
	if env["NET_INTERFACE"] != "net1" || env["NET_ADDRESS"] != "10.99.0.10/24" {
		t.Errorf("dnsmasq env = %v", env)
	}
	if len(spec.InitContainers) != 1 || spec.InitContainers[0].Image != "ghcr.io/slauger/foreman-operator-boot-files:test" {
		t.Errorf("init containers = %+v", spec.InitContainers)
	}

	vols := map[string]corev1.Volume{}
	for _, v := range spec.Volumes {
		vols[v.Name] = v
	}
	if vols["data"].PersistentVolumeClaim == nil || vols["data"].PersistentVolumeClaim.ClaimName != "pxe-data" {
		t.Error("the TFTP root must use the PVC")
	}
	if vols["dns-key"].Secret == nil || vols["dns-key"].Secret.SecretName != "tsig" {
		t.Error("the TSIG key must be mounted")
	}
	items := vols["settings-d"].ConfigMap.Items
	for _, it := range items {
		if it.Key == "settings.yml" || it.Key == "dnsmasq.conf" {
			t.Errorf("%s must not be mounted into settings.d", it.Key)
		}
	}
	if len(items) == 0 {
		t.Error("settings.d has no items")
	}
}

func TestSmartProxyRegistersAndRefreshes(t *testing.T) {
	f, secrets := readyForeman()
	c := newTestClient(f, secrets, testSmartProxy())
	api := newFakeForemanAPI()
	r := newSmartProxyReconciler(c, api)

	got := reconcileSmartProxy(t, r)
	if !controllerutil.ContainsFinalizer(got, smartProxyFinalizer) {
		t.Error("finalizer missing")
	}
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonRollingOut)
	cert := mustGet(t, c, "pxe-tls", newCertificateObject())
	usages, _, _ := unstructured.NestedStringSlice(cert.Object, "spec", "usages")
	if !slices.Contains(usages, "client auth") || !slices.Contains(usages, "server auth") {
		t.Errorf("usages = %v", usages)
	}
	mustGet(t, c, "pxe", &corev1.Service{})

	markDeploymentAvailable(t, c, "pxe")
	got = reconcileSmartProxy(t, r)
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionReady, metav1.ConditionTrue, reasonReconciled)
	if got.Status.ForemanID != 1 || api.proxies["pxe"].URL != "https://pxe.foreman.svc:8443" {
		t.Errorf("foremanID = %d, proxies = %v", got.Status.ForemanID, api.proxies)
	}
	if api.baseURL != "http://prod-web.foreman.svc:3000" || api.publicURL != "https://foreman.example.com" {
		t.Errorf("api urls = %s, %s", api.baseURL, api.publicURL)
	}

	// A spec change refreshes the features in Foreman.
	got.Spec.URL = "https://pxe2.foreman.svc:8443"
	got.Generation = 2
	if err := c.Update(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	markDeploymentAvailable(t, c, "pxe")
	got = reconcileSmartProxy(t, r)
	if len(api.refreshed) != 1 || api.proxies["pxe"].URL != "https://pxe2.foreman.svc:8443" {
		t.Errorf("refreshed = %v, url = %s", api.refreshed, api.proxies["pxe"].URL)
	}
	if got.Status.RegisteredGeneration != got.Generation {
		t.Errorf("registered generation = %d, want %d", got.Status.RegisteredGeneration, got.Generation)
	}
}

func TestSmartProxyAdoptsExistingRegistration(t *testing.T) {
	f, secrets := readyForeman()
	c := newTestClient(f, secrets, testSmartProxy())
	api := newFakeForemanAPI()
	api.proxies["pxe"] = &foremanapi.SmartProxy{ID: 42, Name: "pxe", URL: "https://old:8443"}
	r := newSmartProxyReconciler(c, api)

	reconcileSmartProxy(t, r)
	markDeploymentAvailable(t, c, "pxe")
	got := reconcileSmartProxy(t, r)
	if got.Status.ForemanID != 42 || api.proxies["pxe"].URL != "https://pxe.foreman.svc:8443" {
		t.Errorf("foremanID = %d, url = %s", got.Status.ForemanID, api.proxies["pxe"].URL)
	}
}

func TestSmartProxyRegistrationFailure(t *testing.T) {
	f, secrets := readyForeman()
	c := newTestClient(f, secrets, testSmartProxy())
	api := newFakeForemanAPI()
	api.createErr = errors.New("unable to communicate with the proxy")
	r := newSmartProxyReconciler(c, api)

	reconcileSmartProxy(t, r)
	markDeploymentAvailable(t, c, "pxe")
	got := reconcileSmartProxy(t, r)
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonRegistrationFail)
}

func TestSmartProxyWaitsForReadyForeman(t *testing.T) {
	_, secrets := readyForeman()
	c := newTestClient(testForeman(), secrets, testSmartProxy())
	r := newSmartProxyReconciler(c, newFakeForemanAPI())
	reconcileSmartProxy(t, r)
	markDeploymentAvailable(t, c, "pxe")
	got := reconcileSmartProxy(t, r)
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonWaitingForeman)
}

func TestSmartProxyDeletionUnregisters(t *testing.T) {
	f, secrets := readyForeman()
	c := newTestClient(f, secrets, testSmartProxy())
	api := newFakeForemanAPI()
	r := newSmartProxyReconciler(c, api)
	reconcileSmartProxy(t, r)
	markDeploymentAvailable(t, c, "pxe")
	got := reconcileSmartProxy(t, r)

	if err := c.Delete(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(context.Background(), reconcileRequest(got)); err != nil {
		t.Fatal(err)
	}
	if len(api.deleted) != 1 || api.deleted[0] != got.Status.ForemanID {
		t.Errorf("deleted = %v", api.deleted)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(got), &foremanv1alpha1.SmartProxy{}); err == nil {
		t.Error("the SmartProxy should be gone once the finalizer is removed")
	}
}

func TestSmartProxyDeletionWithoutForeman(t *testing.T) {
	p := testSmartProxy()
	p.Finalizers = []string{smartProxyFinalizer}
	p.Status.ForemanID = 3
	c := newTestClient(p)
	api := newFakeForemanAPI()
	r := newSmartProxyReconciler(c, api)
	if err := c.Delete(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(context.Background(), reconcileRequest(p)); err != nil {
		t.Fatal(err)
	}
	if len(api.deleted) != 0 {
		t.Error("nothing to unregister without a Foreman")
	}
}

func TestProxyEndpoint(t *testing.T) {
	tests := []struct {
		url  string
		host string
		port int32
	}{
		{"https://proxy.example.com:9443", "proxy.example.com", 9443},
		{"https://proxy.example.com", "proxy.example.com", 443},
	}
	for _, tt := range tests {
		p := testSmartProxy()
		p.Spec.URL = tt.url
		host, port, err := proxyEndpoint(p)
		if err != nil || host != tt.host || port != tt.port {
			t.Errorf("%s: host = %s, port = %d, err = %v", tt.url, host, port, err)
		}
	}
}
