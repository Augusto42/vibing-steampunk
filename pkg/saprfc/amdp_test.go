package saprfc

import (
	"strings"
	"testing"
)

// The id of an AMDP session arrives in a header, not in the body. The body
// carries the HANA session id, which is a different field, and mistaking one
// for the other cost an afternoon.
func TestMainIDComesFromTheLocationHeader(t *testing.T) {
	if got := mainIDFromLocation("/sap/bc/adt/amdp/debugger/main/0242AC11"); got != "0242AC11" {
		t.Fatalf("main id is %q", got)
	}
	if got := mainIDFromLocation(""); got != "" {
		t.Fatalf("no header, no id; got %q", got)
	}
	if got := mainIDFromLocation("/sap/bc/adt/amdp/debugger"); got != "" {
		t.Fatalf("a location that names no session yields nothing; got %q", got)
	}
}

func TestStartParametersCarryTheHANABinding(t *testing.T) {
	body := []byte(`<?xml version="1.0" encoding="utf-8"?>` +
		`<amdpdbg:startParameters xmlns:amdpdbg="http://www.sap.com/adt/amdp/debugger">` +
		`<amdpdbg:parameter amdpdbg:key="HANA_SESSION_ID" amdpdbg:value="host:30203:300215"/>` +
		`</amdpdbg:startParameters>`)
	if got := amdpStartParameter(body, "HANA_SESSION_ID"); got != "host:30203:300215" {
		t.Fatalf("HANA session is %q", got)
	}
	if got := amdpStartParameter(body, "NOT_THERE"); got != "" {
		t.Fatalf("an absent parameter is empty, got %q", got)
	}
}

// The position is an ordinary adtcore reference, not anything AMDP-specific.
// Guessing an amdpdbg-namespaced attribute got nowhere; the transformation the
// resource class names says plainly which it is.
func TestBreakpointDocumentUsesAnAdtcoreReference(t *testing.T) {
	doc := string(amdpBreakpointDocument(AMDPSyncFull, []AMDPBreakpoint{{
		ClientID: "vsp-1",
		URI:      "/sap/bc/adt/oo/classes/zcl_demo/source/main#start=41",
		Name:     "ZCL_DEMO",
		Type:     "CLAS/OC",
	}}))

	for _, want := range []string{
		`amdpdbg:syncMode="FULL"`,
		`amdpdbg:clientId="vsp-1"`,
		`adtcore:uri="/sap/bc/adt/oo/classes/zcl_demo/source/main#start=41"`,
		`xmlns:adtcore=`,
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("document is missing %s:\n%s", want, doc)
		}
	}
}

// Object names reach this from a dump, a stack or an argument.
func TestBreakpointDocumentEscapesAttributes(t *testing.T) {
	doc := string(amdpBreakpointDocument(AMDPSyncFull, []AMDPBreakpoint{{
		ClientID: `a"b&c`,
		Name:     `<x>`,
	}}))
	if strings.Contains(doc, `a"b&c`) || strings.Contains(doc, "<x>") {
		t.Fatalf("attributes were not escaped:\n%s", doc)
	}
	if !strings.Contains(doc, "&quot;") || !strings.Contains(doc, "&amp;") {
		t.Fatalf("expected escapes, got:\n%s", doc)
	}
}

const amdpAckDocument = `<amdpdbg:mainResponseList xmlns:amdpdbg="http://www.sap.com/adt/amdp/debugger">` +
	`<amdpdbg:mainResponse amdpdbg:kind="ON_TOGGLE_BREAKPOINTS" amdpdbg:debuggeeId="">` +
	`<amdpdbg:value><amdpdbg:onToggleBreakpoints><amdpdbg:breakpoints>` +
	`<amdpdbg:breakpoint amdpdbg:clientId="vsp-1" amdpdbg:state="VALID" amdpdbg:errorMessage=""/>` +
	`</amdpdbg:breakpoints></amdpdbg:onToggleBreakpoints></amdpdbg:value>` +
	`</amdpdbg:mainResponse></amdpdbg:mainResponseList>`

const amdpBreakDocument = `<amdpdbg:mainResponseList xmlns:amdpdbg="http://www.sap.com/adt/amdp/debugger">` +
	`<amdpdbg:mainResponse amdpdbg:kind="ON_BREAK" amdpdbg:debuggeeId="host:30203:300215">` +
	`<amdpdbg:value><amdpdbg:abapPosition amdpdbg:procedureName="ZCL_DEMO=&gt;CALCULATE"/>` +
	`</amdpdbg:value></amdpdbg:mainResponse></amdpdbg:mainResponseList>`

// The trap this API sets, and the reason a breakpoint that works can look like
// one that does not: the answers arrive as a queue, and acknowledgements sit at
// its head. A client that resumes once and sees SYNC_BREAKPOINTS or
// ON_TOGGLE_BREAKPOINTS concludes nothing fired — while the debuggee is, at
// that moment, blocked on the breakpoint.
func TestAcknowledgementsAreNotStops(t *testing.T) {
	kind, debuggee := AMDPEventKindOf([]byte(amdpAckDocument))
	if kind != AMDPEventToggleBreakpoints {
		t.Fatalf("kind is %q", kind)
	}
	if debuggee != "" {
		t.Fatalf("an acknowledgement concerns no debuggee, got %q", debuggee)
	}
	if !amdpAcknowledgements[kind] {
		t.Fatal("this kind must be waited past, not returned as a stop")
	}
}

func TestABreakIsAStop(t *testing.T) {
	kind, debuggee := AMDPEventKindOf([]byte(amdpBreakDocument))
	if kind != "ON_BREAK" {
		t.Fatalf("kind is %q", kind)
	}
	if debuggee == "" {
		t.Fatal("a stop names the debuggee it stopped")
	}
	if amdpAcknowledgements[kind] {
		t.Fatal("a break must not be skipped past")
	}
}

// SAP says whether it understood the position, and it says it on the way to the
// stop — so it has to be kept rather than skipped past unseen.
func TestBreakpointStateIsReadFromTheAcknowledgement(t *testing.T) {
	state, reason := amdpBreakpointState([]byte(amdpAckDocument))
	if state != "VALID" {
		t.Fatalf("state is %q", state)
	}
	if reason != "" {
		t.Fatalf("a valid breakpoint carries no reason, got %q", reason)
	}
}

func TestUnparseableAnswerIsNotAStop(t *testing.T) {
	kind, debuggee := AMDPEventKindOf([]byte("not xml at all"))
	if kind != "" || debuggee != "" {
		t.Fatalf("got %q/%q", kind, debuggee)
	}
}
