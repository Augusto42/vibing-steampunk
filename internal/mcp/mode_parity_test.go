package mcp

import (
	"strings"
	"testing"
)

// The universal router used to be registered only in hyperfocused mode. Since
// every analyze type is routed through it and registered as a tool nowhere,
// agents in focused and expert could not reach a single one of them — eight
// analysis handlers, every post-mortem type, every AMDP target. Two of three
// modes advertised a surface missing a third of itself.
func TestEveryModeCanReachTheAnalyzeSurface(t *testing.T) {
	for _, mode := range []string{"hyperfocused", "focused", "expert"} {
		t.Run(mode, func(t *testing.T) {
			srv := serverForMode(t, mode)
			var found bool
			for _, n := range srv.RegisteredTools() {
				if n == "SAP" {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s does not register SAP, so none of the %d analyze types can be called from it",
					mode, len(srv.AnalyzeTypes()))
			}
		})
	}
}

// Registration is not reachability. This drives an actual call through the
// dispatch an agent uses, in the mode that could not do it before, and asks for
// something answerable without a system.
func TestAnalyzeAnswersInFocusedMode(t *testing.T) {
	srv := serverForMode(t, "focused")
	result, err := srv.handleUniversalTool(t.Context(), newRequest(map[string]any{
		"action": "analyze",
		"params": map[string]any{"type": "parse_abap", "source": "REPORT zdemo.\nWRITE 'x'.\n"},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(result)
	if strings.Contains(text, "No handler found") {
		t.Fatalf("focused mode still cannot route analyze:\n%s", text)
	}
	if strings.TrimSpace(text) == "" {
		t.Fatal("the offline parser answered with nothing")
	}
}

// Hyperfocused must stay exactly one tool. The fix registers SAP everywhere,
// and the way to get that wrong is to also let the individual tools in.
func TestHyperfocusedIsStillOnlyTheUniversalTool(t *testing.T) {
	names := serverForMode(t, "hyperfocused").RegisteredTools()
	if len(names) != 1 || names[0] != "SAP" {
		t.Errorf("hyperfocused registers %v, want exactly [SAP]", names)
	}
}

// A deployment that does not want the universal tool must still be able to
// switch it off, which is why it goes through shouldRegister rather than being
// special-cased.
func TestTheUniversalToolCanBeSwitchedOffByName(t *testing.T) {
	srv := NewServer(&Config{
		BaseURL:     "https://example.invalid",
		Mode:        "expert",
		ToolsConfig: map[string]bool{"SAP": false},
	})
	for _, n := range srv.RegisteredTools() {
		if n == "SAP" {
			t.Fatal("SAP was registered despite being disabled by name in the tool config")
		}
	}
}
