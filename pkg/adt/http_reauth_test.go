package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// redirectingClient answers ADT requests the way a SAP system does once the
// session behind them has expired: not with a 401, but with whatever the
// identity provider serves, under a 200, from its own host.
type redirectingClient struct {
	expired  bool
	requests int
}

func (c *redirectingClient) Do(req *http.Request) (*http.Response, error) {
	c.requests++
	resp := &http.Response{Header: http.Header{}, Request: req}
	if c.expired {
		idp := *req.URL
		idp.Host = "login.idp.example"
		resp.Request = &http.Request{URL: &idp}
		resp.StatusCode = http.StatusOK
		resp.Body = io.NopCloser(strings.NewReader("<html>sign in</html>"))
		return resp, nil
	}
	resp.StatusCode = http.StatusOK
	resp.Header.Set("X-CSRF-Token", "fresh-token")
	resp.Body = io.NopCloser(strings.NewReader("<xml/>"))
	return resp, nil
}

func TestFetchCSRFTokenRecoversFromAnIdentityProviderRedirect(t *testing.T) {
	client := &redirectingClient{expired: true}
	reauths := 0
	cfg := NewConfig("https://sap.example", "", "",
		WithCookies(map[string]string{"SAP_SESSIONID_X": "stale"}),
		WithReauthFunc(func(ctx context.Context) (map[string]string, error) {
			reauths++
			client.expired = false
			return map[string]string{"SAP_SESSIONID_X": "fresh"}, nil
		}),
	)
	transport := NewTransportWithClient(cfg, client)

	if err := transport.fetchCSRFToken(context.Background()); err != nil {
		t.Fatalf("fetchCSRFToken: %v", err)
	}
	if reauths != 1 {
		t.Errorf("re-authenticated %d times, want exactly 1", reauths)
	}
	if got := transport.getCSRFToken(); got != "fresh-token" {
		t.Errorf("token = %q, want the one minted after re-authenticating", got)
	}
}

func TestFetchCSRFTokenDoesNotReauthenticateOnForbidden(t *testing.T) {
	// 403 is an authorization verdict on a session the server recognises.
	// Re-authenticating would replace a good session and hit the same wall.
	client := &mockHTTPClient{responses: []*http.Response{
		newMockResponse(http.StatusForbidden, "", nil),
		newMockResponse(http.StatusForbidden, "", nil),
	}}
	reauths := 0
	cfg := NewConfig("https://sap.example", "", "",
		WithCookies(map[string]string{"SAP_SESSIONID_X": "live"}),
		WithReauthFunc(func(ctx context.Context) (map[string]string, error) {
			reauths++
			return nil, nil
		}),
	)
	transport := NewTransportWithClient(cfg, client)

	err := transport.fetchCSRFToken(context.Background())
	if err == nil {
		t.Fatal("fetchCSRFToken succeeded on a 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %v, want it to name the 403", err)
	}
	if reauths != 0 {
		t.Errorf("re-authenticated %d times on a 403, want 0", reauths)
	}
}

func TestFetchCSRFTokenWithoutReauthFuncReportsPlainly(t *testing.T) {
	// Basic auth has nothing to re-run, so the diagnosis must stay the old one.
	client := &mockHTTPClient{responses: []*http.Response{
		newMockResponse(http.StatusUnauthorized, "", nil),
		newMockResponse(http.StatusUnauthorized, "", nil),
	}}
	cfg := NewConfig("https://sap.example", "user", "pass")
	transport := NewTransportWithClient(cfg, client)

	err := transport.fetchCSRFToken(context.Background())
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %v, want a plain 401 report", err)
	}
}
