package saprfc

import (
	"context"
	"strings"
	"testing"
)

// The second cassette covers what the first did not: reading a structure and a
// table apart, writing a variable, and moving between frames. Recorded from a
// live 7.58 session over plain ADT, same as the first.
//
// These follow the recorded order deliberately. The debugger asks the same URI
// many times over a session and gets a different answer each time — that is
// what a session is — so a test that wanders off the path is asking a question
// the recording never asked.
func attachToInspection(t *testing.T) (*Debugger, context.Context) {
	t.Helper()
	rt := loadFixture(t, "a4h-inspect.jsonl")
	dbg := NewADTDebugger(rt, fixtureUser)
	ctx := context.Background()

	who, err := dbg.ADTListen(ctx, fixtureUser, IDEID, TerminalID, 120)
	if err != nil || who == nil {
		t.Fatalf("listening: %v", err)
	}
	if _, err := dbg.ADTAttach(ctx, who.ID, fixtureUser); err != nil {
		t.Fatalf("attaching: %v", err)
	}
	if _, err := dbg.StackInfo(ctx); err != nil {
		t.Fatalf("reading the stack: %v", err)
	}
	if _, err := dbg.Locals(ctx); err != nil {
		t.Fatalf("reading locals: %v", err)
	}
	return dbg, ctx
}

// A structure expands into its components, each with its own value.
func TestReplayExpandsAStructure(t *testing.T) {
	dbg, ctx := attachToInspection(t)

	info, err := dbg.Expand(ctx, "LS_ROW")
	if err != nil {
		t.Fatalf("expanding the work area: %v", err)
	}
	byName := map[string]string{}
	for _, v := range info.Variables {
		byName[strings.ToUpper(v.Name)] = strings.TrimSpace(v.Value)
	}
	if byName["NAME"] != "second" {
		t.Fatalf("the work area holds the row appended last; NAME should be \"second\", got %q", byName["NAME"])
	}
	if byName["ID"] != "2" {
		t.Fatalf("ID should be 2, got %q", byName["ID"])
	}
}

// An internal table does not expand by its own name — SAP answers with an empty
// body rather than an error, so "expand this table" used to read as "this table
// is empty", even for one it had just described as holding two rows. Its rows
// are addressable one subscript at a time, and that is what expanding a table
// now does.
func TestReplayExpandsAnInternalTableIntoItsRows(t *testing.T) {
	dbg, ctx := attachToInspection(t)

	info, err := dbg.Expand(ctx, "LT_ROWS")
	if err != nil {
		t.Fatalf("expanding the table: %v", err)
	}
	if len(info.Variables) == 0 {
		t.Fatal("the table held two rows at the stop; expanding it returned nothing")
	}

	var values []string
	for _, v := range info.Variables {
		if strings.EqualFold(v.Name, "NAME") {
			values = append(values, strings.TrimSpace(v.Value))
		}
	}
	if len(values) != 2 {
		t.Fatalf("both rows should come back, got %d: %v", len(values), namesOf(info.Variables))
	}
	if values[0] != "first" || values[1] != "second" {
		t.Fatalf("rows should arrive in table order, got %v", values)
	}
}

// Writing a variable is the difference between reading a program and debugging
// one. The recorded session set LV_DOUBLED to 99 and read it back.
func TestReplayWritesAVariableAndReadsItBack(t *testing.T) {
	dbg, ctx := attachToInspection(t)

	// Follow the recording: the structure and the table were read first.
	if _, err := dbg.Expand(ctx, "LS_ROW"); err != nil {
		t.Fatalf("expanding the work area: %v", err)
	}
	if _, err := dbg.Expand(ctx, "LT_ROWS"); err != nil {
		t.Fatalf("expanding the table: %v", err)
	}

	if err := dbg.SetVariable(ctx, "LV_DOUBLED", "99"); err != nil {
		t.Fatalf("setting the variable: %v", err)
	}

	locals, err := dbg.Locals(ctx)
	if err != nil {
		t.Fatalf("reading locals back: %v", err)
	}
	for _, v := range locals {
		if strings.EqualFold(v.Name, "LV_DOUBLED") {
			if strings.TrimSpace(v.Value) != "99" {
				t.Fatalf("LV_DOUBLED was set to 99; the system reports %q", v.Value)
			}
			return
		}
	}
	t.Fatalf("LV_DOUBLED should still be in scope, got %v", namesOf(locals))
}

// Moving between frames by number rather than by URI. Frame 1 is the innermost;
// inside a PERFORM that is the subroutine, and only there are its parameters in
// scope. Getting this backwards would read the wrong frame's variables and look
// entirely plausible, which is why the assertion is on the parameters.
func TestReplayMovesBetweenFramesByNumber(t *testing.T) {
	dbg, ctx := attachToInspection(t)

	if _, err := dbg.Expand(ctx, "LS_ROW"); err != nil {
		t.Fatalf("expanding the work area: %v", err)
	}
	if _, err := dbg.Expand(ctx, "LT_ROWS"); err != nil {
		t.Fatalf("expanding the table: %v", err)
	}
	if err := dbg.SetVariable(ctx, "LV_DOUBLED", "99"); err != nil {
		t.Fatalf("setting the variable: %v", err)
	}
	if _, err := dbg.Locals(ctx); err != nil {
		t.Fatalf("reading locals: %v", err)
	}
	if _, err := dbg.ADTStep(ctx, "stepInto"); err != nil {
		t.Fatalf("stepping into the subroutine: %v", err)
	}
	if _, err := dbg.StackInfo(ctx); err != nil {
		t.Fatalf("reading the stack: %v", err)
	}

	// The caller: the program's own variables, and no subroutine parameters.
	if err := dbg.GoToFrameAt(ctx, 2); err != nil {
		t.Fatalf("moving to the calling frame: %v", err)
	}
	caller, err := dbg.Locals(ctx)
	if err != nil {
		t.Fatalf("reading the caller's variables: %v", err)
	}
	if contains(namesOf(caller), "IV_IN") {
		t.Fatalf("the subroutine's parameters are not in scope in its caller, got %v", namesOf(caller))
	}
	if !contains(namesOf(caller), "LV_COUNTER") {
		t.Fatalf("the program's own variables should be in scope, got %v", namesOf(caller))
	}

	// Back to the subroutine, where they are.
	if err := dbg.GoToFrameAt(ctx, 1); err != nil {
		t.Fatalf("moving back to the subroutine: %v", err)
	}
	inner, err := dbg.Locals(ctx)
	if err != nil {
		t.Fatalf("reading the subroutine's variables: %v", err)
	}
	for _, want := range []string{"IV_IN", "CV_OUT"} {
		if !contains(namesOf(inner), want) {
			t.Fatalf("%s should be in scope inside the subroutine, got %v", want, namesOf(inner))
		}
	}
}

// Asking for a frame that is not there is a mistake worth naming, not a request
// worth sending.
func TestFrameNumberIsCheckedAgainstTheStack(t *testing.T) {
	dbg, ctx := attachToInspection(t)

	err := dbg.GoToFrameAt(ctx, 99)
	if err == nil {
		t.Fatal("there is no frame 99; moving there should fail")
	}
	if !strings.Contains(err.Error(), "no frame 99") {
		t.Fatalf("the failure should say which frame was asked for, got: %v", err)
	}
}

// A table larger than one expansion can afford. Everything here rests on an
// assumption worth checking against a real system rather than a mock: that SAP
// answers for an arbitrary subscript, not just the first few. The recorded
// system was asked for rows 1-33, 109-141 and 218-250 of a 250-row table.
func TestReplaySamplesALargeTableAcrossItsWholeLength(t *testing.T) {
	rt := loadFixture(t, "a4h-bigtable.jsonl")
	dbg := NewADTDebugger(rt, fixtureUser)
	ctx := context.Background()

	who, err := dbg.ADTListen(ctx, fixtureUser, IDEID, TerminalID, 120)
	if err != nil || who == nil {
		t.Fatalf("listening: %v", err)
	}
	if _, err := dbg.ADTAttach(ctx, who.ID, fixtureUser); err != nil {
		t.Fatalf("attaching: %v", err)
	}
	if _, err := dbg.StackInfo(ctx); err != nil {
		t.Fatalf("reading the stack: %v", err)
	}

	info, err := dbg.Expand(ctx, "LT_ROWS")
	if err != nil {
		t.Fatalf("expanding the table: %v", err)
	}

	sample := dbg.LastTableSample()
	if !sample.Partial() {
		t.Fatal("a 250-row table cannot be read whole within the budget; the expansion should say it sampled")
	}
	if sample.Lines != 250 {
		t.Fatalf("the table held 250 rows, the sample reports %d", sample.Lines)
	}

	// The rows the caller is shown must span the table, not just its head.
	values := map[string]bool{}
	for _, v := range info.Variables {
		if strings.EqualFold(v.Name, "NAME") {
			values[strings.TrimSpace(v.Value)] = true
		}
	}
	for _, want := range []string{"row1", "row250"} {
		if !values[want] {
			t.Fatalf("%s should be in the sample; the head and the end are both the point", want)
		}
	}
	middle := false
	for value := range values {
		if strings.HasPrefix(value, "row1") && len(value) == 5 && value != "row1" {
			middle = true // row1NN, from the middle window
		}
	}
	if !middle {
		t.Fatalf("nothing from the middle of the table came back: %d distinct values", len(values))
	}

	// And the note a reader is shown.
	if got := FormatRowRanges(sample.Rows); !strings.Contains(got, "-") || !strings.Contains(got, ",") {
		t.Fatalf("the sample should read as separate ranges, got %q", got)
	}
}
