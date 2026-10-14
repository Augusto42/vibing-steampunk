package adt

import (
	"encoding/xml"
	"strings"
	"testing"
)

// TestMessageClassWriteShape pins the request body to what ADT actually serves for
// a message class: a namespaced <mc:messageClass> root whose messages carry
// mc:msgno and mc:msgtext as attributes. Marshalling the read model produced a bare
// <MessageClass> with <Messages> children, which the server cannot interpret — so
// writing texts silently did nothing useful.
func TestMessageClassWriteShape(t *testing.T) {
	mc := messageClassWrite{
		XMLNSmc:      "http://www.sap.com/adt/MessageClass",
		XMLNSadtcore: "http://www.sap.com/adt/core",
		Name:         "ZDEMO_MSG",
		Messages: []messageWrite{
			{Number: "001", Text: "Order & created"},
			{Number: "002", Text: `Value "&1" is invalid`},
		},
	}
	out, err := xml.Marshal(mc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)

	for _, want := range []string{
		`<mc:messageClass`,
		`xmlns:mc="http://www.sap.com/adt/MessageClass"`,
		`xmlns:adtcore="http://www.sap.com/adt/core"`,
		`adtcore:name="ZDEMO_MSG"`,
		`<mc:messages mc:msgno="001"`,
		`mc:msgtext="Order &amp; created"`,
		`mc:msgno="002"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("body is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "<MessageClass") {
		t.Fatalf("the bare read model leaked into the request:\n%s", got)
	}
}

// The read model must keep parsing what the server sends, namespaces and all.
func TestMessageClassReadStillParses(t *testing.T) {
	const served = `<?xml version="1.0" encoding="utf-8"?>
<mc:messageClass xmlns:mc="http://www.sap.com/adt/MessageClass" xmlns:adtcore="http://www.sap.com/adt/core"
  adtcore:name="ZLLM_00" adtcore:description="Lite LLM Module">
  <mc:messages mc:msgno="000" mc:msgtext="&amp; &amp; &amp; &amp;" adtcore:name=""/>
  <mc:messages mc:msgno="001" mc:msgtext="Something happened" adtcore:name=""/>
</mc:messageClass>`

	var mc MessageClass
	if err := xml.Unmarshal([]byte(served), &mc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if mc.Name != "ZLLM_00" || mc.Description != "Lite LLM Module" {
		t.Fatalf("header not parsed: %+v", mc)
	}
	if len(mc.Messages) != 2 || mc.Messages[1].Number != "001" || mc.Messages[1].Text != "Something happened" {
		t.Fatalf("messages not parsed: %+v", mc.Messages)
	}
}
