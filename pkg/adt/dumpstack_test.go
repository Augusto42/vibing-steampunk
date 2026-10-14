package adt

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// Real ST22 layout, synthetic names. Note the two shapes the line number
// arrives in — right-aligned in a wide column on one row, left-shifted on
// another — which is why this is parsed from the ends inward rather than by
// column offset. "MODULE (PBO)" is here because the type is sometimes two
// words, which a naive field split gets wrong.
const formattedDumpSample = `
----------------------------------------------------------------------------------------------------
|Active Calls/Events                                                                               |
----------------------------------------------------------------------------------------------------
|No.   Ty.          Program                             Include                             Line   |
|      Name                                                                                        |
----------------------------------------------------------------------------------------------------
|    4 METHOD       ZCL_DEMO_SPLIT===============CP    ZCL_DEMO_SPLIT===============CM003    22  |
|      ZCL_DEMO_SPLIT=>CONSTRUCTOR                                                                 |
|    3 METHOD       ZCL_DEMO_ORDER===============CP    ZCL_DEMO_ORDER===============CM008  8      |
|      ZCL_DEMO_ORDER=>POST                                                                        |
|    2 FUNCTION     SAPLZDEMO_FG                        LZDEMO_FGU01                          19  |
|      ZDEMO_POST_ORDER                                                                            |
|    1 MODULE (PBO) SAPMSSY1                            SAPMSSY1                              36  |
|      %_RFC_START                                                                                 |
----------------------------------------------------------------------------------------------------

----------------------------------------------------------------------------------------------------
|Selected Variables                                                                                |
----------------------------------------------------------------------------------------------------
|SY-XPROG                                                                                          |
|    SAPMSSY0                                                                                      |
`

func TestDumpStackParsesEveryFrame(t *testing.T) {
	frames := parseDumpStack(formattedDumpSample)
	if len(frames) != 4 {
		t.Fatalf("four frames, got %d: %+v", len(frames), frames)
	}

	top := frames[0]
	if top.Position != 4 || top.Type != "METHOD" || top.Line != 22 {
		t.Fatalf("top frame parsed as %+v", top)
	}
	if top.Program != "ZCL_DEMO_SPLIT===============CP" {
		t.Fatalf("program is %q", top.Program)
	}
	if top.Name != "ZCL_DEMO_SPLIT=>CONSTRUCTOR" {
		t.Fatalf("name is %q", top.Name)
	}

	// The type is two words here, and the include and line still have to land
	// in the right places.
	bottom := frames[3]
	if bottom.Type != "MODULE (PBO)" {
		t.Fatalf("a two-word type should survive, got %q", bottom.Type)
	}
	if bottom.Program != "SAPMSSY1" || bottom.Include != "SAPMSSY1" || bottom.Line != 36 {
		t.Fatalf("bottom frame parsed as %+v", bottom)
	}
}

// The chapter ends where the next begins; variable values are not frames.
func TestDumpStackStopsAtTheNextChapter(t *testing.T) {
	for _, f := range parseDumpStack(formattedDumpSample) {
		if f.Program == "SY-XPROG" || f.Name == "SY-XPROG" {
			t.Fatalf("read past the chapter into the variables: %+v", f)
		}
	}
}

func TestDumpStackOfSomethingWithNoStack(t *testing.T) {
	if frames := parseDumpStack("nothing like a dump at all"); frames != nil {
		t.Fatalf("expected nothing, got %+v", frames)
	}
}

func TestStackProgramsAreDistinct(t *testing.T) {
	programs := StackPrograms([]DumpFrame{
		{Program: "ZCL_DEMO_A"}, {Program: "ZCL_DEMO_A"}, {Program: "ZCL_DEMO_B"}, {Program: ""},
	})
	if len(programs) != 2 {
		t.Fatalf("two distinct programs, got %v", programs)
	}
}

// The rung this was all for. A log written by a frame of the dump's own stack
// is on the causal path by construction, and outranks anything the clock alone
// can offer — including an entry from the same user one second away.
func TestOnTheStackOutranksTheClock(t *testing.T) {
	dumpAt := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	dump := Dump{Program: "SAPMSSY1", User: "TESTUSER", At: dumpAt}
	stack := parseDumpStack(formattedDumpSample)

	onStack := AppLogEntry{Program: "SAPLZDEMO_FG", User: "SOMEONE", At: dumpAt.Add(-90 * time.Second)}
	sameUser := AppLogEntry{Program: "SAPMHTTP", User: "TESTUSER", At: dumpAt.Add(-1 * time.Second)}

	stackScore, why := rankLogAgainstDump(onStack, dump, stack, dumpAt.Sub(onStack.At))
	clockScore, _ := rankLogAgainstDump(sameUser, dump, stack, dumpAt.Sub(sameUser.At))

	if stackScore <= clockScore {
		t.Fatalf("a frame of the stack should outrank a coincidence: %d vs %d", stackScore, clockScore)
	}
	// The reason has to name the frame, or a person cannot check it.
	if !strings.Contains(why, "frame 2") || !strings.Contains(why, "ZDEMO_POST_ORDER") {
		t.Fatalf("the reason should name the frame and what ran there, got %q", why)
	}
}

// The program that actually died still outranks the rest of its own stack.
func TestTheDumpingProgramStillLeads(t *testing.T) {
	dumpAt := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	dump := Dump{Program: "SAPMSSY1", User: "TESTUSER", At: dumpAt}
	stack := parseDumpStack(formattedDumpSample)

	dying := AppLogEntry{Program: "SAPMSSY1", At: dumpAt.Add(-10 * time.Second)}
	elsewhere := AppLogEntry{Program: "SAPLZDEMO_FG", At: dumpAt.Add(-1 * time.Second)}

	dyingScore, _ := rankLogAgainstDump(dying, dump, stack, dumpAt.Sub(dying.At))
	otherScore, _ := rankLogAgainstDump(elsewhere, dump, stack, dumpAt.Sub(elsewhere.At))
	if dyingScore <= otherScore {
		t.Fatalf("the program that dumped leads its own stack: %d vs %d", dyingScore, otherScore)
	}
}

// The rung below the stack: something a frame calls. It has returned by the
// time of the failure, so it is not on the path — but it is where a bad value
// is usually prepared, and it is still an argument rather than a coincidence.
func TestCalledFromTheStackOutranksTheClockButNotTheStack(t *testing.T) {
	dumpAt := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	dump := Dump{Program: "SAPMSSY1", User: "TESTUSER", At: dumpAt}
	stack := parseDumpStack(formattedDumpSample)
	callees := map[string]string{"ZCL_DEMO_PRICING": "ZCL_DEMO_ORDER===============CP"}

	called := AppLogEntry{Program: "ZCL_DEMO_PRICING", User: "SOMEONE", At: dumpAt.Add(-30 * time.Second)}
	onStack := AppLogEntry{Program: "SAPLZDEMO_FG", User: "SOMEONE", At: dumpAt.Add(-30 * time.Second)}
	sameUser := AppLogEntry{Program: "SAPMHTTP", User: "TESTUSER", At: dumpAt.Add(-1 * time.Second)}

	calledScore, why := rankLogAgainstDumpWithGraph(called, dump, stack, callees, dumpAt.Sub(called.At))
	stackScore, _ := rankLogAgainstDumpWithGraph(onStack, dump, stack, callees, dumpAt.Sub(onStack.At))
	clockScore, _ := rankLogAgainstDumpWithGraph(sameUser, dump, stack, callees, dumpAt.Sub(sameUser.At))

	if !(stackScore > calledScore && calledScore > clockScore) {
		t.Fatalf("the ladder should read stack > called > clock, got %d, %d, %d",
			stackScore, calledScore, clockScore)
	}
	if !strings.Contains(why, "called from") {
		t.Fatalf("the reason should say where it was called from, got %q", why)
	}
}

// Class pools arrive from a dump padded with '=' and are not addressable that
// way; neither are function pools, and this test used to assert that they were.
//
// It asked for SAPLZDEMO_FG under /programs/programs and got it, which is a 404
// on any system — and a 404 here is silent, because the caller treats a frame
// whose graph cannot be read as a frame that contributes nothing. The
// expectation was the bug. unitForFrame in dumpimpact.go does the real mapping
// and programURI now defers to it.
func TestProgramURIUnwrapsAClassPool(t *testing.T) {
	if got := programURI("ZCL_DEMO_ORDER===============CP"); got != "/sap/bc/adt/oo/classes/zcl_demo_order" {
		t.Fatalf("class pool should resolve to the class, got %q", got)
	}
	if got := programURI("SAPLZDEMO_FG"); got != "/sap/bc/adt/functions/groups/zdemo_fg" {
		t.Fatalf("a function pool should resolve to its group, got %q", got)
	}
	if got := programURI("ZDEMO_REPORT"); got != "/sap/bc/adt/programs/programs/zdemo_report" {
		t.Fatalf("a report should resolve to a program, got %q", got)
	}
	if got := programURI("  "); got != "" {
		t.Fatalf("nothing in, nothing out, got %q", got)
	}
}

// A release that has the dump feed but not the detail resource is not a
// failure, and must not be reported as one: 7.50 is exactly that, the same way
// it has the debugger but not /debugger/stack. Everything else about the dump
// still works, so the correlation continues with one rung unused.
func TestAnAbsentDetailResourceIsNotAFailure(t *testing.T) {
	if !errors.Is(ErrDumpDetailUnavailable, ErrDumpDetailUnavailable) {
		t.Fatal("the sentinel must be comparable with errors.Is")
	}
	if !strings.Contains(ErrDumpDetailUnavailable.Error(), "call stack") {
		t.Fatalf("the message should say what is missing, got %q", ErrDumpDetailUnavailable)
	}
}
