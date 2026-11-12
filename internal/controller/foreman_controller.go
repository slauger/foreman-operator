package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	foremanv1alpha1 "github.com/slauger/foreman-operator/api/v1alpha1"
)

// Reasons used in Foreman conditions.
const (
	reasonReconciled       = "Reconciled"
	reasonReconcileError   = "ReconcileError"
	reasonWaitingDatabase  = "WaitingForDatabase"
	reasonStoppingWriters  = "StoppingWriters"
	reasonMigrating        = "Migrating"
	reasonMigrationFailed  = "MigrationFailed"
	reasonRollingOut       = "RollingOut"
	reasonPaused           = "Paused"
	reasonMissingAPI       = "MissingAPI"
	reasonWaitingForeman   = "WaitingForForeman"
	reasonRegistrationFail = "RegistrationFailed"
)

const requeueShort = 10 * time.Second

// ForemanReconciler reconciles a Foreman object.
type ForemanReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Images       Images
	Capabilities Capabilities
}

// +kubebuilder:rbac:groups=platform.theforeman.org,resources=foremen,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=platform.theforeman.org,resources=foremen/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets;configmaps;services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete;deletecollection
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives a Foreman towards its spec. Database migrations run in a Job while all
// writers are stopped, and only one migration hash is applied at a time.
func (r *ForemanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	f := &foremanv1alpha1.Foreman{}
	if err := r.Get(ctx, req.NamespacedName, f); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	original := f.DeepCopy()

	result, err := r.reconcile(ctx, f)
	if err != nil {
		f.Status.Phase = foremanv1alpha1.ForemanPhaseFailed
		setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonReconcileError, err.Error())
	}
	f.Status.ObservedGeneration = f.Generation
	if statusErr := r.Status().Patch(ctx, f, client.MergeFrom(original)); statusErr != nil && !apierrors.IsNotFound(statusErr) {
		return ctrl.Result{}, statusErr
	}
	return result, err
}

func (r *ForemanReconciler) reconcile(ctx context.Context, f *foremanv1alpha1.Foreman) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if isPaused(f) {
		setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionPaused, metav1.ConditionTrue, reasonPaused, "reconciliation is paused by annotation")
		return ctrl.Result{}, nil
	}
	setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionPaused, metav1.ConditionFalse, reasonReconciled, "")

	if f.Spec.TLS.IssuerRef != nil && !r.Capabilities.CertManager {
		return ctrl.Result{}, errMissingCapability("cert-manager")
	}

	secrets, err := r.ensureSecrets(ctx, f)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("secrets: %w", err)
	}
	if f.Spec.TLS.IssuerRef != nil {
		if err := ensureCertificate(ctx, r.Client, r.Scheme, f, certificateRequest{
			name:       foremanClientCertName(f),
			namespace:  f.Namespace,
			issuer:     *f.Spec.TLS.IssuerRef,
			commonName: f.Spec.FQDN,
			dnsNames:   []string{f.Spec.FQDN},
			usages:     []string{"client auth", "digital signature", "key encipherment"},
			labels:     commonLabels(foremanAppName, f.Name, componentWeb),
		}); err != nil {
			return ctrl.Result{}, fmt.Errorf("client certificate: %w", err)
		}
	}

	configChecksum, err := r.ensureConfig(ctx, f, secrets)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("configuration: %w", err)
	}
	certChecksum, err := secretChecksum(ctx, r.Client, f.Namespace, foremanClientCertName(f))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("client certificate: %w", err)
	}
	configChecksum = checksum(configChecksum, certChecksum)
	frontendChecksum, err := r.ensureFrontend(ctx, f)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("frontend: %w", err)
	}

	if err := r.Get(ctx, client.ObjectKey{Namespace: f.Namespace, Name: foremanDBSecretName(f)}, &corev1.Secret{}); err != nil {
		if apierrors.IsNotFound(err) {
			f.Status.Phase = foremanv1alpha1.ForemanPhasePending
			setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonWaitingDatabase,
				fmt.Sprintf("database credentials Secret %q does not exist yet", foremanDBSecretName(f)))
			return ctrl.Result{RequeueAfter: requeueShort}, nil
		}
		return ctrl.Result{}, err
	}

	hash := migrationHash(f, r.Images)
	if f.Status.AppliedMigrationHash != hash {
		done, err := r.migrate(ctx, f, hash)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !done {
			return ctrl.Result{RequeueAfter: requeueShort}, nil
		}
		log.Info("database migrated", "hash", hash)
		f.Status.AppliedMigrationHash = hash
	}

	ready, err := r.ensureWorkloads(ctx, f, configChecksum, frontendChecksum)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("workloads: %w", err)
	}
	if !ready {
		f.Status.Phase = foremanv1alpha1.ForemanPhaseProgressing
		setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionProgressing, metav1.ConditionTrue, reasonRollingOut, "workloads are rolling out")
		setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonRollingOut, "workloads are rolling out")
		return ctrl.Result{RequeueAfter: requeueShort}, nil
	}

	f.Status.Phase = foremanv1alpha1.ForemanPhaseRunning
	setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionProgressing, metav1.ConditionFalse, reasonReconciled, "")
	setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionReady, metav1.ConditionTrue, reasonReconciled, "Foreman is running")
	return ctrl.Result{RequeueAfter: resyncPeriod}, nil
}

// ensureSecrets creates the secrets Secret and fills in missing keys. The Secret has no owner
// reference: losing the encryption key would make the encrypted database columns unreadable.
func (r *ForemanReconciler) ensureSecrets(ctx context.Context, f *foremanv1alpha1.Foreman) (map[string]string, error) {
	secret := &corev1.Secret{}
	key := client.ObjectKey{Namespace: f.Namespace, Name: foremanSecretsName(f)}
	err := r.Get(ctx, key, secret)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	exists := err == nil

	generated := map[string]int{
		keyEncryptionKey:       32,
		keySecretKeyBase:       64,
		keyAdminPassword:       16,
		keyOAuthConsumerKey:    16,
		keyOAuthConsumerSecret: 32,
	}
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	changed := false
	for k, size := range generated {
		if len(secret.Data[k]) > 0 {
			continue
		}
		value, err := randomHex(size)
		if err != nil {
			return nil, err
		}
		secret.Data[k] = []byte(value)
		changed = true
	}

	if !exists {
		secret.Name = key.Name
		secret.Namespace = key.Namespace
		secret.Labels = commonLabels(foremanAppName, f.Name, componentWeb)
		if err := r.Create(ctx, secret); err != nil {
			return nil, err
		}
	} else if changed {
		if err := r.Update(ctx, secret); err != nil {
			return nil, err
		}
	}

	out := make(map[string]string, len(secret.Data))
	for k, v := range secret.Data {
		out[k] = string(v)
	}
	return out, nil
}

func randomHex(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// ensureConfig writes settings.yaml and the Dynflow configurations and returns their checksum.
func (r *ForemanReconciler) ensureConfig(ctx context.Context, f *foremanv1alpha1.Foreman, secrets map[string]string) (string, error) {
	settings := map[string]string{
		"settings.yaml": foremanSettings(f, secrets[keyOAuthConsumerKey], secrets[keyOAuthConsumerSecret]),
	}
	settingsSecret := &corev1.Secret{}
	settingsSecret.Name = foremanSettingsName(f)
	settingsSecret.Namespace = f.Namespace
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, settingsSecret, func() error {
		settingsSecret.Labels = commonLabels(foremanAppName, f.Name, componentWeb)
		settingsSecret.StringData = nil
		settingsSecret.Data = map[string][]byte{"settings.yaml": []byte(settings["settings.yaml"])}
		return controllerutil.SetControllerReference(f, settingsSecret, r.Scheme)
	}); err != nil {
		return "", err
	}

	dynflow := dynflowConfigs()
	cm := &corev1.ConfigMap{}
	cm.Name = foremanDynflowName(f)
	cm.Namespace = f.Namespace
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Labels = commonLabels(foremanAppName, f.Name, componentDynflowWorker)
		cm.Data = dynflow
		return controllerutil.SetControllerReference(f, cm, r.Scheme)
	}); err != nil {
		return "", err
	}

	return checksum(checksumMap(settings), checksumMap(dynflow), secrets[keyEncryptionKey], secrets[keySecretKeyBase]), nil
}

// desiredDeployments builds all workloads. Only the web pods run the frontend, so only they
// roll when its configuration or certificates change.
func (r *ForemanReconciler) desiredDeployments(f *foremanv1alpha1.Foreman, configChecksum, frontendChecksum string) []*appsv1.Deployment {
	web := int32(2)
	if f.Spec.Web.Replicas != nil {
		web = *f.Spec.Web.Replicas
	}
	workers := int32(1)
	if f.Spec.Dynflow.WorkerReplicas != nil {
		workers = *f.Spec.Dynflow.WorkerReplicas
	}
	hostsQueue := int32(0)
	if f.Spec.Dynflow.HostsQueueReplicas != nil {
		hostsQueue = *f.Spec.Dynflow.HostsQueueReplicas
	}
	return []*appsv1.Deployment{
		foremanWebDeployment(f, r.Images, r.Capabilities, web, checksum(configChecksum, frontendChecksum)),
		dynflowDeployment(f, r.Images, r.Capabilities, componentDynflowOrchestr, "orchestrator.yml", 1, configChecksum),
		dynflowDeployment(f, r.Images, r.Capabilities, componentDynflowWorker, "worker.yml", workers, configChecksum),
		dynflowDeployment(f, r.Images, r.Capabilities, componentDynflowHostsQueue, "worker-hosts-queue.yml", hostsQueue, configChecksum),
	}
}

// ensureWorkloads applies all Deployments and reports whether web and orchestrator are available.
func (r *ForemanReconciler) ensureWorkloads(ctx context.Context, f *foremanv1alpha1.Foreman, configChecksum, frontendChecksum string) (bool, error) {
	ready := true
	for _, desired := range r.desiredDeployments(f, configChecksum, frontendChecksum) {
		d, err := r.applyDeployment(ctx, f, desired)
		if err != nil {
			return false, err
		}
		component := desired.Labels[labelComponent]
		if component != componentWeb && component != componentDynflowOrchestr {
			continue
		}
		if !deploymentAvailable(d, min(*desired.Spec.Replicas, 1)) {
			ready = false
		}
	}
	return ready, nil
}

func (r *ForemanReconciler) applyDeployment(ctx context.Context, f *foremanv1alpha1.Foreman, desired *appsv1.Deployment) (*appsv1.Deployment, error) {
	d := &appsv1.Deployment{}
	d.Name = desired.Name
	d.Namespace = desired.Namespace
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, d, func() error {
		d.Labels = desired.Labels
		d.Spec.Replicas = desired.Spec.Replicas
		d.Spec.Strategy = desired.Spec.Strategy
		if d.Spec.Selector == nil {
			d.Spec.Selector = desired.Spec.Selector
		}
		setPodTemplate(d, desired.Spec.Template)
		return controllerutil.SetControllerReference(f, d, r.Scheme)
	})
	return d, err
}

// migrate stops all writers, runs the migration Job for hash, and reports completion.
func (r *ForemanReconciler) migrate(ctx context.Context, f *foremanv1alpha1.Foreman, hash string) (bool, error) {
	f.Status.Phase = foremanv1alpha1.ForemanPhaseMigrating
	setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonMigrating, "database migration in progress")

	running, err := r.stopWriters(ctx, f)
	if err != nil {
		return false, err
	}
	if running > 0 {
		setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionProgressing, metav1.ConditionTrue, reasonStoppingWriters,
			fmt.Sprintf("waiting for %d Foreman pods to stop before migrating", running))
		return false, nil
	}

	job := &batchv1.Job{}
	err = r.Get(ctx, client.ObjectKey{Namespace: f.Namespace, Name: migrationJobName(f, hash)}, job)
	if apierrors.IsNotFound(err) {
		if err := r.deleteOldMigrations(ctx, f); err != nil {
			return false, err
		}
		job = migrationJob(f, r.Images, r.Capabilities, hash)
		if err := controllerutil.SetControllerReference(f, job, r.Scheme); err != nil {
			return false, err
		}
		if err := r.Create(ctx, job); err != nil {
			return false, err
		}
		setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionProgressing, metav1.ConditionTrue, reasonMigrating, "migration Job created")
		return false, nil
	}
	if err != nil {
		return false, err
	}

	for _, c := range job.Status.Conditions {
		if c.Status != corev1.ConditionTrue {
			continue
		}
		switch c.Type {
		case batchv1.JobComplete:
			return true, nil
		case batchv1.JobFailed:
			f.Status.Phase = foremanv1alpha1.ForemanPhaseFailed
			setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionDegraded, metav1.ConditionTrue, reasonMigrationFailed,
				fmt.Sprintf("migration Job %s failed, see its logs; delete the Job to retry", job.Name))
			return false, nil
		}
	}
	setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionProgressing, metav1.ConditionTrue, reasonMigrating, "migration Job is running")
	return false, nil
}

// stopWriters scales all Foreman Deployments to zero and returns the number of pods left.
func (r *ForemanReconciler) stopWriters(ctx context.Context, f *foremanv1alpha1.Foreman) (int, error) {
	for _, component := range []string{componentWeb, componentDynflowOrchestr, componentDynflowWorker, componentDynflowHostsQueue} {
		d := &appsv1.Deployment{}
		err := r.Get(ctx, client.ObjectKey{Namespace: f.Namespace, Name: foremanComponentName(f, component)}, d)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if d.Spec.Replicas != nil && *d.Spec.Replicas == 0 {
			continue
		}
		patch := client.MergeFrom(d.DeepCopy())
		d.Spec.Replicas = new(int32(0))
		if err := r.Patch(ctx, d, patch); err != nil {
			return 0, err
		}
	}

	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, client.InNamespace(f.Namespace), client.MatchingLabels{labelName: foremanAppName, labelInstance: f.Name}); err != nil {
		return 0, err
	}
	running := 0
	for _, p := range pods.Items {
		if p.Labels[labelComponent] != componentMigration {
			running++
		}
	}
	return running, nil
}

func (r *ForemanReconciler) deleteOldMigrations(ctx context.Context, f *foremanv1alpha1.Foreman) error {
	return r.DeleteAllOf(ctx, &batchv1.Job{},
		client.InNamespace(f.Namespace),
		client.MatchingLabels{labelName: foremanAppName, labelInstance: f.Name, labelComponent: componentMigration},
		client.PropagationPolicy(metav1.DeletePropagationBackground),
	)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ForemanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&foremanv1alpha1.Foreman{}).
		Owns(&appsv1.Deployment{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Named("foreman")
	if r.Capabilities.CertManager {
		b = b.Owns(newCertificateObject())
	}
	if r.Capabilities.GatewayAPI {
		b = b.Owns(&gwapiv1.TLSRoute{}).Owns(&gwapiv1.HTTPRoute{})
	}
	return b.Complete(r)
}
