package ctxcomp

// Choosing which dependencies a reader gets, when there is a budget.
//
// # When AND, and when OR
//
// The analyzer runs several layers and the tempting model is that they check
// each other: something two layers found is real, something one layer found is
// suspect. That model cost this package a defect — every service a dispatcher
// dispatches to was reported as a likely false positive — and it was wrong for a
// reason worth stating once, properly:
//
//	**Corroboration is only meaningful between layers that answer the same
//	question.**
//
// The layers here do not all answer the same question:
//
//   - The regex layer reads **declarations** — INHERITING FROM, INTERFACES,
//     TYPE REF TO, CREATE OBJECT.
//   - The parser layer reads **calls**: it is the graph's edge extractor.
//   - SCAN ABAP-SOURCE is the SAP kernel's own tokenizer over the same text.
//   - CROSS/WBCROSSGT is an **index over the activated version**, not the text.
//
// So:
//
//	regex vs parser        different questions        → OR. Absence proves nothing.
//	parser vs SCAN         same question, same text   → AND is real corroboration.
//	source vs CROSS        same question, different
//	                       source of truth            → disagreement is information.
//
// That last row is the one worth keeping. An index-only dependency means the
// activated version references something the current text does not: the source
// was edited and not activated, or the index is stale. A source-only dependency
// means the opposite. Neither is a false positive, and reporting either as one
// throws away the only signal that says which.
//
// # Choosing under a budget
//
// A context has room for perhaps twenty contracts and a class may name sixty.
// Filtering answers "is this real"; this answers "of the real ones, which
// twenty does a reader need", and they are different questions too.
//
// The old answer was: custom names first, then whichever appeared earliest in
// the file. Line order is not a measure of importance — it puts a type used
// once in a signature above the superclass, because the superclass is declared
// on line two and sorted only by being standard.
//
// The order below is what a person needs to read the code at all, descending:
//
//  1. **Obligations** — the superclass and the interfaces implemented. Without
//     these the class cannot be understood; half its behaviour is elsewhere.
//  2. **Signature types** — what a caller has to construct or receive. These
//     appear in the public API, so a reader meets them before any code.
//  3. **Collaborators** — classes whose methods this code calls, most-called
//     first. Frequency is the honest proxy for how central one is.
//  4. **Exceptions** — a CX_* contract is almost never what a reader needs, and
//     it is the first thing to drop when the budget runs out.
//
// Custom code outranks standard within each band, because a reader plausibly
// knows CL_ABAP_TYPEDESCR and cannot know ZCL_VSP_GIT_SERVICE.

import (
	"regexp"
	"sort"
	"strings"
)

// DepRole is what a dependency is to the code that names it.
type DepRole int

const (
	// RoleUnclassified is the zero value, and it exists so that the zero value
	// is not a meaningful role. Without it RoleObligation was zero, every
	// dependency started life classified as a superclass, and the guard that
	// read `role != RoleObligation` never fired — so nothing was ever
	// recognised as a signature type.
	RoleUnclassified DepRole = iota
	// RoleObligation is a superclass or an implemented interface.
	RoleObligation
	// RoleSignature is a type named in the public API.
	RoleSignature
	// RoleCollaborator is something this code calls or instantiates.
	RoleCollaborator
	// RoleException is an exception class.
	RoleException
)

func (r DepRole) String() string {
	switch r {
	case RoleUnclassified:
		return "unclassified"
	case RoleObligation:
		return "obligation"
	case RoleSignature:
		return "signature"
	case RoleCollaborator:
		return "collaborator"
	case RoleException:
		return "exception"
	}
	return "unknown"
}

// RankedDep is a dependency with what the ranking needs to order it.
type RankedDep struct {
	Dependency
	Role   DepRole
	Uses   int // how many lines mention it
	Custom bool
}

var (
	reInheritsFrom = regexp.MustCompile(`(?i)\bINHERITING\s+FROM\s+([A-Z_/0-9]+)`)
	reImplements   = regexp.MustCompile(`(?i)^\s*INTERFACES\s+([A-Z_/0-9]+)`)
)

// RankCandidates orders dependencies by what a reader needs first.
func RankCandidates(source string, deps []Dependency) []RankedDep {
	upper := strings.ToUpper(source)
	lines := strings.Split(upper, "\n")

	obligations := map[string]bool{}
	for _, m := range reInheritsFrom.FindAllStringSubmatch(upper, -1) {
		obligations[m[1]] = true
	}
	for _, line := range lines {
		if m := reImplements.FindStringSubmatch(line); m != nil {
			obligations[m[1]] = true
		}
	}

	// The public section ends where the protected or private one begins;
	// anything named before that is part of what a caller sees.
	publicEnd := len(lines)
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "PROTECTED SECTION") || strings.HasPrefix(t, "PRIVATE SECTION") {
			publicEnd = i
			break
		}
	}

	out := make([]RankedDep, 0, len(deps))
	for _, d := range deps {
		r := RankedDep{Dependency: d, Custom: isCustom(d.Name)}
		// Occurrences, not lines. Three calls on one line is three uses, and
		// counting lines made a busy collaborator indistinguishable from one
		// mentioned once.
		for i, line := range lines {
			n := strings.Count(line, d.Name)
			if n == 0 {
				continue
			}
			r.Uses += n
			if i < publicEnd {
				r.Role = RoleSignature
			}
		}
		switch {
		case obligations[d.Name]:
			r.Role = RoleObligation
		case isExceptionClass(d.Name):
			r.Role = RoleException
		case r.Role == RoleSignature:
			// keep it
		default:
			r.Role = RoleCollaborator
		}
		out = append(out, r)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return out[i].Role < out[j].Role
		}
		if out[i].Custom != out[j].Custom {
			return out[i].Custom
		}
		if out[i].Uses != out[j].Uses {
			return out[i].Uses > out[j].Uses
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// isExceptionClass reports whether a name is an exception class by SAP's own
// naming, which is the only thing available without reading it.
func isExceptionClass(name string) bool {
	n := strings.ToUpper(name)
	for _, p := range []string{"CX_", "ZCX_", "YCX_", "/ZCX_"} {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	// A namespaced exception: /BOBF/CX_FOO.
	if i := strings.Index(n[1:], "/"); strings.HasPrefix(n, "/") && i > 0 {
		return strings.HasPrefix(n[i+2:], "CX_")
	}
	return false
}
