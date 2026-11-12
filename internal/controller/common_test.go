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
