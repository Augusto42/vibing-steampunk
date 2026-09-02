package graph

import (
	"strings"
	"testing"
)

// A package where a third of the objects answered 404 printed "No crossings
// found." on stdout with every failure on stderr, so `vsp boundaries X >
// report.txt` captured a clean bill over a package read in part. The report now
// carries the gap as data, which is what a text renderer prints and what JSON
// keeps as fields rather than prose.
func TestAPartialScanIsNotReportedAsClean(t *testing.T) {
	r := &CrossingReport{RootPackage: "SBRF", SourceAttempted: 222, SourceRead: 167}
	if r.Complete() {
		t.Error("a scan that read 167 of 222 is not complete")
	}
	if r.Missed() != 55 {
		t.Errorf("Missed() = %d, want 55", r.Missed())
	}
	caveat := r.Caveat()
	for _, want := range []string{"167", "222", "55", "missing from this report"} {
		if !strings.Contains(caveat, want) {
			t.Errorf("the caveat does not say %q: %q", want, caveat)
		}
	}
}

// A complete scan must say nothing, or the caveat becomes noise and stops
// being read on the run where it matters.
func TestACompleteScanCarriesNoCaveat(t *testing.T) {
	r := &CrossingReport{RootPackage: "SAI_PROXY_VERI", SourceAttempted: 11, SourceRead: 11}
	if !r.Complete() {
		t.Error("11 of 11 is complete")
	}
	if got := r.Caveat(); got != "" {
		t.Errorf("a complete scan produced a caveat: %q", got)
	}
}

// A report from a caller that does not track source coverage at all must not
// claim to be incomplete. Absence of the numbers is not evidence of a gap.
func TestNoCoverageNumbersMeansNoClaimEitherWay(t *testing.T) {
	r := &CrossingReport{RootPackage: "X"}
	if !r.Complete() {
		t.Error("a report with no coverage numbers must not read as partial")
	}
	if r.Caveat() != "" {
		t.Error("a report with no coverage numbers must not invent a caveat")
	}
}

// ObjectsScanned counts nodes in the graph, which includes targets discovered
// through edges. It is not a coverage figure and must not be used as one — a
// header saying 167 once sat above a caveat saying 167 of 222 because both
// numbers were read off the same field.
func TestNodeCountIsNotACoverageFigure(t *testing.T) {
	r := &CrossingReport{ObjectsScanned: 400, SourceAttempted: 222, SourceRead: 167}
	if strings.Contains(r.Caveat(), "400") {
		t.Errorf("the caveat used the node count: %q", r.Caveat())
	}
	if r.Missed() != 55 {
		t.Errorf("Missed() = %d; the node count leaked into coverage", r.Missed())
	}
}
