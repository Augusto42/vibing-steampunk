package adt

import (
	"strings"
	"testing"
	"time"
)

// atMinute names a moment inside one hour. The watch tests define their own
// `at` taking an RFC3339 string; two helpers with one name is what parallel
// work produces, and the two are not interchangeable — this one is a minute
// offset, that one is a whole timestamp.
func atMinute(minute int) time.Time {
	return time.Date(2026, 8, 22, 10, minute, 0, 0, time.UTC)
}

// sig builds a signature whose detail was read. Everything the ladder's upper
// rungs depend on is present, so a test that wants "detail not read" has to
// say so explicitly — which is the case worth being explicit about.
func sig(id, errorType, program, include string, line int, component string, minute int) DumpSignature {
	return DumpSignature{
		Dump:       Dump{ID: id, ErrorType: errorType, Program: program, User: "TESTUSER", At: atMinute(minute)},
		Include:    include,
		Line:       line,
		Component:  component,
		DetailRead: true,
	}
}

func TestSameLineIsRungOne(t *testing.T) {
	subject := sig("a", "RAISE_EXCEPTION", "ZCL_DEMO_ORDER===============CP", "ZCL_DEMO_ORDER===============CM007", 94, "", 10)
	other := sig("b", "RAISE_EXCEPTION", "ZCL_DEMO_ORDER===============CP", "ZCL_DEMO_ORDER===============CM007", 94, "", 5)

	matches := RankSimilarDumps(subject, []DumpSignature{other})
	if len(matches) != 1 || matches[0].Rung != RungLine {
		t.Fatalf("expected one rung-1 match, got %+v", matches)
	}
	if !strings.Contains(matches[0].Why, "the same bug") {
		t.Fatalf("the reason should say what rung 1 claims, got %q", matches[0].Why)
	}
}

// The trap this rung exists to avoid. Three unrelated function groups on a
// live system all terminate at SAPMSSY1 line 36, because that is where the RFC
// entry point sits. The line is identical and the bug is not, so the program
// has to be part of rung 1.
func TestSameLineInADifferentProgramIsNotTheSameBug(t *testing.T) {
	subject := sig("a", "CALL_FUNCTION_NOT_REMOTE", "SAPLZDEMO_BAL", "SAPMSSY1", 36, "BC-SRV-BAL", 10)
	other := sig("b", "CALL_FUNCTION_NOT_REMOTE", "SAPLZDEMO_ENT", "SAPMSSY1", 36, "BC-CST-EQ", 5)

	matches := RankSimilarDumps(subject, []DumpSignature{other})
	if len(matches) != 1 || matches[0].Rung != RungClass {
		t.Fatalf("expected rung 4, got %+v", matches)
	}
	if !strings.Contains(matches[0].Why, "not the same bug") {
		t.Fatalf("rung 4 must say so in words, got %q", matches[0].Why)
	}
}

// A class pool is one program with a per-method include, so the line number
// alone is not a position inside it.
func TestSameLineInADifferentIncludeIsNotTheSameBug(t *testing.T) {
	subject := sig("a", "RAISE_EXCEPTION", "ZCL_DEMO_ORDER===============CP", "ZCL_DEMO_ORDER===============CM001", 22, "", 10)
	other := sig("b", "RAISE_EXCEPTION", "ZCL_DEMO_ORDER===============CP", "ZCL_DEMO_ORDER===============CM008", 22, "", 5)

	matches := RankSimilarDumps(subject, []DumpSignature{other})
	if len(matches) != 1 || matches[0].Rung != RungProgram {
		t.Fatalf("expected rung 2, got %+v", matches)
	}
	if !strings.Contains(matches[0].Why, "a different line") {
		t.Fatalf("the reason should name the difference, got %q", matches[0].Why)
	}
}

// "Not the same line" and "nobody read the line" are different claims, and
// only one of them is evidence.
func TestAnUnreadDetailIsNotEvidenceOfADifferentLine(t *testing.T) {
	subject := sig("a", "SYNTAX_ERROR", "ZCL_DEMO_ORDER===============CP", "ZCL_DEMO_ORDER===============CM001", 22, "", 10)
	other := DumpSignature{Dump: Dump{ID: "b", ErrorType: "SYNTAX_ERROR", Program: "ZCL_DEMO_ORDER===============CP", At: atMinute(5)}}

	matches := RankSimilarDumps(subject, []DumpSignature{other})
	if len(matches) != 1 || matches[0].Rung != RungProgram {
		t.Fatalf("expected rung 2, got %+v", matches)
	}
	if !strings.Contains(matches[0].Why, "were not read") {
		t.Fatalf("an unread detail must be admitted, got %q", matches[0].Why)
	}
}

// A dump with no source position at all — every DYNPRO_SEND_IN_BACKGROUND
// looks like this — must not be reported as a line that differs.
func TestNoRecordedLineIsSaidPlainly(t *testing.T) {
	subject := sig("a", "DYNPRO_SEND_IN_BACKGROUND", "SAPMSSY0", "", 0, "BC-ABA-LA", 10)
	other := sig("b", "DYNPRO_SEND_IN_BACKGROUND", "SAPMSSY0", "", 0, "BC-ABA-LA", 5)

	matches := RankSimilarDumps(subject, []DumpSignature{other})
	if len(matches) != 1 || matches[0].Rung != RungProgram {
		t.Fatalf("expected rung 2, got %+v", matches)
	}
	if !strings.Contains(matches[0].Why, "no failing line is recorded") {
		t.Fatalf("got %q", matches[0].Why)
	}
}

func TestSameComponentIsRungThree(t *testing.T) {
	subject := sig("a", "DELTA_NO_OBJECT", "ZDEMO_RFC_BRIDGE_TEST", "ZDEMO_RFC_BRIDGE_TEST", 41, "BC-MID-RFC", 10)
	other := sig("b", "DELTA_NO_OBJECT", "SAPLZDEMO_CRFC", "LZDEMO_CRFCU01", 7, "BC-MID-RFC", 5)

	matches := RankSimilarDumps(subject, []DumpSignature{other})
	if len(matches) != 1 || matches[0].Rung != RungComponent {
		t.Fatalf("expected rung 3, got %+v", matches)
	}
	if !strings.Contains(matches[0].Why, "neighbourhood") {
		t.Fatalf("got %q", matches[0].Why)
	}
}

// Custom code is routinely filed under no component at all. If that counted as
// a component, every unassigned object in the system would be one
// neighbourhood — the least useful grouping available, dressed up as the third
// rung of a ladder.
func TestTwoUnassignedComponentsAreNotANeighbourhood(t *testing.T) {
	subject := sig("a", "SYNTAX_ERROR", "ZCL_DEMO_ONE=================CP", "ZCL_DEMO_ONE=================CM001", 3, "", 10)
	other := sig("b", "SYNTAX_ERROR", "ZCL_DEMO_TWO=================CP", "ZCL_DEMO_TWO=================CM001", 3, "", 5)

	matches := RankSimilarDumps(subject, []DumpSignature{other})
	if len(matches) != 1 || matches[0].Rung != RungClass {
		t.Fatalf("expected rung 4, got %+v", matches)
	}
}

// A different runtime error is not a weak match, it is not a match. A list
// that ends in non-matches teaches people to stop reading its bottom.
func TestADifferentErrorTypeIsNotOnTheLadder(t *testing.T) {
	subject := sig("a", "SYNTAX_ERROR", "ZCL_DEMO_ONE=================CP", "ZCL_DEMO_ONE=================CM001", 3, "BC-MID-RFC", 10)
	other := sig("b", "RAISE_EXCEPTION", "ZCL_DEMO_ONE=================CP", "ZCL_DEMO_ONE=================CM001", 3, "BC-MID-RFC", 5)

	if matches := RankSimilarDumps(subject, []DumpSignature{other}); len(matches) != 0 {
		t.Fatalf("expected no matches, got %+v", matches)
	}
}

func TestTheSubjectIsNotItsOwnMatch(t *testing.T) {
	subject := sig("a", "SYNTAX_ERROR", "ZCL_DEMO_ONE=================CP", "ZCL_DEMO_ONE=================CM001", 3, "", 10)
	if matches := RankSimilarDumps(subject, []DumpSignature{subject}); len(matches) != 0 {
		t.Fatalf("expected no matches, got %+v", matches)
	}
}

// Strongest rung first, and within a rung the most recent first: the top of
// the list is the closest thing to this dump that also happened most recently.
func TestRankingIsRungThenRecency(t *testing.T) {
	subject := sig("a", "DELTA_NO_OBJECT", "ZDEMO_ONE", "ZDEMO_ONE", 41, "BC-MID-RFC", 30)
	candidates := []DumpSignature{
		sig("far", "DELTA_NO_OBJECT", "ZDEMO_OTHER", "ZDEMO_OTHER", 9, "BC-XXX", 29),
		sig("near-old", "DELTA_NO_OBJECT", "ZDEMO_ONE", "ZDEMO_ONE", 41, "BC-MID-RFC", 1),
		sig("neighbour", "DELTA_NO_OBJECT", "ZDEMO_TWO", "ZDEMO_TWO", 5, "BC-MID-RFC", 28),
		sig("near-new", "DELTA_NO_OBJECT", "ZDEMO_ONE", "ZDEMO_ONE", 41, "BC-MID-RFC", 20),
		sig("sibling", "DELTA_NO_OBJECT", "ZDEMO_ONE", "ZDEMO_ONE", 9, "BC-MID-RFC", 27),
	}

	matches := RankSimilarDumps(subject, candidates)
	got := make([]string, 0, len(matches))
	for _, m := range matches {
		got = append(got, m.Dump.ID)
	}
	want := []string{"near-new", "near-old", "sibling", "neighbour", "far"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order is %v, want %v", got, want)
	}
}

// Per rung, not cumulative. "47 occurrences of this bug" when 46 of them were
// merely the same error somewhere else is the exact overstatement the ladder
// exists to prevent.
func TestSummaryCountsEachMatchOnce(t *testing.T) {
	subject := sig("a", "DELTA_NO_OBJECT", "ZDEMO_ONE", "ZDEMO_ONE", 41, "BC-MID-RFC", 30)
	candidates := []DumpSignature{
		sig("b", "DELTA_NO_OBJECT", "ZDEMO_ONE", "ZDEMO_ONE", 41, "BC-MID-RFC", 20),
		sig("c", "DELTA_NO_OBJECT", "ZDEMO_ONE", "ZDEMO_ONE", 41, "BC-MID-RFC", 1),
		sig("d", "DELTA_NO_OBJECT", "ZDEMO_TWO", "ZDEMO_TWO", 5, "BC-MID-RFC", 15),
		sig("e", "DELTA_NO_OBJECT", "ZDEMO_THREE", "ZDEMO_THREE", 5, "BC-OTHER", 12),
	}

	summary := SummarizeSimilar(RankSimilarDumps(subject, candidates))
	if len(summary) != 3 {
		t.Fatalf("expected rungs 1, 3 and 4, got %+v", summary)
	}
	if summary[0].Rung != RungLine || summary[0].Count != 2 {
		t.Fatalf("rung 1 should hold two dumps, got %+v", summary[0])
	}
	if summary[1].Rung != RungComponent || summary[1].Count != 1 {
		t.Fatalf("rung 3 should hold one dump, got %+v", summary[1])
	}
	if summary[2].Rung != RungClass || summary[2].Count != 1 {
		t.Fatalf("rung 4 should hold one dump, got %+v", summary[2])
	}
	// The window is what answers "is this new": first and last across the rung.
	if !summary[0].First.Equal(atMinute(1)) || !summary[0].Last.Equal(atMinute(20)) {
		t.Fatalf("rung 1 window is %s to %s", summary[0].First, summary[0].Last)
	}
	if len(summary[0].Users) != 1 || summary[0].Users[0] != "TESTUSER" {
		t.Fatalf("users are %v", summary[0].Users)
	}
}

func TestRungLabelsRefuseToPromote(t *testing.T) {
	if strings.Contains(RungLabel(RungClass), "the same bug") {
		t.Fatalf("rung 4 must not be described as the same bug: %q", RungLabel(RungClass))
	}
	if !strings.Contains(RungLabel(RungLine), "the same bug") {
		t.Fatalf("rung 1 is the same bug: %q", RungLabel(RungLine))
	}
	if RungLabel(RungNone) != "not comparable" {
		t.Fatalf("got %q", RungLabel(RungNone))
	}
}

// The detail budget is the only expensive part, so it must never be spent on a
// candidate that cannot move: a different runtime error is off the ladder
// whatever its detail says.
func TestDeepenOrderSpendsTheBudgetWhereItCanMoveARung(t *testing.T) {
	subject := Dump{ID: "a", ErrorType: "DELTA_NO_OBJECT", Program: "ZDEMO_ONE", At: atMinute(30)}
	order := DeepenOrder(subject, []Dump{
		{ID: "elsewhere", ErrorType: "DELTA_NO_OBJECT", Program: "ZDEMO_TWO", At: atMinute(29)},
		{ID: "unrelated", ErrorType: "SYNTAX_ERROR", Program: "ZDEMO_ONE", At: atMinute(28)},
		{ID: "a", ErrorType: "DELTA_NO_OBJECT", Program: "ZDEMO_ONE", At: atMinute(30)},
		{ID: "same-old", ErrorType: "DELTA_NO_OBJECT", Program: "ZDEMO_ONE", At: atMinute(2)},
		{ID: "same-new", ErrorType: "DELTA_NO_OBJECT", Program: "ZDEMO_ONE", At: atMinute(27)},
	})

	got := make([]string, 0, len(order))
	for _, d := range order {
		got = append(got, d.ID)
	}
	// Same program first, because only those can reach rung 1; then by
	// recency. The subject and the unrelated error type are not there at all.
	want := "same-new,same-old,elsewhere"
	if strings.Join(got, ",") != want {
		t.Fatalf("order is %v, want %s", got, want)
	}
}

func TestFindDumpTakesLatestOrPartOfAnID(t *testing.T) {
	dumps := []Dump{
		{ID: "/sap/bc/adt/vit/runtime/dumps/20260822231925host_A4H_00 TESTUSER 001 4"},
		{ID: "/sap/bc/adt/vit/runtime/dumps/20260822230807host_A4H_00 TESTUSER 001 4"},
	}
	if d, ok := FindDump(dumps, "latest"); !ok || !strings.Contains(d.ID, "20260822231925") {
		t.Fatalf("latest is the first, got %+v", d)
	}
	// Nobody types a whole id: it carries the server instance, the user, the
	// client and a counter. The timestamp printed in the listing is the handle.
	if d, ok := FindDump(dumps, "20260822230807"); !ok || !strings.Contains(d.ID, "230807") {
		t.Fatalf("a timestamp prefix should find it, got %+v", d)
	}
	if _, ok := FindDump(dumps, "nothing"); ok {
		t.Fatal("a miss must be reported as a miss")
	}
	if _, ok := FindDump(nil, "latest"); ok {
		t.Fatal("there is no latest dump in an empty listing")
	}
}

func TestSignatureOfMarksAnUnreadDetail(t *testing.T) {
	if SignatureOf(Dump{ID: "a"}, nil).DetailRead {
		t.Fatal("a nil detail means nobody read it")
	}
	read := SignatureOf(Dump{ID: "a"}, &DumpDetail{Component: "BC-MID-RFC", Line: 12, Include: "ZDEMO", Exception: "CX_DEMO"})
	if !read.DetailRead || read.Component != "BC-MID-RFC" || read.Line != 12 || read.Exception != "CX_DEMO" {
		t.Fatalf("got %+v", read)
	}
}

// For UNCAUGHT_EXCEPTION and RAISE_EXCEPTION the runtime error names only the
// mechanism. The exception class is the part a person recognises, so it is
// worth saying when it agrees — without inventing a fifth rung for it.
func TestTheExceptionClassIsMentionedWhenItAgrees(t *testing.T) {
	subject := sig("a", "UNCAUGHT_EXCEPTION", "ZCL_DEMO_ONE=================CP", "ZCL_DEMO_ONE=================CM001", 5, "", 10)
	subject.Exception = "CX_DEMO_INVALID_ORDER"
	same := sig("b", "UNCAUGHT_EXCEPTION", "ZCL_DEMO_ONE=================CP", "ZCL_DEMO_ONE=================CM001", 5, "", 5)
	same.Exception = "CX_DEMO_INVALID_ORDER"
	other := sig("c", "UNCAUGHT_EXCEPTION", "ZCL_DEMO_ONE=================CP", "ZCL_DEMO_ONE=================CM001", 5, "", 4)
	other.Exception = "CX_DEMO_NO_AUTHORITY"

	matches := RankSimilarDumps(subject, []DumpSignature{same, other})
	if len(matches) != 2 {
		t.Fatalf("both are rung 1, got %+v", matches)
	}
	if !strings.Contains(matches[0].Why, "CX_DEMO_INVALID_ORDER") {
		t.Fatalf("an agreeing exception should be named, got %q", matches[0].Why)
	}
	if strings.Contains(matches[1].Why, "the same exception") {
		t.Fatalf("a differing exception must not be claimed, got %q", matches[1].Why)
	}
}
