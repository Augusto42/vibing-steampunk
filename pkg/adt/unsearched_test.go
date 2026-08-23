package adt

import (
	"strings"
	"testing"
)

// The sentence that stops a wrong conclusion. Without it a reader is told "no
// matches found in 50 objects" about a search that reached thirty, acts on it,
// and nothing in the output was false on its own.
func TestNoteNamesWhatWasMissedAndOutOfHowMany(t *testing.T) {
	note := UnsearchedNote([]Unsearched{
		{Object: "ZCL_DEMO_A", Reason: "not authorised"},
		{Object: "ZCL_DEMO_B", Reason: "timed out"},
	}, 50, "object")

	for _, want := range []string{"2 of 50", "not a complete answer", "ZCL_DEMO_A", "not authorised"} {
		if !strings.Contains(note, want) {
			t.Fatalf("the note should carry %q:\n%s", want, note)
		}
	}
}

// A complete sweep says nothing, or every clean result would carry a caveat
// and readers would learn to skip it.
func TestACompleteSweepSaysNothing(t *testing.T) {
	if note := UnsearchedNote(nil, 50, "object"); note != "" {
		t.Fatalf("nothing was missed; the note should be empty, got %q", note)
	}
}

// The reason is carried verbatim rather than categorised: an authorisation
// failure, a timeout and a missing object call for different next steps, and a
// caller that flattens them to "error" has thrown away the useful half.
func TestReasonsAreCarriedNotCategorised(t *testing.T) {
	note := UnsearchedNote([]Unsearched{
		{Object: "ZCL_DEMO", Reason: "ADT API error: status 403: Not authorised for package $ZDEMO"},
	}, 2, "object")
	if !strings.Contains(note, "403") || !strings.Contains(note, "$ZDEMO") {
		t.Fatalf("the failure should survive intact:\n%s", note)
	}
}

// A hundred names is a wall. The count is the part a reader acts on; a sample
// is enough to recognise the shape.
func TestALongListIsSampledNotDumped(t *testing.T) {
	var many []Unsearched
	for i := 0; i < 40; i++ {
		many = append(many, Unsearched{Object: "ZCL_DEMO", Reason: "timed out"})
	}
	note := UnsearchedNote(many, 100, "object")
	if !strings.Contains(note, "40 of 100") {
		t.Fatalf("the count must survive:\n%s", note)
	}
	if !strings.Contains(note, "and 35 more") {
		t.Fatalf("the remainder should be counted, not listed:\n%s", note)
	}
	if lines := strings.Count(note, "\n"); lines > 7 {
		t.Fatalf("the note should stay readable, got %d lines", lines)
	}
}

func TestPluralsReadCorrectly(t *testing.T) {
	one := UnsearchedNote([]Unsearched{{Object: "A", Reason: "x"}}, 3, "package")
	if !strings.Contains(one, "packages could not be searched") {
		t.Fatalf("got %q", one)
	}
	// A noun that is already plural must not grow another s.
	already := UnsearchedNote([]Unsearched{{Object: "A", Reason: "x"}}, 3, "objects")
	if strings.Contains(already, "objectss") {
		t.Fatalf("got %q", already)
	}
}
