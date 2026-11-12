package v1alpha1

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func issuerTLS() TLSSpec {
	return TLSSpec{IssuerRef: &IssuerReference{Name: "internal-ca"}}
}

func validForeman() *Foreman {
	return &Foreman{
		GenerateName: "test-foreman-",
		Namespace:    "default",
		Spec: ForemanSpec{
			FQDN:     "foreman.example.com",
			TLS:      issuerTLS(),
			Database: DatabaseSpec{Cluster: CNPGClusterRef{Name: "foreman-db"}},
			Valkey:   ValkeySpec{Host: "valkey"},
			Frontend: ForemanFrontendSpec{
				TLS: FrontendTLSSpec{Server: issuerTLS(), ClientCA: issuerTLS()},
			},
		},
	}
}

func validSmartProxy() *SmartProxy {
	return &SmartProxy{
		GenerateName: "test-proxy-",
		Namespace:    "default",
		Spec: SmartProxySpec{
			ForemanRef: "production",
			Profile:    SmartProxyProfileProvisioning,
			URL:        "https://proxy.example.com:8443",
			TLS:        issuerTLS(),
			Networks:   []SmartProxyNetwork{{Attachment: "vlan100", Address: "192.0.2.10/24"}},
			Features: SmartProxyFeatures{
				TFTP: &TFTPFeature{},
				DHCP: &DHCPFeature{
					Provider: DHCPProviderDnsmasq,
					Subnets:  []DHCPSubnet{{Network: "192.0.2.0/24", Range: "192.0.2.100-192.0.2.200"}},
				},
			},
			Storage: &SmartProxyStorage{Size: resource.MustParse("5Gi")},
		},
	}
}

func checkCreate(t *testing.T, obj client.Object, wantErr string) {
	t.Helper()
	err := k8sClient.Create(context.Background(), obj)
	if wantErr == "" {
		if err != nil {
			t.Fatalf("expected acceptance, got: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected rejection containing %q, got acceptance", wantErr)
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("expected error containing %q, got: %v", wantErr, err)
	}
}

func TestForemanValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Foreman)
		wantErr string
	}{
		{name: "valid", mutate: func(_ *Foreman) {}},
		{
			name:   "catalog plugin",
			mutate: func(f *Foreman) { f.Spec.Plugins = []ForemanPlugin{"foreman_remote_execution"} },
		},
		{
			name:    "plugin outside the catalog",
			mutate:  func(f *Foreman) { f.Spec.Plugins = []ForemanPlugin{"foreman_bootdisk"} },
			wantErr: "spec.plugins",
		},
		{
			name:    "duplicate plugin",
			mutate:  func(f *Foreman) { f.Spec.Plugins = []ForemanPlugin{"foreman_discovery", "foreman_discovery"} },
			wantErr: "spec.plugins",
		},
		{
			name:   "secret instead of issuer",
			mutate: func(f *Foreman) { f.Spec.TLS = TLSSpec{SecretName: "internal-tls"} },
		},
		{
			name:    "no certificate source",
			mutate:  func(f *Foreman) { f.Spec.TLS = TLSSpec{} },
			wantErr: "exactly one of issuerRef or secretName must be set",
		},
		{
			name:    "two certificate sources",
			mutate:  func(f *Foreman) { f.Spec.TLS.SecretName = "internal-tls" },
			wantErr: "exactly one of issuerRef or secretName must be set",
		},
		{
			name:    "empty FQDN",
			mutate:  func(f *Foreman) { f.Spec.FQDN = "" },
			wantErr: "spec.fqdn",
		},
		{
			name:    "empty database cluster",
			mutate:  func(f *Foreman) { f.Spec.Database.Cluster.Name = "" },
			wantErr: "spec.database.cluster.name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := validForeman()
			tt.mutate(obj)
			checkCreate(t, obj, tt.wantErr)
		})
	}
}

func TestForemanDefaults(t *testing.T) {
	obj := validForeman()
	if err := k8sClient.Create(context.Background(), obj); err != nil {
		t.Fatalf("create: %v", err)
	}

	if got := obj.Spec.Dynflow.WorkerReplicas; got == nil || *got != 1 {
		t.Errorf("workerReplicas = %v, want 1", got)
	}
	if got := obj.Spec.Dynflow.HostsQueueReplicas; got == nil || *got != 0 {
		t.Errorf("hostsQueueReplicas = %v, want 0", got)
	}
	if got := obj.Spec.Web.Replicas; got == nil || *got != 2 {
		t.Errorf("web replicas = %v, want 2", got)
	}
	if got := obj.Spec.Valkey.Port; got != 6379 {
		t.Errorf("valkey port = %d, want 6379", got)
	}
}

func TestFrontendValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ForemanFrontendSpec)
		wantErr string
	}{
		{
			name: "TLSRoute with gateway",
			mutate: func(f *ForemanFrontendSpec) {
				f.Exposure = FrontendExposureSpec{
					Type:       FrontendExposureTLSRoute,
					GatewayRef: &GatewayReference{Name: "public", TLSSectionName: "https"},
				}
			},
		},
		{
			name: "TLSRoute without gateway",
			mutate: func(f *ForemanFrontendSpec) {
				f.Exposure = FrontendExposureSpec{Type: FrontendExposureTLSRoute}
			},
			wantErr: "gatewayRef is required when type is TLSRoute",
		},
		{
			name: "unknown exposure",
			mutate: func(f *ForemanFrontendSpec) {
				f.Exposure = FrontendExposureSpec{Type: "NodePort"}
			},
			wantErr: "spec.frontend.exposure.type",
		},
		{
			name:    "missing client CA",
			mutate:  func(f *ForemanFrontendSpec) { f.TLS.ClientCA = TLSSpec{} },
			wantErr: "exactly one of issuerRef or secretName must be set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := validForeman()
			tt.mutate(&obj.Spec.Frontend)
			checkCreate(t, obj, tt.wantErr)
		})
	}
}

func TestSmartProxyValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*SmartProxy)
		wantErr string
	}{
		{name: "valid provisioning", mutate: func(_ *SmartProxy) {}},
		{
			name: "dns only needs no network",
			mutate: func(p *SmartProxy) {
				p.Spec.Networks = nil
				p.Spec.Features = SmartProxyFeatures{DNS: &DNSFeature{Server: "dns.example.com"}}
			},
		},
		{
			name: "valid execution",
			mutate: func(p *SmartProxy) {
				p.Spec.Profile = SmartProxyProfileExecution
				p.Spec.Networks = nil
				p.Spec.Features = SmartProxyFeatures{
					RemoteExecutionSSH: &RemoteExecutionSSHFeature{SSHKeySecretRef: "ssh"},
					Ansible:            &AnsibleFeature{},
				}
			},
		},
		{
			name: "execution with dhcp",
			mutate: func(p *SmartProxy) {
				p.Spec.Profile = SmartProxyProfileExecution
			},
			wantErr: "the Execution profile does not support dhcp, tftp, or dns",
		},
		{
			name: "provisioning with ansible",
			mutate: func(p *SmartProxy) {
				p.Spec.Features.Ansible = &AnsibleFeature{}
			},
			wantErr: "the Provisioning profile does not support remoteExecutionSSH or ansible",
		},
		{
			name:    "dnsmasq without network",
			mutate:  func(p *SmartProxy) { p.Spec.Networks = nil },
			wantErr: "the dnsmasq provider requires at least one network",
		},
		{
			name:    "dnsmasq without range",
			mutate:  func(p *SmartProxy) { p.Spec.Features.DHCP.Subnets[0].Range = "" },
			wantErr: "the dnsmasq provider requires a range",
		},
		{
			name:    "malformed range",
			mutate:  func(p *SmartProxy) { p.Spec.Features.DHCP.Subnets[0].Range = "192.0.2.100" },
			wantErr: "range",
		},
		{
			name: "dnsmasq with two subnets",
			mutate: func(p *SmartProxy) {
				p.Spec.Features.DHCP.Subnets = append(p.Spec.Features.DHCP.Subnets, DHCPSubnet{Network: "198.51.100.0/24"})
			},
			wantErr: "the dnsmasq provider supports exactly one subnet per SmartProxy",
		},
		{
			name:    "infoblox is not in the image",
			mutate:  func(p *SmartProxy) { p.Spec.Features.DHCP.Provider = "infoblox" },
			wantErr: "spec.features.dhcp.provider",
		},
		{
			name: "dns_nsupdate_gss has no keytab",
			mutate: func(p *SmartProxy) {
				p.Spec.Features.DNS = &DNSFeature{Provider: "dns_nsupdate_gss", Server: "dns.example.com"}
			},
			wantErr: "spec.features.dns.provider",
		},
		{
			name: "tftp without network",
			mutate: func(p *SmartProxy) {
				p.Spec.Networks = nil
				p.Spec.Features.DHCP = nil
			},
			wantErr: "tftp requires a network",
		},
		{
			name: "two networks",
			mutate: func(p *SmartProxy) {
				p.Spec.Networks = append(p.Spec.Networks, SmartProxyNetwork{Attachment: "vlan200", Address: "198.51.100.10/24"})
			},
			wantErr: "spec.networks",
		},
		{
			name:    "plain http URL",
			mutate:  func(p *SmartProxy) { p.Spec.URL = "http://proxy.example.com:8000" },
			wantErr: "spec.url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := validSmartProxy()
			tt.mutate(obj)
			checkCreate(t, obj, tt.wantErr)
		})
	}
}
