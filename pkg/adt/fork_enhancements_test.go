package adt

import (
	"context"
	"strings"
	"testing"
)

func TestGetEnhancementTransportOwner_ResolvesParentRequest(t *testing.T) {
	mock := &queryRoutedMock{
		byPathBody: map[string]string{
			"/sap/bc/adt/discovery":      "OK",
			"/sap/bc/adt/core/discovery": "OK",
		},
		byDdicEntityKeyBody: map[string]string{
			"E071": dataPreviewBody([]string{"TRKORR"}, [][]string{{"NPLK900078"}}),
			"E070": dataPreviewBody([]string{"TRKORR", "STRKORR"}, [][]string{{"NPLK900078", "NPLK900075"}}),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	task, request, err := client.getEnhancementTransportOwner(context.Background(), "ZSYNTHETIC_ENHO")
	if err != nil {
		t.Fatalf("getEnhancementTransportOwner returned error: %v", err)
	}
	if task != "NPLK900078" || request != "NPLK900075" {
		t.Fatalf("unexpected CTS owner: task=%q request=%q", task, request)
	}
	if !transportMatchesCTSOwner("NPLK900075", task, request) || !transportMatchesCTSOwner("NPLK900078", task, request) {
		t.Fatal("parent request and owning task should both be accepted")
	}
	if transportMatchesCTSOwner("NPLK900076", task, request) {
		t.Fatal("unrelated request should be rejected")
	}
}

func TestGetEnhancement_RFCFallbackResolvesPaddedIncludeFromENHINCINX(t *testing.T) {
	const name = "ZSYNTHETIC_HOOK"
	searchBody := `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/enhancements/enhoxh/zsynthetic_hook" adtcore:type="ENHO/XH" adtcore:name="ZSYNTHETIC_HOOK" adtcore:packageName="$TMP"/>
</adtcore:objectReferences>`
	enhincinxBody := dataPreviewBody(
		[]string{"ENHNAME", "PROGRAMNAME", "FULL_NAME", "ENHINCLUDE"},
		[][]string{{
			name,
			"ZSYNTHETIC_PROGRAM",
			`\PR:ZSYNTHETIC_PROGRAM\FO:SYNTHETIC_FORM\SE:BEGIN\EI`,
			"ZSYNTHETIC_HOOK============E",
		}},
	)
	mock := &queryRoutedMock{
		byPathBody: map[string]string{
			"/sap/bc/adt/repository/informationsystem/search": searchBody,
			"/sap/bc/adt/discovery":                           "OK",
			"/sap/bc/adt/core/discovery":                      "OK",
		},
		byDdicEntityKeyBody: map[string]string{"ENHINCINX": enhincinxBody},
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	stub := &stubRFCSourceFetcher{sourceLines: []string{
		"ENHANCEMENT 1 ZSYNTHETIC_HOOK.",
		"  WRITE 'resolved'.",
		"ENDENHANCEMENT.",
	}}
	client.rfcFetcherFactory = func(ctx context.Context) (rfcSourceFetcher, error) {
		return stub, nil
	}

	got, err := client.GetEnhancement(context.Background(), name)
	if err != nil {
		t.Fatalf("GetEnhancement should resolve ENHINCINX include, got: %v", err)
	}
	if !strings.Contains(got, "WRITE 'resolved'") {
		t.Fatalf("expected RFC source body, got: %q", got)
	}
	if len(stub.readCalls) != 1 || stub.readCalls[0] != "ZSYNTHETIC_HOOK============E" {
		t.Fatalf("expected padded ENHINCLUDE, got calls: %#v", stub.readCalls)
	}
}

func TestWriteEnhancementUpdateByRef_WritesAndVerifiesActiveSource(t *testing.T) {
	const source = "ENHANCEMENT 1 ZSYNTHETIC_ENHO.\n  DATA lv_marker TYPE string VALUE 'SYNTHETIC'.\nENDENHANCEMENT."
	stub := &stubRFCSourceFetcher{sourceLines: []string{
		"ENHANCEMENT 1.",
		"  DATA lv_marker TYPE string VALUE 'BEFORE'.",
		"ENDENHANCEMENT.",
	}}
	client := &Client{rfcFetcherFactory: func(ctx context.Context) (rfcSourceFetcher, error) {
		return stub, nil
	}}
	ref := &EnhancementRef{
		Name:        "ZSYNTHETIC_ENHO",
		Kind:        EnhancementKind("XH"),
		URI:         "/sap/bc/adt/enhancements/enhoxh/zsynthetic_enho",
		PackageName: "$TMP",
		EnhInclude:  "ZSYNTHETIC_ENHO===============E",
	}

	result, err := client.writeEnhancementUpdateByRef(context.Background(), ref, source, "")
	if err != nil {
		t.Fatalf("writeEnhancementUpdateByRef returned error: %v", err)
	}
	if !result.Success || result.Activation == nil || !result.Activation.Success {
		t.Fatalf("expected verified success, got: %+v", result)
	}
	if stub.writeName != ref.Name || stub.writeSource != source || stub.writeTransport != "" {
		t.Fatalf("unexpected writer call: name=%q source=%q transport=%q", stub.writeName, stub.writeSource, stub.writeTransport)
	}
	if len(stub.readCalls) != 2 || stub.readCalls[0] != ref.EnhInclude || stub.readCalls[1] != ref.EnhInclude {
		t.Fatalf("expected preflight and post-write active read-back from %q, got %#v", ref.EnhInclude, stub.readCalls)
	}
}
