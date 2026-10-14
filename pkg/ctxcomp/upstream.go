package ctxcomp

// Who calls this, which the source cannot say.
//
// Everything else in this package reads downward: the source names what it
// calls, and the contracts of those things are fetched and trimmed. That is the
// whole of what a text can tell you, and it is half the picture.
//
// A unit does not know its callers. Nothing in a class's source says that
// seventy-three other classes depend on it, or that the one caller is a test.
// Only the repository knows, and knowing changes how the code should be read:
//
//   - **Blast radius.** A method called from one place can be changed. The same
//     method called from 3,760 places across 400 packages cannot, and no amount
//     of reading the source reveals which one it is.
//   - **Real usage.** Callers name the *method* they call from, so a reader
//     sees which parts of the surface are actually used and which are dead
//     weight.
//   - **Ownership.** Callers concentrated in one package mean a private
//     collaborator; callers spread across forty mean a published interface,
//     whatever the class says about itself.
//
// The measurement that shaped this: 73 callers of a normal class come back in
// 0.6 s and all 73 name the calling method; a hub class has 3,760 and takes
// 3.9 s. So this is a **summary with a few examples**, never a list — 3,760
// names is not context — and it is opt-in, because four seconds is not a price
// a plain read should pay without being asked.

import (
	"fmt"
	"sort"
	"strings"
)

// Caller is one place that references the object being read.
type Caller struct {
	Name      string
	Type      string
	Component string // the method or routine holding the reference
	Package   string
	IsTest    bool
}

// UpstreamSummary is what a reader needs to know about who calls this.
type UpstreamSummary struct {
	Total    int
	Packages int
	Tests    int
	// Examples are a few callers worth naming, ranked. Never all of them.
	Examples []Caller
	// Truncated says how many were not named, so the summary cannot be read as
	// the whole list.
	Truncated int
}

// SummariseCallers builds the summary, naming at most maxExamples.
//
// Ranking, in order: custom code before standard, because a reader knows what
// CL_GUI_ALV_GRID is and cannot know what ZCL_VSP_GIT_SERVICE is; then one
// caller per package before a second from the same one, because forty callers
// in one package say the same thing forty times; then non-test before test.
func SummariseCallers(callers []Caller, maxExamples int) UpstreamSummary {
	if maxExamples <= 0 {
		maxExamples = 5
	}
	sum := UpstreamSummary{Total: len(callers)}

	pkgs := map[string]bool{}
	for _, c := range callers {
		if c.Package != "" {
			pkgs[c.Package] = true
		}
		if c.IsTest {
			sum.Tests++
		}
	}
	sum.Packages = len(pkgs)

	ranked := make([]Caller, len(callers))
	copy(ranked, callers)
	sort.SliceStable(ranked, func(i, j int) bool {
		iCustom, jCustom := isCustom(ranked[i].Name), isCustom(ranked[j].Name)
		if iCustom != jCustom {
			return iCustom
		}
		if ranked[i].IsTest != ranked[j].IsTest {
			return !ranked[i].IsTest
		}
		return ranked[i].Name < ranked[j].Name
	})

	seenPkg := map[string]bool{}
	for _, c := range ranked {
		if len(sum.Examples) >= maxExamples {
			break
		}
		// One per package first; a second from the same package says nothing
		// new about who depends on this.
		if c.Package != "" && seenPkg[c.Package] {
			continue
		}
		seenPkg[c.Package] = true
		sum.Examples = append(sum.Examples, c)
	}
	sum.Truncated = sum.Total - len(sum.Examples)
	return sum
}

// Text renders the summary as prologue lines, or empty when there is nothing to
// say.
//
// "No callers" is said out loud rather than omitted: for a class that exists to
// be called, it is the most interesting sentence on the page, and silence would
// read as "not asked".
func (u UpstreamSummary) Text() string {
	var sb strings.Builder
	if u.Total == 0 {
		return "* Called from nowhere the repository records — dead, or reached only dynamically or through an interface.\n"
	}

	sb.WriteString(fmt.Sprintf("* === Called from %d place(s)", u.Total))
	if u.Packages > 0 {
		sb.WriteString(fmt.Sprintf(" in %d package(s)", u.Packages))
	}
	if u.Tests > 0 {
		sb.WriteString(fmt.Sprintf(", %d of them tests", u.Tests))
	}
	sb.WriteString(" ===\n")

	for _, c := range u.Examples {
		line := "*   " + c.Name
		if c.Component != "" {
			line += " → " + c.Component
		}
		if c.Package != "" {
			line += "  [" + c.Package + "]"
		}
		if c.IsTest {
			line += "  (test)"
		}
		sb.WriteString(line + "\n")
	}
	if u.Truncated > 0 {
		sb.WriteString(fmt.Sprintf("*   … and %d more, not listed\n", u.Truncated))
	}
	return sb.String()
}
