package controller

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	foremanv1alpha1 "github.com/slauger/foreman-operator/api/v1alpha1"
)

func newForemanReconciler(c client.Client) *ForemanReconciler {
	return &ForemanReconciler{Client: c, Scheme: c.Scheme(), Images: DefaultImages("test"), Capabilities: allCapabilities}
}

func reconcileForeman(t *testing.T, r *ForemanReconciler, f *foremanv1alpha1.Foreman) *foremanv1alpha1.Foreman {
	t.Helper()
	if _, err := r.Reconcile(context.Background(), reconcileRequest(f)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	return mustGet(t, r.Client, f.Name, &foremanv1alpha1.Foreman{})
}

func completeJob(t *testing.T, c client.Client, name string, condType batchv1.JobConditionType) {
	t.Helper()
	job := mustGet(t, c, name, &batchv1.Job{})
	job.Status.Conditions = append(job.Status.Conditions, batchv1.JobCondition{Type: condType, Status: corev1.ConditionTrue})
	if err := c.Status().Update(context.Background(), job); err != nil {
		t.Fatal(err)
	}
}

func TestForemanWaitsForDatabase(t *testing.T) {
	f := testForeman()
	c := newTestClient(f)
	r := newForemanReconciler(c)

	got := reconcileForeman(t, r, f)
	if got.Status.Phase != foremanv1alpha1.ForemanPhasePending {
		t.Errorf("phase = %s, want Pending", got.Status.Phase)
	}
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonWaitingDatabase)

	secrets := mustGet(t, c, "prod-secrets", &corev1.Secret{})
	for _, k := range []string{keyEncryptionKey, keySecretKeyBase, keyAdminPassword, keyOAuthConsumerKey, keyOAuthConsumerSecret} {
		if len(secrets.Data[k]) == 0 {
			t.Errorf("secret key %s not generated", k)
		}
	}
	if len(secrets.OwnerReferences) != 0 {
		t.Error("the secrets Secret must not be owned, or deleting the Foreman loses the encryption key")
	}

	settings := string(mustGet(t, c, "prod-settings", &corev1.Secret{}).Data["settings.yaml"])
	for _, want := range []string{
		`:foreman_url: "https://foreman.example.com"`,
		`  - "puppet.example.com"`,
		`  - "prod-web.foreman.svc"`,
		`:oauth_consumer_key: "` + string(secrets.Data[keyOAuthConsumerKey]) + `"`,
		`    - "redis://valkey:6379/4"`,
		`:ssl_certificate: "/etc/foreman/tls/tls.crt"`,
	} {
		if !strings.Contains(settings, want) {
			t.Errorf("settings.yaml lacks %q:\n%s", want, settings)
		}
	}

	cert := mustGet(t, c, "prod-client", newCertificateObject())
	cn, _, _ := unstructured.NestedString(cert.Object, "spec", "commonName")
	if cn != "foreman.example.com" {
		t.Errorf("client certificate CN = %q", cn)
	}
	mustGet(t, c, "prod-web", &corev1.Service{})
	mustGet(t, c, "prod-dynflow", &corev1.ConfigMap{})
}

func TestForemanKeepsExistingSecrets(t *testing.T) {
	f := testForeman()
	existing := &corev1.Secret{
		Name: "prod-secrets", Namespace: testNamespace,
		Data: map[string][]byte{keyEncryptionKey: []byte("keep-me")},
	}
	c := newTestClient(f, existing)
	reconcileForeman(t, newForemanReconciler(c), f)

	got := mustGet(t, c, "prod-secrets", &corev1.Secret{})
	if string(got.Data[keyEncryptionKey]) != "keep-me" {
		t.Error("existing encryption key was replaced")
	}
	if len(got.Data[keyAdminPassword]) == 0 {
		t.Error("missing admin password was not generated")
	}
}

func TestForemanInstallMigratesBeforeDeploying(t *testing.T) {
	f := testForeman()
	c := newTestClient(f, dbSecret())
	r := newForemanReconciler(c)

	got := reconcileForeman(t, r, f)
	if got.Status.Phase != foremanv1alpha1.ForemanPhaseMigrating {
		t.Fatalf("phase = %s, want Migrating", got.Status.Phase)
	}
	hash := migrationHash(f, r.Images)
	jobName := migrationJobName(f, hash)
	job := mustGet(t, c, jobName, &batchv1.Job{})
	if !strings.Contains(strings.Join(job.Spec.Template.Spec.Containers[0].Command, " "), "upgrade:run") {
		t.Errorf("migration command = %v", job.Spec.Template.Spec.Containers[0].Command)
	}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: "prod-web"}, &appsv1.Deployment{}); err == nil {
		t.Fatal("web deployment created before the migration finished")
	}

	completeJob(t, c, jobName, batchv1.JobComplete)
	got = reconcileForeman(t, r, got)
	if got.Status.AppliedMigrationHash != hash {
		t.Errorf("applied hash = %q, want %q", got.Status.AppliedMigrationHash, hash)
	}
	if got.Status.Phase != foremanv1alpha1.ForemanPhaseProgressing {
		t.Errorf("phase = %s, want Progressing", got.Status.Phase)
	}

	web := mustGet(t, c, "prod-web", &appsv1.Deployment{})
	if links := web.Spec.Template.Spec.EnableServiceLinks; links == nil || *links {
		t.Error("service links must be off, they inject FOREMAN_PORT into Puma")
	}
	if *web.Spec.Replicas != 2 {
		t.Errorf("web replicas = %d, want 2", *web.Spec.Replicas)
	}
	if web.Spec.Template.Spec.Containers[0].Image != "ghcr.io/slauger/foreman-operator-foreman:test" {
		t.Errorf("image = %s", web.Spec.Template.Spec.Containers[0].Image)
	}
	orch := mustGet(t, c, "prod-dynflow-orchestrator", &appsv1.Deployment{})
	if orch.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType || *orch.Spec.Replicas != 1 {
		t.Errorf("orchestrator strategy = %s, replicas = %d", orch.Spec.Strategy.Type, *orch.Spec.Replicas)
	}
	hostsQueue := mustGet(t, c, "prod-dynflow-hosts-queue", &appsv1.Deployment{})
	if *hostsQueue.Spec.Replicas != 0 {
		t.Errorf("hosts queue replicas = %d, want 0", *hostsQueue.Spec.Replicas)
	}

	markDeploymentAvailable(t, c, "prod-web")
	markDeploymentAvailable(t, c, "prod-dynflow-orchestrator")
	got = reconcileForeman(t, r, got)
	if got.Status.Phase != foremanv1alpha1.ForemanPhaseRunning {
		t.Errorf("phase = %s, want Running", got.Status.Phase)
	}
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionReady, metav1.ConditionTrue, reasonReconciled)
}

func TestForemanUpgradeStopsWritersFirst(t *testing.T) {
	f := testForeman()
	c := newTestClient(f, dbSecret())
	r := newForemanReconciler(c)

	got := reconcileForeman(t, r, f)
	completeJob(t, c, migrationJobName(f, migrationHash(f, r.Images)), batchv1.JobComplete)
	got = reconcileForeman(t, r, got)

	// A running web pod must stop before the new migration starts.
	pod := &corev1.Pod{
		Name: "prod-web-abc", Namespace: testNamespace,
		Labels: selectorLabels(foremanAppName, "prod", componentWeb)}
	if err := c.Create(context.Background(), pod); err != nil {
		t.Fatal(err)
	}

	got.Spec.Image.Tag = "next"
	if err := c.Update(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	got = reconcileForeman(t, r, got)
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionProgressing, metav1.ConditionTrue, reasonStoppingWriters)
	for _, name := range []string{"prod-web", "prod-dynflow-orchestrator", "prod-dynflow-worker"} {
		if d := mustGet(t, c, name, &appsv1.Deployment{}); *d.Spec.Replicas != 0 {
			t.Errorf("%s replicas = %d, want 0", name, *d.Spec.Replicas)
		}
	}
	newJob := migrationJobName(got, migrationHash(got, r.Images))
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: newJob}, &batchv1.Job{}); err == nil {
		t.Fatal("migration started while a writer was still running")
	}

	if err := c.Delete(context.Background(), pod); err != nil {
		t.Fatal(err)
	}
	reconcileForeman(t, r, got)
	mustGet(t, c, newJob, &batchv1.Job{})
}

func TestForemanFailedMigration(t *testing.T) {
	f := testForeman()
	c := newTestClient(f, dbSecret())
	r := newForemanReconciler(c)

	got := reconcileForeman(t, r, f)
	completeJob(t, c, migrationJobName(f, migrationHash(f, r.Images)), batchv1.JobFailed)
	got = reconcileForeman(t, r, got)
	if got.Status.Phase != foremanv1alpha1.ForemanPhaseFailed {
		t.Errorf("phase = %s, want Failed", got.Status.Phase)
	}
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionDegraded, metav1.ConditionTrue, reasonMigrationFailed)
}

func TestForemanPaused(t *testing.T) {
	f := testForeman()
	f.Annotations = map[string]string{foremanv1alpha1.PausedAnnotation: "true"}
	c := newTestClient(f)
	got := reconcileForeman(t, newForemanReconciler(c), f)
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionPaused, metav1.ConditionTrue, reasonPaused)
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: "prod-secrets"}, &corev1.Secret{}); err == nil {
		t.Error("a paused Foreman must not be reconciled")
	}
}

func TestForemanRequiresCertManagerForIssuer(t *testing.T) {
	f := testForeman()
	c := newTestClient(f)
	r := newForemanReconciler(c)
	r.Capabilities = Capabilities{}
	if _, err := r.Reconcile(context.Background(), reconcileRequest(f)); err == nil {
		t.Fatal("expected an error without cert-manager")
	}
	got := mustGet(t, c, f.Name, &foremanv1alpha1.Foreman{})
	requireCondition(t, got.Status.Conditions, foremanv1alpha1.ConditionReady, metav1.ConditionFalse, reasonReconcileError)
}

func TestMigrationHashTracksImageAndPlugins(t *testing.T) {
	images := DefaultImages("test")
	a := testForeman()
	a.Spec.Plugins = []foremanv1alpha1.ForemanPlugin{"foreman_remote_execution", "foreman_discovery"}
	b := testForeman()
	b.Spec.Plugins = []foremanv1alpha1.ForemanPlugin{"foreman_discovery", "foreman_remote_execution"}
	if migrationHash(a, images) != migrationHash(b, images) {
		t.Error("plugin order must not change the hash")
	}
	c := testForeman()
	c.Spec.Plugins = []foremanv1alpha1.ForemanPlugin{"foreman_discovery"}
	if migrationHash(a, images) == migrationHash(c, images) {
		t.Error("a different plugin set must change the hash")
	}
	b.Spec.Image.Tag = "other"
	if migrationHash(a, images) == migrationHash(b, images) {
		t.Error("a different image must change the hash")
	}
}

func TestEnabledPlugins(t *testing.T) {
	tests := []struct {
		plugins []foremanv1alpha1.ForemanPlugin
		want    string
	}{
		// Without plugins the variable is empty, which keeps every installed plugin off.
		{nil, ""},
		{[]foremanv1alpha1.ForemanPlugin{"foreman_discovery"}, "foreman_discovery"},
		{[]foremanv1alpha1.ForemanPlugin{"foreman_remote_execution"}, "foreman-tasks foreman_remote_execution"},
		{[]foremanv1alpha1.ForemanPlugin{"foreman_remote_execution", "foreman_ansible"}, "foreman-tasks foreman_ansible foreman_remote_execution"},
	}
	for _, tt := range tests {
		f := testForeman()
		f.Spec.Plugins = tt.plugins
		var got *corev1.EnvVar
		for _, e := range foremanEnv(f, 5) {
			if e.Name == "FOREMAN_ENABLED_PLUGINS" {
				got = &e
			}
		}
		if got == nil || got.Value != tt.want {
			t.Errorf("plugins %v: FOREMAN_ENABLED_PLUGINS = %+v, want %q", tt.plugins, got, tt.want)
		}
	}
}

func TestPumaWorkers(t *testing.T) {
	tests := []struct {
		limit string
		want  int64
	}{
		{"", 2},
		{"1Gi", 1},
		{"3Gi", 2},
		{"8Gi", 5},
		{"64Gi", 16},
	}
	for _, tt := range tests {
		res := corev1.ResourceRequirements{}
		if tt.limit != "" {
			res.Limits = corev1.ResourceList{corev1.ResourceMemory: resource.MustParse(tt.limit)}
		}
		if got := pumaWorkers(res); got != tt.want {
			t.Errorf("pumaWorkers(%q) = %d, want %d", tt.limit, got, tt.want)
		}
	}
}

func TestImageRef(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	tests := []struct {
		name string
		spec foremanv1alpha1.ImageSpec
		want string
	}{
		{"default", foremanv1alpha1.ImageSpec{}, "repo/foreman:dev"},
		{"tag", foremanv1alpha1.ImageSpec{Tag: "1.0"}, "repo/foreman:1.0"},
		{"repository", foremanv1alpha1.ImageSpec{Repository: "mirror/foreman"}, "mirror/foreman:dev"},
		{"digest wins", foremanv1alpha1.ImageSpec{Tag: "1.0", Digest: digest}, "repo/foreman@" + digest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := imageRef(tt.spec, "repo/foreman", "dev"); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRubyMap(t *testing.T) {
	inner := rubyMap{}
	inner.set("compress", true)
	m := rubyMap{}
	m.set("name", `quote " inside`)
	m.set("port", 8443)
	m.set("hosts", []string{"a", "b"})
	m.set("empty", []string{})
	m.set("options", inner)
	want := "---\n:name: \"quote \\\" inside\"\n:port: 8443\n:hosts:\n  - \"a\"\n  - \"b\"\n:empty: []\n:options:\n  :compress: true\n"
	if got := m.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
