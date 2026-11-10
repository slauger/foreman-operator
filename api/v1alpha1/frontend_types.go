package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Foreman",type=string,JSONPath=`.spec.foremanRef`
// +kubebuilder:printcolumn:name="Exposure",type=string,JSONPath=`.spec.exposure.type`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Frontend is the Schema for the frontends API.
// It terminates mutual TLS in the cluster and exposes Foreman to clients.
type Frontend struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FrontendSpec   `json:"spec,omitempty"`
	Status FrontendStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FrontendList contains a list of Frontend.
type FrontendList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Frontend `json:"items"`
}

// FrontendSpec defines the desired state of Frontend.
type FrontendSpec struct {
	// ForemanRef names the Foreman served by this Frontend.
	// +kubebuilder:validation:MinLength=1
	ForemanRef string `json:"foremanRef"`

	// Replicas is the number of frontend pods.
	// +kubebuilder:default=2
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Image selects the frontend container image.
	// +optional
	Image ImageSpec `json:"image,omitempty"`

	// TLS selects the server certificate and the client CA.
	TLS FrontendTLSSpec `json:"tls"`

	// Exposure configures how clients reach this Frontend.
	// +optional
	Exposure FrontendExposureSpec `json:"exposure,omitempty"`
}

// FrontendTLSSpec selects the server certificate and the client CA.
type FrontendTLSSpec struct {
	// Server selects the certificate presented to clients.
	Server TLSSpec `json:"server"`

	// ClientCA selects the CA that verifies client certificates.
	ClientCA TLSSpec `json:"clientCA"`
}

// FrontendExposureType selects the exposure mechanism.
// +kubebuilder:validation:Enum=Auto;LoadBalancer;Route;TLSRoute;ClusterIP
type FrontendExposureType string

// Frontend exposure types.
const (
	// FrontendExposureAuto uses a Route on OpenShift and a LoadBalancer elsewhere.
	FrontendExposureAuto         FrontendExposureType = "Auto"
	FrontendExposureLoadBalancer FrontendExposureType = "LoadBalancer"
	FrontendExposureRoute        FrontendExposureType = "Route"
	FrontendExposureTLSRoute     FrontendExposureType = "TLSRoute"
	FrontendExposureClusterIP    FrontendExposureType = "ClusterIP"
)

// FrontendExposureSpec configures how clients reach a Frontend.
// +kubebuilder:validation:XValidation:rule="self.type != 'TLSRoute' || has(self.gatewayRef)",message="gatewayRef is required when type is TLSRoute"
type FrontendExposureSpec struct {
	// Type selects the exposure mechanism.
	// +kubebuilder:default=Auto
	Type FrontendExposureType `json:"type,omitempty"`

	// GatewayRef selects the Gateway and listeners for type TLSRoute.
	// +optional
	GatewayRef *GatewayReference `json:"gatewayRef,omitempty"`

	// Annotations are added to the created Service or Route.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// GatewayReference selects a Gateway and its listeners.
type GatewayReference struct {
	// Name of the Gateway.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace of the Gateway. Defaults to the namespace of the Frontend.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// TLSSectionName is the listener with TLS passthrough for the TLSRoute.
	// +optional
	TLSSectionName string `json:"tlsSectionName,omitempty"`

	// HTTPSectionName is the listener for the HTTPRoute used by installers.
	// +optional
	HTTPSectionName string `json:"httpSectionName,omitempty"`
}

// FrontendStatus defines the observed state of Frontend.
type FrontendStatus struct {
	// ObservedGeneration is the generation last processed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase is the current lifecycle phase.
	// +optional
	Phase string `json:"phase,omitempty"`

	// Conditions represent the latest observations.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &Frontend{}, &FrontendList{})
		return nil
	})
}
