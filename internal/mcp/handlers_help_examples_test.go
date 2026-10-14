package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// The help is documentation that can be wrong in a way nothing notices, and it
// was: `revisions` was documented with object_type/object_name while the
// handler reads type/name, `text_pool` with name while the handler reads
// program_name, `compare_languages` with a list where the handler wants two
// separately named languages. Six of eleven documented examples were refused
// by parameter validation before any request left the process.
//
// The obvious test — "the help for X mentions X" — passed on all six, because
// the fallback overview names every action and so contains every word. What it
// checked was that a string was present somewhere, which is true of a help text
// that documents the wrong thing.
//
// So this one **executes** them. Every `SAP(...)` line in every help topic is
// parsed back out of the text and run through the same router a caller reaches,
// against a server pointed at example.invalid. What comes back is a transport
// failure, and a transport failure is the pass: it means the arguments were
// accepted and the handler got as far as trying. A refusal that names a missing
// parameter is the failure, and it names which topic wrote which name.
func TestEveryDocumentedExampleIsAcceptedByItsHandler(t *testing.T) {
	srv := serverForMode(t, "hyperfocused")

	topics := append(helpTopicsUnderTest(), "")
	seen := 0
	for _, topic := range topics {
		result, err := srv.handleUniversalTool(t.Context(), newRequest(map[string]any{
			"action": "help", "target": topic,
		}))
		if err != nil {
			t.Fatalf("help %q: %v", topic, err)
		}
		for _, ex := range parseSAPExamples(resultText(result)) {
			if ex.action == "help" || ex.action == "" {
				continue
			}
			seen++
			args := map[string]any{"action": ex.action}
			if ex.target != "" {
				args["target"] = ex.target
			}
			if ex.params != nil {
				args["params"] = ex.params
			}
			out, err := srv.handleUniversalTool(t.Context(), newRequest(args))
			if err != nil {
				continue // a transport error is what a live call would hit here
			}
			text := resultText(out)
			if reason, bad := looksLikeAParameterRefusal(text); bad {
				t.Errorf("help %q documents an example its handler refuses:\n  %s\n  -> %s",
					topic, ex.line, reason)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no examples were extracted, so this test asserts nothing")
	}
	t.Logf("executed %d documented examples", seen)
}

// helpTopicsUnderTest is the list of targets `help` answers with a topic of its
// own. Written out rather than derived, because deriving it from the same
// switch it checks would make the test agree with the code by construction.
func helpTopicsUnderTest() []string {
	return []string{
		"read", "edit", "create", "delete", "search", "query", "grep",
		"test", "analyze", "debug", "system", "rfc", "i18n", "revisions",
		"lint", "tips",
	}
}

type sapExample struct {
	line   string
	action string
	target string
	params map[string]any
}

// parseSAPExamples pulls every `SAP(action="x", ...)` call out of a help text.
//
// The params object is written as JSON in the help, deliberately: an example a
// reader can copy is an example a test can parse, and one that cannot be parsed
// here is one a caller has to retype by hand.
func parseSAPExamples(text string) []sapExample {
	var out []sapExample
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		// A topic's own heading is written in the same shape —
		// `SAP(action="lint") - Static analysis` — and dispatching it bare
		// reports the action as unroutable. The trailing text is what tells
		// them apart: an example is the whole line.
		if !strings.HasPrefix(trimmed, "SAP(action=") || !strings.HasSuffix(trimmed, ")") {
			continue
		}
		ex := sapExample{line: trimmed}
		ex.action = quotedAfter(trimmed, `action=`)
		ex.target = quotedAfter(trimmed, `target=`)
		if i := strings.Index(trimmed, "params="); i >= 0 {
			raw := balancedBraces(trimmed[i+len("params="):])
			if raw == "" {
				continue
			}
			if json.Unmarshal([]byte(raw), &ex.params) != nil {
				continue
			}
		}
		out = append(out, ex)
	}
	return out
}

// quotedAfter returns the double-quoted value that follows a key, or "".
func quotedAfter(s, key string) string {
	i := strings.Index(s, key+`"`)
	if i < 0 {
		return ""
	}
	rest := s[i+len(key)+1:]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

// balancedBraces returns the leading {...} of s, counting nesting so that a
// nested object does not end the match at its own closing brace.
func balancedBraces(s string) string {
	if !strings.HasPrefix(s, "{") {
		return ""
	}
	depth, inString, escaped := 0, false, false
	for i, r := range s {
		switch {
		case escaped:
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			inString = !inString
		case inString:
		case r == '{':
			depth++
		case r == '}':
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return ""
}

// looksLikeAParameterRefusal reports whether a result is the handler saying it
// was called wrongly, rather than the system saying no.
//
// Matched on the shapes the handlers actually produce, not on the word "error":
// a connection failure to example.invalid is also an error and is the expected
// outcome here.
func looksLikeAParameterRefusal(text string) (string, bool) {
	lower := strings.ToLower(text)
	for _, marker := range []string{
		" is required", " are required", "missing required",
		"no handler found", "unknown action", "unknown operation",
	} {
		if i := strings.Index(lower, marker); i >= 0 {
			return firstLineAround(text, i), true
		}
	}
	return "", false
}

func firstLineAround(text string, i int) string {
	start := strings.LastIndex(text[:i], "\n") + 1
	end := strings.Index(text[i:], "\n")
	if end < 0 {
		return strings.TrimSpace(text[start:])
	}
	return strings.TrimSpace(text[start : i+end])
}
