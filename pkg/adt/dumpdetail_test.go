package adt

import "testing"

// Real ST22 layout, synthetic names. Three things here are deliberate and all
// three were taken from live dumps: the sentence about the termination point
// wraps between the line number and the include it belongs to, "occurred in
// ABAP program" has lost the space before "ABAP" where the wrapper broke it,
// and the header value is separated from its label by a column gap rather than
// by a single space.
const formattedDumpDetailSample = `
----------------------------------------------------------------------------------------------------
Category               ABAP programming error
Runtime Errors         RAISE_EXCEPTION
Except.                CX_DEMO_INVALID_ORDER
ABAP: Program          ZCL_DEMO_ORDER===============CP
Application Component  BC-SRV-BAL
Date and Time          22.08.2026 23:08:07 (UTC)
----------------------------------------------------------------------------------------------------

----------------------------------------------------------------------------------------------------
|Short Text                                                                                        |
|    Exception condition "INVALID_ORDER" raised.                                                   |
----------------------------------------------------------------------------------------------------

----------------------------------------------------------------------------------------------------
|Information on where terminated                                                                   |
|    The termination occurred inABAP program or include                                            |
|     "ZCL_DEMO_SPLIT===============CP", in "CONSTRUCTOR". The                                     |
|    main program was "RS_DEMO_TEST_INT".                                                          |
|                                                                                                  |
|    In the source code, the termination point is in line 94 of include                            |
|     "ZCL_DEMO_SPLIT===============CM007".                                                        |
|    include "ZCL_DEMO_SPLIT===============CM007".                                                 |
----------------------------------------------------------------------------------------------------

----------------------------------------------------------------------------------------------------
|Active Calls/Events                                                                               |
----------------------------------------------------------------------------------------------------
|No.   Ty.          Program                             Include                             Line   |
|      Name                                                                                        |
----------------------------------------------------------------------------------------------------
|    1 METHOD       ZCL_DEMO_SPLIT===============CP    ZCL_DEMO_SPLIT===============CM007    94  |
|      ZCL_DEMO_SPLIT=>CONSTRUCTOR                                                                 |
----------------------------------------------------------------------------------------------------
`

func TestDumpDetailReadsHeaderByLabel(t *testing.T) {
	detail := parseDumpDetail(formattedDumpDetailSample)

	if detail.ErrorType != "RAISE_EXCEPTION" {
		t.Fatalf("error type is %q", detail.ErrorType)
	}
	if detail.Exception != "CX_DEMO_INVALID_ORDER" {
		t.Fatalf("exception is %q", detail.Exception)
	}
	if detail.Component != "BC-SRV-BAL" {
		t.Fatalf("component is %q", detail.Component)
	}
	// The header program is the caller here, not the class that raised. Both
	// are in the dump and they are not interchangeable.
	if detail.Program != "ZCL_DEMO_ORDER===============CP" {
		t.Fatalf("header program is %q", detail.Program)
	}
	if len(detail.Stack) != 1 {
		t.Fatalf("the stack comes out of the same document, got %d frames", len(detail.Stack))
	}
}

// The sentence that carries the failing line wraps, and it wraps precisely
// between the number and the include name on any dump whose include is long —
// which is every class pool. A line-by-line match finds nothing there.
func TestDumpDetailReadsTerminationAcrossAWrap(t *testing.T) {
	detail := parseDumpDetail(formattedDumpDetailSample)

	if detail.Line != 94 {
		t.Fatalf("line is %d, so the wrapped sentence was not joined before matching", detail.Line)
	}
	if detail.Include != "ZCL_DEMO_SPLIT===============CM007" {
		t.Fatalf("include is %q", detail.Include)
	}
	if detail.MainProgram != "RS_DEMO_TEST_INT" {
		t.Fatalf("main program is %q", detail.MainProgram)
	}
}

// "occurred inABAP program" is how a live dump reads when SAP's wrapper eats
// the space. Requiring it loses the procedure name silently.
func TestDumpDetailSurvivesTheMissingSpaceBeforeABAP(t *testing.T) {
	detail := parseDumpDetail(formattedDumpDetailSample)
	if detail.Procedure != "CONSTRUCTOR" {
		t.Fatalf("procedure is %q", detail.Procedure)
	}
}

const formattedDumpNoPositionSample = `
----------------------------------------------------------------------------------------------------
Category               ABAP programming error
Runtime Errors         DYNPRO_SEND_IN_BACKGROUND
ABAP: Program          SAPMSSY0
Application Component  Not assigned
Date and Time          22.08.2026 23:57:03 (UTC)
----------------------------------------------------------------------------------------------------

----------------------------------------------------------------------------------------------------
|Information on where terminated                                                                   |
|    The termination occurred in ABAP program or include "SAPMSSY0", in                            |
|     "SYSTEM-EXIT". The                                                                           |
|    main program was "ZDEMO_BATCH_JOB".                                                           |
|                                                                                                  |
|    In the source code, the termination point is in line 0 of include " ".                        |
|    include " ".                                                                                  |
----------------------------------------------------------------------------------------------------
`

// A dump that died outside ABAP source says so as line 0 of include " ". If
// that were stored as a position, every such dump would look like it failed at
// the same place, and the strongest rung of the ladder would be reached by the
// dumps that carry the least information.
func TestDumpDetailRefusesAPositionThatDoesNotExist(t *testing.T) {
	detail := parseDumpDetail(formattedDumpNoPositionSample)

	if detail.Line != 0 || detail.Include != "" {
		t.Fatalf("line 0 of include \" \" is not a position, got %s:%d", detail.Include, detail.Line)
	}
	if detail.Procedure != "SYSTEM-EXIT" {
		t.Fatalf("the procedure is still readable, got %q", detail.Procedure)
	}
}

// "Not assigned" is the absence of a component. Treating it as a value would
// put every unassigned object in the system into one neighbourhood.
func TestDumpDetailTreatsNotAssignedAsNoComponent(t *testing.T) {
	detail := parseDumpDetail(formattedDumpNoPositionSample)
	if detail.Component != "" {
		t.Fatalf("component is %q, expected none", detail.Component)
	}
}

// A resource bottleneck — SYSTEM_NO_ROLL and its family — has a shorter header
// table with neither the program nor the component in it. Reading the table by
// label rather than by row number is what keeps this from shifting every value
// up by two.
const formattedDumpShortHeaderSample = `
----------------------------------------------------------------------------------------------------
Category               Resource bottleneck
Runtime Errors         SYSTEM_NO_ROLL
Date and Time          21.08.2026 00:55:45 (UTC)
----------------------------------------------------------------------------------------------------

----------------------------------------------------------------------------------------------------
|Information on where terminated                                                                   |
|    The termination occurred in ABAP program or include                                           |
|     "ZCL_DEMO_JSON===============CP", in "DUMP_SYMBOLS". The                                     |
|    main program was "SAPMSSY1".                                                                  |
|                                                                                                  |
|    In the source code, the termination point is in line 48 of include                            |
|     "ZCL_DEMO_JSON===============CM00K".                                                         |
|    include "ZCL_DEMO_JSON===============CM00K".                                                  |
----------------------------------------------------------------------------------------------------
`

func TestDumpDetailSurvivesAHeaderWithRowsMissing(t *testing.T) {
	detail := parseDumpDetail(formattedDumpShortHeaderSample)

	if detail.ErrorType != "SYSTEM_NO_ROLL" {
		t.Fatalf("error type is %q, so the rows were read by position rather than by label", detail.ErrorType)
	}
	if detail.Program != "" || detail.Component != "" {
		t.Fatalf("this header names neither, got program %q component %q", detail.Program, detail.Component)
	}
	if detail.Line != 48 {
		t.Fatalf("the termination point is still there, got line %d", detail.Line)
	}
}

func TestDumpDetailSurvivesRubbish(t *testing.T) {
	detail := parseDumpDetail("this is not a dump")
	if detail.ErrorType != "" || detail.Line != 0 || detail.Component != "" {
		t.Fatalf("expected an empty detail, got %+v", detail)
	}
}
