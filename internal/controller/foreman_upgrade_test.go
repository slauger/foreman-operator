package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	foremanv1alpha1 "github.com/slauger/foreman-operator/api/v1alpha1"
	"github.com/slauger/foreman-operator/internal/foremanapi"
)

func TestCheckUpgradePath(t *testing.T) {
	tests := []struct {
		from, to string
		ok       bool
	}{
		{"5.0.1", "5.0.1", true},
		{"5.0.1", "5.0.2", true},
		{"5.0.2", "5.1.0", true},
		{"3.19.1", "5.0.0", true},
		{"5.0.1", "5.1.0-develop", true},
		{"5.0.1", "5.2.0", false},
		{"5.1.0", "5.0.1", false},
		{"5.1.0", "6.1.0", false},
		{"5.0.1", "garbage", false},
	}
	for _, tt := range tests {
		if err := checkUpgradePath(tt.from, tt.to); (err == nil) != tt.ok {
			t.Errorf("checkUpgradePath(%s, %s) = %v, want ok=%v", tt.from, tt.to, err, tt.ok)
		}
	}
}

// upgradeFixture installs a running Foreman 5.0.1 and returns its reconciler and fake API.
func upgradeFixture(t *testing.T, plugins ...foremanv1alpha1.ForemanPlugin) (*ForemanReconciler, *fakeForemanAPI, *foremanv1alpha1.Foreman) {
	t.Helper()
	f := testForeman()
	f.Spec.Plugins = plugins
	c := newTestClient(f, dbSecret(), serverCert())
	api := newFakeForemanAPI()
	r := newForemanReconciler(c)
	r.NewAPI = api.factory
	got := install(t, r, f, "5.0.1")
	markDeploymentAvailable(t, c, "prod-web")
	markDeploymentAvailable(t, c, "prod-dynflow-orchestrator")
	return r, api, reconcileForeman(t, r, got)
}

func updateSpec(t *testing.T, r *ForemanReconciler, f *foremanv1alpha1.Foreman, change func(*foremanv1alpha1.Foreman)) *foremanv1alpha1.Foreman {
	t.Helper()
	change(f)
	if err := r.Update(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return reconcileForeman(t, r, f)
}

func requirePreflightPassed(t *testing.T, r *ForemanReconciler, f *foremanv1alpha1.Foreman) {
	t.Helper()
	if f.Status.PendingMigrationHash != migrationHash(f, r.Images) {
		t.Fatalf("preflight did not pass: phase %s, conditions %v", f.Status.Phase, f.Status.Conditions)
	}
}

func requireNotFenced(t *testing.T, r *ForemanReconciler) {
	t.Helper()
	if d := mustGet(t, r.Client, "prod-web", &appsv1.Deployment{}); *d.Spec.Replicas == 0 {
		t.Fatal("Foreman was stopped although the preflight did not pass")
	}
}

func TestUpgradeBlocksSkippedRelease(t *testing.T) {
	r, _, f := upgradeFixture(t)
	got := updateSpec(t, r, f, func(f *foremanv1alpha1.Foreman) { f.Spec.Image.Tag = "5.2" })
	completePreflight(t, r, got, "5.2.0")
	got = reconcileForeman(t, r, got)

	if got.Status.Phase != foremanv1alpha1.ForemanPhaseBlocked {
		t.Fatalf("phase = %s, want Blocked", got.Status.Phase)
	}
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionDegraded, metav1.ConditionTrue, reasonUpgradePath)
	requireNotFenced(t, r)

	got = updateSpec(t, r, got, func(f *foremanv1alpha1.Foreman) {
		f.Annotations = map[string]string{foremanv1alpha1.SkipUpgradeChecksAnnotation: "true"}
	})
	if got.Status.PendingMigrationHash != migrationHash(got, r.Images) {
		t.Error("the annotation must let the upgrade proceed")
	}
}

func TestUpgradeReadsRunningVersionWithoutRecord(t *testing.T) {
	r, api, f := upgradeFixture(t)
	// Installed by an operator version that did not record the version.
	f.Status.ForemanVersion = ""
	if err := r.Status().Update(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	api.version = "5.0.1"

	got := updateSpec(t, r, mustGet(t, r.Client, "prod", &foremanv1alpha1.Foreman{}), func(f *foremanv1alpha1.Foreman) { f.Spec.Image.Tag = "5.2" })
	completePreflight(t, r, got, "5.2.0")
	got = reconcileForeman(t, r, got)
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionDegraded, metav1.ConditionTrue, reasonUpgradePath)

	api.statusErr = errors.New("unreachable")
	got = updateSpec(t, r, got, func(f *foremanv1alpha1.Foreman) { f.Spec.Image.Tag = "5.1" })
	completePreflight(t, r, got, "5.1.0")
	got = reconcileForeman(t, r, got)
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionProgressing, metav1.ConditionTrue, reasonPreflight)
	requireNotFenced(t, r)
}

func TestUpgradeWaitsForActiveTasks(t *testing.T) {
	r, api, f := upgradeFixture(t, foremanv1alpha1.ForemanPlugin("foreman_remote_execution"))
	api.tasks = 2

	got := updateSpec(t, r, f, func(f *foremanv1alpha1.Foreman) { f.Spec.Image.Tag = "5.1" })
	completePreflight(t, r, got, "5.1.0")
	got = reconcileForeman(t, r, got)
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionProgressing, metav1.ConditionTrue, reasonWaitingForTasks)
	requireNotFenced(t, r)
	if len(api.searches) == 0 || api.searches[len(api.searches)-1] != activeTasksSearch {
		t.Errorf("task searches = %v", api.searches)
	}

	api.tasks = 0
	got = reconcileForeman(t, r, got)
	requirePreflightPassed(t, r, got)
}

func TestUpgradeWithoutForemanTasks(t *testing.T) {
	r, api, f := upgradeFixture(t)
	api.tasksErr = foremanapi.ErrNotFound

	got := updateSpec(t, r, f, func(f *foremanv1alpha1.Foreman) { f.Spec.Image.Tag = "5.1" })
	completePreflight(t, r, got, "5.1.0")
	got = reconcileForeman(t, r, got)
	requirePreflightPassed(t, r, got)
}

func TestPluginRemoval(t *testing.T) {
	r, _, f := upgradeFixture(t, "foreman_remote_execution", "foreman_templates")
	if want := []string{"foreman-tasks", "foreman_remote_execution", "foreman_templates"}; strings.Join(f.Status.AppliedPlugins, ",") != strings.Join(want, ",") {
		t.Fatalf("applied plugins = %v, want %v", f.Status.AppliedPlugins, want)
	}

	got := updateSpec(t, r, f, func(f *foremanv1alpha1.Foreman) {
		f.Spec.Plugins = []foremanv1alpha1.ForemanPlugin{"foreman_templates"}
	})
	completePreflight(t, r, got, "5.0.1")
	got = reconcileForeman(t, r, got)
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionDegraded, metav1.ConditionTrue, reasonPluginRemoval)
	requireNotFenced(t, r)

	got = updateSpec(t, r, got, func(f *foremanv1alpha1.Foreman) {
		f.Spec.Plugins = []foremanv1alpha1.ForemanPlugin{"foreman_remote_execution"}
	})
	completePreflight(t, r, got, "5.0.1")
	got = reconcileForeman(t, r, got)
	requirePreflightPassed(t, r, got)
}

// startMigration upgrades f to tag and lets the migration Job start.
func startMigration(t *testing.T, r *ForemanReconciler, f *foremanv1alpha1.Foreman, tag, version string) *foremanv1alpha1.Foreman {
	t.Helper()
	got := updateSpec(t, r, f, func(f *foremanv1alpha1.Foreman) { f.Spec.Image.Tag = tag })
	completePreflight(t, r, got, version)
	got = reconcileForeman(t, r, got)
	mustGet(t, r.Client, migrationJobName(got, migrationHash(got, r.Images)), &batchv1.Job{})
	return got
}

func TestRollbackAfterStartedMigrationIsRefused(t *testing.T) {
	r, _, f := upgradeFixture(t)
	oldTag := f.Spec.Image.Tag
	got := startMigration(t, r, f, "5.1", "5.1.0")
	completeJob(t, r.Client, migrationJobName(got, migrationHash(got, r.Images)), batchv1.JobFailed)
	got = reconcileForeman(t, r, got)
	if got.Status.Phase != foremanv1alpha1.ForemanPhaseRollForwardRequired {
		t.Fatalf("phase = %s, want RollForwardRequired", got.Status.Phase)
	}

	got = updateSpec(t, r, got, func(f *foremanv1alpha1.Foreman) { f.Spec.Image.Tag = oldTag })
	if got.Status.Phase != foremanv1alpha1.ForemanPhaseRollForwardRequired {
		t.Fatalf("phase = %s after rollback, want RollForwardRequired", got.Status.Phase)
	}
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonRollForward)
	if d := mustGet(t, r.Client, "prod-web", &appsv1.Deployment{}); *d.Spec.Replicas != 0 {
		t.Fatal("the old image must not start on a partly migrated database")
	}

	// Rolling forward runs the preflight without the stopped Foreman and starts a new migration.
	got = updateSpec(t, r, got, func(f *foremanv1alpha1.Foreman) { f.Spec.Image.Tag = "5.1-fixed" })
	completePreflight(t, r, got, "5.1.1")
	got = reconcileForeman(t, r, got)
	mustGet(t, r.Client, migrationJobName(got, migrationHash(got, r.Images)), &batchv1.Job{})
	if got.Status.PendingMigrationHash != migrationHash(got, r.Images) {
		t.Errorf("pending = %q", got.Status.PendingMigrationHash)
	}
}

func TestRollbackBeforeMigrationStarted(t *testing.T) {
	r, _, f := upgradeFixture(t)
	oldTag := f.Spec.Image.Tag
	// A web pod that has not stopped yet keeps the migration from starting.
	pod := &corev1.Pod{Name: "prod-web-abc", Namespace: testNamespace, Labels: selectorLabels(foremanAppName, "prod", componentWeb)}
	if err := r.Create(context.Background(), pod); err != nil {
		t.Fatal(err)
	}
	got := updateSpec(t, r, f, func(f *foremanv1alpha1.Foreman) { f.Spec.Image.Tag = "5.1" })
	completePreflight(t, r, got, "5.1.0")
	got = reconcileForeman(t, r, got)
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionProgressing, metav1.ConditionTrue, reasonStoppingWriters)

	got = updateSpec(t, r, got, func(f *foremanv1alpha1.Foreman) { f.Spec.Image.Tag = oldTag })
	if got.Status.PendingMigrationHash != "" || got.Status.Phase == foremanv1alpha1.ForemanPhaseRollForwardRequired {
		t.Fatalf("pending = %q, phase = %s: no migration ran, so going back must be allowed", got.Status.PendingMigrationHash, got.Status.Phase)
	}
	if d := mustGet(t, r.Client, "prod-web", &appsv1.Deployment{}); *d.Spec.Replicas == 0 {
		t.Error("the previous version must start again")
	}
}

func TestVerifyAfterMigration(t *testing.T) {
	r, api, f := upgradeFixture(t)
	requireCondition(t, f.Status.Conditions, foremanv1alpha1.ConditionVerified, metav1.ConditionTrue, reasonVerified)
	if f.Status.VerifiedMigrationHash != f.Status.AppliedMigrationHash {
		t.Fatal("verified hash not recorded")
	}

	proxy := testSmartProxy()
	if err := r.Create(context.Background(), proxy); err != nil {
		t.Fatal(err)
	}
	api.version = "5.1.0"
	got := startMigration(t, r, f, "5.1", "5.1.0")
	completeJob(t, r.Client, migrationJobName(got, migrationHash(got, r.Images)), batchv1.JobComplete)
	got = reconcileForeman(t, r, got)
	markDeploymentAvailable(t, r.Client, "prod-web")
	markDeploymentAvailable(t, r.Client, "prod-dynflow-orchestrator")
	got = reconcileForeman(t, r, got)
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionReady, metav1.ConditionTrue, reasonReconciled)
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionVerified, metav1.ConditionFalse, reasonVerifying)

	proxy = mustGet(t, r.Client, "pxe", &foremanv1alpha1.SmartProxy{})
	proxy.Status.ForemanMigrationHash = got.Status.AppliedMigrationHash
	proxy.Status.Conditions = []metav1.Condition{{Type: foremanv1alpha1.ConditionReady, Status: metav1.ConditionTrue, Reason: reasonReconciled, LastTransitionTime: metav1.Now()}}
	if err := r.Status().Update(context.Background(), proxy); err != nil {
		t.Fatal(err)
	}
	api.version = "5.0.1"
	got = reconcileForeman(t, r, got)
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionVerified, metav1.ConditionFalse, reasonVerificationFailed)

	api.version = "5.1.0"
	got = reconcileForeman(t, r, got)
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionVerified, metav1.ConditionTrue, reasonVerified)
	if got.Status.VerifiedMigrationHash != got.Status.AppliedMigrationHash || got.Status.ForemanVersion != "5.1.0" {
		t.Errorf("verified = %q, applied = %q, version = %q", got.Status.VerifiedMigrationHash, got.Status.AppliedMigrationHash, got.Status.ForemanVersion)
	}
}

func TestStopWritersIgnoresFinishedPods(t *testing.T) {
	f := testForeman()
	c := newTestClient(f)
	r := newForemanReconciler(c)
	pod := &corev1.Pod{Name: "prod-web-done", Namespace: testNamespace, Labels: selectorLabels(foremanAppName, "prod", componentWeb)}
	if err := c.Create(context.Background(), pod); err != nil {
		t.Fatal(err)
	}
	pod.Status.Phase = corev1.PodSucceeded
	if err := c.Status().Update(context.Background(), pod); err != nil {
		t.Fatal(err)
	}
	preflight := &corev1.Pod{Name: "prod-preflight-x", Namespace: testNamespace, Labels: selectorLabels(foremanAppName, "prod", componentPreflight)}
	if err := c.Create(context.Background(), preflight); err != nil {
		t.Fatal(err)
	}
	running, err := r.stopWriters(context.Background(), f)
	if err != nil || running != 0 {
		t.Errorf("running = %d, err = %v, want 0", running, err)
	}
}
