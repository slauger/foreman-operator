package controller

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	foremanv1alpha1 "github.com/slauger/foreman-operator/api/v1alpha1"
)

const (
	smartProxyAppName   = "foreman-smart-proxy"
	componentSmartProxy = "smart-proxy"

	proxyHTTPPort = 8000

	proxyConfigDir   = "/etc/foreman-proxy"
	proxyTLSDir      = "/etc/foreman-proxy/tls"
	proxyDNSKeyDir   = "/etc/foreman-proxy/dns"
	proxyDHCPDir     = "/var/lib/foreman-proxy/dhcp"
	proxyTFTPRoot    = "/var/lib/tftpboot"
	dnsmasqConfigDir = "/etc/dnsmasq-foreman"
	// Subdirectories of the data volume. The DHCP state lives outside the TFTP root, so
	// reservations and leases are not served over TFTP.
	dataTFTPSubPath   = "tftpboot"
	dataDHCPSubPath   = "dhcp"
	dnsmasqConfigFile = dnsmasqConfigDir + "/dnsmasq.conf"
	// The proxy checks the lease file at startup, so it lives in the directory both share.
	dnsmasqLeaseFile = proxyDHCPDir + "/dnsmasq.leases"

	// multusAnnotation requests secondary networks from Multus.
	multusAnnotation = "k8s.v1.cni.cncf.io/networks"

	// smartProxyFinalizer removes the proxy from Foreman before the resource goes away.
	smartProxyFinalizer = "platform.theforeman.org/smart-proxy-registration"
)

func smartProxyCertName(p *foremanv1alpha1.SmartProxy) string {
	return tlsSecretName(p.Spec.TLS, p.Name+"-tls")
}

// proxyEndpoint returns host and HTTPS port of spec.url.
func proxyEndpoint(p *foremanv1alpha1.SmartProxy) (string, int32, error) {
	u, err := url.Parse(p.Spec.URL)
	if err != nil {
		return "", 0, fmt.Errorf("invalid url %q: %w", p.Spec.URL, err)
	}
	port := int32(8443)
	if s := u.Port(); s != "" {
		n, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return "", 0, fmt.Errorf("invalid port in url %q", p.Spec.URL)
		}
		port = int32(n)
	} else if u.Scheme == "https" {
		port = 443
	}
	return u.Hostname(), port, nil
}

// needsDnsmasq reports whether the pod runs dnsmasq for DHCP or TFTP.
func needsDnsmasq(p *foremanv1alpha1.SmartProxy) bool {
	dhcp := p.Spec.Features.DHCP != nil && p.Spec.Features.DHCP.Provider == foremanv1alpha1.DHCPProviderDnsmasq
	return dhcp || (p.Spec.Features.TFTP != nil && len(p.Spec.Networks) > 0)
}

func needsHTTP(p *foremanv1alpha1.SmartProxy) bool {
	return p.Spec.Features.Templates != nil || p.Spec.Features.HTTPBoot != nil
}

// subnetMask converts a CIDR into network address and dotted netmask.
func subnetMask(cidr string) (string, string, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "", err
	}
	if len(ipnet.Mask) != net.IPv4len {
		return "", "", fmt.Errorf("only IPv4 subnets are supported: %s", cidr)
	}
	return ipnet.IP.String(), net.IP(ipnet.Mask).String(), nil
}

// tftpServerName is the address announced to hosts for TFTP.
func tftpServerName(p *foremanv1alpha1.SmartProxy) string {
	if p.Spec.Features.TFTP != nil && p.Spec.Features.TFTP.ServerName != "" {
		return p.Spec.Features.TFTP.ServerName
	}
	if len(p.Spec.Networks) > 0 {
		ip, _, err := net.ParseCIDR(p.Spec.Networks[0].Address)
		if err == nil {
			return ip.String()
		}
	}
	return ""
}

// moduleSettings renders a module file. enabled is true, false, "http", or "https"; the Smart
// Proxy treats the strings "true" and "false" as invalid and as enabled respectively.
func moduleSettings(enabled any, extra ...rubyEntry) string {
	m := rubyMap{}
	m.set("enabled", enabled)
	for _, e := range extra {
		m.set(e.key, e.value)
	}
	return m.String()
}

// moduleEnabled returns the Smart Proxy "enabled" value: https, or true (HTTP and HTTPS) for
// features that hosts reach.
func moduleEnabled(on bool, value any) any {
	if !on {
		return false
	}
	return value
}

// smartProxyConfig renders settings.yml, the module settings, and the dnsmasq configuration.
// ConfigMap keys cannot contain a slash, so module settings use their plain file names and
// are mounted into settings.d through an item list.
func smartProxyConfig(p *foremanv1alpha1.SmartProxy, f *foremanv1alpha1.Foreman) (map[string]string, error) {
	_, httpsPort, err := proxyEndpoint(p)
	if err != nil {
		return nil, err
	}

	s := rubyMap{}
	s.set("settings_directory", proxyConfigDir+"/settings.d")
	s.set("ssl_certificate", proxyTLSDir+"/tls.crt")
	s.set("ssl_private_key", proxyTLSDir+"/tls.key")
	s.set("ssl_ca_file", proxyTLSDir+"/ca.crt")
	s.set("foreman_ssl_cert", proxyTLSDir+"/tls.crt")
	s.set("foreman_ssl_key", proxyTLSDir+"/tls.key")
	s.set("foreman_ssl_ca", proxyTLSDir+"/ca.crt")
	s.set("trusted_hosts", []string{f.Spec.FQDN})
	s.set("foreman_url", foremanPublicURL(f))
	s.set("bind_host", []string{"*"})
	s.set("https_port", httpsPort)
	if needsHTTP(p) {
		s.set("http_port", proxyHTTPPort)
	}
	s.set("log_file", "STDOUT")
	s.set("log_level", "INFO")

	feat := p.Spec.Features
	files := map[string]string{
		"settings.yml": s.String(),
	}
	add := func(name, content string) { files[name] = content }

	add("tftp.yml", moduleSettings(moduleEnabled(feat.TFTP != nil, "https"),
		rubyEntry{"tftproot", proxyTFTPRoot},
		rubyEntry{"tftp_servername", tftpServerName(p)},
	))
	templateURL := ""
	if feat.Templates != nil {
		templateURL = feat.Templates.URL
	}
	add("templates.yml", moduleSettings(moduleEnabled(feat.Templates != nil, true),
		rubyEntry{"template_url", templateURL},
	))
	add("httpboot.yml", moduleSettings(moduleEnabled(feat.HTTPBoot != nil, true),
		rubyEntry{"root_dir", proxyTFTPRoot},
	))
	add("bmc.yml", moduleSettings(moduleEnabled(feat.BMC != nil, "https"),
		rubyEntry{"bmc_default_provider", bmcProvider(feat.BMC)},
	))
	add("logs.yml", moduleSettings("https"))
	for _, disabled := range []string{"puppet.yml", "puppetca.yml", "realm.yml", "facts.yml"} {
		add(disabled, moduleSettings(false))
	}

	if dhcp := feat.DHCP; dhcp != nil {
		subnets := make([]string, 0, len(dhcp.Subnets))
		for _, sn := range dhcp.Subnets {
			network, mask, err := subnetMask(sn.Network)
			if err != nil {
				return nil, err
			}
			subnets = append(subnets, network+"/"+mask)
		}
		add("dhcp.yml", moduleSettings("https",
			rubyEntry{"use_provider", "dhcp_" + string(dhcp.Provider)},
			rubyEntry{"server", "127.0.0.1"},
			rubyEntry{"subnets", subnets},
		))
		if dhcp.Provider == foremanv1alpha1.DHCPProviderDnsmasq {
			add("dhcp_dnsmasq.yml", moduleSettings(true,
				rubyEntry{"config", dnsmasqConfigFile},
				rubyEntry{"target_dir", proxyDHCPDir + "/"},
				rubyEntry{"lease_file", dnsmasqLeaseFile},
				rubyEntry{"reload_cmd", "touch " + proxyDHCPDir + "/.reload"},
				// The packaged plugin has no default for it. The value is in seconds.
				rubyEntry{"blacklist_duration_minutes", 1800},
			))
		}
	} else {
		add("dhcp.yml", moduleSettings(false))
	}

	if dns := feat.DNS; dns != nil {
		provider := dns.Provider
		if provider == "" {
			provider = "dns_nsupdate"
		}
		add("dns.yml", moduleSettings("https", rubyEntry{"use_provider", provider}))
		nsupdate := []rubyEntry{{"dns_server", dns.Server}}
		if dns.KeySecretRef != "" {
			nsupdate = append(nsupdate, rubyEntry{"dns_key", proxyDNSKeyDir + "/key"})
		}
		add(provider+".yml", moduleSettings(true, nsupdate...))
	} else {
		add("dns.yml", moduleSettings(false))
	}

	if needsDnsmasq(p) {
		conf, err := dnsmasqConfig(p)
		if err != nil {
			return nil, err
		}
		files["dnsmasq.conf"] = conf
	}
	return files, nil
}

func bmcProvider(b *foremanv1alpha1.BMCFeature) string {
	if b == nil || b.DefaultProvider == "" {
		return "ipmitool"
	}
	return b.DefaultProvider
}

// dnsmasqConfig renders the dnsmasq configuration for the first secondary network.
func dnsmasqConfig(p *foremanv1alpha1.SmartProxy) (string, error) {
	lines := []string{
		"# Generated by foreman-operator. Changes are overwritten.",
		"port=0",
		"interface=net1",
		"bind-interfaces",
		"user=dnsmasq",
		"pid-file=",
		"keep-in-foreground",
		"log-facility=-",
		"log-dhcp",
	}

	if dhcp := p.Spec.Features.DHCP; dhcp != nil && dhcp.Provider == foremanv1alpha1.DHCPProviderDnsmasq {
		sn := dhcp.Subnets[0]
		_, mask, err := subnetMask(sn.Network)
		if err != nil {
			return "", err
		}
		// The dnsmasq plugin reads the subnet from the last four fields of dhcp-range and
		// only understands numeric DHCP option codes.
		first, last, _ := strings.Cut(sn.Range, "-")
		lines = append(lines, fmt.Sprintf("dhcp-range=%s,%s,%s,12h", first, last, mask))
		if sn.Gateway != "" {
			lines = append(lines, "dhcp-option=3,"+sn.Gateway)
		} else {
			// dnsmasq would otherwise announce itself as the router.
			lines = append(lines, "dhcp-option=3")
		}
		if len(sn.DNSServers) > 0 {
			lines = append(lines, "dhcp-option=6,"+strings.Join(sn.DNSServers, ","))
		}
		lines = append(lines,
			"dhcp-leasefile="+dnsmasqLeaseFile,
			"dhcp-hostsfile="+proxyDHCPDir+"/dhcphosts",
			"dhcp-optsfile="+proxyDHCPDir+"/dhcpopts.conf",
		)
	}

	if p.Spec.Features.TFTP != nil {
		lines = append(lines, "enable-tftp", "tftp-root="+proxyTFTPRoot)
	}
	return strings.Join(lines, "\n") + "\n", nil
}

type multusNetwork struct {
	Name      string `json:"name"`
	Interface string `json:"interface"`
}

func multusNetworks(p *foremanv1alpha1.SmartProxy) (string, error) {
	nets := make([]multusNetwork, 0, len(p.Spec.Networks))
	for i, n := range p.Spec.Networks {
		nets = append(nets, multusNetwork{Name: n.Attachment, Interface: "net" + strconv.Itoa(i+1)})
	}
	data, err := json.Marshal(nets)
	return string(data), err
}

// moduleItems maps the module settings of a ConfigMap into the settings.d directory.
func moduleItems(config map[string]string) []corev1.KeyToPath {
	keys := make([]string, 0, len(config))
	for k := range config {
		if k != "settings.yml" && strings.HasSuffix(k, ".yml") {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	items := make([]corev1.KeyToPath, 0, len(keys))
	for _, k := range keys {
		items = append(items, corev1.KeyToPath{Key: k, Path: k})
	}
	return items
}

func smartProxyDeployment(p *foremanv1alpha1.SmartProxy, images Images, caps Capabilities, config map[string]string, certChecksum string) (*appsv1.Deployment, error) {
	host, httpsPort, err := proxyEndpoint(p)
	if err != nil {
		return nil, err
	}
	image := imageRef(p.Spec.Image, images.SmartProxy, images.Tag)
	labels := commonLabels(smartProxyAppName, p.Name, componentSmartProxy)

	volumes := []corev1.Volume{
		{Name: "config", ConfigMap: &corev1.ConfigMapVolumeSource{
			Name: p.Name,
		}},
		{Name: "settings-d", ConfigMap: &corev1.ConfigMapVolumeSource{
			Name:  p.Name,
			Items: moduleItems(config),
		}},
		{Name: "tls", Secret: &corev1.SecretVolumeSource{SecretName: smartProxyCertName(p)}},
		{Name: "tmp", EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}
	if p.Spec.Storage != nil {
		volumes = append(volumes, corev1.Volume{Name: "data",
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: p.Name + "-data"}})
	} else {
		volumes = append(volumes, corev1.Volume{Name: "data", EmptyDir: &corev1.EmptyDirVolumeSource{}})
	}

	proxyMounts := []corev1.VolumeMount{
		{Name: "config", MountPath: proxyConfigDir + "/settings.yml", SubPath: "settings.yml", ReadOnly: true},
		{Name: "settings-d", MountPath: proxyConfigDir + "/settings.d", ReadOnly: true},
		{Name: "tls", MountPath: proxyTLSDir, ReadOnly: true},
		{Name: "data", MountPath: proxyDHCPDir, SubPath: dataDHCPSubPath},
		{Name: "data", MountPath: proxyTFTPRoot, SubPath: dataTFTPSubPath},
		{Name: "tmp", MountPath: "/tmp"},
	}
	if dns := p.Spec.Features.DNS; dns != nil && dns.KeySecretRef != "" {
		volumes = append(volumes, corev1.Volume{Name: "dns-key", Secret: &corev1.SecretVolumeSource{
			SecretName: dns.KeySecretRef,
			Items:      []corev1.KeyToPath{{Key: "key", Path: "key"}},
		}})
		proxyMounts = append(proxyMounts, corev1.VolumeMount{Name: "dns-key", MountPath: proxyDNSKeyDir, ReadOnly: true})
	}
	if needsDnsmasq(p) {
		proxyMounts = append(proxyMounts, corev1.VolumeMount{Name: "config", MountPath: dnsmasqConfigFile, SubPath: "dnsmasq.conf", ReadOnly: true})
	}

	ports := []corev1.ContainerPort{{Name: "https", ContainerPort: httpsPort}}
	if needsHTTP(p) {
		ports = append(ports, corev1.ContainerPort{Name: "http", ContainerPort: proxyHTTPPort})
	}
	// A TCP probe closes the connection before the TLS handshake, and WEBrick logs an SSL error
	// with a stack trace for each one. curl completes the handshake and checks the served
	// certificate against the CA. Any HTTP status counts, since the proxy answers 403 to hosts
	// outside trusted_hosts.
	endpoint := net.JoinHostPort(host, strconv.Itoa(int(httpsPort)))
	probe := &corev1.Probe{
		Exec: &corev1.ExecAction{Command: []string{"curl", "-s", "-o", "/dev/null", "-m", "4",
			"--cacert", proxyTLSDir + "/ca.crt", "--cert", proxyTLSDir + "/tls.crt", "--key", proxyTLSDir + "/tls.key",
			"--resolve", endpoint + ":127.0.0.1", "https://" + endpoint + "/version"}},
		PeriodSeconds:  10,
		TimeoutSeconds: 5,
	}
	proxySC := restrictedContainerSecurityContext()
	proxySC.RunAsNonRoot = new(true)
	// The directory shared with dnsmasq is writable for group 0. Without an explicit group the
	// runtime takes the primary group of the image user from /etc/passwd instead.
	proxySC.RunAsGroup = new(int64(0))

	containers := []corev1.Container{{
		Name:            "proxy",
		Image:           image,
		ImagePullPolicy: pullPolicy(p.Spec.Image),
		Ports:           ports,
		SecurityContext: proxySC,
		ReadinessProbe:  probe,
		LivenessProbe:   probe,
		Resources:       defaultResources(p.Spec.Resources, "50m", "256Mi", "512Mi"),
		VolumeMounts:    proxyMounts,
	}}
	if needsDnsmasq(p) {
		containers = append(containers, dnsmasqContainer(p, image))
	}

	psc := podSecurityContext(caps.OpenShift)
	// dnsmasq starts as root, so the pod as a whole cannot require non-root.
	psc.RunAsNonRoot = nil

	annotations := map[string]string{checksumAnnotation: checksum(checksumMap(config), certChecksum)}
	if len(p.Spec.Networks) > 0 {
		nets, err := multusNetworks(p)
		if err != nil {
			return nil, err
		}
		annotations[multusAnnotation] = nets
	}

	d := &appsv1.Deployment{}
	d.Name = p.Name
	d.Namespace = p.Namespace
	d.Labels = labels
	d.Spec = appsv1.DeploymentSpec{
		Replicas: new(int32(1)),
		// One pod owns the leases and the secondary address, so two must never overlap.
		Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
		Selector: &metav1.LabelSelector{MatchLabels: selectorLabels(smartProxyAppName, p.Name, componentSmartProxy)},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels, Annotations: annotations},
			Spec: corev1.PodSpec{
				EnableServiceLinks: new(false),
				SecurityContext:    psc,
				InitContainers:     bootFilesInitContainers(p, images),
				Containers:         containers,
				Volumes:            volumes,
			},
		},
	}
	return d, nil
}

// bootFilesInitContainers copies the PXE boot loaders into the TFTP root before the proxy
// starts, so a new image version also updates the loaders on a persistent volume.
func bootFilesInitContainers(p *foremanv1alpha1.SmartProxy, images Images) []corev1.Container {
	tftp := p.Spec.Features.TFTP
	if tftp == nil {
		return nil
	}
	return []corev1.Container{{
		Name:            "boot-files",
		Image:           imageRef(tftp.BootFiles, images.BootFiles, images.Tag),
		ImagePullPolicy: pullPolicy(tftp.BootFiles),
		SecurityContext: restrictedContainerSecurityContext(),
		Resources:       defaultResources(corev1.ResourceRequirements{}, "10m", "16Mi", "64Mi"),
		VolumeMounts:    []corev1.VolumeMount{{Name: "data", MountPath: proxyTFTPRoot, SubPath: dataTFTPSubPath}},
	}}
}

// dnsmasqContainer runs dnsmasq under the supervisor of the image. It is the only privileged
// part of the pod: dnsmasq needs raw sockets and interface control for DHCP.
func dnsmasqContainer(p *foremanv1alpha1.SmartProxy, image string) corev1.Container {
	env := []corev1.EnvVar{
		{Name: "DNSMASQ_CONFIG", Value: dnsmasqConfigFile},
		{Name: "RELOAD_TRIGGER", Value: proxyDHCPDir + "/.reload"},
	}
	if len(p.Spec.Networks) > 0 {
		env = append(env,
			corev1.EnvVar{Name: "NET_INTERFACE", Value: "net1"},
			corev1.EnvVar{Name: "NET_ADDRESS", Value: p.Spec.Networks[0].Address},
		)
	}
	return corev1.Container{
		Name:            "dnsmasq",
		Image:           image,
		ImagePullPolicy: pullPolicy(p.Spec.Image),
		Command:         []string{"/usr/local/bin/dnsmasq-supervisor"},
		Env:             env,
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:                new(int64(0)),
			RunAsNonRoot:             new(false),
			AllowPrivilegeEscalation: new(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
				// KILL lets the supervisor signal dnsmasq after it dropped to its own user.
				Add: []corev1.Capability{"NET_ADMIN", "NET_RAW", "NET_BIND_SERVICE", "SETUID", "SETGID", "KILL"},
			},
		},
		Resources: defaultResources(corev1.ResourceRequirements{}, "10m", "32Mi", "128Mi"),
		VolumeMounts: []corev1.VolumeMount{
			{Name: "config", MountPath: dnsmasqConfigFile, SubPath: "dnsmasq.conf", ReadOnly: true},
			{Name: "data", MountPath: proxyDHCPDir, SubPath: dataDHCPSubPath},
			{Name: "data", MountPath: proxyTFTPRoot, SubPath: dataTFTPSubPath, ReadOnly: true},
		},
	}
}

func smartProxyService(p *foremanv1alpha1.SmartProxy) (*corev1.Service, error) {
	_, httpsPort, err := proxyEndpoint(p)
	if err != nil {
		return nil, err
	}
	svc := &corev1.Service{}
	svc.Name = p.Name
	svc.Namespace = p.Namespace
	svc.Labels = commonLabels(smartProxyAppName, p.Name, componentSmartProxy)
	svc.Spec = corev1.ServiceSpec{
		Selector: selectorLabels(smartProxyAppName, p.Name, componentSmartProxy),
		Ports: []corev1.ServicePort{{
			Name:       "https",
			Port:       httpsPort,
			TargetPort: intstr.FromString("https"),
		}},
	}
	return svc, nil
}

func smartProxyPVC(p *foremanv1alpha1.SmartProxy) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{}
	pvc.Name = p.Name + "-data"
	pvc.Namespace = p.Namespace
	pvc.Labels = commonLabels(smartProxyAppName, p.Name, componentSmartProxy)
	pvc.Spec = corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceStorage: p.Spec.Storage.Size},
		},
	}
	if p.Spec.Storage.StorageClass != "" {
		pvc.Spec.StorageClassName = new(p.Spec.Storage.StorageClass)
	}
	return pvc
}
