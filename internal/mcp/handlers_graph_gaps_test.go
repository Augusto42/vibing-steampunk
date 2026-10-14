package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
)

func toolResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("the tool returned no content")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("the tool returned %T, expected text", result.Content[0])
	}
	return text.Text
}

// A boundary verdict is only as good as the package labels it rests on, and a
// failed TADIR lookup leaves a node with no label. graph.classify sends an
// unlabelled node to VerdictUnknown and never to VerdictViolation, so the error
// can only ever hide violations — which makes CLEAN the exact answer where the
// caveat matters most.

func TestAFailedPackageLookupSaysWhatItCostsTheVerdict(t *testing.T) {
	note := unresolvedPackageNote([]adt.Unsearched{
		{Object: "ZCL_DEMO_A", Reason: "operation 'RunQuery' (type F) is blocked by safety configuration"},
	})
	for _, want := range []string{"unknown rather than checked", "under-report violations", "ZCL_DEMO_A", "blocked by safety configuration"} {
		if !strings.Contains(note, want) {
			t.Fatalf("the note should carry %q:\n%s", want, note)
		}
	}
}

func TestAGraphWithEveryPackageResolvedSaysNothing(t *testing.T) {
	if note := unresolvedPackageNote(nil); note != "" {
		t.Fatalf("nothing was missed, so there is nothing to warn about, got %q", note)
	}
}

// Both output formats have to carry the caveat. The JSON one is what an agent
// reads, and appending prose to a JSON document would break the only reader it
// has, so the notes travel as a field.
func TestBoundaryNotesSurviveBothFormats(t *testing.T) {
	report := &graph.BoundaryReport{RootPackage: "$ZDEMO", CrossedPackages: map[string]int{}}
	const note = "1 of 3 objects could not be searched"

	jsonResult, err := formatBoundaryResult(report, "json", note)
	if err != nil {
		t.Fatal(err)
	}
	body := toolResultText(t, jsonResult)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("the json format must stay parseable: %v\n%s", err, body)
	}
	if !strings.Contains(body, note) {
		t.Fatalf("the caveat is missing from the json an agent reads:\n%s", body)
	}

	textResult, err := formatBoundaryResult(report, "text", note)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(toolResultText(t, textResult), note) {
		t.Fatalf("the caveat is missing from the text output")
	}
}

// A complete sweep must not grow an empty "notes": [] — the caveat has to stay
// rare enough to be worth reading.
func TestACompleteBoundarySweepAddsNoNotesField(t *testing.T) {
	report := &graph.BoundaryReport{RootPackage: "$ZDEMO", CrossedPackages: map[string]int{}}
	result, err := formatBoundaryResult(report, "json", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(toolResultText(t, result), "notes") {
		t.Fatalf("a clean sweep should say nothing extra:\n%s", toolResultText(t, result))
	}
}
