package adt

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// "Similar" is a ladder rather than a boolean, and every rung is equality on a
// field we already have — no scoring weights, no distance metric, nothing that
// has to be tuned. That is the point: a person can check a rung by reading it.
//
// The ladder is entered only through the error type. Two dumps with different
// runtime errors are not the same failure however close they sit in the code,
// so RAISE_EXCEPTION in a program is not a match for SYNTAX_ERROR in the same
// program. Within one error type the rungs narrow:
//
//	1. same program and same failing line — the same bug
//	2. same program — the same bug or its siblings
//	3. same application component — a neighbourhood
//	4. same error type — a class of failure
//
// Rungs 2 and 4 need only the feed. Rungs 1 and 3 need the failing line and
// the application component, and both of those cost one detail fetch per
// candidate, which is why a caller decides how many to pay for.

// SimilarityRung numbers the ladder as the design does: 1 is the strongest
// claim and 4 the weakest. Zero means the two dumps are not comparable.
type SimilarityRung int

const (
	// RungNone: different runtime errors, so not on the ladder at all.
	RungNone SimilarityRung = 0
	// RungLine: same error, same program, same failing line.
	RungLine SimilarityRung = 1
	// RungProgram: same error, same program, and either a different line or a
	// line nobody has read yet.
	RungProgram SimilarityRung = 2
	// RungComponent: same error somewhere else in the same application
	// component.
	RungComponent SimilarityRung = 3
	// RungClass: same error, and nothing else in common.
	RungClass SimilarityRung = 4
)

// DumpSignature is a dump plus the parts of its detail the ladder needs.
type DumpSignature struct {
	Dump
	// Component, Include and Line come from the detail. They are empty or zero
	// both when the detail was never read and when the dump genuinely has
	// none, which is why DetailRead exists.
	Component string `json:"component,omitempty"`
	Include   string `json:"include,omitempty"`
	Line      int    `json:"line,omitempty"`
	Exception string `json:"exception,omitempty"`
	// DetailRead says the detail was fetched. Without it a dump whose detail
	// was skipped for cost and a dump that has no failing line look identical,
	// and the difference decides whether "not rung 1" means "a different line"
	// or "nobody looked".
	DetailRead bool `json:"detailRead"`
}

// SignatureOf builds a signature from a dump and its detail. A nil detail is
// the honest representation of a dump nobody paid to read.
func SignatureOf(dump Dump, detail *DumpDetail) DumpSignature {
	sig := DumpSignature{Dump: dump}
	if detail == nil {
		return sig
	}
	sig.DetailRead = true
	sig.Component, sig.Include, sig.Line, sig.Exception = detail.Component, detail.Include, detail.Line, detail.Exception
	return sig
}

// DumpMatch is one candidate, the rung it reached, and the argument for it.
type DumpMatch struct {
	Dump Dump           `json:"dump"`
	Rung SimilarityRung `json:"rung"`
	// Why is the rung in words, and it is the part that matters. A rung-4
	// match is the same class of failure; it is not the same bug, and saying
	// so in the row is what stops the ranking being read as a verdict.
	Why string `json:"why"`
}

// RankSimilarDumps places every candidate on the ladder against one subject.
//
// The subject itself is dropped by id, and anything that does not share the
// runtime error is dropped entirely rather than ranked low: a weak match and a
// non-match are different things, and a list that contains non-matches teaches
// people to ignore the bottom of it.
func RankSimilarDumps(subject DumpSignature, candidates []DumpSignature) []DumpMatch {
	matches := make([]DumpMatch, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.ID == subject.ID {
			continue
		}
		rung, why := rungFor(subject, candidate)
		if rung == RungNone {
			continue
		}
		matches = append(matches, DumpMatch{Dump: candidate.Dump, Rung: rung, Why: why})
	}
	// Strongest rung first, then most recent, so the top of the list is the
	// closest thing to this dump that also happened most recently.
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Rung != matches[j].Rung {
			return matches[i].Rung < matches[j].Rung
		}
		return matches[i].Dump.At.After(matches[j].Dump.At)
	})
	return matches
}

func rungFor(subject, candidate DumpSignature) (SimilarityRung, string) {
	if !equalFoldTrim(subject.ErrorType, candidate.ErrorType) {
		return RungNone, ""
	}

	if equalFoldTrim(subject.Program, candidate.Program) {
		// Rung 1 needs the include as well as the line. A line number on its
		// own is not a position: for a class pool the program is the pool and
		// the code lives in per-method includes, so "line 94" means nothing
		// until you say of which include.
		samePlace := subject.Line > 0 && subject.Line == candidate.Line &&
			equalFoldTrim(subject.Include, candidate.Include)
		if samePlace {
			return RungLine, fmt.Sprintf("the same bug: %s in %s, both at %s line %d%s",
				subject.ErrorType, subject.Program, subject.Include, subject.Line, sameExceptionNote(subject, candidate))
		}
		return RungProgram, fmt.Sprintf("the same bug or a sibling of it: %s in %s%s%s",
			subject.ErrorType, subject.Program, lineComparison(subject, candidate), sameExceptionNote(subject, candidate))
	}

	// Rung 3 is only a neighbourhood if both dumps actually name one. An
	// unassigned component is not a place; see normalizeComponent.
	if subject.Component != "" && equalFoldTrim(subject.Component, candidate.Component) {
		return RungComponent, fmt.Sprintf("a neighbourhood: %s in %s rather than %s, both filed under application component %s",
			subject.ErrorType, candidate.Program, subject.Program, subject.Component)
	}

	return RungClass, fmt.Sprintf("the same class of failure, not the same bug: %s, but in %s rather than %s",
		subject.ErrorType, candidate.Program, subject.Program)
}

// lineComparison says why two dumps in the same program did not reach rung 1,
// and the two reasons are not interchangeable: a different line is evidence,
// an unread detail is an admission.
func lineComparison(subject, candidate DumpSignature) string {
	switch {
	case !subject.DetailRead || !candidate.DetailRead:
		return " (the failing lines were not read, so this may yet be the same line)"
	case subject.Line == 0 || candidate.Line == 0:
		return " (no failing line is recorded, so the program is as close as this gets)"
	case subject.Line != candidate.Line || !equalFoldTrim(subject.Include, candidate.Include):
		return fmt.Sprintf(" — a different line, %s:%d against %s:%d", candidate.Include, candidate.Line, subject.Include, subject.Line)
	}
	return ""
}

// sameExceptionNote adds the exception class when both dumps carry one and
// they agree. It is not a rung of its own — the design's ladder has four — but
// for UNCAUGHT_EXCEPTION and RAISE_EXCEPTION the runtime error names only the
// mechanism, and the exception class is the part a person recognises.
func sameExceptionNote(subject, candidate DumpSignature) string {
	if subject.Exception == "" || !equalFoldTrim(subject.Exception, candidate.Exception) {
		return ""
	}
	return ", and the same exception " + subject.Exception
}

// RungSummary is one rung collapsed: the answer to "is this new, and how often
// does it happen".
type RungSummary struct {
	Rung  SimilarityRung `json:"rung"`
	Label string         `json:"label"`
	Count int            `json:"count"`
	First time.Time      `json:"first"`
	Last  time.Time      `json:"last"`
	Users []string       `json:"users,omitempty"`
}

// RungLabel names a rung in the words the design uses, and deliberately
// refuses to call a weak rung a strong one.
func RungLabel(rung SimilarityRung) string {
	switch rung {
	case RungLine:
		return "same error, same program, same line — the same bug"
	case RungProgram:
		return "same error, same program — the same bug or its siblings"
	case RungComponent:
		return "same error, same application component — a neighbourhood"
	case RungClass:
		return "same error — a class of failure"
	default:
		return "not comparable"
	}
}

// SummarizeSimilar collapses matches per rung.
//
// Counts are per rung and not cumulative: a match already counted as the same
// bug is not counted again as the same class of failure. Cumulative counts
// read as "47 occurrences of this bug" when 46 of them were merely the same
// error somewhere else, which is the specific overstatement this whole file is
// arranged to avoid.
func SummarizeSimilar(matches []DumpMatch) []RungSummary {
	byRung := map[SimilarityRung]*RungSummary{}
	users := map[SimilarityRung]map[string]bool{}

	for _, m := range matches {
		summary, seen := byRung[m.Rung]
		if !seen {
			summary = &RungSummary{Rung: m.Rung, Label: RungLabel(m.Rung), First: m.Dump.At, Last: m.Dump.At}
			byRung[m.Rung] = summary
			users[m.Rung] = map[string]bool{}
		}
		summary.Count++
		if !m.Dump.At.IsZero() {
			if summary.First.IsZero() || m.Dump.At.Before(summary.First) {
				summary.First = m.Dump.At
			}
			if m.Dump.At.After(summary.Last) {
				summary.Last = m.Dump.At
			}
		}
		if m.Dump.User != "" {
			users[m.Rung][m.Dump.User] = true
		}
	}

	out := make([]RungSummary, 0, len(byRung))
	for rung, summary := range byRung {
		for user := range users[rung] {
			summary.Users = append(summary.Users, user)
		}
		sort.Strings(summary.Users)
		out = append(out, *summary)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rung < out[j].Rung })
	return out
}

// DeepenOrder ranks candidates by how much a detail fetch would tell us.
//
// Reading details is the only expensive part of the ladder, and it is only
// ever worth it for a candidate that could still move: one already in the same
// program might turn out to be the same line, and one in a different program
// might turn out to be the same component. A candidate whose error type
// differs cannot move at all and is not returned, so a budget is never spent
// proving that two unrelated dumps are unrelated.
func DeepenOrder(subject Dump, candidates []Dump) []Dump {
	var worth []Dump
	for _, c := range candidates {
		if c.ID == subject.ID || !equalFoldTrim(subject.ErrorType, c.ErrorType) {
			continue
		}
		worth = append(worth, c)
	}
	sameProgram := func(d Dump) bool { return equalFoldTrim(subject.Program, d.Program) }
	sort.SliceStable(worth, func(i, j int) bool {
		if sameProgram(worth[i]) != sameProgram(worth[j]) {
			return sameProgram(worth[i])
		}
		return worth[i].At.After(worth[j].At)
	})
	return worth
}

// FindDump picks one dump out of a listing by "latest" or by part of its id.
//
// Ids are long and carry the server instance, the user and a counter, so
// nobody types one whole. Matching on a substring is what makes the timestamp
// prefix printed in the listing usable as a handle.
func FindDump(dumps []Dump, which string) (Dump, bool) {
	if len(dumps) == 0 {
		return Dump{}, false
	}
	which = strings.TrimSpace(which)
	if which == "" || strings.EqualFold(which, "latest") {
		return dumps[0], true
	}
	for _, d := range dumps {
		if strings.Contains(d.ID, which) {
			return d, true
		}
	}
	return Dump{}, false
}
