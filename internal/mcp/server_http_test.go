package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAPIKey(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := requireAPIKey("s3cret", ok)

	cases := []struct {
		name   string
		header map[string]string
		want   int
	}{
		{"no credentials", nil, http.StatusUnauthorized},
		{"wrong bearer", map[string]string{"Authorization": "Bearer nope"}, http.StatusUnauthorized},
		{"correct bearer", map[string]string{"Authorization": "Bearer s3cret"}, http.StatusTeapot},
		{"correct x-api-key", map[string]string{"X-API-Key": "s3cret"}, http.StatusTeapot},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			for k, v := range c.header {
				r.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != c.want {
				t.Fatalf("status = %d, want %d", w.Code, c.want)
			}
			if c.want == http.StatusUnauthorized && w.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("a 401 must offer WWW-Authenticate")
			}
		})
	}

	// An empty key disables the check (loopback-only, enforced by ServeHTTP).
	w := httptest.NewRecorder()
	requireAPIKey("", ok).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/", nil))
	if w.Code != http.StatusTeapot {
		t.Fatalf("empty key must pass through, got %d", w.Code)
	}
}

// TestValidateOrigin covers the DNS-rebinding case: a page on another origin must
// not be able to drive a loopback endpoint, while a non-browser client (no Origin)
// still can.
func TestValidateOrigin(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := validateOrigin(ok)

	cases := []struct {
		name, origin, host string
		want               int
	}{
		{"no origin (API client)", "", "127.0.0.1:8080", http.StatusTeapot},
		{"same origin", "http://127.0.0.1:8080", "127.0.0.1:8080", http.StatusTeapot},
		{"same host, other port", "http://127.0.0.1:9999", "127.0.0.1:8080", http.StatusTeapot},
		{"rebound attacker origin", "https://evil.example", "127.0.0.1:8080", http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			r.Host = c.host
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != c.want {
				t.Fatalf("status = %d, want %d", w.Code, c.want)
			}
		})
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	loopback := []string{"127.0.0.1:8080", "localhost:8080", "[::1]:8080"}
	exposed := []string{":8080", "0.0.0.0:8080", "[::]:8080", "192.168.1.10:8080"}
	for _, a := range loopback {
		if !isLoopbackAddr(a) {
			t.Fatalf("%s should be loopback", a)
		}
	}
	for _, a := range exposed {
		if isLoopbackAddr(a) {
			t.Fatalf("%s should NOT be loopback (it needs an API key)", a)
		}
	}
}
