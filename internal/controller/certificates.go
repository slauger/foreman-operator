package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	foremanv1alpha1 "github.com/slauger/foreman-operator/api/v1alpha1"
)

// certificateGVK is the cert-manager Certificate. The operator uses it untyped to avoid a
// dependency on the cert-manager module.
var certificateGVK = schema.GroupVersionKind{Group: "cert-manager.io", Version: "v1", Kind: "Certificate"}

func newCertificateObject() *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(certificateGVK)
	return u
}

// certificateRequest describes a certificate the operator requests from cert-manager.
type certificateRequest struct {
	name       string
	namespace  string
	issuer     foremanv1alpha1.IssuerReference
	commonName string
	dnsNames   []string
	usages     []string
	labels     map[string]string
}

// ensureCertificate creates or updates a cert-manager Certificate whose Secret has the same name.
func ensureCertificate(ctx context.Context, c client.Client, scheme *runtime.Scheme, owner client.Object, req certificateRequest) error {
	cert := newCertificateObject()
	cert.SetName(req.name)
	cert.SetNamespace(req.namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, c, cert, func() error {
		cert.SetLabels(req.labels)
		kind := req.issuer.Kind
		if kind == "" {
			kind = "Issuer"
		}
		spec := map[string]any{
			"secretName": req.name,
			"commonName": req.commonName,
			"dnsNames":   toAnySlice(req.dnsNames),
			"usages":     toAnySlice(req.usages),
			"issuerRef": map[string]any{
				"name":  req.issuer.Name,
				"kind":  kind,
				"group": "cert-manager.io",
			},
			"privateKey": map[string]any{
				"algorithm":      "RSA",
				"size":           int64(4096),
				"rotationPolicy": "Always",
			},
		}
		if err := unstructured.SetNestedMap(cert.Object, spec, "spec"); err != nil {
			return err
		}
		return controllerutil.SetControllerReference(owner, cert, scheme)
	})
	return err
}

func toAnySlice(in []string) []any {
	out := make([]any, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}

// tlsSecretName returns the Secret that holds a TLSSpec's certificate: either the referenced
// Secret or the one cert-manager writes for the generated Certificate.
func tlsSecretName(spec foremanv1alpha1.TLSSpec, generatedName string) string {
	if spec.SecretName != "" {
		return spec.SecretName
	}
	return generatedName
}
