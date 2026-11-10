package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Profile",type=string,JSONPath=`.spec.profile`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.url`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SmartProxy is the Schema for the smartproxies API.
// Each resource is one Smart Proxy registered in Foreman, always with one replica.
type SmartProxy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SmartProxySpec   `json:"spec,omitempty"`
	Status SmartProxyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SmartProxyList contains a list of SmartProxy.
type SmartProxyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SmartProxy `json:"items"`
}

// SmartProxyProfile selects the feature set and the required privileges.
// +kubebuilder:validation:Enum=Provisioning;Execution
type SmartProxyProfile string

// Smart Proxy profiles.
const (
	SmartProxyProfileProvisioning SmartProxyProfile = "Provisioning"
	SmartProxyProfileExecution    SmartProxyProfile = "Execution"
)

// SmartProxySpec defines the desired state of SmartProxy.
// +kubebuilder:validation:XValidation:rule="self.profile != 'Execution' || (!has(self.features.dhcp) && !has(self.features.tftp) && !has(self.features.dns))",message="the Execution profile does not support dhcp, tftp, or dns"
// +kubebuilder:validation:XValidation:rule="self.profile != 'Provisioning' || (!has(self.features.remoteExecutionSSH) && !has(self.features.ansible))",message="the Provisioning profile does not support remoteExecutionSSH or ansible"
// +kubebuilder:validation:XValidation:rule="!(has(self.features.dhcp) && self.features.dhcp.provider == 'dnsmasq') || (has(self.networks) && size(self.networks) > 0)",message="the dnsmasq provider requires at least one network"
// +kubebuilder:validation:XValidation:rule="!(has(self.features.dhcp) && self.features.dhcp.provider == 'dnsmasq') || size(self.features.dhcp.subnets) == 1",message="the dnsmasq provider supports exactly one subnet per SmartProxy"
// +kubebuilder:validation:XValidation:rule="!(has(self.features.dhcp) && self.features.dhcp.provider == 'dnsmasq') || self.features.dhcp.subnets.all(s, has(s.range))",message="the dnsmasq provider requires a range"
type SmartProxySpec struct {
	// ForemanRef names the Foreman this proxy registers with.
	// +kubebuilder:validation:MinLength=1
	ForemanRef string `json:"foremanRef"`

	// Profile selects the feature set and the required privileges.
	Profile SmartProxyProfile `json:"profile"`

	// URL is the address registered in Foreman. Its hostname must match the certificate.
	// +kubebuilder:validation:Pattern=`^https://`
	URL string `json:"url"`

	// Image selects the Smart Proxy container image.
	// +optional
	Image ImageSpec `json:"image,omitempty"`

	// TLS selects the source of the server and client certificates.
	TLS TLSSpec `json:"tls"`

	// Networks attaches the pod to secondary networks through Multus.
	// +optional
	Networks []SmartProxyNetwork `json:"networks,omitempty"`

	// Features configures the enabled Smart Proxy features.
	Features SmartProxyFeatures `json:"features"`

	// Storage configures the volume for leases and boot files.
	// +optional
	Storage *SmartProxyStorage `json:"storage,omitempty"`

	// Resources sets the compute resources of the proxy container.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// SmartProxyNetwork attaches the pod to a secondary network.
type SmartProxyNetwork struct {
	// Attachment names a NetworkAttachmentDefinition in the same namespace.
	// +kubebuilder:validation:MinLength=1
	Attachment string `json:"attachment"`

	// Address is the static address in CIDR notation.
	// +kubebuilder:validation:MinLength=1
	Address string `json:"address"`
}

// SmartProxyFeatures configures the enabled Smart Proxy features.
type SmartProxyFeatures struct {
	// TFTP enables the TFTP feature.
	// +optional
	TFTP *TFTPFeature `json:"tftp,omitempty"`

	// Templates enables the templates feature.
	// +optional
	Templates *TemplatesFeature `json:"templates,omitempty"`

	// DHCP enables the DHCP feature.
	// +optional
	DHCP *DHCPFeature `json:"dhcp,omitempty"`

	// DNS enables the DNS feature.
	// +optional
	DNS *DNSFeature `json:"dns,omitempty"`

	// BMC enables the BMC feature.
	// +optional
	BMC *BMCFeature `json:"bmc,omitempty"`

	// HTTPBoot enables the httpboot feature.
	// +optional
	HTTPBoot *HTTPBootFeature `json:"httpBoot,omitempty"`

	// RemoteExecutionSSH enables Remote Execution over SSH.
	// +optional
	RemoteExecutionSSH *RemoteExecutionSSHFeature `json:"remoteExecutionSSH,omitempty"`

	// Ansible enables the Ansible feature.
	// +optional
	Ansible *AnsibleFeature `json:"ansible,omitempty"`
}

// TFTPFeature configures TFTP.
type TFTPFeature struct {
	// ServerName overrides the TFTP server name announced to hosts.
	// +optional
	ServerName string `json:"serverName,omitempty"`
	// BootFiles selects the image whose PXE boot loaders are copied into the TFTP root.
	// +optional
	BootFiles ImageSpec `json:"bootFiles,omitempty"`
}

// TemplatesFeature configures the template endpoint for hosts being built.
type TemplatesFeature struct {
	// URL is the address hosts use to fetch templates.
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`
}

// DHCPProvider selects the Smart Proxy DHCP provider.
// +kubebuilder:validation:Enum=dnsmasq;infoblox
type DHCPProvider string

// DHCP providers.
const (
	DHCPProviderDnsmasq  DHCPProvider = "dnsmasq"
	DHCPProviderInfoblox DHCPProvider = "infoblox"
)

// DHCPFeature configures DHCP.
type DHCPFeature struct {
	// Provider selects the DHCP provider.
	Provider DHCPProvider `json:"provider"`

	// Subnets lists the subnets served by this proxy.
	// +kubebuilder:validation:MinItems=1
	Subnets []DHCPSubnet `json:"subnets"`
}

// DHCPSubnet describes one served subnet.
type DHCPSubnet struct {
	// Network is the subnet in CIDR notation.
	// +kubebuilder:validation:MinLength=1
	Network string `json:"network"`

	// Range is the dynamic range as "first-last". Without it, only reserved hosts get an address.
	// +kubebuilder:validation:Pattern=`^[0-9.]+-[0-9.]+$`
	// +optional
	Range string `json:"range,omitempty"`
	// Gateway is announced as the default router. Without it, no router is announced.
	// +optional
	Gateway string `json:"gateway,omitempty"`
	// DNSServers are announced to hosts.
	// +optional
	DNSServers []string `json:"dnsServers,omitempty"`
}

// DNSFeature configures DNS.
type DNSFeature struct {
	// Provider selects the DNS provider.
	// +kubebuilder:validation:Enum=dns_nsupdate;dns_nsupdate_gss
	// +kubebuilder:default=dns_nsupdate
	// +optional
	Provider string `json:"provider,omitempty"`

	// Server is the DNS server to update.
	// +kubebuilder:validation:MinLength=1
	Server string `json:"server"`

	// KeySecretRef names the Secret with the TSIG key under "key".
	// +optional
	KeySecretRef string `json:"keySecretRef,omitempty"`
}

// BMCFeature configures power management.
type BMCFeature struct {
	// DefaultProvider selects the BMC provider.
	// +kubebuilder:validation:Enum=ipmitool;freeipmi;redfish;shell;ssh
	// +optional
	DefaultProvider string `json:"defaultProvider,omitempty"`
}

// HTTPBootFeature enables UEFI HTTP Boot.
type HTTPBootFeature struct{}

// RemoteExecutionSSHFeature configures Remote Execution over SSH.
type RemoteExecutionSSHFeature struct {
	// SSHKeySecretRef names the Secret with "id_rsa" and "id_rsa.pub".
	// +kubebuilder:validation:MinLength=1
	SSHKeySecretRef string `json:"sshKeySecretRef"`
}

// AnsibleFeature enables the Ansible feature.
type AnsibleFeature struct{}

// SmartProxyStorage configures the volume for leases and boot files.
type SmartProxyStorage struct {
	// Size of the volume.
	Size resource.Quantity `json:"size"`

	// StorageClass of the volume. Empty uses the cluster default.
	// +optional
	StorageClass string `json:"storageClass,omitempty"`
}

// SmartProxyStatus defines the observed state of SmartProxy.
type SmartProxyStatus struct {
	// ObservedGeneration is the generation last processed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase is the current lifecycle phase.
	// +optional
	Phase string `json:"phase,omitempty"`

	// ForemanID is the identifier assigned by Foreman on registration.
	// +optional
	ForemanID int64 `json:"foremanID,omitempty"`
	// RegisteredGeneration is the generation whose features were last refreshed in Foreman.
	// +optional
	RegisteredGeneration int64 `json:"registeredGeneration,omitempty"`

	// Conditions represent the latest observations.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &SmartProxy{}, &SmartProxyList{})
		return nil
	})
}
