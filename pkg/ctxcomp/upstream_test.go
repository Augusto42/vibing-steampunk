package ctxcomp

import (
	"strings"
	"testing"
)

// 3,760 names is not context. The summary is what a reader can use: how many,
// across how many packages, how many of those are tests — and a handful of
// examples chosen to say different things from each other.
func TestAHubIsSummarisedRatherThanListed(t *testing.T) {
	var callers []Caller
	for i := 0; i < 300; i++ {
		callers = append(callers, Caller{
			Name: "CL_STANDARD_" + string(rune('A'+i%26)), Package: "SPKG", IsTest: true,
		})
	}
	sum := SummariseCallers(callers, 5)
	if sum.Total != 300 {
		t.Errorf("Total = %d, want 300", sum.Total)
	}
	if len(sum.Examples) > 5 {
		t.Errorf("%d examples named; the cap is 5", len(sum.Examples))
	}
	if sum.Truncated != 300-len(sum.Examples) {
		t.Errorf("Truncated = %d; the count must say how many were not named", sum.Truncated)
	}
	if !strings.Contains(sum.Text(), "not listed") {
		t.Error("the text does not say that names were left out, so it reads as the whole list")
	}
}

// One caller per package before a second from the same one: forty callers in
// one package say the same thing forty times, and the budget for examples is
// better spent saying forty different things.
func TestExamplesPreferDistinctPackages(t *testing.T) {
	callers := []Caller{
		{Name: "ZCL_A1", Package: "$ZONE"},
		{Name: "ZCL_A2", Package: "$ZONE"},
		{Name: "ZCL_A3", Package: "$ZONE"},
		{Name: "ZCL_B1", Package: "$ZTWO"},
		{Name: "ZCL_C1", Package: "$ZTHREE"},
	}
	sum := SummariseCallers(callers, 3)
	seen := map[string]bool{}
	for _, e := range sum.Examples {
		if seen[e.Package] {
			t.Errorf("two examples from %s; a second says nothing new", e.Package)
		}
		seen[e.Package] = true
	}
	if sum.Packages != 3 {
		t.Errorf("Packages = %d, want 3", sum.Packages)
	}
}

// Custom before standard, and real callers before tests: a reader knows what
// CL_GUI_ALV_GRID is and cannot know what ZCL_VSP_GIT_SERVICE is, and a test
// caller says less about how the code is used in anger.
func TestCustomAndNonTestComeFirst(t *testing.T) {
	callers := []Caller{
		{Name: "CL_STANDARD", Package: "SPKG"},
		{Name: "ZCL_CUSTOM_TEST", Package: "$ZA", IsTest: true},
		{Name: "ZCL_CUSTOM_REAL", Package: "$ZB"},
	}
	sum := SummariseCallers(callers, 3)
	if sum.Examples[0].Name != "ZCL_CUSTOM_REAL" {
		t.Errorf("first example is %s; the custom non-test caller should lead", sum.Examples[0].Name)
	}
	if sum.Tests != 1 {
		t.Errorf("Tests = %d, want 1", sum.Tests)
	}
}

// For a class that exists to be called, "nobody calls this" is the most
// interesting sentence available, and silence would read as "not asked".
func TestNoCallersIsSaidOutLoud(t *testing.T) {
	text := SummariseCallers(nil, 5).Text()
	if text == "" {
		t.Fatal("an object with no callers produced no line at all")
	}
	for _, want := range []string{"nowhere", "dynamically", "interface"} {
		if !strings.Contains(text, want) {
			t.Errorf("the line does not mention %q, so a reader cannot tell it apart from a real absence: %q", want, text)
		}
	}
}

// The calling method is the part a source can never supply, so it must survive
// into the rendering.
func TestTheCallingMethodIsNamed(t *testing.T) {
	sum := SummariseCallers([]Caller{
		{Name: "ZCL_CALLER", Component: "SAVE_ORDER", Package: "$ZDEMO"},
	}, 5)
	text := sum.Text()
	for _, want := range []string{"ZCL_CALLER", "SAVE_ORDER", "$ZDEMO"} {
		if !strings.Contains(text, want) {
			t.Errorf("the rendering drops %q: %q", want, text)
		}
	}
}
