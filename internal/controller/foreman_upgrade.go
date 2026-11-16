package controller

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	foremanv1alpha1 "github.com/slauger/foreman-operator/api/v1alpha1"
	"github.com/slauger/foreman-operator/internal/foremanapi"
)

// Reasons used by the upgrade checks.
const (
	reasonPreflight          = "Preflight"
	reasonPreflightFailed    = "PreflightFailed"
	reasonUpgradePath        = "UnsupportedUpgradePath"
	reasonPluginRemoval      = "PluginRemovalRefused"
	reasonWaitingForTasks    = "WaitingForTasks"
	reasonRollForward        = "RollForwardRequired"
	reasonVerifying          = "Verifying"
	reasonVerificationFailed = "VerificationFailed"
	reasonVerified           = "SmokeChecksPassed"
)

const (
	componentPreflight = "preflight"
	requeueTasks       = 30 * time.Second
	// activeTasksSearch matches tasks that a migration would interrupt. Scheduled tasks, such as
	// recurring cleanups, survive an upgrade.
	activeTasksSearch = "state ^ (planning, running, paused)"
)

// taskPlugins schedule Dynflow tasks that reference their code. Disabling one while such tasks
// exist crashes the Dynflow orchestrator, and foreman-tasks always schedules recurring tasks.
var taskPlugins = []string{"foreman-tasks", "foreman_ansible", "foreman_remote_execution"}

func skipUpgradeChecks(f *foremanv1alpha1.Foreman) bool {
	return f.GetAnnotations()[foremanv1alpha1.SkipUpgradeChecksAnnotation] == "true"
}

func preflightJobName(f *foremanv1alpha1.Foreman, hash string) string {
	return f.Name + "-preflight-" + hash[:8]
}

// preflightJob reads the Foreman version from the desired image into the termination message.
func preflightJob(f *foremanv1alpha1.Foreman, images Images, caps Capabilities, hash string) *batchv1.Job {
	ctr := corev1.Container{
		Name:                     componentPreflight,
		Command:                  []string{"/bin/sh", "-c", "cat /usr/share/foreman/VERSION > /dev/termination-log"},
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
		Resources:                defaultResources(corev1.ResourceRequirements{}, "10m", "32Mi", "64Mi"),
	}
	podSpec := foremanPodSpec(f, images, caps, ctr)
	podSpec.RestartPolicy = corev1.RestartPolicyNever

	labels := commonLabels(foremanAppName, f.Name, componentPreflight)
	job := &batchv1.Job{}
	job.Name = preflightJobName(f, hash)
	job.Namespace = f.Namespace
	job.Labels = labels
	job.Spec = batchv1.JobSpec{
		BackoffLimit: new(int32(1)),
		Template:     corev1.PodTemplateSpec{Spec: podSpec},
	}
	job.Spec.Template.Labels = labels
	return job
}

// imageVersion runs the preflight Job for hash and returns the Foreman version of the image,
// or "" while the Job runs.
func (r *ForemanReconciler) imageVersion(ctx context.Context, f *foremanv1alpha1.Foreman, hash string) (string, error) {
	job := &batchv1.Job{}
	err := r.Get(ctx, client.ObjectKey{Namespace: f.Namespace, Name: preflightJobName(f, hash)}, job)
	if apierrors.IsNotFound(err) {
		if err := r.DeleteAllOf(ctx, &batchv1.Job{},
			client.InNamespace(f.Namespace),
			client.MatchingLabels{labelName: foremanAppName, labelInstance: f.Name, labelComponent: componentPreflight},
			client.PropagationPolicy(metav1.DeletePropagationBackground),
		); err != nil {
			return "", err
		}
		job = preflightJob(f, r.Images, r.Capabilities, hash)
		if err := controllerutil.SetControllerReference(f, job, r.Scheme); err != nil {
			return "", err
		}
		return "", r.Create(ctx, job)
	}
	if err != nil {
		return "", err
	}
	if jobFinished(job, batchv1.JobFailed) {
		return "", fmt.Errorf("preflight Job %s failed, see its logs; delete the Job to retry", job.Name)
	}
	if !jobFinished(job, batchv1.JobComplete) {
		return "", nil
	}

	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, client.InNamespace(f.Namespace), client.MatchingLabels{batchv1.JobNameLabel: job.Name}); err != nil {
		return "", err
	}
	for _, p := range pods.Items {
		for _, cs := range p.Status.ContainerStatuses {
			if t := cs.State.Terminated; t != nil && t.ExitCode == 0 {
				if version := strings.TrimSpace(t.Message); version != "" {
					return version, nil
				}
			}
		}
	}
	return "", fmt.Errorf("preflight Job %s completed without reporting a version; delete the Job to retry", job.Name)
}

func jobFinished(job *batchv1.Job, condType batchv1.JobConditionType) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == condType && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// checkUpgradePath allows a rebuild or patch release, the next minor release, and the first
// minor release of a later major version. Foreman supports upgrades one release at a time.
func checkUpgradePath(from, to string) error {
	fromMajor, fromMinor, err := majorMinor(from)
	if err != nil {
		return err
	}
	toMajor, toMinor, err := majorMinor(to)
	if err != nil {
		return err
	}
	switch {
	case toMajor == fromMajor && toMinor == fromMinor:
		return nil
	case toMajor == fromMajor && toMinor == fromMinor+1:
		return nil
	case toMajor > fromMajor && toMinor == 0:
		return nil
	case toMajor < fromMajor || (toMajor == fromMajor && toMinor < fromMinor):
		return fmt.Errorf("downgrading Foreman from %s to %s is not supported", from, to)
	default:
		return fmt.Errorf("upgrading Foreman from %s to %s skips a release; upgrade one release at a time", from, to)
	}
}

func majorMinor(version string) (int, int, error) {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("cannot parse Foreman version %q", version)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("cannot parse Foreman version %q", version)
	}
	minor, err := strconv.Atoi(strings.TrimRightFunc(parts[1], func(r rune) bool { return r < '0' || r > '9' }))
	if err != nil {
		return 0, 0, fmt.Errorf("cannot parse Foreman version %q", version)
	}
	return major, minor, nil
}

// removedTaskPlugins returns the plugins that schedule tasks and are applied but no longer enabled.
func removedTaskPlugins(applied, enabled []string) []string {
	var removed []string
	for _, p := range applied {
		if slices.Contains(taskPlugins, p) && !slices.Contains(enabled, p) {
			removed = append(removed, p)
		}
	}
	return removed
}

// preflight checks a migration of an installed Foreman before any writer stops. It returns
// the Foreman version of the desired image once the migration may start. Failed checks set
// the phase and conditions and return "".
func (r *ForemanReconciler) preflight(ctx context.Context, f *foremanv1alpha1.Foreman, hash string) string {
	f.Status.Phase = foremanv1alpha1.ForemanPhaseProgressing
	version, err := r.imageVersion(ctx, f, hash)
	if err != nil {
		r.blocked(f, reasonPreflightFailed, err.Error())
		return ""
	}
	if version == "" {
		setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionProgressing, metav1.ConditionTrue, reasonPreflight, "reading the Foreman version of the new image")
		return ""
	}
	if f.Status.AppliedMigrationHash == "" || skipUpgradeChecks(f) {
		return version
	}

	if removed := removedTaskPlugins(f.Status.AppliedPlugins, enabledPlugins(f)); len(removed) > 0 {
		r.blocked(f, reasonPluginRemoval, fmt.Sprintf(
			"disabling %s is refused: its scheduled tasks would crash the Dynflow orchestrator; set the annotation %s to override",
			strings.Join(removed, ", "), foremanv1alpha1.SkipUpgradeChecksAnnotation))
		return ""
	}

	if f.Status.PendingMigrationHash != "" {
		// Rolling forward after a started migration: Foreman is stopped, so only the version
		// path can be checked, against the last completed migration.
		if f.Status.ForemanVersion != "" {
			if err := checkUpgradePath(f.Status.ForemanVersion, version); err != nil {
				r.blocked(f, reasonUpgradePath, fmt.Sprintf("%v; set the annotation %s to override", err, foremanv1alpha1.SkipUpgradeChecksAnnotation))
				return ""
			}
		}
		return version
	}

	api, err := foremanAPIFor(ctx, r.Client, r.NewAPI, f)
	if err != nil {
		r.waitingForPreflight(f, fmt.Sprintf("cannot reach the running Foreman: %v", err))
		return ""
	}
	current := f.Status.ForemanVersion
	if current == "" {
		// Installed by an operator version that did not record the version yet.
		status, err := api.Status(ctx)
		if err != nil {
			r.waitingForPreflight(f, fmt.Sprintf("cannot read the version of the running Foreman: %v", err))
			return ""
		}
		current = status.Version
	}
	if err := checkUpgradePath(current, version); err != nil {
		r.blocked(f, reasonUpgradePath, fmt.Sprintf("%v; set the annotation %s to override", err, foremanv1alpha1.SkipUpgradeChecksAnnotation))
		return ""
	}

	active, err := api.CountTasks(ctx, activeTasksSearch)
	switch {
	case errors.Is(err, foremanapi.ErrNotFound):
		// foreman-tasks is not enabled, so there are no long-running tasks.
	case err != nil:
		r.waitingForPreflight(f, fmt.Sprintf("cannot list the running tasks: %v", err))
		return ""
	case active > 0:
		setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionProgressing, metav1.ConditionTrue, reasonWaitingForTasks,
			fmt.Sprintf("waiting for %d planning, running, or paused tasks before stopping Foreman; resume or cancel paused tasks", active))
		return ""
	}
	return version
}

func (r *ForemanReconciler) blocked(f *foremanv1alpha1.Foreman, reason, message string) {
	f.Status.Phase = foremanv1alpha1.ForemanPhaseBlocked
	setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionProgressing, metav1.ConditionFalse, reason, message)
	setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionDegraded, metav1.ConditionTrue, reason, message)
}

func (r *ForemanReconciler) waitingForPreflight(f *foremanv1alpha1.Foreman, message string) {
	setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionProgressing, metav1.ConditionTrue, reasonPreflight, message)
}

// verify runs the smoke checks after a migration: the Foreman API answers with the migrated
// version, and every SmartProxy of this Foreman refreshed its features since. It reports
// whether the checks passed.
func (r *ForemanReconciler) verify(ctx context.Context, f *foremanv1alpha1.Foreman) bool {
	fail := func(reason, message string) bool {
		setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionVerified, metav1.ConditionFalse, reason, message)
		return false
	}

	api, err := foremanAPIFor(ctx, r.Client, r.NewAPI, f)
	if err != nil {
		return fail(reasonVerificationFailed, fmt.Sprintf("cannot reach Foreman: %v", err))
	}
	status, err := api.Status(ctx)
	if err != nil {
		return fail(reasonVerificationFailed, fmt.Sprintf("status API: %v", err))
	}
	if f.Status.ForemanVersion != "" && status.Version != f.Status.ForemanVersion {
		return fail(reasonVerificationFailed, fmt.Sprintf("Foreman reports version %s, want %s", status.Version, f.Status.ForemanVersion))
	}

	proxies := &foremanv1alpha1.SmartProxyList{}
	if err := r.List(ctx, proxies, client.InNamespace(f.Namespace)); err != nil {
		return fail(reasonVerificationFailed, err.Error())
	}
	var pending []string
	total := 0
	for _, p := range proxies.Items {
		if p.Spec.ForemanRef != f.Name || !p.DeletionTimestamp.IsZero() {
			continue
		}
		total++
		if p.Status.ForemanMigrationHash != f.Status.AppliedMigrationHash || !meta.IsStatusConditionTrue(p.Status.Conditions, foremanv1alpha1.ConditionReady) {
			pending = append(pending, p.Name)
		}
	}
	if len(pending) > 0 {
		return fail(reasonVerifying, fmt.Sprintf("waiting for SmartProxies to refresh their features: %s", strings.Join(pending, ", ")))
	}

	setCondition(&f.Status.Conditions, f.Generation, foremanv1alpha1.ConditionVerified, metav1.ConditionTrue, reasonVerified,
		fmt.Sprintf("Foreman %s answers and %d SmartProxies refreshed their features", status.Version, total))
	f.Status.VerifiedMigrationHash = f.Status.AppliedMigrationHash
	return true
}
