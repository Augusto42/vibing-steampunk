package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A deploy whose object does not activate reported "Successfully created and
// activated" and exited zero, for the same reason `vsp execute` reported a
// syntax error as a success: activation refuses inside a 200, the refusal is in
// the checklist body, and the result of Activate was thrown away unread.
//
// The syntax check earlier in the flow catches most of this, but it checks the
// source it was handed and activation is the step with the last word — and an
// update that leaves the object inactive has already overwritten source that
// worked. So the server below passes the check and refuses the activation,
// which is precisely the disagreement the old code could not see.
func TestDeployReportsAnActivationThatWasRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/discovery"):
			w.Header().Set("X-CSRF-Token", "TOKEN")
			w.WriteHeader(http.StatusOK)

		case strings.Contains(path, "/checkruns"):
			w.Header().Set("Content-Type", "application/vnd.sap.adt.checkmessages+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist"/>`))

		case strings.Contains(path, "/activation"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(refusedActivationXML("ZDEPLOY_REFUSED")))

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
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, "zdeploy_refused.prog.abap")
	if err := os.WriteFile(file, []byte("REPORT zdeploy_refused.\nWRITE 'hello'.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := NewConfig(srv.URL, "TESTUSER", "secret")
	client := NewClientWithTransport(cfg, NewTransport(cfg))

	result, err := client.DeployFromFile(context.Background(), file, "$TMP", "")
	if err != nil {
		t.Fatalf("DeployFromFile: %v", err)
	}
	if result.Success {
		t.Fatal("an object SAP refused to activate was reported as deployed")
	}
	if len(result.SyntaxErrors) == 0 {
		t.Fatal("the deploy failed without passing on a word of SAP's reason")
	}
	if !strings.Contains(strings.Join(result.SyntaxErrors, " "), "Tables with headers") {
		t.Fatalf("SAP's message did not survive: %v", result.SyntaxErrors)
	}
}
