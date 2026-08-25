package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TADIR calls an include a PROG and ADT does not: an include answers at
// /programs/includes and 404s at /programs/programs. So a whole class of object
// that a package lists as a program could not be read as one — 53 of them in
// one stock package, every one with source and active, every one reported as
// unreadable. A boundary report over that package said "No crossings found"
// having failed to open a quarter of it.

func includeFallbackServer(t *testing.T, programStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-csrf-token", "test-token")
		switch {
		case strings.Contains(r.URL.Path, "/programs/programs/"):
			w.WriteHeader(programStatus)
			// The body has to match the status. The first version of this
			// fixture sent a not-found document under a 403, which no system
			// does, and the retry fired on it — so the test failed and the code
			// was right. A fixture that models something impossible tests
			// nothing and accuses the code.
			if programStatus == http.StatusNotFound {
				w.Write([]byte(`<exc:exception><type id="ExceptionResourceNotFound"/></exc:exception>`))
				return
			}
			w.Write([]byte(`<exc:exception><type id="ExceptionAuthorization"/><message>not authorised</message></exc:exception>`))
		case strings.Contains(r.URL.Path, "/programs/includes/"):
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("*& Include ZDEMO_DRV\nWRITE 'x'.\n"))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
}

func TestAProgramThatIsReallyAnIncludeIsStillRead(t *testing.T) {
	srv := includeFallbackServer(t, http.StatusNotFound)
	defer srv.Close()

	c := NewClient(srv.URL, "user", "pass")
	src, err := c.GetProgram(context.Background(), "ZDEMO_DRV")
	if err != nil {
		t.Fatalf("the object has source at the include address: %v", err)
	}
	if !strings.Contains(src, "Include ZDEMO_DRV") {
		t.Errorf("expected the include's source, got %q", src)
	}
}

func TestAnAuthorisationFailureIsNotRetriedAsAnInclude(t *testing.T) {
	// The retry is narrow on purpose. Retrying a 403 or a timeout would turn
	// one clear error into two vague ones, and the second would be about the
	// wrong resource.
	srv := includeFallbackServer(t, http.StatusForbidden)
	defer srv.Close()

	c := NewClient(srv.URL, "user", "pass")
	if _, err := c.GetProgram(context.Background(), "ZDEMO_DRV"); err == nil {
		t.Fatal("a refusal is not a wrong address and must reach the caller")
	} else if strings.Contains(err.Error(), "include") {
		t.Errorf("the error should be about the program that was refused, got %v", err)
	}
}

func TestIsNotFoundIsNarrow(t *testing.T) {
	for _, c := range []struct {
		msg  string
		want bool
	}{
		{"ADT API error: status 404 at /x", true},
		{"<type id=\"ExceptionResourceNotFound\"/>", true},
		{"ADT API error: status 403 at /x", false},
		{"context deadline exceeded", false},
		{"", false},
	} {
		var err error
		if c.msg != "" {
			err = &stringError{c.msg}
		}
		if got := isNotFound(err); got != c.want {
			t.Errorf("isNotFound(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
}

type stringError struct{ s string }

func (e *stringError) Error() string { return e.s }
