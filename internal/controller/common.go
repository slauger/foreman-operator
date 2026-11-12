// Package controller contains the reconcilers of the operator.
package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	foremanv1alpha1 "github.com/slauger/foreman-operator/api/v1alpha1"
)

// resyncPeriod re-reads healthy resources, so renewed certificates reach the pods even
// though their Secrets belong to cert-manager and are not watched.
const resyncPeriod = 10 * time.Minute

// secretChecksum digests a Secret's data. A missing Secret yields an empty string, so the
// pods restart once when cert-manager issues the certificate and on every renewal.
func secretChecksum(ctx context.Context, c client.Reader, namespace, name string) (string, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	data := make(map[string]string, len(secret.Data))
	for k, v := range secret.Data {
		data[k] = string(v)
	}
	return checksumMap(data), nil
}

// Capabilities describes optional APIs detected in the cluster at startup.
type Capabilities struct {
	// CertManager is true when cert-manager Certificates can be created.
	CertManager bool
	// GatewayAPI is true when TLSRoute and HTTPRoute are served.
	GatewayAPI bool
	// OpenShift is true when the OpenShift Route API exists.
	OpenShift bool
}

// Images holds the default image of each component. A resource's spec.image overrides it.
type Images struct {
	Foreman    string
	Frontend   string
	SmartProxy string
	BootFiles  string
	// Tag is used when neither the resource nor the default image names one.
	Tag string
}

// DefaultImages returns the images published by this project.
func DefaultImages(tag string) Images {
	return Images{
		Foreman:    "ghcr.io/slauger/foreman-operator-foreman",
		Frontend:   "ghcr.io/slauger/foreman-operator-frontend",
		SmartProxy: "ghcr.io/slauger/foreman-operator-smart-proxy",
		BootFiles:  "ghcr.io/slauger/foreman-operator-boot-files",
		Tag:        tag,
	}
}

// imageRef resolves an image reference from the spec and the default repository.
func imageRef(spec foremanv1alpha1.ImageSpec, defaultRepository, defaultTag string) string {
	repository := spec.Repository
	if repository == "" {
		repository = defaultRepository
	}
	if spec.Digest != "" {
		return repository + "@" + spec.Digest
	}
	tag := spec.Tag
	if tag == "" {
		tag = defaultTag
	}
	return repository + ":" + tag
}

func pullPolicy(spec foremanv1alpha1.ImageSpec) corev1.PullPolicy {
	if spec.PullPolicy == "" {
		return corev1.PullIfNotPresent
	}
	return spec.PullPolicy
}

// Label keys and values shared by all managed resources.
const (
	labelName      = "app.kubernetes.io/name"
	labelInstance  = "app.kubernetes.io/instance"
	labelComponent = "app.kubernetes.io/component"
	labelPartOf    = "app.kubernetes.io/part-of"
	labelManagedBy = "app.kubernetes.io/managed-by"

	partOf    = "foreman"
	managedBy = "foreman-operator"

	// checksumAnnotation restarts pods when generated configuration changes.
	checksumAnnotation = "platform.theforeman.org/config-checksum"

	// restartedAtAnnotation is set on the pod template by "kubectl rollout restart".
	restartedAtAnnotation = "kubectl.kubernetes.io/restartedAt"
)

// setPodTemplate replaces the pod template of a Deployment but keeps the annotation of
// "kubectl rollout restart". Without it, the operator would undo every requested restart.
func setPodTemplate(d *appsv1.Deployment, desired corev1.PodTemplateSpec) {
	restartedAt, restarted := d.Spec.Template.Annotations[restartedAtAnnotation]
	d.Spec.Template = *desired.DeepCopy()
	if restarted {
		if d.Spec.Template.Annotations == nil {
			d.Spec.Template.Annotations = map[string]string{}
		}
		d.Spec.Template.Annotations[restartedAtAnnotation] = restartedAt
	}
}

// selectorLabels identifies the pods of one component of one resource. They never change.
func selectorLabels(name, instance, component string) map[string]string {
	return map[string]string{
		labelName:      name,
		labelInstance:  instance,
		labelComponent: component,
	}
}

func commonLabels(name, instance, component string) map[string]string {
	labels := selectorLabels(name, instance, component)
	labels[labelPartOf] = partOf
	labels[labelManagedBy] = managedBy
	return labels
}

// podSecurityContext leaves UID and GID to the image or, on OpenShift, to the SCC.
// fsGroup matters only for persistent volumes, which OpenShift handles on its own.
func podSecurityContext(openShift bool) *corev1.PodSecurityContext {
	psc := &corev1.PodSecurityContext{
		RunAsNonRoot: new(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
	if !openShift {
		psc.FSGroup = new(int64(1001))
		psc.FSGroupChangePolicy = new(corev1.FSGroupChangeOnRootMismatch)
	}
	return psc
}

// restrictedContainerSecurityContext is the baseline for every container. The root filesystem is
// read-only, so each writable path must be a volume.
func restrictedContainerSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: new(false),
		ReadOnlyRootFilesystem:   new(true),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}
}

// checksum returns a stable digest over the given strings.
func checksum(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// checksumMap returns a stable digest over a map, for ConfigMap and Secret data.
func checksumMap(data map[string]string) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, 2*len(keys))
	for _, k := range keys {
		parts = append(parts, k, data[k])
	}
	return checksum(parts...)
}

// setCondition records a condition with the current generation.
func setCondition(conditions *[]metav1.Condition, generation int64, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
	})
}

func isPaused(obj metav1.Object) bool {
	return obj.GetAnnotations()[foremanv1alpha1.PausedAnnotation] == "true"
}

// deploymentAvailable reports whether a Deployment has rolled out its current spec: at least
// desired pods are available and no pod of an older template is left. Old pods alone would
// still serve the previous version.
func deploymentAvailable(d *appsv1.Deployment, desired int32) bool {
	s := d.Status
	return s.ObservedGeneration >= d.Generation && s.UpdatedReplicas == s.Replicas && s.AvailableReplicas >= desired
}

func errMissingCapability(what string) error {
	return fmt.Errorf("%s is not installed in the cluster", what)
}
