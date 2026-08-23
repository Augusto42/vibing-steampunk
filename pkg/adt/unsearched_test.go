package adt

import (
	"strings"
	"testing"
	"unicode/utf8"
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

// An expired session does not answer 401 with a sentence: ICF returns a whole
// HTML logon page under it. Carried verbatim into the note, that buries the
// count the note exists to deliver — the same argument that caps the list at
// five names. The Reason field keeps the failure whole for JSON callers.
func TestALogonPageDoesNotBecomeTheCaveat(t *testing.T) {
	page := "ADT API error: status 401: <html><head><title>Logon failed</title><style>body { background: #ffffff; }" +
		strings.Repeat("x", 4000) + "</style></head><body>401 Not authorized</body></html>"
	note := UnsearchedNote([]Unsearched{{Object: "ZCL_DEMO", Reason: page}}, 2, "object")

	if !strings.Contains(note, "1 of 2") {
		t.Fatalf("the count must survive the page:\n%s", note)
	}
	if len(note) > 400 {
		t.Fatalf("the note is %d bytes; a caveat nobody can read stops nothing", len(note))
	}
	if !strings.Contains(note, "401") {
		t.Fatalf("the useful head of the reason should survive:\n%s", note)
	}
	if strings.Count(note, "\n") > 2 {
		t.Fatalf("a multi-line reason must not spill down the terminal:\n%s", note)
	}
}

// A reason cut mid-character would print a replacement glyph; SAP messages
// arrive in whatever code page the system felt like.
func TestATruncatedReasonStaysValidText(t *testing.T) {
	note := UnsearchedNote([]Unsearched{{Object: "A", Reason: strings.Repeat("ä", 500)}}, 2, "object")
	if !utf8.ValidString(note) {
		t.Fatal("the note must stay valid UTF-8 after truncation")
	}
}

// A failure with nothing to say still has to look like a failure.
func TestAnEmptyReasonStillReadsAsOne(t *testing.T) {
	note := UnsearchedNote([]Unsearched{{Object: "ZCL_DEMO", Reason: "   "}}, 2, "object")
	if !strings.Contains(note, "no reason given") {
		t.Fatalf("got %q", note)
	}
}
