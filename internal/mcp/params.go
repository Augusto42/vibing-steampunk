package mcp

// Naming a parameter the caller did not use.
//
// The universal tool chains routes and each one answers "not mine" by returning
// handled=false. That is right for an action nobody claims. It is wrong for an
// action a route *does* claim but whose parameters it did not recognise,
// because the chain then runs out and the caller is told
//
//	No handler found for action="grep".
//
// which is false. The action exists, is documented, and works. What was wrong
// was one key. An agent reading that message concludes the capability is
// missing and stops asking — which is how `query` and `grep` came to be
// reported dead by a sweep against a build where both were fine.
//
// Two rules follow, and this file exists to make them cheap to obey:
//
//  1. **A route that recognises the action owns the answer.** Once it has
//     decided the action is its own, it never returns handled=false. It either
//     runs, or it says what it needed.
//  2. **The obvious name is accepted.** `sql` for `sql_query`, `package` for
//     `package_name`, `object` for `object_url` — these are the names the CLI
//     flags use and the names a person writes first. Rejecting them teaches
//     nothing; accepting them costs a line.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// firstParam returns the first of keys present and non-empty in params, so a
// handler can accept several spellings of the same argument.
func firstParam(params map[string]any, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(getStringParam(params, k)); v != "" {
			return v
		}
	}
	return ""
}

// hasAnyParam reports whether any of keys is present at all, whatever its type.
// Used where the value may be a list rather than a string.
func hasAnyParam(params map[string]any, keys ...string) bool {
	for _, k := range keys {
		if _, ok := params[k]; ok {
			return true
		}
	}
	return false
}

// needParams builds the answer for a call that named a real action and did not
// carry what it needs.
//
// It lists what arrived, because the caller's own mistake is usually visible in
// that list and a message that only states the requirement leaves them
// comparing two things in their head. And it ends in a call that works, because
// an example is the shortest correct documentation there is.
func needParams(action string, params map[string]any, accepted []string, example string) *mcp.CallToolResult {
	// Keys *and* short values. A caller who mistyped the value — op:
	// "no_such_operation" rather than op: "texts" — is told which keys arrived,
	// which tells them nothing they did not already know. The value is the part
	// they got wrong and the part they need to see.
	//
	// Short only: a params map can carry a whole ABAP source, and echoing it
	// back turns an error message into a wall.
	got := make([]string, 0, len(params))
	for k, v := range params {
		if sv, ok := v.(string); ok && sv != "" && len(sv) <= 40 && !strings.ContainsAny(sv, "\n\r") {
			got = append(got, fmt.Sprintf("%s=%q", k, sv))
			continue
		}
		got = append(got, k)
	}
	sort.Strings(got)

	var b strings.Builder
	fmt.Fprintf(&b, "action=%q needs one of: %s.\n", action, strings.Join(accepted, ", "))
	if len(got) == 0 {
		b.WriteString("No parameters were passed.\n")
	} else {
		fmt.Fprintf(&b, "Passed: %s.\n", strings.Join(got, ", "))
	}
	fmt.Fprintf(&b, "\n%s\n", example)
	b.WriteString("\nSAP(action=\"help\", target=\"" + action + "\") lists every form.")
	return newToolResultError(b.String())
}

// copyParams returns a shallow copy, so a route may normalise an alias into the
// name its handler reads without editing the caller's map.
func copyParams(params map[string]any) map[string]any {
	out := make(map[string]any, len(params)+1)
	for k, v := range params {
		out[k] = v
	}
	return out
}
