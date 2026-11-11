package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

// ForemanFrontendSpec configures the Apache container in each web pod. It terminates TLS and
// mutual TLS and forwards to Puma over a Unix socket, so Puma is not reachable from the network.
type ForemanFrontendSpec struct {
	// Image selects the frontend container image.
	// +optional
	Image ImageSpec `json:"image,omitempty"`

	// Resources sets the compute resources of the frontend container.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// TLS selects the server certificate and the client CA.
	TLS FrontendTLSSpec `json:"tls"`

	// Exposure configures how clients reach Foreman.
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

// FrontendExposureSpec configures how clients reach Foreman.
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

	// Namespace of the Gateway. Defaults to the namespace of the Foreman.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// TLSSectionName is the listener with TLS passthrough for the TLSRoute.
	// +optional
	TLSSectionName string `json:"tlsSectionName,omitempty"`

	// HTTPSectionName is the listener for the HTTPRoute used by installers.
	// +optional
	HTTPSectionName string `json:"httpSectionName,omitempty"`
}
