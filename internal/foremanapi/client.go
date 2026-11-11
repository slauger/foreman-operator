// Package foremanapi is a minimal client for the parts of the Foreman API the operator uses.
package foremanapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ErrNotFound is returned when Foreman answers with 404.
var ErrNotFound = errors.New("not found")

// Client talks to Foreman over the in-cluster Service of the web workload.
//
// Requests carry the public hostname and X-Forwarded-Proto, as the Frontend would send them.
// Foreman verifies the OAuth signature against that public URL, so it is signed as well.
type Client struct {
	baseURL    *url.URL
	publicURL  *url.URL
	signer     *oauthSigner
	httpClient *http.Client
}

// New returns a client that connects to baseURL and presents itself as publicURL.
func New(baseURL, publicURL, consumerKey, consumerSecret string) (*Client, error) {
	b, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	p, err := url.Parse(publicURL)
	if err != nil {
		return nil, fmt.Errorf("parse public URL: %w", err)
	}
	return &Client{
		baseURL:   b,
		publicURL: p,
		signer: &oauthSigner{
			consumerKey:    consumerKey,
			consumerSecret: consumerSecret,
			now:            time.Now,
			nonce:          randomNonce,
		},
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}, nil
}

// SmartProxy is a Smart Proxy as registered in Foreman.
type SmartProxy struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type smartProxyBody struct {
	SmartProxy smartProxyFields `json:"smart_proxy"`
}

type smartProxyFields struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// FindSmartProxy returns the Smart Proxy with the given name, or ErrNotFound.
func (c *Client) FindSmartProxy(ctx context.Context, name string) (*SmartProxy, error) {
	query := url.Values{}
	query.Set("search", fmt.Sprintf("name = %q", name))
	query.Set("per_page", "1")

	var list struct {
		Results []SmartProxy `json:"results"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/smart_proxies", query, nil, &list); err != nil {
		return nil, err
	}
	for i := range list.Results {
		if list.Results[i].Name == name {
			return &list.Results[i], nil
		}
	}
	return nil, ErrNotFound
}

// CreateSmartProxy registers a Smart Proxy. Foreman contacts the proxy to read its features.
func (c *Client) CreateSmartProxy(ctx context.Context, name, proxyURL string) (*SmartProxy, error) {
	var created SmartProxy
	body := smartProxyBody{SmartProxy: smartProxyFields{Name: name, URL: proxyURL}}
	if err := c.do(ctx, http.MethodPost, "/api/smart_proxies", nil, body, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// UpdateSmartProxy changes the name and URL of a registered Smart Proxy.
func (c *Client) UpdateSmartProxy(ctx context.Context, id int64, name, proxyURL string) error {
	body := smartProxyBody{SmartProxy: smartProxyFields{Name: name, URL: proxyURL}}
	return c.do(ctx, http.MethodPut, "/api/smart_proxies/"+strconv.FormatInt(id, 10), nil, body, nil)
}

// RefreshSmartProxy makes Foreman read the features of a Smart Proxy again.
func (c *Client) RefreshSmartProxy(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodPut, "/api/smart_proxies/"+strconv.FormatInt(id, 10)+"/refresh", nil, nil, nil)
}

// DeleteSmartProxy removes a Smart Proxy. A proxy that no longer exists is not an error.
func (c *Client) DeleteSmartProxy(ctx context.Context, id int64) error {
	err := c.do(ctx, http.MethodDelete, "/api/smart_proxies/"+strconv.FormatInt(id, 10), nil, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	target := c.baseURL.JoinPath(path)
	public := c.publicURL.JoinPath(path)
	if query != nil {
		target.RawQuery = query.Encode()
		public.RawQuery = query.Encode()
	}

	// Foreman rejects POST and PUT without a JSON content type, even when there is no body.
	sendsJSON := body != nil || method == http.MethodPost || method == http.MethodPut
	if sendsJSON && body == nil {
		body = struct{}{}
	}

	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return err
	}
	auth, err := c.signer.authorization(method, public.String())
	if err != nil {
		return err
	}
	req.Host = c.publicURL.Host
	req.Header.Set("Authorization", auth)
	req.Header.Set("X-Forwarded-Proto", c.publicURL.Scheme)
	req.Header.Set("Accept", "application/json")
	if sendsJSON {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &APIError{StatusCode: resp.StatusCode, Message: errorMessage(data)}
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

// APIError is a non-successful answer from Foreman.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("foreman API returned %d: %s", e.StatusCode, e.Message)
}

// errorMessage extracts the human-readable part of a Foreman error response.
func errorMessage(data []byte) string {
	var parsed struct {
		Error struct {
			Message     string   `json:"message"`
			FullMessage []string `json:"full_messages"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &parsed); err == nil {
		if len(parsed.Error.FullMessage) > 0 {
			return fmt.Sprint(parsed.Error.FullMessage)
		}
		if parsed.Error.Message != "" {
			return parsed.Error.Message
		}
	}
	if len(data) > 200 {
		data = data[:200]
	}
	return string(data)
}
