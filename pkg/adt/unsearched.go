package adt

import (
	"fmt"
	"strings"
)

// A sweep over many objects fails on some of them, and the question is what it
// then tells the caller.
//
// The wrong answer, which this codebase gave in several places, is to skip the
// failure and report the total anyway: "no matches found in 50 objects" when
// thirty were searched and twenty could not be reached. The reader draws the
// conclusion the sentence invites — the pattern is not used — and acts on it.
// Nothing in the output is false on its own; the whole is.
//
// So: a result that could not look everywhere says so, and names what it
// missed. The rule is narrow and worth stating plainly, because it is the
// defect class this exists to close.
//
//	A caller that cannot tell "nothing found" from "the question failed"
//	must not return an empty result as if it were an answer.

// Unsearched is one thing a sweep could not look at, and why.
type Unsearched struct {
	// Object is what was skipped — an object URL, a package, whatever the
	// sweep was iterating.
	Object string `json:"object"`
	// Reason is the failure as it came back, not a category. The reader needs
	// to know whether it was an authorisation, a timeout or a missing object,
	// and those call for different next steps.
	Reason string `json:"reason"`
}

// Note renders what was missed, or the empty string when nothing was.
//
// It is deliberately blunt rather than tactful: "3 of 50 could not be searched"
// is the sentence that stops a wrong conclusion, and softening it defeats the
// purpose.
func UnsearchedNote(missed []Unsearched, total int, noun string) string {
	if len(missed) == 0 {
		return ""
	}
	if noun == "" {
		noun = "item"
	}
	plural := noun + "s"
	if strings.HasSuffix(noun, "s") {
		plural = noun
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d of %d %s could not be searched, so this is not a complete answer:", len(missed), total, plural)
	// A handful is worth naming outright; a hundred is a wall, and the count
	// plus a sample is what a reader can act on.
	const named = 5
	for i, m := range missed {
		if i >= named {
			fmt.Fprintf(&b, "\n  … and %d more", len(missed)-named)
			break
		}
		fmt.Fprintf(&b, "\n  %s: %s", m.Object, m.Reason)
	}
	return b.String()
}
