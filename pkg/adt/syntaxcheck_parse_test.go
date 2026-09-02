package adt

import "testing"

// The checkrun response as ADT sends it: prefixed element and attribute names
// throughout, the position in the uri fragment, the text in a shortText
// attribute. There was no unit test on this parser at all — only integration
// tests, which need a live SAP system and so never run in CI.
const syntaxCheckErrorsXML = `<?xml version="1.0" encoding="utf-8"?>
<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun">
  <chkrun:checkReport chkrun:reporter="abapCheckRun" chkrun:triggeringUri="/sap/bc/adt/oo/classes/zcl_demo" chkrun:status="processed" chkrun:statusText="">
    <chkrun:checkMessageList>
      <chkrun:checkMessage chkrun:uri="/sap/bc/adt/oo/classes/zcl_demo/source/main#start=12,4" chkrun:type="E" chkrun:shortText="Field &quot;FOO&quot; is unknown"/>
      <chkrun:checkMessage chkrun:uri="/sap/bc/adt/oo/classes/zcl_demo/source/main#start=19,0" chkrun:type="W" chkrun:shortText="Variable is never used"/>
    </chkrun:checkMessageList>
  </chkrun:checkReport>
</chkrun:checkRunReports>`

func TestParseSyntaxCheckResults(t *testing.T) {
	results, err := parseSyntaxCheckResults([]byte(syntaxCheckErrorsXML))
	if err != nil {
		t.Fatalf("parseSyntaxCheckResults: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2: %+v", len(results), results)
	}

	first := results[0]
	if first.Severity != "E" {
		t.Fatalf("severity = %q, want E", first.Severity)
	}
	if first.Text != `Field "FOO" is unknown` {
		t.Fatalf("text = %q", first.Text)
	}
	if first.Line != 12 || first.Offset != 4 {
		t.Fatalf("position = %d,%d, want 12,4", first.Line, first.Offset)
	}
	if first.URI != "/sap/bc/adt/oo/classes/zcl_demo/source/main" {
		t.Fatalf("uri kept its fragment: %q", first.URI)
	}

	if results[1].Severity != "W" || results[1].Line != 19 {
		t.Fatalf("second message not parsed: %+v", results[1])
	}
}

// The parser used to delete the string "chkrun:" from the entire document
// before unmarshalling, on the false premise that encoding/xml cannot match a
// prefixed attribute — the same premise #136 was filed on. It can. The strip
// only ever managed to rewrite SAP's own message text.
func TestParseSyntaxCheckResultsDoesNotRewriteMessageText(t *testing.T) {
	body := `<?xml version="1.0" encoding="utf-8"?>
<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun">
  <chkrun:checkReport chkrun:reporter="abapCheckRun">
    <chkrun:checkMessageList>
      <chkrun:checkMessage chkrun:uri="/x/main#start=9,0" chkrun:type="E" chkrun:shortText="Reporter chkrun:syntaxCheck rejected the artifact"/>
    </chkrun:checkMessageList>
  </chkrun:checkReport>
</chkrun:checkRunReports>`

	results, err := parseSyntaxCheckResults([]byte(body))
	if err != nil {
		t.Fatalf("parseSyntaxCheckResults: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].Text != "Reporter chkrun:syntaxCheck rejected the artifact" {
		t.Fatalf("SAP's text was rewritten in transit: %q", results[0].Text)
	}
}

// A clean check is an empty report, not an error.
func TestParseSyntaxCheckResultsCleanIsEmpty(t *testing.T) {
	body := `<?xml version="1.0" encoding="utf-8"?>
<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun">
  <chkrun:checkReport chkrun:reporter="abapCheckRun" chkrun:status="processed">
    <chkrun:checkMessageList/>
  </chkrun:checkReport>
</chkrun:checkRunReports>`

	results, err := parseSyntaxCheckResults([]byte(body))
	if err != nil {
		t.Fatalf("parseSyntaxCheckResults: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("a clean check reported %d problems: %+v", len(results), results)
	}
}
