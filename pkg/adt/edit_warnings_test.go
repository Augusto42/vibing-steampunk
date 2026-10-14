package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// warningSyntaxServer answers an edit flow whose syntax check reports one
// warning (severity "W") and no errors.
func warningSyntaxServer(t *testing.T, severity string) *httptest.Server {
	t.Helper()
	const source = "REPORT zdemo_warn.\nDATA lv_unused TYPE i.\nWRITE / 'x'.\n"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-csrf-token", "TOKEN")
		switch {
		case strings.Contains(r.URL.Path, "checkruns"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
			<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun">
			  <chkrun:checkReport chkrun:reporter="abapCheckRun">
			    <chkrun:checkMessageList>
			      <chkrun:checkMessage
			        chkrun:uri="/sap/bc/adt/programs/programs/ZDEMO_WARN/source/main#start=2,0"
			        chkrun:type="` + severity + `"
			        chkrun:shortText="Variable LV_UNUSED is never used"/>
			    </chkrun:checkMessageList>
			  </chkrun:checkReport>
			</chkrun:checkRunReports>`))
		case r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?><asx:abap xmlns:asx="http://www.sap.com/abapxml">
			  <asx:values><DATA><LOCK_HANDLE>HANDLE-1</LOCK_HANDLE>
			  <MODIFICATION_SUPPORT>Modification</MODIFICATION_SUPPORT></DATA></asx:values></asx:abap>`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/source/main"):
			_, _ = w.Write([]byte(source))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
}

// TestEditSource_WarningsDoNotBlock pins the decision behind #131: a syntax
// warning is reported, not enforced. It used to refuse the write unless the
// caller passed ignore_warnings on that individual call, which made vsp
// stricter than Eclipse ADT and cost a write to say something the result
// already carried.
func TestEditSource_WarningsDoNotBlock(t *testing.T) {
	srv := warningSyntaxServer(t, "W")
	defer srv.Close()

	c := NewClient(srv.URL, "TESTUSER", "pw")
	res, err := c.EditSourceWithOptions(context.Background(),
		"/sap/bc/adt/programs/programs/ZDEMO_WARN",
		"WRITE / 'x'.", "WRITE / 'y'.",
		&EditSourceOptions{SyntaxCheck: true}) // note: no IgnoreWarnings
	if err != nil {
		t.Fatalf("EditSourceWithOptions: %v", err)
	}

	if !res.Success {
		t.Fatalf("a warning must not stop the write; got Success=false, message=%q", res.Message)
	}
	if len(res.SyntaxWarnings) != 1 {
		t.Fatalf("the warning must still be reported; SyntaxWarnings=%v", res.SyntaxWarnings)
	}
	if len(res.SyntaxErrors) != 0 {
		t.Fatalf("a warning must not be reported as an error; SyntaxErrors=%v", res.SyntaxErrors)
	}
	if !strings.Contains(res.Message, "warning") {
		t.Fatalf("the message must name the warnings, or the only trace is a field nobody reads; got %q", res.Message)
	}
}

// TestEditSource_ErrorsStillBlock is the other half of the same decision, and
// the more important one: dropping the warning gate must not weaken the error
// gate. An error means the object would not compile.
func TestEditSource_ErrorsStillBlock(t *testing.T) {
	srv := warningSyntaxServer(t, "E")
	defer srv.Close()

	c := NewClient(srv.URL, "TESTUSER", "pw")
	res, err := c.EditSourceWithOptions(context.Background(),
		"/sap/bc/adt/programs/programs/ZDEMO_WARN",
		"WRITE / 'x'.", "WRITE / 'y'.",
		&EditSourceOptions{SyntaxCheck: true})
	if err != nil {
		t.Fatalf("EditSourceWithOptions: %v", err)
	}
	if res.Success {
		t.Fatal("a syntax error must still stop the write")
	}
	if len(res.SyntaxErrors) != 1 {
		t.Fatalf("the error must be reported; SyntaxErrors=%v", res.SyntaxErrors)
	}
	if !strings.Contains(res.Message, "NOT saved") {
		t.Fatalf("the refusal must say the change was not saved; got %q", res.Message)
	}
}
