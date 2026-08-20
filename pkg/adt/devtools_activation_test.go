package adt

import "testing"

// A real ADT activation failure: the checklist messages are the ROOT element,
// not a child of a wrapper. If the parser only looks for a nested <messages>,
// it reports success for a failed activation — silent data loss for the caller.
const activationFailedRoot = `<?xml version="1.0" encoding="utf-8"?>
<chkl:messages xmlns:chkl="http://www.sap.com/abapxml/checklist">
  <msg objDescr="Class ZCL_DEMO" type="E" line="12" href="/sap/bc/adt/oo/classes/zcl_demo/source/main#start=12,4" forceSupported="false">
    <shortText><txt>Field "FOO" is unknown</txt></shortText>
  </msg>
</chkl:messages>`

// Some releases wrap the same payload; both shapes must be understood.
const activationFailedWrapped = `<?xml version="1.0" encoding="utf-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core" xmlns:chkl="http://www.sap.com/abapxml/checklist">
  <chkl:messages>
    <msg objDescr="Class ZCL_DEMO" type="E" line="12" href="x" forceSupported="false">
      <shortText><txt>Field "FOO" is unknown</txt></shortText>
    </msg>
  </chkl:messages>
</adtcore:objectReferences>`

// A warning must not fail the activation.
const activationWarning = `<?xml version="1.0" encoding="utf-8"?>
<chkl:messages xmlns:chkl="http://www.sap.com/abapxml/checklist">
  <msg objDescr="Class ZCL_DEMO" type="W" line="3" href="x" forceSupported="true">
    <shortText><txt>Variable is never used</txt></shortText>
  </msg>
</chkl:messages>`

func TestParseActivationResultFailure(t *testing.T) {
	for name, body := range map[string]string{
		"messages as root": activationFailedRoot,
		"messages wrapped": activationFailedWrapped,
	} {
		t.Run(name, func(t *testing.T) {
			res, err := parseActivationResult([]byte(body))
			if err != nil {
				t.Fatalf("parseActivationResult: %v", err)
			}
			if res.Success {
				t.Fatal("an activation with an error message must not report success")
			}
			if len(res.Messages) != 1 {
				t.Fatalf("messages = %d, want 1", len(res.Messages))
			}
			m := res.Messages[0]
			if m.Type != "E" || m.Line != 12 || m.ShortText != `Field "FOO" is unknown` {
				t.Fatalf("message not parsed: %+v", m)
			}
		})
	}
}

func TestParseActivationResultWarningStillSucceeds(t *testing.T) {
	res, err := parseActivationResult([]byte(activationWarning))
	if err != nil {
		t.Fatalf("parseActivationResult: %v", err)
	}
	if !res.Success {
		t.Fatal("a warning must not fail the activation")
	}
	if len(res.Messages) != 1 || res.Messages[0].Type != "W" {
		t.Fatalf("warning not parsed: %+v", res.Messages)
	}
}

func TestParseActivationResultEmptyIsSuccess(t *testing.T) {
	res, err := parseActivationResult(nil)
	if err != nil {
		t.Fatalf("parseActivationResult: %v", err)
	}
	if !res.Success || len(res.Messages) != 0 {
		t.Fatalf("an empty body means a clean activation, got %+v", res)
	}
}
