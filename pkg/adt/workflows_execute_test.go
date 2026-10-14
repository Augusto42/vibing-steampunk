package adt

import (
	"strings"
	"testing"
)

// What SAP actually sends when the executed code divides by zero, taken from a
// live run and with the generated program name kept: it is a timestamp, so it
// names nothing and nobody. The point of keeping it verbatim is that every
// field the failure report reads is a field SAP really sent.
var zeroDivideAlert = UnitTestAlert{
	Kind:     "exception",
	Severity: "critical",
	Title:    "Exception Error <COMPUTE_INT_ZERODIVIDE>",
	Details: []string{
		"Division by zero",
		"Test 'LTC_EXECUTOR->EXECUTE_PAYLOAD' in Main Program 'ZTEMP_EXEC_43321686'",
	},
	Stack: []UnitTestStackEntry{{
		URI:         "/sap/bc/adt/programs/programs/ztemp_exec_43321686/source/main#start=18,0",
		Type:        "PROG/P",
		Name:        "ZTEMP_EXEC_43321686",
		Description: "Include: <ZTEMP_EXEC_43321686> Line: <18>",
	}},
}

// The bug this exists to keep fixed: the run above reported "Executed
// successfully (no output captured)" while the evidence sat in the response
// untouched.
func TestPayloadFailureFindsAnUncaughtException(t *testing.T) {
	alert := PayloadFailure([]UnitTestAlert{zeroDivideAlert})
	if alert == nil {
		t.Fatal("a zero divide is a failure, and it was called a success")
	}
	if !strings.Contains(alert.Title, "COMPUTE_INT_ZERODIVIDE") {
		t.Fatalf("wrong alert: %q", alert.Title)
	}
}

// Every run that reaches the end fails on purpose — that assertion is how a
// value leaves a test method — so the marker, not the failure, is what tells
// success from failure here.
func TestPayloadFailureIgnoresTheDeliberateAssertion(t *testing.T) {
	alerts := []UnitTestAlert{{
		Kind:     "failedAssertion",
		Severity: "critical",
		Title:    execResultMarker + "42",
	}}
	if alert := PayloadFailure(alerts); alert != nil {
		t.Fatalf("the closing assertion was read as a failure: %q", alert.Title)
	}
}

// The value can arrive in the details instead of the title, and it is the same
// deliberate assertion either way.
func TestPayloadFailureIgnoresAMarkerInTheDetails(t *testing.T) {
	alerts := []UnitTestAlert{{
		Kind:    "failedAssertion",
		Title:   "Assertion failed",
		Details: []string{execResultMarker + "hello"},
	}}
	if alert := PayloadFailure(alerts); alert != nil {
		t.Fatalf("the closing assertion was read as a failure: %q", alert.Title)
	}
}

// Warnings are things ABAP Unit noticed, not things that ended the run.
func TestPayloadFailureLeavesWarningsAlone(t *testing.T) {
	alerts := []UnitTestAlert{{Kind: "warning", Severity: "tolerable", Title: "Test takes a long time"}}
	if alert := PayloadFailure(alerts); alert != nil {
		t.Fatalf("a warning was treated as a failure: %q", alert.Title)
	}
}

// An exception outranks anything else, wherever it sits in the list: the
// closing assertion may be reported first and still not be the reason the run
// ended.
func TestPayloadFailurePrefersTheException(t *testing.T) {
	alerts := []UnitTestAlert{
		{Kind: "failedAssertion", Severity: "critical", Title: "Assertion failed", Details: []string{execResultMarker}},
		{Kind: "warning", Severity: "tolerable", Title: "noise"},
		zeroDivideAlert,
	}
	alert := PayloadFailure(alerts)
	if alert == nil || !strings.Contains(alert.Title, "COMPUTE_INT_ZERODIVIDE") {
		t.Fatalf("the exception should have won, got %+v", alert)
	}
}

// The caller wrote a few lines; SAP compiled a report with a header, a class
// definition and a method around them, and reports the line of that. Reporting
// SAP's number back would send whoever wrote the code looking at a line they
// never wrote and cannot see.
func TestPayloadLineIsCountedInTheCallersCode(t *testing.T) {
	source := executeWrapperSource("ZTEMP_EXEC_43321686", "RISK LEVEL HARMLESS", "lv_result",
		"DATA(lv_a) = 1. DATA(lv_b) = 0. DATA(lv_c) = lv_a / lv_b.")
	line := payloadLine(zeroDivideAlert, "ZTEMP_EXEC_43321686", payloadOffset(source))
	if line != 1 {
		t.Fatalf("the failure was on the first line the caller wrote, reported as %d", line)
	}
}

// The offset is derived from the generated source rather than counted by hand,
// so that editing the wrapper cannot quietly start reporting line numbers that
// are off by three. This checks the derivation against the source itself.
func TestPayloadOffsetPointsAtTheFirstLineOfThePayload(t *testing.T) {
	source := executeWrapperSource("ZTEMP_EXEC_00000000", "RISK LEVEL HARMLESS", "lv_result", "WRITE 'sentinel'.")
	offset := payloadOffset(source)
	lines := strings.Split(source, "\n")
	if offset <= 0 || offset > len(lines) {
		t.Fatalf("offset %d is outside a source of %d lines", offset, len(lines))
	}
	if !strings.Contains(lines[offset-1], "sentinel") {
		t.Fatalf("line %d of the wrapper is %q, which is not where the payload starts", offset, lines[offset-1])
	}
}

// A failure raised deep inside SAP standard code has frames belonging to other
// programs, and none of their line numbers mean anything to the caller.
func TestPayloadLineRefusesToGuessFromAnotherProgram(t *testing.T) {
	alert := UnitTestAlert{
		Kind: "exception",
		Stack: []UnitTestStackEntry{{
			URI:  "/sap/bc/adt/programs/includes/lsbal_dbu01/source/main#start=214,0",
			Name: "SAPLSBAL_DB",
		}},
	}
	if line := payloadLine(alert, "ZTEMP_EXEC_00000000", 18); line != 0 {
		t.Fatalf("a line in somebody else's program was reported as ours: %d", line)
	}
}

// The closing assertion is generated from the same constant the parser strips,
// so a rename cannot break the round trip in silence.
func TestWrapperCarriesTheMarkerTheParserLooksFor(t *testing.T) {
	source := executeWrapperSource("ZTEMP_EXEC_00000000", "RISK LEVEL HARMLESS", "lv_result", "")
	if !strings.Contains(source, execResultMarker) {
		t.Fatal("the wrapper no longer emits the marker the result parser strips")
	}
	if !strings.Contains(source, payloadStartMarker) {
		t.Fatal("the wrapper no longer marks where the payload starts")
	}
}

// The value comes back inside a sentence of SAP's own making, quoted: the
// message |EXEC_RESULT:42| is reported as "Critical Assertion Error:
// 'EXEC_RESULT:42'". Looking for the marker at the start of the title is why
// every successful execute used to say "no output captured".
func TestExecResultSurvivesSAPsWrapping(t *testing.T) {
	value, found := execResult("Critical Assertion Error: 'EXEC_RESULT:hello from A4H'")
	if !found {
		t.Fatal("the marker was not found inside SAP's sentence")
	}
	if value != "hello from A4H" {
		t.Fatalf("value came back as %q", value)
	}
}

// A message that was not wrapped keeps its last character, whatever it is.
func TestExecResultKeepsAnUnquotedValueWhole(t *testing.T) {
	value, found := execResult("EXEC_RESULT:it's")
	if !found || value != "it's" {
		t.Fatalf("value came back as %q (found=%v)", value, found)
	}
}

func TestExecResultIgnoresUnrelatedText(t *testing.T) {
	if _, found := execResult("Exception Error <COMPUTE_INT_ZERODIVIDE>"); found {
		t.Fatal("an unrelated alert was read as a returned value")
	}
}

// The wrapped form has to be recognised here too, or the deliberate closing
// assertion is reported as the code having died.
func TestPayloadFailureIgnoresTheWrappedMarker(t *testing.T) {
	alerts := []UnitTestAlert{{
		Kind:     "failedAssertion",
		Severity: "critical",
		Title:    "Critical Assertion Error: 'EXEC_RESULT:42'",
	}}
	if alert := PayloadFailure(alerts); alert != nil {
		t.Fatalf("a successful run was reported as a failure: %q", alert.Title)
	}
}
