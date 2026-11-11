package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	foremanv1alpha1 "github.com/slauger/foreman-operator/api/v1alpha1"
)

const testNamespace = "foreman"

var allCapabilities = Capabilities{CertManager: true, GatewayAPI: true}

// testScheme registers all types the controllers use, including the untyped cert-manager
// and OpenShift kinds, which the fake client otherwise cannot map.
func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(foremanv1alpha1.AddToScheme(s))
	utilruntime.Must(gwapiv1.Install(s))
	for _, gvk := range []schema.GroupVersionKind{certificateGVK, routeGVK} {
		s.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		s.AddKnownTypeWithName(gvk.GroupVersion().WithKind(gvk.Kind+"List"), &unstructured.UnstructuredList{})
	}
	return s
}

func newTestClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(objs...).
		WithStatusSubresource(&foremanv1alpha1.Foreman{}, &foremanv1alpha1.Frontend{}, &foremanv1alpha1.SmartProxy{}).
		Build()
}

func testForeman() *foremanv1alpha1.Foreman {
	return &foremanv1alpha1.Foreman{
		Name: "prod", Namespace: testNamespace, Generation: 1,
		Spec: foremanv1alpha1.ForemanSpec{
			FQDN:     "foreman.example.com",
			Aliases:  []string{"puppet.example.com"},
			TLS:      foremanv1alpha1.TLSSpec{IssuerRef: &foremanv1alpha1.IssuerReference{Name: "internal-ca"}},
			Database: foremanv1alpha1.DatabaseSpec{Cluster: foremanv1alpha1.CNPGClusterRef{Name: "foreman-db"}},
			Valkey:   foremanv1alpha1.ValkeySpec{Host: "valkey", Port: 6379},
		},
	}
}

func dbSecret() *corev1.Secret {
	return &corev1.Secret{
		Name: "foreman-db-app", Namespace: testNamespace,
		Data: map[string][]byte{"uri": []byte("postgresql://foreman:pw@foreman-db-rw:5432/foreman")},
	}
}

func reconcileRequest(obj client.Object) ctrl.Request {
	return ctrl.Request{NamespacedName: client.ObjectKeyFromObject(obj)}
}

func mustGet[T client.Object](t *testing.T, c client.Client, name string, obj T) T {
	t.Helper()
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: name}, obj); err != nil {
		t.Fatalf("get %T %s: %v", obj, name, err)
	}
	return obj
}

// markDeploymentAvailable sets the status a Deployment controller would report.
func markDeploymentAvailable(t *testing.T, c client.Client, name string) {
	t.Helper()
	d := mustGet(t, c, name, &appsv1.Deployment{})
	d.Status.ObservedGeneration = d.Generation
	d.Status.AvailableReplicas = *d.Spec.Replicas
	d.Status.ReadyReplicas = *d.Spec.Replicas
	if err := c.Status().Update(context.Background(), d); err != nil {
		t.Fatalf("update deployment status: %v", err)
	}
}

func requireCondition(t *testing.T, conditions []metav1.Condition, condType string, status metav1.ConditionStatus, reason string) {
	t.Helper()
	cond := meta.FindStatusCondition(conditions, condType)
	if cond == nil {
		t.Fatalf("condition %s missing", condType)
	}
	if cond.Status != status || cond.Reason != reason {
		t.Fatalf("condition %s = %s/%s (%s), want %s/%s", condType, cond.Status, cond.Reason, cond.Message, status, reason)
	}
}
