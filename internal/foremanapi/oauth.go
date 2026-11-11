package foremanapi

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // OAuth 1.0a HMAC-SHA1 is the only signature method Foreman accepts
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// oauthSigner signs requests with two-legged OAuth 1.0a (RFC 5849), as used by Foreman.
// Request bodies are JSON and therefore not part of the signature.
type oauthSigner struct {
	consumerKey    string
	consumerSecret string
	now            func() time.Time
	nonce          func() string
}

// authorization returns the Authorization header value for a request to rawURL.
func (s *oauthSigner) authorization(method, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	oauthParams := map[string]string{
		"oauth_consumer_key":     s.consumerKey,
		"oauth_nonce":            s.nonce(),
		"oauth_signature_method": "HMAC-SHA1",
		"oauth_timestamp":        strconv.FormatInt(s.now().Unix(), 10),
		"oauth_version":          "1.0",
	}

	oauthParams["oauth_signature"] = s.signature(method, u, oauthParams)

	keys := make([]string, 0, len(oauthParams))
	for k := range oauthParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, percentEncode(k)+`="`+percentEncode(oauthParams[k])+`"`)
	}
	return "OAuth " + strings.Join(parts, ", "), nil
}

func (s *oauthSigner) signature(method string, u *url.URL, oauthParams map[string]string) string {
	type pair struct{ k, v string }
	var pairs []pair
	for k, v := range oauthParams {
		pairs = append(pairs, pair{percentEncode(k), percentEncode(v)})
	}
	for k, vs := range u.Query() {
		for _, v := range vs {
			pairs = append(pairs, pair{percentEncode(k), percentEncode(v)})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k == pairs[j].k {
			return pairs[i].v < pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})

	normalized := make([]string, 0, len(pairs))
	for _, p := range pairs {
		normalized = append(normalized, p.k+"="+p.v)
	}

	baseURL := strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + u.EscapedPath()
	base := strings.ToUpper(method) + "&" + percentEncode(baseURL) + "&" + percentEncode(strings.Join(normalized, "&"))

	// Two-legged OAuth has no token, so the token secret is empty.
	mac := hmac.New(sha1.New, []byte(percentEncode(s.consumerSecret)+"&"))
	mac.Write([]byte(base))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// percentEncode encodes per RFC 5849 section 3.6: only unreserved characters stay as they are.
func percentEncode(s string) string {
	var b strings.Builder
	for i := range len(s) {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteString("%" + strings.ToUpper(hex.EncodeToString([]byte{c})))
	}
	return b.String()
}

func randomNonce() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
