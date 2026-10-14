package adt

import (
	"strings"
	"testing"
)

// Both ST05 parsers were written against a shape the server does not send, and
// nothing contradicted them because the request carried an Accept the resource
// answers with 406 — so no response was ever parsed. These fixtures are what a
// 7.58 system actually returned, with the host renamed.
//
// The lesson is in the ordering: fixing the Accept made the call succeed and the
// parser wrong in the same minute. A capability can be broken twice over, and
// the second break is invisible while the first one holds.

const traceStateDoc = `<?xml version="1.0" encoding="utf-8"?><ts:traceStateInstanceTable xmlns:ts="http://www.sap.com/adt/tools/performance/tracestate"><ts:traceStateInstance><ts:instance>devsys_A4H_00</ts:instance><ts:host>devsys</ts:host><ts:isLocal>true</ts:isLocal><ts:isSelected>false</ts:isSelected><ts:modificationUser/><ts:modificationDateTime>2026-08-19T11:54:01Z</ts:modificationDateTime><ts:traceTypes><ts:sqlOn>false</ts:sqlOn><ts:bufOn>false</ts:bufOn><ts:enqOn>false</ts:enqOn><ts:rfcOn>false</ts:rfcOn><ts:httpOn>false</ts:httpOn><ts:apcOn>false</ts:apcOn><ts:amcOn>false</ts:amcOn><ts:authOn>false</ts:authOn></ts:traceTypes><ts:traceProperties><ts:includeMissingTableNameOn>false</ts:includeMissingTableNameOn><ts:authErrorsOnly>false</ts:authErrorsOnly><ts:stackTraceOn>false</ts:stackTraceOn><ts:includedTables/><ts:excludedTables/></ts:traceProperties><ts:traceFilter><ts:traceUser/><ts:transactionCode/><ts:program/><ts:rfcFunction/><ts:url/><ts:wpId/></ts:traceFilter></ts:traceStateInstance></ts:traceStateInstanceTable>`

func TestParseTraceStateReadsTheInstanceTable(t *testing.T) {
	states, err := parseSQLTraceState([]byte(traceStateDoc))
	if err != nil {
		t.Fatalf("parsing the document the server sends: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("got %d instances, want 1", len(states))
	}
	s := states[0]
	if s.Instance != "devsys_A4H_00" || s.Host != "devsys" {
		t.Errorf("instance = %q on %q", s.Instance, s.Host)
	}
	if !s.IsLocal {
		t.Error("isLocal was true in the document and false in the model")
	}
	// Eight switches, not one. A model with a single Active flag could not
	// answer "is the RFC trace on" at all.
	for _, want := range []string{"sql", "buffer", "enqueue", "rfc", "http", "apc", "amc", "authorization"} {
		if _, ok := s.Types[want]; !ok {
			t.Errorf("trace type %q is missing from the model", want)
		}
	}
	if s.Active() || s.SQLTraceOn() {
		t.Error("every switch is false in the document; Active() must agree")
	}
	if s.ModifiedAt != "2026-08-19T11:54:01Z" {
		t.Errorf("modifiedAt = %q", s.ModifiedAt)
	}
}

func TestActiveIsTrueWhenAnySwitchIs(t *testing.T) {
	s := SQLTraceState{Types: map[string]bool{"sql": false, "rfc": true}}
	if !s.Active() {
		t.Error("a running RFC trace must count as active")
	}
	if s.SQLTraceOn() {
		t.Error("the SQL switch is off and must read as off")
	}
}

const traceDirectoryDoc = `<?xml version="1.0" encoding="utf-8"?><td:traceDirectory xmlns:td="http://www.sap.com/adt/tools/performance/tracedirectory"><td:uri>http://devsys:50000/sap/bc/stmc/ui5/?sap-client=001#navigation_id=SQL_TRACE_ANALYSIS</td:uri></td:traceDirectory>`

// The release answers the directory with a link, not a list. An empty slice
// would read as "this system has no traces", which is a claim about the system;
// what is true is a claim about the resource.
func TestADirectoryThatListsNothingSaysWhyNot(t *testing.T) {
	dir, err := parseSQLTraceDirectory([]byte(traceDirectoryDoc))
	if err != nil {
		t.Fatalf("parsing the document the server sends: %v", err)
	}
	if len(dir.Entries) != 0 {
		t.Errorf("got %d entries from a document with none", len(dir.Entries))
	}
	if dir.AnalysisURL == "" {
		t.Fatal("the URI the release offers instead was dropped, so the empty list is unexplained")
	}
	if !strings.Contains(dir.AnalysisURL, "stmc") {
		t.Errorf("analysisUrl = %q", dir.AnalysisURL)
	}
}

// The older feed shape must still parse, because that is what a release which
// does list files returns.
func TestTheFeedShapeStillParses(t *testing.T) {
	const feed = `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><entry><id>t1</id><author><name>TESTUSER</name></author><link href="/sap/bc/adt/st05/trace/t1"/><content><trace traceType="SQL" startTime="2026-08-01T10:00:00Z" recordCount="42" size="1024"/></content></entry></feed>`
	dir, err := parseSQLTraceDirectory([]byte(feed))
	if err != nil {
		t.Fatalf("the feed shape no longer parses: %v", err)
	}
	if len(dir.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(dir.Entries))
	}
	e := dir.Entries[0]
	if e.User != "TESTUSER" || e.RecordCount != 42 || e.Size != 1024 || e.TraceType != "SQL" {
		t.Errorf("entry parsed as %+v", e)
	}
	if dir.AnalysisURL != "" {
		t.Error("a feed with entries must not claim the release offers a link instead")
	}
}

func TestXMLBoolReadsSAPsSpellings(t *testing.T) {
	for _, on := range []string{"true", "TRUE", "X", "1", " true "} {
		if !xmlBool(on) {
			t.Errorf("xmlBool(%q) = false", on)
		}
	}
	for _, off := range []string{"false", "", " ", "-", "0"} {
		if xmlBool(off) {
			t.Errorf("xmlBool(%q) = true", off)
		}
	}
}
