package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// The whole reason this file exists: a day of post-mortem work landed in the
// CLI and stayed invisible to an agent, because nothing connects the two
// surfaces automatically. These tests are the connection that can be checked
// without a SAP system — the types the router answers, the names the help text
// publishes, and the parameter spellings that were promised to keep working.

func analyzeHelpText(t *testing.T) string {
	t.Helper()
	result := handleHelp("analyze")
	if len(result.Content) == 0 {
		t.Fatal("the analyze help returned no content")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("the analyze help returned %T, expected text", result.Content[0])
	}
	return text.Text
}

func TestEveryDumpTypeIsDocumented(t *testing.T) {
	// A type that works and is documented nowhere is indistinguishable from a
	// type that does not exist — which is exactly how the CLI's dump work
	// stayed invisible for a day.
	help := analyzeHelpText(t)
	s := serverForMode(t, "hyperfocused")
	for name := range s.dumpAnalysisTypes() {
		if !strings.Contains(help, `"type": "`+name+`"`) {
			t.Errorf("analyze type %q is routed but never shown in SAP(action=\"help\", target=\"analyze\")", name)
		}
	}
}

func TestDumpTypesDoNotShadowAnotherRouter(t *testing.T) {
	// routeDumpsAction runs before routeAnalysisAction in the universal chain,
	// so a name claimed in both places would silently win here and the other
	// handler would never run again.
	s := serverForMode(t, "hyperfocused")
	analysisTypes := map[string]bool{
		"call_graph": true, "object_structure": true, "callers": true, "callees": true,
		"analyze_call_graph": true, "compare_call_graphs": true, "trace_execution": true,
		"check_boundaries": true, "graph_stats": true, "co_change": true, "impact": true,
		"where_used_config": true, "usage_examples": true, "health": true,
		"cr_history": true, "tr_boundaries": true, "cr_boundaries": true,
	}
	for name := range s.dumpAnalysisTypes() {
		if analysisTypes[name] {
			t.Errorf("analyze type %q is claimed by both the dumps router and the analysis router", name)
		}
	}
}

func TestRouteDumpsActionClaimsOnlyItsOwn(t *testing.T) {
	s := serverForMode(t, "hyperfocused")
	ctx := context.Background()

	// Nothing below reaches a handler, so nothing dials: the router decides on
	// the action and the type alone.
	if _, handled, _ := s.routeDumpsAction(ctx, "read", "CLAS", "ZCL_DEMO", map[string]any{"type": "list_dumps"}); handled {
		t.Error("the dumps router claimed action=read")
	}
	if _, handled, _ := s.routeDumpsAction(ctx, "analyze", "", "", map[string]any{"type": "call_graph"}); handled {
		t.Error("the dumps router claimed type=call_graph, which belongs to the analysis router")
	}
	if _, handled, _ := s.routeDumpsAction(ctx, "analyze", "", "", map[string]any{}); handled {
		t.Error("the dumps router claimed an analyze call with no type at all")
	}
}

func TestDumpFilterKeepsTheOldSpellingsWorking(t *testing.T) {
	// The old MCP path published exception_type, date_from and date_to. Renaming
	// them to match the CLI is right; breaking a caller that learned the old
	// names from an older tool listing is not.
	filter, notes, err := dumpFilterFrom(map[string]any{
		"exception_type": "CX_SY_ZERODIVIDE",
		"date_from":      "20260801",
		"date_to":        "20260803",
		"program":        "ZDEMO_POST",
		"user":           "TESTUSER",
		"max_results":    float64(25),
	})
	if err != nil {
		t.Fatalf("the old spellings were rejected: %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("unexpected notes: %v", notes)
	}
	if filter.ErrorType != "CX_SY_ZERODIVIDE" || filter.Program != "ZDEMO_POST" || filter.User != "TESTUSER" {
		t.Errorf("filter did not carry the names through: %+v", filter)
	}
	if filter.Limit != 25 {
		t.Errorf("max_results became %d, expected 25", filter.Limit)
	}
	if got := filter.From.Format("2006-01-02"); got != "2026-08-01" {
		t.Errorf("date_from became %s", got)
	}
	// A date-only upper bound means the whole of that day. Read as midnight it
	// silently drops the day the caller asked about, which looks like missing
	// data rather than a bug.
	if got := filter.To.Format("2006-01-02 15:04:05"); got != "2026-08-03 23:59:59" {
		t.Errorf("date_to became %s, expected the end of that day", got)
	}
}

func TestDumpFilterPrefersTheNewSpellings(t *testing.T) {
	filter, _, err := dumpFilterFrom(map[string]any{
		"error_type": "CALL_FUNCTION_NOT_REMOTE",
		"since":      "2026-08-01",
		"until":      "2026-08-02T12:00:00Z",
		"top":        float64(7),
	})
	if err != nil {
		t.Fatalf("dumpFilterFrom: %v", err)
	}
	if filter.ErrorType != "CALL_FUNCTION_NOT_REMOTE" || filter.Limit != 7 {
		t.Errorf("unexpected filter: %+v", filter)
	}
	// A full timestamp is taken as given: a caller who names an hour means that
	// hour, and stretching it to the end of the day would overrule them.
	if got := filter.To.UTC().Format(time.RFC3339); got != "2026-08-02T12:00:00Z" {
		t.Errorf("until became %s", got)
	}
}

func TestDumpFilterSaysWhatItCannotDo(t *testing.T) {
	// package was accepted by the old path and built into an OData $filter that
	// the dumps feed ignores, so it never filtered anything. Dropping it in
	// silence would keep the same lie alive in a new place.
	_, notes, err := dumpFilterFrom(map[string]any{"package": "$ZDEMO"})
	if err != nil {
		t.Fatalf("dumpFilterFrom: %v", err)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "$ZDEMO") {
		t.Errorf("a package filter passed without a word about it: %v", notes)
	}
}

func TestDumpFilterRejectsAnUnreadableDate(t *testing.T) {
	// Guessing at "last tuesday" would turn a caller's mistake into a silent
	// full-range scan of a busy system's dump feed.
	if _, _, err := dumpFilterFrom(map[string]any{"since": "last tuesday"}); err == nil {
		t.Error("an unparseable date was accepted")
	} else if !strings.Contains(err.Error(), "since") {
		t.Errorf("the error does not name the parameter: %v", err)
	}
}

func TestApplicationLogFilterNames(t *testing.T) {
	filter, err := appLogFilterFrom(map[string]any{
		"program":   "ZDEMO_POST",
		"user":      "TESTUSER",
		"object":    "ZDEMO_LOG",
		"subobject": "POST",
		"since":     "2026-08-01",
		"limit":     float64(20),
	})
	if err != nil {
		t.Fatalf("appLogFilterFrom: %v", err)
	}
	if filter.Program != "ZDEMO_POST" || filter.Object != "ZDEMO_LOG" || filter.SubObject != "POST" || filter.Limit != 20 {
		t.Errorf("unexpected filter: %+v", filter)
	}
	if filter.From.IsZero() {
		t.Error("since was dropped")
	}
}

func TestToleranceTakesADurationOrMinutes(t *testing.T) {
	cases := []struct {
		args map[string]any
		want time.Duration
	}{
		{map[string]any{}, 5 * time.Minute},
		{map[string]any{"tolerance": "90s"}, 90 * time.Second},
		{map[string]any{"tolerance_minutes": float64(30)}, 30 * time.Minute},
		// A duration nobody can parse falls back to the default rather than to
		// zero, which CorrelateDump would read as "use the default" anyway —
		// but a zero window looks deliberate in a log and is not.
		{map[string]any{"tolerance": "half an hour"}, 5 * time.Minute},
	}
	for _, c := range cases {
		if got := toleranceFrom(c.args); got != c.want {
			t.Errorf("toleranceFrom(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestDeepBudgetIsBoundedAndCanBeZero(t *testing.T) {
	if got := deepBudgetFrom(map[string]any{}); got != 10 {
		t.Errorf("default budget is %d, expected 10", got)
	}
	// Zero is a real answer: rungs 2 and 4 come out of the feed, so a caller who
	// wants no detail fetches still gets a ladder.
	if got := deepBudgetFrom(map[string]any{"deep": float64(0)}); got != 0 {
		t.Errorf("deep=0 became %d", got)
	}
	if got := deepBudgetFrom(map[string]any{"deep": float64(-4)}); got != 0 {
		t.Errorf("a negative budget became %d, expected 0", got)
	}
}

func TestDumpSelectorDefaultsToLatest(t *testing.T) {
	if got := dumpSelectorFrom(map[string]any{}); got != "latest" {
		t.Errorf("no dump_id became %q, expected latest", got)
	}
	if got := dumpSelectorFrom(map[string]any{"dump": "20260823"}); got != "20260823" {
		t.Errorf("dump alias was ignored: %q", got)
	}
}

func TestParseDumpDateSpellings(t *testing.T) {
	for _, raw := range []string{"2026-08-01", "20260801", "2026-08-01T06:30:00Z", "2026-08-01 06:30:00"} {
		if _, err := parseDumpDate(raw, false); err != nil {
			t.Errorf("parseDumpDate(%q): %v", raw, err)
		}
	}
	if _, err := parseDumpDate("2026-13-99", false); err == nil {
		t.Error("an impossible date parsed")
	}
}

// Answerable() is all-or-nothing: one unit that came back makes it true, and
// the "not a finding of zero callers" caveat then stays silent. But "exposed"
// is the union over units, so a unit whose where-used list failed subtracts
// callers from the headline list without subtracting anything from the
// reader's confidence in it.
func TestAPartlyAnsweredImpactStillNamesWhatItCouldNotAsk(t *testing.T) {
	result := &adt.DumpImpactResult{Units: []adt.ImpactUnit{
		{Object: "ZCL_DEMO_OK"},
		{Object: "ZCL_DEMO_DENIED", Err: "ADT API error: status 403"},
		{Object: "ZCL_DEMO_LOCAL", Note: "a local class has no where-used list"},
	}}
	if !result.Answerable() {
		t.Fatal("one unit answered, so the all-or-nothing caveat does not fire — that is the gap this covers")
	}

	unanswered := unansweredUnits(result)
	if len(unanswered) != 2 {
		t.Fatalf("both the error and the note leave a hole in the answer, got %d: %v", len(unanswered), unanswered)
	}
	joined := strings.Join(unanswered, "; ")
	for _, want := range []string{"ZCL_DEMO_DENIED", "403", "ZCL_DEMO_LOCAL", "no where-used list"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("the reader needs %q to know what to do next:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "ZCL_DEMO_OK") {
		t.Fatalf("a unit that answered is not a gap:\n%s", joined)
	}
}

// Every unit answering means nothing to disclose.
func TestAFullyAnsweredImpactHasNoGaps(t *testing.T) {
	result := &adt.DumpImpactResult{Units: []adt.ImpactUnit{{Object: "ZCL_DEMO_OK"}}}
	if got := unansweredUnits(result); len(got) != 0 {
		t.Fatalf("nothing was missed, got %v", got)
	}
}
