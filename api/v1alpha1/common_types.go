package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

// Condition types shared by all resources.
const (
	// ConditionReady indicates that the resource is fully reconciled and serving.
	ConditionReady = "Ready"
	// ConditionProgressing indicates that a change is being rolled out.
	ConditionProgressing = "Progressing"
	// ConditionDegraded indicates that the resource runs with reduced function.
	ConditionDegraded = "Degraded"
	// ConditionPaused indicates that reconciliation is suspended by annotation.
	ConditionPaused = "Paused"
)

// PausedAnnotation suspends reconciliation of a single resource when set to "true".
const PausedAnnotation = "platform.theforeman.org/paused"

// ImageSpec selects a container image.
type ImageSpec struct {
	// Repository is the image repository. The operator picks a default if empty.
	// +optional
	Repository string `json:"repository,omitempty"`

	// Tag is the image tag.
	// +optional
	Tag string `json:"tag,omitempty"`

	// Digest pins the image and takes precedence over Tag.
	// +kubebuilder:validation:Pattern=`^sha256:[a-f0-9]{64}$`
	// +optional
	Digest string `json:"digest,omitempty"`

	// PullPolicy is the image pull policy.
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	// +kubebuilder:default=IfNotPresent
	// +optional
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
}

// IssuerReference identifies a cert-manager issuer.
type IssuerReference struct {
	// Name of the issuer.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Kind of the issuer.
	// +kubebuilder:validation:Enum=Issuer;ClusterIssuer
	// +kubebuilder:default=Issuer
	// +optional
	Kind string `json:"kind,omitempty"`
}

// TLSSpec selects where certificates come from.
// +kubebuilder:validation:XValidation:rule="has(self.issuerRef) != has(self.secretName)",message="exactly one of issuerRef or secretName must be set"
type TLSSpec struct {
	// IssuerRef requests certificates from a cert-manager issuer.
	// +optional
	IssuerRef *IssuerReference `json:"issuerRef,omitempty"`

	// SecretName references an existing Secret with tls.crt, tls.key, and ca.crt.
	// +optional
	SecretName string `json:"secretName,omitempty"`
}
