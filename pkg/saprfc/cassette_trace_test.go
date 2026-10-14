package saprfc

import (
	"context"
	"testing"
)

// The recorder is the thing worth having: a statement-level trace of a real
// program with the values at each step, assembled from SAP's own debugger over
// plain ADT. It is also the part that ran unexercised the longest, because
// checking it needs a stopped program.
//
// Replayed from a live 7.58 recording: a breakpoint on the first assignment,
// then six statements walked with values captured at each one.
func TestReplayRecordsAStatementLevelTrace(t *testing.T) {
	rt := loadFixture(t, "a4h-trace.jsonl")
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

	var trace []StopRecord
	n, err := dbg.Record(ctx, RecordOptions{MaxStops: 6}, func(r StopRecord) error {
		trace = append(trace, r)
		return nil
	})
	if err != nil {
		t.Fatalf("recording: %v", err)
	}
	if n != len(trace) {
		t.Fatalf("the recorder reported %d stops and emitted %d", n, len(trace))
	}
	if len(trace) < 6 {
		t.Fatalf("six statements were walked, replay produced %d", len(trace))
	}

	// The program's own line order, blank lines and declarations skipped. A
	// recorder that lost its place would still produce plausible-looking
	// records, so the sequence is the assertion.
	wantLines := []int{18, 19, 20, 22, 23, 24}
	for i, want := range wantLines {
		if trace[i].Line != want {
			t.Fatalf("stop %d should be on line %d, got %d", i, want, trace[i].Line)
		}
		if trace[i].Program != "ZVSP_DEBUG_DEMO" {
			t.Fatalf("stop %d wandered out of the program into %q", i, trace[i].Program)
		}
	}
	if trace[0].Kind != "enter" {
		t.Fatalf("the first stop is where the recording started; kind should be enter, got %q", trace[0].Kind)
	}
}

// Values are the whole point, and they were missing from every trace this
// produced: the batched capture asked for the children of @LOCALS, a root this
// release does not have, and an absent root answers empty rather than failing.
// Every recorded trace came out with lines and no values in them.
func TestReplayTraceCarriesValues(t *testing.T) {
	rt := loadFixture(t, "a4h-trace.jsonl")
	dbg := NewADTDebugger(rt, fixtureUser)
	ctx := context.Background()

	who, _ := dbg.ADTListen(ctx, fixtureUser, IDEID, TerminalID, 120)
	if who == nil {
		t.Fatal("listening caught nothing")
	}
	if _, err := dbg.ADTAttach(ctx, who.ID, fixtureUser); err != nil {
		t.Fatalf("attaching: %v", err)
	}
	if _, err := dbg.StackInfo(ctx); err != nil {
		t.Fatalf("reading the stack: %v", err)
	}

	var first StopRecord
	seen := false
	if _, err := dbg.Record(ctx, RecordOptions{MaxStops: 6}, func(r StopRecord) error {
		if !seen {
			first, seen = r, true
		}
		return nil
	}); err != nil {
		t.Fatalf("recording: %v", err)
	}

	if len(first.Vars) == 0 {
		t.Fatal("a trace with no values is a list of line numbers")
	}
	if got := first.Vars["LV_COUNTER"]; got != "7" {
		t.Fatalf("LV_COUNTER was 7 at that line, the trace says %q", got)
	}
	if got := first.Vars["LV_DOUBLED"]; got != "0" {
		t.Fatalf("LV_DOUBLED was not yet assigned; the trace says %q", got)
	}
	// Structures and tables are named rather than flattened, so a caller can
	// expand the ones it cares about instead of paying for all of them.
	if _, ok := first.Composite["LT_ROWS"]; !ok {
		t.Fatalf("the internal table should be listed as expandable, got %v", first.Composite)
	}
}

// Redaction is on by default because a trace of real code is business data by
// construction. It must hide values without hiding the shape of the run.
func TestTraceRedactionHidesValuesNotStructure(t *testing.T) {
	rt := loadFixture(t, "a4h-trace.jsonl")
	dbg := NewADTDebugger(rt, fixtureUser)
	ctx := context.Background()

	who, _ := dbg.ADTListen(ctx, fixtureUser, IDEID, TerminalID, 120)
	if who == nil {
		t.Fatal("listening caught nothing")
	}
	if _, err := dbg.ADTAttach(ctx, who.ID, fixtureUser); err != nil {
		t.Fatalf("attaching: %v", err)
	}
	if _, err := dbg.StackInfo(ctx); err != nil {
		t.Fatalf("reading the stack: %v", err)
	}

	var first StopRecord
	seen := false
	if _, err := dbg.Record(ctx, RecordOptions{MaxStops: 6, Redact: true}, func(r StopRecord) error {
		if !seen {
			first, seen = r, true
		}
		return nil
	}); err != nil {
		t.Fatalf("recording: %v", err)
	}

	if first.Line != 18 {
		t.Fatalf("redaction must not disturb the trace itself; line is %d", first.Line)
	}
	if got := first.Vars["LV_COUNTER"]; got == "7" {
		t.Fatal("redaction was asked for and the value came through anyway")
	}
	if _, named := first.Vars["LV_COUNTER"]; !named {
		t.Fatalf("redaction hides values, not names, got %v", first.Vars)
	}
}
