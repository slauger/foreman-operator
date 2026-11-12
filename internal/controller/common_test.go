package controller

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestSetPodTemplateKeepsRestart(t *testing.T) {
	desired := corev1.PodTemplateSpec{Annotations: map[string]string{checksumAnnotation: "new"}}

	d := &appsv1.Deployment{}
	d.Spec.Template.Annotations = map[string]string{checksumAnnotation: "old", restartedAtAnnotation: "2026-11-12T01:40:47Z"}
	setPodTemplate(d, desired)
	if d.Spec.Template.Annotations[checksumAnnotation] != "new" || d.Spec.Template.Annotations[restartedAtAnnotation] != "2026-11-12T01:40:47Z" {
		t.Errorf("annotations = %v", d.Spec.Template.Annotations)
	}
	if _, ok := desired.Annotations[restartedAtAnnotation]; ok {
		t.Error("the desired template must not be modified")
	}

	fresh := &appsv1.Deployment{}
	setPodTemplate(fresh, desired)
	if _, ok := fresh.Spec.Template.Annotations[restartedAtAnnotation]; ok {
		t.Error("a template that was never restarted must not get the annotation")
	}
}

func TestEveryContainerHasReadOnlyRootFilesystem(t *testing.T) {
	f := testForeman()
	f.Spec.Dynflow.HostsQueueReplicas = new(int32(1))
	r := &ForemanReconciler{Images: DefaultImages("test")}
	specs := map[string]corev1.PodSpec{}
	for _, d := range r.desiredDeployments(f, "config", "frontend") {
		specs[d.Name] = d.Spec.Template.Spec
	}
	specs["migration"] = migrationJob(f, r.Images, r.Capabilities, "0123456789abcdef").Spec.Template.Spec

	p := testSmartProxy()
	files, err := smartProxyConfig(p, f)
	if err != nil {
		t.Fatal(err)
	}
	d, err := smartProxyDeployment(p, r.Images, r.Capabilities, files, "")
	if err != nil {
		t.Fatal(err)
	}
	specs[d.Name] = d.Spec.Template.Spec

	containers := 0
	for name, spec := range specs {
		for _, c := range append(spec.InitContainers, spec.Containers...) {
			containers++
			if sc := c.SecurityContext; sc == nil || sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
				t.Errorf("%s/%s: the root filesystem must be read-only", name, c.Name)
			}
		}
	}
	// web (Puma and httpd), three Dynflow workloads, migration, and the proxy with dnsmasq
	// and the boot files init container.
	if containers != 9 {
		t.Errorf("checked %d containers, want 9", containers)
	}
}
