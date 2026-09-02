package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// A syntax check is a *stateless* request. Sent while an object is locked it
// ends the stateful session the lock lives in, and the write that follows fails
// with 423 InvalidLockHandle — which is where the deploy path's share of the
// "invalid lock handle" reports came from. The fix is an ordering one: check
// first, then lock.
//
// The server below behaves the way SAP does about that, so the test fails if
// anyone puts the check back between the lock and the write.
func TestDeployChecksSyntaxBeforeLocking(t *testing.T) {
	var mu sync.Mutex
	var order []string
	locked, sessionKilled := false, false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		path := r.URL.Path
		switch {
		case strings.Contains(path, "/core/discovery"), strings.Contains(path, "/discovery"):
			w.Header().Set("X-CSRF-Token", "TOKEN")
			w.WriteHeader(http.StatusOK)

		case strings.Contains(path, "/checkruns"):
			order = append(order, "checkrun")
			// Stateless by nature: if a lock is held, the session it belongs to
			// is gone from here on.
			if locked {
				sessionKilled = true
			}
			w.Header().Set("Content-Type", "application/vnd.sap.adt.checkmessages+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist"/>`))

		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			order = append(order, "lock")
			locked = true
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA>
<LOCK_HANDLE>HANDLE-1</LOCK_HANDLE><MODIFICATION_SUPPORT>NoModification</MODIFICATION_SUPPORT>
</DATA></asx:values></asx:abap>`))

		case r.Method == http.MethodPut && strings.HasSuffix(path, "/source/main"):
			order = append(order, "write")
			if sessionKilled {
				w.WriteHeader(http.StatusLocked) // 423, exactly as SAP answers
				_, _ = w.Write([]byte("ExceptionResourceInvalidLockHandle"))
				return
			}
			w.WriteHeader(http.StatusOK)

		default:
			order = append(order, r.Method+" "+path)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, "zdeploy_order.prog.abap")
	if err := os.WriteFile(file, []byte("REPORT zdeploy_order.\nWRITE 'hello'.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := NewConfig(srv.URL, "TESTER", "secret")
	client := NewClientWithTransport(cfg, NewTransport(cfg))

	if _, err := client.DeployFromFile(context.Background(), file, "$TMP", ""); err != nil {
		t.Fatalf("DeployFromFile: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if sessionKilled {
		t.Fatal("the syntax check ran while the object was locked, which ends the session the lock lives in")
	}
	checkAt, lockAt := indexOf(order, "checkrun"), indexOf(order, "lock")
	if checkAt < 0 || lockAt < 0 {
		t.Fatalf("expected both a checkrun and a lock, got %v", order)
	}
	if checkAt > lockAt {
		t.Fatalf("the syntax check must precede the lock, got %v", order)
	}
}

func indexOf(items []string, want string) int {
	for i, s := range items {
		if s == want {
			return i
		}
	}
	return -1
}
