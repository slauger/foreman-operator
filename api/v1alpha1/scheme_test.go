package v1alpha1

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestAddToSchemeRegistersKinds(t *testing.T) {
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	for _, kind := range []string{"Foreman", "ForemanList", "SmartProxy", "SmartProxyList"} {
		if !s.Recognizes(GroupVersion.WithKind(kind)) {
			t.Errorf("kind %s is not registered", kind)
		}
	}
}

func TestSmartProxyDeepCopyIsIndependent(t *testing.T) {
	original := &SmartProxy{
		Spec: SmartProxySpec{
			Networks: []SmartProxyNetwork{{Attachment: "vlan100", Address: "192.0.2.10/24"}},
			Features: SmartProxyFeatures{
				DHCP: &DHCPFeature{
					Provider: DHCPProviderDnsmasq,
					Subnets:  []DHCPSubnet{{Network: "192.0.2.0/24"}},
				},
			},
			Storage: &SmartProxyStorage{Size: resource.MustParse("5Gi")},
		},
	}

	clone := original.DeepCopy()
	clone.Spec.Networks[0].Address = "198.51.100.10/24"
	clone.Spec.Features.DHCP.Subnets[0].Network = "198.51.100.0/24"

	if got := original.Spec.Networks[0].Address; got != "192.0.2.10/24" {
		t.Errorf("original network changed: %s", got)
	}
	if got := original.Spec.Features.DHCP.Subnets[0].Network; got != "192.0.2.0/24" {
		t.Errorf("original subnet changed: %s", got)
	}
}
