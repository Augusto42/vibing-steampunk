package adt

import (
	"net/url"
	"strings"
	"testing"
)

func TestCompatChecksAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	quick := 0
	for _, c := range CompatChecks() {
		if seen[c.ID] {
			t.Errorf("duplicate check id %q — two systems could not be diffed on it", c.ID)
		}
		seen[c.ID] = true
		if c.Purpose == "" {
			t.Errorf("%s has no purpose; a report nobody can read is not a report", c.ID)
		}
		if len(c.Accepts) == 0 {
			t.Errorf("%s offers no content type", c.ID)
		}
		if c.Quick {
			quick++
		}
	}
	if quick == 0 {
		t.Error("no check is marked quick, so the quick pass would ask nothing")
	}
	if quick == len(CompatChecks()) {
		t.Error("every check is quick, so the two depths are the same probe")
	}
}

func TestFillTargets(t *testing.T) {
	targets := CompatTargets{FunctionGroup: "ZDEMO_FG", Class: "ZCL_DEMO", Program: "ZDEMO_PROG", Package: "$TMP"}

	got, ok := fillTargets("/sap/bc/adt/functions/groups/{group}/objectstructure", targets)
	if !ok || got != "/sap/bc/adt/functions/groups/ZDEMO_FG/objectstructure" {
		t.Errorf("got (%q, %v)", got, ok)
	}

	// A package name starting with $ has to survive into the path.
	got, ok = fillTargets("/sap/bc/adt/packages/{package}", targets)
	if !ok || !strings.HasSuffix(got, "TMP") {
		t.Errorf("got (%q, %v)", got, ok)
	}

	// A check whose target is unknown must not run. Reporting it as absent
	// would put a missing feature on the system's record for the probe's fault.
	if _, ok := fillTargets("/sap/bc/adt/oo/classes/{class}", CompatTargets{}); ok {
		t.Error("a check ran without the object it needs")
	}

	// A path with no placeholder always runs.
	if _, ok := fillTargets("/sap/bc/adt/discovery", CompatTargets{}); !ok {
		t.Error("a check needing no target was skipped")
	}
}

func TestFillQueryTargetsDoesNotDoubleEncode(t *testing.T) {
	q := url.Values{}
	q.Set("parent_name", "{group}")
	q.Set("parent_type", "FUGR/F")

	got, ok := fillQueryTargets(q, CompatTargets{FunctionGroup: "ZDEMO_FG"})
	if !ok {
		t.Fatal("query was skipped")
	}
	// The encoder escapes query values; escaping here too turns FUGR/F into
	// FUGR%252FF and the server answers about a type that does not exist.
	if got.Get("parent_type") != "FUGR/F" {
		t.Errorf("parent_type = %q, want it unescaped", got.Get("parent_type"))
	}
	if got.Get("parent_name") != "ZDEMO_FG" {
		t.Errorf("parent_name = %q", got.Get("parent_name"))
	}
}

func TestRoutesPreferTheTunnelForRFC(t *testing.T) {
	// Both routes present: the tunnel wins because it carries a session and
	// SOAP cannot.
	r := &CompatReport{Results: []CompatResult{
		{ID: "core.discovery", Outcome: CompatOK},
		{ID: "apc.zadt_vsp", Outcome: CompatOK},
		{ID: "soap.rfc", Outcome: CompatOK},
	}}
	if got := routeFor(r, "rfc"); got.Preferred != "zadt-vsp-ws" {
		t.Errorf("preferred = %q, want the tunnel", got.Preferred)
	}
}

func TestRoutesFallBackToSOAP(t *testing.T) {
	r := &CompatReport{Results: []CompatResult{
		{ID: "core.discovery", Outcome: CompatOK},
		{ID: "apc.zadt_vsp", Outcome: CompatError},
		{ID: "soap.rfc", Outcome: CompatOK},
	}}
	got := routeFor(r, "rfc")
	if got.Preferred != "soap-rfc" {
		t.Errorf("preferred = %q, want SOAP", got.Preferred)
	}
	if !strings.Contains(got.Note, "stateless") {
		t.Errorf("note = %q, want the statelessness said out loud", got.Note)
	}
}

func TestRoutesReportNoHTTPRouteForRFC(t *testing.T) {
	r := &CompatReport{Results: []CompatResult{
		{ID: "core.discovery", Outcome: CompatOK},
		{ID: "apc.zadt_vsp", Outcome: CompatAbsent},
		{ID: "soap.rfc", Outcome: CompatForbidden},
	}}
	if got := routeFor(r, "rfc"); got.Preferred != "gateway-only" {
		t.Errorf("preferred = %q, want the gateway", got.Preferred)
	}
}

func TestSkippedIsNotAbsent(t *testing.T) {
	// "not probed" and "not there" are different answers, and only one of them
	// is about the system.
	r := &CompatReport{Results: []CompatResult{
		{ID: "core.discovery", Outcome: CompatOK},
		{ID: "repository.nodestructure", Outcome: CompatSkipped},
		{ID: "fugr.objectstructure", Outcome: CompatSkipped},
	}}
	got := routeFor(r, "function-module-list")
	if got.Preferred != "unknown" {
		t.Errorf("preferred = %q, want unknown rather than none", got.Preferred)
	}
}

func TestFirstLineDropsHTMLErrorPages(t *testing.T) {
	// SAP refusals arrive as a whole error page on one line; quoting its first
	// hundred characters yields a doctype and a stylesheet.
	in := `ADT API error: status 403 at /sap/bc/soap/rfc: <html><head><style>body{}</style></head></html>`
	got := firstLine(in)
	if strings.Contains(got, "<html") || strings.Contains(got, "style") {
		t.Errorf("got %q, want the markup dropped", got)
	}
	if !strings.Contains(got, "403") {
		t.Errorf("got %q, want the status kept", got)
	}
}

// routeFor is a test helper: the routes are derived as a set, and a test cares
// about one of them.
func routeFor(r *CompatReport, capability string) CompatRoute {
	for _, route := range r.Routes() {
		if route.Capability == capability {
			return route
		}
	}
	return CompatRoute{}
}
