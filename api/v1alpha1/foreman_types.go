package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="FQDN",type=string,JSONPath=`.spec.fqdn`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Foreman is the Schema for the foremen API.
// It owns the Foreman web workload, the Dynflow workloads, and recurring tasks.
type Foreman struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ForemanSpec   `json:"spec,omitempty"`
	Status ForemanStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ForemanList contains a list of Foreman.
type ForemanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Foreman `json:"items"`
}

// ForemanSpec defines the desired state of Foreman.
type ForemanSpec struct {
	// FQDN is the primary hostname of this Foreman instance.
	// +kubebuilder:validation:MinLength=1
	FQDN string `json:"fqdn"`

	// Aliases lists additional hostnames accepted by Foreman.
	// +optional
	Aliases []string `json:"aliases,omitempty"`

	// Image selects the Foreman container image.
	// +optional
	Image ImageSpec `json:"image,omitempty"`

	// Plugins lists the Foreman plugins to enable from the catalog in the image. Their
	// dependencies are enabled with them. Changing the list runs a migration.
	// +listType=set
	// +optional
	Plugins []ForemanPlugin `json:"plugins,omitempty"`

	// TLS selects the source of the internal certificates.
	TLS TLSSpec `json:"tls"`

	// Database configures the PostgreSQL connection.
	Database DatabaseSpec `json:"database"`

	// Valkey configures the Valkey connection.
	Valkey ValkeySpec `json:"valkey"`

	// SecretsRef names the Secret with "encryption-key", "secret-key-base", "admin-password",
	// "oauth-consumer-key", and "oauth-consumer-secret". The operator generates missing keys.
	// Defaults to "<name>-secrets".
	// +optional
	SecretsRef string `json:"secretsRef,omitempty"`

	// Web configures the web workload.
	// +optional
	Web ForemanWebSpec `json:"web,omitempty"`

	// Frontend configures TLS termination and exposure. It runs as a second container in
	// every web pod.
	Frontend ForemanFrontendSpec `json:"frontend"`

	// Dynflow configures the task workloads. Foreman uses Dynflow as its ActiveJob backend.
	// +kubebuilder:default={}
	// +optional
	Dynflow DynflowSpec `json:"dynflow,omitempty"`
}

// ForemanPlugin names a plugin from the catalog in the Foreman image.
// +kubebuilder:validation:Enum=foreman_ansible;foreman_discovery;foreman_puppet;foreman_remote_execution;foreman_templates;foreman_webhooks
type ForemanPlugin string

// DatabaseSpec selects the CloudNativePG cluster.
type DatabaseSpec struct {
	// Cluster references a CloudNativePG Cluster in the same namespace.
	Cluster CNPGClusterRef `json:"cluster"`
}

// CNPGClusterRef references a CloudNativePG Cluster.
type CNPGClusterRef struct {
	// Name of the Cluster. The operator connects to its "<name>-rw" Service.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// CredentialsSecretRef overrides the default "<name>-app" Secret.
	// +optional
	CredentialsSecretRef string `json:"credentialsSecretRef,omitempty"`
}

// ValkeySpec configures the Valkey connection.
type ValkeySpec struct {
	// Host of the Valkey server.
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`

	// Port of the Valkey server.
	// +kubebuilder:default=6379
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`
}

// ForemanWebSpec configures the web workload.
type ForemanWebSpec struct {
	// Replicas is the number of web pods.
	// +kubebuilder:default=2
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Resources sets the compute resources. Puma workers are derived from the limits.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// DynflowSpec configures the task workloads. The orchestrator always has one replica.
type DynflowSpec struct {
	// WorkerReplicas is the number of general workers.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +optional
	WorkerReplicas *int32 `json:"workerReplicas,omitempty"`

	// HostsQueueReplicas is the number of workers for the hosts queue, used by Remote Execution.
	// +kubebuilder:default=0
	// +kubebuilder:validation:Minimum=0
	// +optional
	HostsQueueReplicas *int32 `json:"hostsQueueReplicas,omitempty"`

	// Resources sets the compute resources of the workers.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ForemanPhase is the lifecycle phase of a Foreman.
// +kubebuilder:validation:Enum=Pending;Migrating;Progressing;Running;Failed
type ForemanPhase string

// Foreman lifecycle phases.
const (
	ForemanPhasePending     ForemanPhase = "Pending"
	ForemanPhaseMigrating   ForemanPhase = "Migrating"
	ForemanPhaseProgressing ForemanPhase = "Progressing"
	ForemanPhaseRunning     ForemanPhase = "Running"
	ForemanPhaseFailed      ForemanPhase = "Failed"
)

// ForemanStatus defines the observed state of Foreman.
type ForemanStatus struct {
	// ObservedGeneration is the generation last processed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase is the current lifecycle phase.
	// +optional
	Phase ForemanPhase `json:"phase,omitempty"`
	// AppliedMigrationHash identifies the image and plugin set the database was last migrated for.
	// +optional
	AppliedMigrationHash string `json:"appliedMigrationHash,omitempty"`

	// Conditions represent the latest observations.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &Foreman{}, &ForemanList{})
		return nil
	})
}
