package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExplicitWriteModesDoNotMutateOnUnsafeExistence(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    WriteSourceMode
		status  int
		message string
	}{
		{"create existing", WriteModeCreate, 200, "already exists"},
		{"update missing", WriteModeUpdate, 404, "does not exist"},
		{"create inconclusive", WriteModeCreate, 503, "cannot tell whether"},
		{"update inconclusive", WriteModeUpdate, 503, "cannot tell whether"},
		{"upsert inconclusive", WriteModeUpsert, 503, "cannot tell whether"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutations := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					mutations++
				}
				w.WriteHeader(tc.status)
			}))
			defer server.Close()
			client := NewClient(server.URL, "SYNTHETIC_USER", "synthetic-password")
			result, err := client.WriteSource(context.Background(), "PROG", "ZSYNTHETIC", "REPORT zsynthetic.", &WriteSourceOptions{
				Mode: tc.mode, Package: "$TMP", Description: "Synthetic program",
			})
			if err != nil || result == nil || result.Success || !strings.Contains(result.Message, tc.message) {
				t.Fatalf("expected safe refusal: result=%+v, err=%v", result, err)
			}
			if mutations != 0 {
				t.Fatalf("sent %d mutation requests after unsafe existence probe", mutations)
			}
		})
	}
}

func TestWriteSourceTransportableUpdateRequiresExplicitRequest(t *testing.T) {
	mutations := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			mutations++
		}
		switch {
		case strings.Contains(r.URL.Path, "informationsystem/search"):
			w.Write([]byte(`<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core"><adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/ZSYNTHETIC" adtcore:name="ZSYNTHETIC" adtcore:type="PROG/P" adtcore:packageName="ZSYNTHETIC"/></adtcore:objectReferences>`))
		case strings.Contains(r.URL.Path, "/packages/"):
			w.Write([]byte(`<package name="ZSYNTHETIC"><attributes recordChanges="true"/><transport><softwareComponent name="HOME"/></transport></package>`))
		default:
			w.Write([]byte("REPORT zsynthetic."))
		}
	}))
	defer server.Close()
	client := NewClient(server.URL, "SYNTHETIC_USER", "synthetic-password", WithEnableTransports())
	_, err := client.WriteSource(context.Background(), "PROG", "ZSYNTHETIC", "REPORT zsynthetic.", &WriteSourceOptions{Mode: WriteModeUpdate})
	if err == nil || !strings.Contains(err.Error(), "explicit transport") {
		t.Fatalf("expected explicit transport refusal, got %v", err)
	}
	if mutations != 0 {
		t.Fatalf("sent %d mutations without explicit transport", mutations)
	}
}

type describingRFCSourceFetcher struct {
	stubRFCSourceFetcher
	metadata string
}

func (s *describingRFCSourceFetcher) DescribeEnhancement(context.Context, string) (string, error) {
	return s.metadata, nil
}

func TestClassEnhancementGeneratedIncludesSurviveReconciliation(t *testing.T) {
	const base = "ZSYNTHETIC_CLASS==============="
	mock := &queryRoutedMock{byDdicEntityKeyBody: map[string]string{
		"PROGDIR": dataPreviewBody([]string{"NAME"}, [][]string{{base + "EM001"}, {base + "E"}}),
	}, byPathBody: map[string]string{"discovery": "OK"}}
	cfg := NewConfig("https://example.invalid", "SYNTHETIC_USER", "synthetic-password")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))
	stub := &stubRFCSourceFetcher{sourceLines: []string{"* synthetic generated method"}}
	client.rfcFetcherFactory = func(context.Context) (rfcSourceFetcher, error) { return stub, nil }
	source, err := client.GetEnhancementByRef(context.Background(), &EnhancementRef{Name: "ZSYNTHETIC_CLASS", Kind: "XC", ToolType: "CLASENH"})
	if err != nil || !strings.Contains(source, "generated enhancement include") {
		t.Fatalf("class source fallback lost: %q, %v", source, err)
	}
	if len(stub.readCalls) != 2 || stub.readCalls[0] != base+"E" || stub.readCalls[1] != base+"EM001" {
		t.Fatalf("generated includes not read in order: %v", stub.readCalls)
	}
}

func TestBAdIMetadataSurvivesReconciliation(t *testing.T) {
	cfg := NewConfig("https://example.invalid", "SYNTHETIC_USER", "synthetic-password")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, &queryRoutedMock{}))
	stub := &describingRFCSourceFetcher{metadata: `{"implementation":"ZIM_SYNTHETIC"}`}
	client.rfcFetcherFactory = func(context.Context) (rfcSourceFetcher, error) { return stub, nil }
	source, err := client.GetEnhancementByRef(context.Background(), &EnhancementRef{Name: "ZSYNTHETIC_BADI", Kind: "XBD", ToolType: "BADI_IMPL"})
	if err != nil || source != stub.metadata {
		t.Fatalf("BAdI metadata fallback lost: %q, %v", source, err)
	}
	if len(stub.readCalls) != 0 {
		t.Fatalf("BAdI metadata must not guess an XH include: %v", stub.readCalls)
	}
}
