package mcp

import (
	"strings"
	"testing"
)

// The reach pass is the half of the sweep that needs no SAP system, so it runs
// here on every build. Ten gCTS tools were whitelisted for months behind a
// registration function nothing called; this is the assertion that would have
// caught them on the day.
func TestReachSweepFindsNothingUnreachable(t *testing.T) {
	for _, f := range SweepReach() {
		t.Errorf("%s: %s\n  %s", f.Verdict, f.Capability, f.Detail)
	}
}

// Every probe must name a capability the server can actually route, or the
// sweep would report a defect in the product that is really a typo in the
// probe table — and the next reader would go looking for a bug that is not
// there.
func TestEveryProbeTargetsARoutedCapability(t *testing.T) {
	srv := serverForMode(t, "expert")
	routed := map[string]bool{}
	for _, typ := range srv.AnalyzeTypes() {
		routed[typ] = true
	}
	for _, p := range SweepProbes() {
		if p.Action != "analyze" {
			continue
		}
		typ, _ := p.Params["type"].(string)
		if typ == "" {
			t.Errorf("probe %s uses action=analyze with no type", p.ID)
			continue
		}
		if !routed[typ] {
			t.Errorf("probe %s targets analyze type=%q, which no router claims", p.ID, typ)
		}
	}
}

// A sweep that mutated a system to verify it would be worse than no sweep. The
// rule is enforced in runProbe; this asserts the probe table never asks it to.
func TestNoProbeWrites(t *testing.T) {
	for _, p := range SweepProbes() {
		if writeActions[strings.ToLower(p.Action)] {
			t.Errorf("probe %s uses the write action %q; the sweep is read-only", p.ID, p.Action)
		}
	}
}

// Probe ids are how two runs are diffed, so a duplicate silently merges two
// capabilities into one row.
func TestProbeIDsAreUnique(t *testing.T) {
	seen := map[string]string{}
	for _, p := range SweepProbes() {
		if prev, dup := seen[p.ID]; dup {
			t.Errorf("probe id %q used by both %q and %q", p.ID, prev, p.Capability)
		}
		seen[p.ID] = p.Capability
	}
}

// The capabilities that returned an empty list for a year are exactly the ones
// where an empty answer must not be accepted as an answer. Each needs either an
// oracle or an explicit statement that empty is ordinary here — and the four
// below get an oracle, because for them it never is.
func TestCapabilitiesThatShippedDeadCarryAnOracle(t *testing.T) {
	mustHaveOracle := map[string]bool{
		"analyze type=callers":          true,
		"analyze type=callees":          true,
		"analyze type=call_graph":       true,
		"analyze type=object_structure": true,
		"analyze type=usage_examples":   true,
	}
	found := map[string]bool{}
	for _, p := range SweepProbes() {
		if !mustHaveOracle[p.Capability] {
			continue
		}
		found[p.Capability] = true
		if p.Oracle == nil {
			t.Errorf("%s shipped dead once and its probe has no oracle, so an empty answer would pass", p.Capability)
		}
	}
	for cap := range mustHaveOracle {
		if !found[cap] {
			t.Errorf("%s has no probe at all", cap)
		}
	}
}

// A probe with neither an oracle nor EmptyIsFine reports "no second route to
// say whether that is true", which is honest but weak. Keeping the count
// visible stops the table drifting into one where most answers are
// unfalsifiable.
func TestUnfalsifiableProbesAreNamed(t *testing.T) {
	var weak []string
	for _, p := range SweepProbes() {
		if p.Oracle == nil && !p.EmptyIsFine && p.MustContain == "" {
			weak = append(weak, p.ID)
		}
	}
	if len(weak) > 2 {
		t.Errorf("%d probes can neither prove nor disprove an empty answer: %v\n"+
			"give each one an oracle, a MustContain, or EmptyIsFine", len(weak), weak)
	}
}

// The report must state its own coverage. A clean result over a third of the
// surface, printed without the denominator, is the health report that said
// GOOD over a scan that never ran.
func TestReportTextStatesCoverage(t *testing.T) {
	r := &SweepReport{
		System:     "example",
		Advertised: 31,
		Unprobed:   []string{"analyze type=trace_execution"},
		Answer:     []SweepFinding{{ID: "x", Capability: "action=read", Verdict: VerdictAnswered}},
		Live:       true,
	}
	text := r.Text()
	for _, want := range []string{"Coverage", "1 capabilities probed of 31", "analyze type=trace_execution"} {
		if !strings.Contains(text, want) {
			t.Errorf("the report does not say %q:\n%s", want, text)
		}
	}
}

// A dead verdict must read as a defect and carry the count it should have
// returned, because "no results" is what it looked like for a year.
func TestDeadVerdictNamesTheOracleCount(t *testing.T) {
	r := &SweepReport{
		System: "example",
		Live:   true,
		Answer: []SweepFinding{{
			ID: "graph.callers", Capability: "analyze type=callers", Verdict: VerdictDead,
			Detail:      "returned nothing while the where-used list says there are 12",
			OracleCount: 12, OracleHow: "the where-used list",
		}},
	}
	text := r.Text()
	if !strings.Contains(text, "dead") || !strings.Contains(text, "12") {
		t.Errorf("a dead capability is not reported as one:\n%s", text)
	}
	if len(r.Findings()) != 1 {
		t.Errorf("a dead capability is not counted as a finding")
	}
}

// Empty, refused, absent and skipped are facts about the system or the probe,
// not defects. Counting them as findings would bury the real ones.
func TestOnlyRealDefectsCountAsFindings(t *testing.T) {
	for _, v := range []Verdict{VerdictAnswered, VerdictEmpty, VerdictRefused, VerdictAbsent, VerdictSkipped, VerdictMisprobed} {
		if v.Bad() {
			t.Errorf("%s should not count as a finding", v)
		}
	}
	for _, v := range []Verdict{VerdictDead, VerdictBroken, VerdictUnreachable} {
		if !v.Bad() {
			t.Errorf("%s must count as a finding", v)
		}
	}
	// The sweep's own gaps are reported and never counted against the product.
	for _, v := range []Verdict{VerdictMisprobed, VerdictSkipped} {
		if !v.OurFault() {
			t.Errorf("%s is the sweep's fault and must be marked as such", v)
		}
	}
}

// A handler that names the parameter it wanted has answered the call. Reading
// that as a defect is how a findings list fills with the sweep's own mistakes
// and stops being read.
func TestAUsageRejectionIsTheProbesFaultNotTheProducts(t *testing.T) {
	for _, text := range []string{
		"object_type and object_name are required. Example: SAP(action=\"analyze\", ...)",
		"transports is required (comma-separated TR numbers)",
		"action=\"system\" needs a target.",
		"Provide 'source' parameter with ABAP source code",
	} {
		got, _ := classifyError(text)
		if got != VerdictMisprobed {
			t.Errorf("classifyError(%q) = %s, want %s", text, got, VerdictMisprobed)
		}
		if got.Bad() {
			t.Errorf("%s must not count as a finding about the product", got)
		}
	}
}

// looksEmpty decides whether an oracle is consulted at all, so a phrase it
// fails to recognise means a dead capability is reported as answered.
func TestLooksEmptyRecognisesWhatHandlersActuallySay(t *testing.T) {
	empty := []string{
		"", "  ", "[]", "{}",
		"No results found",
		"no callers found for ZCL_X",
		"Found 0 references",
		"no output captured",
	}
	for _, s := range empty {
		if !looksEmpty(s) {
			t.Errorf("looksEmpty(%q) = false; a dead capability saying this would pass", s)
		}
	}
	answers := []string{
		"CLASS zcl_demo DEFINITION.",
		"3 callers:\n  ZCL_A\n  ZCL_B",
		`{"nodes":[{"name":"ZCL_A"}]}`,
	}
	for _, s := range answers {
		if looksEmpty(s) {
			t.Errorf("looksEmpty(%q) = true; a real answer would be sent to an oracle", s)
		}
	}
}

// A 404 is a statement about the release and a "no handler found" is a
// statement about us. Reading one as the other is how a routing gap gets
// filed as a missing SAP feature.
func TestErrorsAreClassifiedByWhoseFaultTheyAre(t *testing.T) {
	cases := map[string]Verdict{
		"no handler found for action analyze":      VerdictUnreachable,
		"HTTP 404: resource not found":             VerdictAbsent,
		"HTTP 403: not authorized for this object": VerdictRefused,
		"operation blocked: system is read-only":   VerdictRefused,
		"unmarshal: unexpected end of XML input":   VerdictBroken,
	}
	for text, want := range cases {
		if got, _ := classifyError(text); got != want {
			t.Errorf("classifyError(%q) = %s, want %s", text, got, want)
		}
	}
}

// A probe that names a target kind it never uses would be skipped forever on a
// system missing that kind, and its capability never checked.
func TestProbesOnlyRequireTargetsTheyUse(t *testing.T) {
	for _, p := range SweepProbes() {
		blob := p.Target
		for _, v := range p.Params {
			if s, ok := v.(string); ok {
				blob += " " + s
			}
		}
		for _, need := range p.Needs {
			// {references} and {references_uri} are both uses of the same
			// target: one is the name, the other the escaped ADT path built
			// from it for the two types that accept only a URI.
			if !strings.Contains(blob, "{"+need+"}") && !strings.Contains(blob, "{"+need+"_uri}") {
				t.Errorf("probe %s requires a %s and never substitutes {%s}", p.ID, need, need)
			}
		}
		// The other half of the same rule. Requires is for preconditions, and a
		// kind that IS interpolated belongs in Needs, where the check above can
		// see it — otherwise the escape hatch quietly becomes the default.
		for _, req := range p.Requires {
			if strings.Contains(blob, "{"+req+"}") || strings.Contains(blob, "{"+req+"_uri}") {
				t.Errorf("probe %s lists %s under Requires but substitutes it; that is a Need", p.ID, req)
			}
		}
	}
}
