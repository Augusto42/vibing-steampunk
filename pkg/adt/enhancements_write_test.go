package adt

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWriteEnhancementUpdateByRefVerified(t *testing.T) {
	source := "ENHANCEMENT 1 ZSYNTHETIC_ENHO.\n  WRITE 'SYNTHETIC'.\nENDENHANCEMENT."
	stub := &stubRFCSourceFetcher{sourceLines: []string{"ENHANCEMENT 1.", "WRITE 'BEFORE'.", "ENDENHANCEMENT."}}
	client := &Client{rfcFetcherFactory: func(context.Context) (rfcSourceFetcher, error) { return stub, nil }}
	ref := &EnhancementRef{Name: "ZSYNTHETIC_ENHO", Kind: "XH", URI: "/sap/bc/adt/enhancements/enhoxh/zsynthetic_enho", PackageName: "$TMP", EnhInclude: "ZSYNTHETIC_ENHO===============E"}
	result, err := client.writeEnhancementUpdateByRef(context.Background(), ref, source, "")
	if err != nil || result == nil || !result.Success || result.Activation == nil || !result.Activation.Success {
		t.Fatalf("expected verified success: result=%+v err=%v", result, err)
	}
	if stub.writeName != ref.Name || stub.writeSource != source || stub.writeTransport != "" {
		t.Fatalf("unexpected writer call: %#v", stub)
	}
	if len(stub.readCalls) != 2 || stub.readCalls[0] != ref.EnhInclude || stub.readCalls[1] != ref.EnhInclude {
		t.Fatalf("expected before/after read-back from generated include: %#v", stub.readCalls)
	}
}

func TestWriteEnhancementUpdateByRefFailsClosed(t *testing.T) {
	stub := &stubRFCSourceFetcher{sourceLines: []string{"old"}, writeErr: errors.New("SAP activation failed")}
	client := &Client{rfcFetcherFactory: func(context.Context) (rfcSourceFetcher, error) { return stub, nil }}
	ref := &EnhancementRef{Name: "ZSYNTHETIC_ENHO", Kind: "XH", PackageName: "$TMP", EnhInclude: "ZSYNTHETIC_ENHO===============E"}
	result, err := client.writeEnhancementUpdateByRef(context.Background(), ref, "new", "")
	if err != nil || result == nil || result.Success || !strings.Contains(result.Message, "SAP activation failed") {
		t.Fatalf("expected a truthful failure: result=%+v err=%v", result, err)
	}
}

func TestWriteSourceEnhancementRejectsAmbiguousModes(t *testing.T) {
	client := &Client{}
	for _, mode := range []WriteSourceMode{WriteModeCreate, WriteModeUpsert} {
		result, err := client.writeSourceEnhancement(context.Background(), "ZSYNTHETIC_ENHO", "source", &WriteSourceOptions{Mode: mode})
		if err != nil || result == nil || result.Success || !strings.Contains(result.Message, "mode=update") {
			t.Fatalf("mode %s should fail closed: result=%+v err=%v", mode, result, err)
		}
	}
}
