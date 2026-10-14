package mcp

import (
	"strings"
	"testing"
)

// The universal tool answers a call it cannot route with "No handler found for
// action=X". For an action nothing claims that is true. For one a route does
// claim — and only failed to recognise a parameter — it is false, and it is the
// worst kind of false: an agent told the capability does not exist stops asking
// about it. `query` and `grep` were both reported dead by a sweep against a
// build where both worked.
func TestARecognisedActionNeverReportsItselfMissing(t *testing.T) {
	srv := serverForMode(t, "expert")
	cases := []struct {
		name   string
		args   map[string]any
		expect []string
	}{
		{
			name: "query with no usable parameter",
			args: map[string]any{"action": "query", "params": map[string]any{"table": "T000"}},
			// It must name what it needs and what arrived, and never deny itself.
			expect: []string{"sql_query", "Passed: table"},
		},
		{
			name:   "grep with no target",
			args:   map[string]any{"action": "grep", "params": map[string]any{"pattern": "SELECT"}},
			expect: []string{"package_name", "object_url", "Passed: pattern"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := srv.handleUniversalTool(t.Context(), newRequest(c.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			text := resultText(result)
			if strings.Contains(text, "No handler found") {
				t.Errorf("the action denied its own existence:\n%s", text)
			}
			for _, want := range c.expect {
				if !strings.Contains(text, want) {
					t.Errorf("the answer does not say %q:\n%s", want, text)
				}
			}
		})
	}
}

// An action nothing claims must still say so — the fix above must not turn
// every typo into a helpful message about some unrelated route.
func TestAnUnknownActionIsStillUnknown(t *testing.T) {
	srv := serverForMode(t, "expert")
	result, err := srv.handleUniversalTool(t.Context(), newRequest(map[string]any{"action": "levitate"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text := resultText(result); !strings.Contains(strings.ToLower(text), "levitate") {
		t.Errorf("an unknown action must be named back to the caller:\n%s", text)
	}
}

// The names a caller writes first are the CLI's own flag names. Accepting only
// the internal spelling is what sent both calls down the chain.
func TestTheObviousParameterNameIsAccepted(t *testing.T) {
	params := map[string]any{"sql": "SELECT * FROM T000", "package": "$TMP"}
	if got := firstParam(params, "sql_query", "sql"); got != "SELECT * FROM T000" {
		t.Errorf("firstParam did not accept `sql`: %q", got)
	}
	if got := firstParam(params, "package_name", "package"); got != "$TMP" {
		t.Errorf("firstParam did not accept `package`: %q", got)
	}
	if got := firstParam(params, "absent", "missing"); got != "" {
		t.Errorf("firstParam invented a value: %q", got)
	}
}

// needParams must not edit the caller's map, and copyParams is what guarantees
// it — a route normalising an alias otherwise mutates what it was handed.
func TestNormalisingAnAliasDoesNotEditTheCallersMap(t *testing.T) {
	original := map[string]any{"package": "$TMP", "pattern": "X"}
	copied := copyParams(original)
	copied["package_name"] = "$TMP"
	if _, leaked := original["package_name"]; leaked {
		t.Error("normalising the alias wrote back into the caller's parameters")
	}
}
