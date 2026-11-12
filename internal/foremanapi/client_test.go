package foremanapi

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPercentEncode(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"abcABC123-._~", "abcABC123-._~"},
		{"a b", "a%20b"},
		{"=&\"", "%3D%26%22"},
		{"ü", "%C3%BC"},
	}
	for _, tt := range tests {
		if got := percentEncode(tt.in); got != tt.want {
			t.Errorf("percentEncode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSignatureMatchesReference(t *testing.T) {
	s := &oauthSigner{
		consumerKey:    "key",
		consumerSecret: "secret",
		now:            func() time.Time { return time.Unix(1700000000, 0) },
		nonce:          func() string { return "nonce" },
	}
	header, err := s.authorization(http.MethodGet, `https://foreman.example.com/api/smart_proxies?search=name+%3D+%22proxy%22`)
	if err != nil {
		t.Fatal(err)
	}
	// Computed independently with openssl from the RFC 5849 base string.
	const want = `oauth_signature="GyToTmNwum%2Bhvre3qj213YlUOUA%3D"`
	if !strings.Contains(header, want) {
		t.Errorf("header %q does not contain %s", header, want)
	}
	if !strings.HasPrefix(header, "OAuth ") || !strings.Contains(header, `oauth_consumer_key="key"`) {
		t.Errorf("unexpected header %q", header)
	}
}

type recorded struct {
	method, path, query, host, auth, body, contentType string
}

func newTestServer(t *testing.T, status int, response string) (*Client, *recorded) {
	t.Helper()
	rec := &recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*rec = recorded{r.Method, r.URL.Path, r.URL.RawQuery, r.Host, r.Header.Get("Authorization"), string(body), r.Header.Get("Content-Type")}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, "https://foreman.example.com", "key", "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	return c, rec
}

func TestFindSmartProxy(t *testing.T) {
	c, rec := newTestServer(t, http.StatusOK, `{"results":[{"id":7,"name":"pxe","url":"https://pxe:8443"}]}`)
	p, err := c.FindSmartProxy(context.Background(), "pxe")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 7 {
		t.Errorf("id = %d, want 7", p.ID)
	}
	if rec.method != http.MethodGet || rec.path != "/api/smart_proxies" {
		t.Errorf("request = %s %s", rec.method, rec.path)
	}
	if !strings.Contains(rec.query, "search=name+%3D+%22pxe%22") {
		t.Errorf("query = %q", rec.query)
	}
	if rec.host != "foreman.example.com" {
		t.Errorf("host = %q", rec.host)
	}
	if !strings.HasPrefix(rec.auth, "OAuth ") {
		t.Errorf("authorization = %q", rec.auth)
	}
}

func TestVerifiesFrontendCertificate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"pxe"}]}`))
	}))
	t.Cleanup(srv.Close)
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())

	// The test certificate is valid for example.com, which is sent as the server name.
	c, err := New(srv.URL, "https://example.com", "key", "secret", pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.FindSmartProxy(context.Background(), "pxe"); err != nil {
		t.Errorf("trusted certificate: %v", err)
	}

	untrusted, err := New(srv.URL, "https://example.com", "key", "secret", x509.NewCertPool())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := untrusted.FindSmartProxy(context.Background(), "pxe"); err == nil {
		t.Error("an untrusted certificate must be rejected")
	}
}

func TestFindSmartProxyNotFound(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{"results":[{"id":1,"name":"other"}]}`)
	if _, err := c.FindSmartProxy(context.Background(), "pxe"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestCreateSmartProxy(t *testing.T) {
	c, rec := newTestServer(t, http.StatusCreated, `{"id":3,"name":"pxe","url":"https://pxe:8443"}`)
	p, err := c.CreateSmartProxy(context.Background(), "pxe", "https://pxe:8443")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 3 || rec.method != http.MethodPost {
		t.Errorf("id = %d, method = %s", p.ID, rec.method)
	}
	var body smartProxyBody
	if err := json.Unmarshal([]byte(rec.body), &body); err != nil {
		t.Fatal(err)
	}
	if body.SmartProxy.Name != "pxe" || body.SmartProxy.URL != "https://pxe:8443" {
		t.Errorf("body = %+v", body)
	}
}

func TestUpdateRefreshDelete(t *testing.T) {
	c, rec := newTestServer(t, http.StatusOK, `{}`)
	ctx := context.Background()

	if err := c.UpdateSmartProxy(ctx, 3, "pxe", "https://pxe:8443"); err != nil || rec.method != http.MethodPut || rec.path != "/api/smart_proxies/3" {
		t.Errorf("update: err = %v, request = %s %s", err, rec.method, rec.path)
	}
	if err := c.RefreshSmartProxy(ctx, 3); err != nil || rec.path != "/api/smart_proxies/3/refresh" {
		t.Errorf("refresh: err = %v, path = %s", err, rec.path)
	}
	// Foreman answers 415 to a PUT without a JSON content type, even without a body.
	if rec.contentType != "application/json" || rec.body != "{}" {
		t.Errorf("refresh: content type = %q, body = %q", rec.contentType, rec.body)
	}
	if err := c.DeleteSmartProxy(ctx, 3); err != nil || rec.method != http.MethodDelete {
		t.Errorf("delete: err = %v, method = %s", err, rec.method)
	}
}

func TestDeleteMissingIsNotAnError(t *testing.T) {
	c, _ := newTestServer(t, http.StatusNotFound, `{}`)
	if err := c.DeleteSmartProxy(context.Background(), 3); err != nil {
		t.Errorf("err = %v", err)
	}
}

func TestAPIErrorMessage(t *testing.T) {
	c, _ := newTestServer(t, http.StatusUnprocessableEntity, `{"error":{"full_messages":["Unable to communicate with the proxy"]}}`)
	_, err := c.CreateSmartProxy(context.Background(), "pxe", "https://pxe:8443")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(apiErr.Message, "communicate") {
		t.Errorf("apiErr = %+v", apiErr)
	}
}

func TestErrorMessageFallbacks(t *testing.T) {
	if got := errorMessage([]byte(`{"error":{"message":"denied"}}`)); got != "denied" {
		t.Errorf("got %q", got)
	}
	long := strings.Repeat("x", 300)
	if got := errorMessage([]byte(long)); len(got) != 200 {
		t.Errorf("len = %d, want 200", len(got))
	}
}
