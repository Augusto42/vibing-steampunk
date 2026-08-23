package saprfc

import (
	"context"
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

const amdpBreakWithPosition = `<amdpdbg:mainResponseList xmlns:amdpdbg="http://www.sap.com/adt/amdp/debugger">` +
	`<amdpdbg:mainResponse amdpdbg:kind="ON_BREAK" amdpdbg:debuggeeId="host:30203:300215">` +
	`<amdpdbg:value><amdpdbg:abapPosition amdpdbg:procedureName="ZCL_DEMO=&gt;CALCULATE" ` +
	`adtcore:uri="/sap/bc/adt/oo/classes/zcl_demo/source/main#start=42"/>` +
	`</amdpdbg:value></amdpdbg:mainResponse></amdpdbg:mainResponseList>`

// The line rides in the URI fragment, the way every ADT position does, rather
// than in an attribute of its own.
func TestStopPositionReadsTheLineOutOfTheFragment(t *testing.T) {
	pos := AMDPStopPosition([]byte(amdpBreakWithPosition))
	if pos == nil {
		t.Fatal("a break carries a position")
	}
	if pos.Line != 42 {
		t.Fatalf("line is %d", pos.Line)
	}
	if pos.Procedure != "ZCL_DEMO=>CALCULATE" {
		t.Fatalf("procedure is %q", pos.Procedure)
	}
	if pos.DebuggeeID == "" {
		t.Fatal("the stop names the debuggee, and every resource below the session needs it")
	}
}

// An acknowledgement is not a stop and carries no position, so nothing should
// be invented for it.
func TestAnAcknowledgementHasNoPosition(t *testing.T) {
	if pos := AMDPStopPosition([]byte(amdpAckDocument)); pos != nil {
		t.Fatalf("an acknowledgement has nowhere to be, got %+v", pos)
	}
}

func TestStopPositionSurvivesRubbish(t *testing.T) {
	if pos := AMDPStopPosition([]byte("not xml")); pos != nil {
		t.Fatalf("got %+v", pos)
	}
	// A position with no fragment is still a position; the line is simply
	// unknown, and reporting zero is better than discarding the procedure.
	body := []byte(`<amdpdbg:mainResponseList xmlns:amdpdbg="http://www.sap.com/adt/amdp/debugger">` +
		`<amdpdbg:mainResponse amdpdbg:kind="ON_BREAK" amdpdbg:debuggeeId="d">` +
		`<amdpdbg:value><amdpdbg:abapPosition amdpdbg:procedureName="ZCL_DEMO=&gt;X"/>` +
		`</amdpdbg:value></amdpdbg:mainResponse></amdpdbg:mainResponseList>`)
	pos := AMDPStopPosition(body)
	if pos == nil || pos.Procedure != "ZCL_DEMO=>X" {
		t.Fatalf("the procedure should survive a missing fragment, got %+v", pos)
	}
	if pos.Line != 0 {
		t.Fatalf("an unknown line is zero, not a guess; got %d", pos.Line)
	}
}

// Only two kinds exist. SQLScript has no "into" because there is nothing below
// the statement to step into, and a caller that asks for one should be told so
// rather than have it silently become something else.
func TestOnlyTwoStepKindsExist(t *testing.T) {
	dbg := NewADTDebugger(&scriptedTransport{}, "TESTUSER")
	dbg.amdpMain = "main-1"
	_, err := dbg.AMDPStep(context.Background(), "d", "into")
	if err == nil {
		t.Fatal("stepInto is not an AMDP step; it must be refused")
	}
	if !strings.Contains(err.Error(), "over or continue") {
		t.Fatalf("the refusal should name what is allowed, got: %v", err)
	}
}

// Every AMDP resource below the session is addressed by debuggee, and the id
// arrives only with a stop. Stepping before anything stopped is a mistake worth
// naming rather than a request worth sending.
func TestSteppingBeforeAnythingStoppedIsRefused(t *testing.T) {
	dbg := NewADTDebugger(&scriptedTransport{}, "TESTUSER")
	dbg.amdpMain = "main-1"
	if _, err := dbg.AMDPStep(context.Background(), "", "over"); err == nil {
		t.Fatal("there is no debuggee yet; stepping should fail")
	}
	if _, err := dbg.AMDPVariable(context.Background(), "", "LV_I"); err == nil {
		t.Fatal("there is no debuggee yet; reading a variable should fail")
	}
}

func TestAMDPCallsNeedASession(t *testing.T) {
	dbg := NewADTDebugger(&scriptedTransport{}, "TESTUSER")
	for _, call := range []func() error{
		func() error { _, err := dbg.AMDPStep(context.Background(), "d", "over"); return err },
		func() error { _, err := dbg.AMDPVariable(context.Background(), "d", "X"); return err },
		func() error { _, err := dbg.AMDPResume(context.Background()); return err },
	} {
		if err := call(); err == nil || !strings.Contains(err.Error(), "start one first") {
			t.Fatalf("without a session the call should say so, got: %v", err)
		}
	}
}

const amdpScalarAnswer = `<amdpdbg:mainResponseList xmlns:amdpdbg="http://www.sap.com/adt/amdp/debugger">` +
	`<amdpdbg:mainResponse amdpdbg:kind="GET_SCALAR_VALUES" amdpdbg:requestId="REQ-1" amdpdbg:debuggeeId="d">` +
	`<amdpdbg:value><amdpdbg:scalarValues>` +
	`<amdpdbg:scalarValue amdpdbg:name="LV_I" amdpdbg:type="INTEGER" amdpdbg:isNullValue="false" ` +
	`amdpdbg:offset="0" amdpdbg:length="1" amdpdbg:originalLength="1">1</amdpdbg:scalarValue>` +
	`</amdpdbg:scalarValues></amdpdbg:value></amdpdbg:mainResponse></amdpdbg:mainResponseList>`

// Reading a variable is asynchronous and does not say so: the resource answers
// with an empty body and puts a request id in Location, so a caller reading the
// body concludes the variable has no value or does not exist. The answer
// arrives later through the same queue, tagged with that id.
func TestAnAnswerIsMatchedToItsRequest(t *testing.T) {
	if !amdpAnswers([]byte(amdpScalarAnswer), "REQ-1") {
		t.Fatal("the answer carries REQ-1 and should be recognised")
	}
	if amdpAnswers([]byte(amdpScalarAnswer), "REQ-2") {
		t.Fatal("a caller waiting for one request must not be handed another's answer")
	}
	if amdpAnswers([]byte(amdpBreakDocument), "REQ-1") {
		t.Fatal("a stop is not an answer to a variable read")
	}
	if amdpAnswers([]byte("not xml"), "REQ-1") {
		t.Fatal("rubbish answers nothing")
	}
}

func TestScalarValuesAreRead(t *testing.T) {
	values := AMDPScalarValues([]byte(amdpScalarAnswer))
	if len(values) != 1 {
		t.Fatalf("expected one variable, got %d", len(values))
	}
	v := values[0]
	if v.Name != "LV_I" || v.Type != "INTEGER" || v.Value != "1" {
		t.Fatalf("read as %+v", v)
	}
	if v.IsNull {
		t.Fatal("isNullValue was false")
	}
	if v.Truncated() {
		t.Fatal("length equals originalLength; nothing was cut")
	}
}

// A value cut at the window boundary is indistinguishable from a short one
// unless the difference is stated.
func TestATruncatedValueSaysSo(t *testing.T) {
	long := AMDPScalar{Name: "LV_TEXT", Type: "NVARCHAR", Value: "abc", Length: 3, OriginalLength: 900}
	if !long.Truncated() {
		t.Fatal("900 characters exist and 3 came back")
	}
	if got := FormatAMDPScalar(long); !strings.Contains(got, "3 of 900") {
		t.Fatalf("the reader must be told what was cut, got %q", got)
	}
	short := AMDPScalar{Name: "LV_I", Type: "INTEGER", Value: "1", Length: 1, OriginalLength: 1}
	if got := FormatAMDPScalar(short); strings.Contains(got, "of") {
		t.Fatalf("a complete value carries no note, got %q", got)
	}
}

// NULL is not the empty string, and rendering it as one would lose the
// difference between a variable that holds nothing and one that holds "".
func TestNullIsNotEmpty(t *testing.T) {
	null := AMDPScalar{Name: "LV_X", Type: "NVARCHAR", IsNull: true}
	if got := FormatAMDPScalar(null); !strings.Contains(got, "NULL") {
		t.Fatalf("got %q", got)
	}
}
