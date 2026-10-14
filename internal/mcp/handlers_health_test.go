package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// The defect this file exists to pin: a health report is read for reassurance,
// so a check that failed to run is the most expensive thing in the codebase to
// drop. Every check that did not happen comes back looking like a check that
// found nothing, and the object is pronounced healthy on the strength of
// questions nobody managed to ask.

func TestAHealthReportWithAGapIsNotGood(t *testing.T) {
	signals := map[string]healthSignal{
		"tests": {
			Status:     "PASS",
			Unsearched: []adt.Unsearched{{Object: "ZCL_DEMO_TEST", Reason: "not authorised"}},
		},
		"atc":        {Status: "CLEAN"},
		"boundaries": {Status: "CLEAN"},
		"staleness":  {Status: "ACTIVE"},
	}

	summary := summarizeHealth(signals)
	if summary.Status == "GOOD" {
		t.Fatalf("one test class never ran, so this is not a clean bill of health: %+v", summary)
	}
	if !strings.Contains(summary.Headline, "tests") {
		t.Fatalf("the headline should name the check that came up short, got %q", summary.Headline)
	}
}

// A check that errored outright is the same lie in a louder voice: the summary
// used to walk straight past ERROR to "No major health issues detected".
func TestAFailedCheckKeepsTheSummaryFromSayingGood(t *testing.T) {
	signals := map[string]healthSignal{
		"tests":      {Status: "NONE"},
		"atc":        {Status: "ERROR", Details: map[string]any{"message": "ATC run failed: 403"}},
		"boundaries": {Status: "CLEAN"},
		"staleness":  {Status: "ACTIVE"},
	}
	if got := summarizeHealth(signals); got.Status == "GOOD" {
		t.Fatalf("ATC never ran; the summary must not call that good: %+v", got)
	}
}

// A complete sweep must still be allowed to say so, or the caveat becomes
// wallpaper and readers learn to skip it.
func TestACompleteHealthReportStillSaysGood(t *testing.T) {
	signals := map[string]healthSignal{
		"tests":      {Status: "PASS"},
		"atc":        {Status: "CLEAN"},
		"boundaries": {Status: "CLEAN"},
		"staleness":  {Status: "ACTIVE"},
	}
	if got := summarizeHealth(signals); got.Status != "GOOD" {
		t.Fatalf("nothing was missed and nothing was wrong; expected GOOD, got %+v", got)
	}
}

// A real finding outranks a gap. Someone whose tests are failing needs to be
// told that first, not told the report was incomplete.
func TestARealFindingOutranksAGap(t *testing.T) {
	signals := map[string]healthSignal{
		"tests": {
			Status:     "FAIL",
			Unsearched: []adt.Unsearched{{Object: "ZCL_DEMO_OTHER", Reason: "timed out"}},
		},
	}
	if got := summarizeHealth(signals); got.Status != "BAD" {
		t.Fatalf("failing tests are the headline, got %+v", got)
	}
}

// An MCP caller reads the JSON and has no stderr. A caveat recorded on a signal
// nobody expands is a caveat that does not exist, so it is lifted to the top.
func TestGapsAreLiftedToTheTopOfTheDocument(t *testing.T) {
	signals := map[string]healthSignal{
		"tests": {
			Status: "ERROR",
			Note:   "2 of 5 test classes could not be searched, so this is not a complete answer:\n  ZCL_DEMO_A: not authorised",
		},
		"staleness": {Status: "ERROR", Details: map[string]any{"message": "revision read failed"}},
	}
	notes := healthNotes(signals)
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "tests:") || !strings.Contains(joined, "not authorised") {
		t.Fatalf("the tests caveat should reach the top level:\n%s", joined)
	}
	if !strings.Contains(joined, "staleness:") || !strings.Contains(joined, "revision read failed") {
		t.Fatalf("a signal that errored with no note still owes the reader an explanation:\n%s", joined)
	}
}

// The whole document is what an agent sees. Marshalling has to keep both the
// gaps and the honest summary in it.
func TestTheMarshalledReportCarriesItsOwnCaveats(t *testing.T) {
	result := &healthResult{
		Scope: healthScope{Kind: "package", Package: "$ZDEMO"},
		Signals: map[string]healthSignal{
			"tests": {
				Status:     "ERROR",
				Unsearched: []adt.Unsearched{{Object: "ZCL_DEMO_TEST", Reason: "not authorised"}},
				Note:       "1 of 1 test classes could not be searched, so this is not a complete answer",
			},
		},
	}
	result.Summary = summarizeHealth(result.Signals)
	result.Notes = healthNotes(result.Signals)

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	for _, want := range []string{"not authorised", "\"notes\"", "\"unsearched\""} {
		if !strings.Contains(doc, want) {
			t.Fatalf("the payload an agent reads is missing %q:\n%s", want, doc)
		}
	}
	if strings.Contains(doc, "No major health issues detected") {
		t.Fatalf("a report that could not run its tests must not carry the reassuring headline:\n%s", doc)
	}
}

// A signal with nothing missing must not be treated as partial, or every report
// carries the caveat and the caveat stops meaning anything.
func TestACleanSignalIsNotCalledIncomplete(t *testing.T) {
	if (healthSignal{Status: "CLEAN"}).incomplete() {
		t.Fatal("a clean signal is complete")
	}
	if !(healthSignal{Status: "ERROR"}).incomplete() {
		t.Fatal("a signal that errored is not evidence of health")
	}
	if !(healthSignal{Status: "PASS", Unsearched: []adt.Unsearched{{Object: "X"}}}).incomplete() {
		t.Fatal("a PASS over a partial sweep is still partial")
	}
	// A signal that wrote itself a caveat has already admitted it is partial;
	// the summary must not then paper over it. This is the shape a live run
	// found: a package whose listing holds only sub-packages scans zero objects
	// and used to answer "CLEAN, 0 violations", which reads as a pass.
	if !(healthSignal{Status: "UNKNOWN", Note: "no source-bearing objects were read"}).incomplete() {
		t.Fatal("a signal carrying a caveat is not a complete answer")
	}
}

func TestJoinNotesDoesNotProduceStrayNewlines(t *testing.T) {
	if got := joinNotes("", "second", ""); got != "second" {
		t.Fatalf("got %q", got)
	}
	if got := joinNotes("", ""); got != "" {
		t.Fatalf("nothing to say should say nothing, got %q", got)
	}
}
