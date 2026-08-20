package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// newCSRFTestTransport points a Transport at a test server.
func newCSRFTestTransport(t *testing.T, srv *httptest.Server) *Transport {
	t.Helper()
	cfg := NewConfig(srv.URL, "user", "pass")
	tr := NewTransport(cfg)
	return tr
}

// TestFetchCSRFTokenHeadFastPath: when HEAD answers with a token, that is the only
// request made — the fast path must not cost a second round trip.
func TestFetchCSRFTokenHeadFastPath(t *testing.T) {
	var heads, gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			heads.Add(1)
			w.Header().Set("X-CSRF-Token", "TOKEN-FROM-HEAD")
			w.WriteHeader(http.StatusOK)
		default:
			gets.Add(1)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	tr := newCSRFTestTransport(t, srv)
	if err := tr.fetchCSRFToken(context.Background()); err != nil {
		t.Fatalf("fetchCSRFToken: %v", err)
	}
	if got := tr.getCSRFToken(); got != "TOKEN-FROM-HEAD" {
		t.Fatalf("token = %q", got)
	}
	if heads.Load() != 1 || gets.Load() != 0 {
		t.Fatalf("expected one HEAD and no GET, got %d/%d", heads.Load(), gets.Load())
	}
}

// TestFetchCSRFTokenGetFallback reproduces BASIS 740 / ECC EhP7: HEAD is answered
// with 400 and no token, and only GET yields one. Without the fallback vsp cannot
// talk to those systems at all.
func TestFetchCSRFTokenGetFallback(t *testing.T) {
	var heads, gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			heads.Add(1)
			w.WriteHeader(http.StatusBadRequest)
		default:
			gets.Add(1)
			w.Header().Set("X-CSRF-Token", "TOKEN-FROM-GET")
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	tr := newCSRFTestTransport(t, srv)
	if err := tr.fetchCSRFToken(context.Background()); err != nil {
		t.Fatalf("fetchCSRFToken: %v", err)
	}
	if got := tr.getCSRFToken(); got != "TOKEN-FROM-GET" {
		t.Fatalf("token = %q", got)
	}
	if heads.Load() != 1 || gets.Load() != 1 {
		t.Fatalf("expected one HEAD then one GET, got %d/%d", heads.Load(), gets.Load())
	}
}

// A 401 must be reported as an authentication failure without a pointless retry.
func TestFetchCSRFTokenUnauthorizedDoesNotRetry(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	tr := newCSRFTestTransport(t, srv)
	err := tr.fetchCSRFToken(context.Background())
	if err == nil {
		t.Fatal("expected an error for 401")
	}
	if requests.Load() != 1 {
		t.Fatalf("401 must not be retried, made %d requests", requests.Load())
	}
}

// The "Required" placeholder is not a token; it must trigger the fallback.
func TestFetchCSRFTokenRequiredPlaceholderFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("X-CSRF-Token", "Required")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("X-CSRF-Token", "REAL-TOKEN")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr := newCSRFTestTransport(t, srv)
	if err := tr.fetchCSRFToken(context.Background()); err != nil {
		t.Fatalf("fetchCSRFToken: %v", err)
	}
	if got := tr.getCSRFToken(); got != "REAL-TOKEN" {
		t.Fatalf("token = %q", got)
	}
}
