package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// The same defect, in the three other places that read only the error from
// Activate. It is one line in each, and the consequences run from a wrong
// sentence to a deleted object, which is why they are all here.

// refusalServer answers every ADT request a write workflow makes, and refuses
// every activation the way SAP does: 200, with the reason in the checklist.
type refusalServer struct {
	mu       sync.Mutex
	requests []string

	// inactive is what the inactive-objects feed returns, for ActivatePackage.
	inactive string
}

func (s *refusalServer) start(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		s.mu.Lock()
		s.requests = append(s.requests, r.Method+" "+path)
		s.mu.Unlock()

		switch {
		case strings.Contains(path, "/discovery"):
			w.Header().Set("X-CSRF-Token", "TOKEN")
			w.WriteHeader(http.StatusOK)

		case strings.Contains(path, "/activation/inactiveobjects"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(s.inactive))

		case strings.Contains(path, "/activation"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(refusedActivationXML("ZDEMO_REFUSED")))

		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA>
<LOCK_HANDLE>HANDLE-1</LOCK_HANDLE><MODIFICATION_SUPPORT>Modification</MODIFICATION_SUPPORT>
</DATA></asx:values></asx:abap>`))

		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := NewConfig(srv.URL, "TESTUSER", "secret")
	return NewClientWithTransport(cfg, NewTransport(cfg))
}

func (s *refusalServer) sawDelete() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, req := range s.requests {
		if strings.HasPrefix(req, http.MethodDelete+" ") {
			return true
		}
	}
	return false
}

// The costly one. RenameObject copies, activates, then deletes the original —
// so an unread refusal deletes the object that worked and leaves the copy that
// does not compile.
func TestRenameKeepsTheOriginalWhenTheCopyWillNotActivate(t *testing.T) {
	srv := &refusalServer{}
	client := srv.start(t)

	result, err := client.RenameObject(context.Background(), ObjectTypeProgram, "ZDEMO_OLD", "ZDEMO_NEW", "$TMP", "")
	if err != nil {
		t.Fatalf("RenameObject: %v", err)
	}
	if srv.sawDelete() {
		t.Fatal("the original was deleted after an activation SAP refused")
	}
	if result.Success {
		t.Fatal("a rename whose copy never activated was reported as done")
	}
	if !strings.Contains(strings.Join(result.Errors, " "), "Tables with headers") {
		t.Fatalf("SAP's reason was not passed on: %v", result.Errors)
	}
}

// "Activated 1 objects, 0 failed" about an object that is still inactive. The
// summary is what anyone reads, and it was counting refusals as successes.
func TestActivatePackageCountsARefusalAsAFailure(t *testing.T) {
	srv := &refusalServer{inactive: `<?xml version="1.0" encoding="UTF-8"?>
<ioc:inactiveObjects xmlns:ioc="http://www.sap.com/adt/inactivectsobjects">
  <ioc:entry><ioc:object ioc:user="TESTUSER">
    <ioc:ref adtcore:uri="/sap/bc/adt/programs/programs/zdemo_refused" adtcore:type="PROG/P" adtcore:name="ZDEMO_REFUSED"
             xmlns:adtcore="http://www.sap.com/adt/core"/>
  </ioc:object></ioc:entry>
</ioc:inactiveObjects>`}
	client := srv.start(t)

	result, err := client.ActivatePackage(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("ActivatePackage: %v", err)
	}
	if len(result.Activated) != 0 {
		t.Fatalf("an object SAP refused was counted as activated: %+v", result.Activated)
	}
	if len(result.Failed) != 1 {
		t.Fatalf("the refusal was not recorded as a failure: %+v", result.Failed)
	}
	if !strings.Contains(result.Failed[0].Reason, "Tables with headers") {
		t.Fatalf("the failure carries no reason: %q", result.Failed[0].Reason)
	}
}

// A table that did not activate is not a table anything can read, and
// CreateTable returned nil — the caller's next SELECT is where they found out.
func TestCreateTableReportsARefusedActivation(t *testing.T) {
	srv := &refusalServer{}
	client := srv.start(t)

	err := client.CreateTable(context.Background(), CreateTableOptions{
		Name:        "ZDEMO_REFUSED",
		Description: "demo",
		Package:     "$TMP",
		Fields: []TableField{
			{Name: "MANDT", Type: "CLNT", Length: 3, IsKey: true, NotNull: true},
		},
	})
	if err == nil {
		t.Fatal("a table that never activated was reported as created")
	}
	if !strings.Contains(err.Error(), "Tables with headers") {
		t.Fatalf("SAP's reason was not passed on: %v", err)
	}
}
