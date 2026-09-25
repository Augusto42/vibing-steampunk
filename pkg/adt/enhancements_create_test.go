package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

type createEnhancementFetcher struct {
	created *bool
}

func (f *createEnhancementFetcher) CallRFC(context.Context, string, map[string]any) (*RFCResult, error) {
	return nil, nil
}
func (f *createEnhancementFetcher) ReadSource(context.Context, string) ([]string, error) {
	return nil, nil
}
func (f *createEnhancementFetcher) Close() error { return nil }
func (f *createEnhancementFetcher) CreateEnhancement(context.Context, CreateEnhancementOptions) error {
	*f.created = true
	return nil
}

type createQueryMock struct {
	queryRoutedMock
	created  *bool
	toolType string
}

func (m *createQueryMock) Do(req *http.Request) (*http.Response, error) {
	if strings.EqualFold(req.URL.Query().Get("ddicEntityName"), "ENHHEADER") {
		rows := [][]string{}
		if *m.created {
			rows = append(rows, []string{m.toolType, "A"})
		}
		return freshResponse(dataPreviewBody([]string{"ENHTOOLTYPE", "VERSION"}, rows)), nil
	}
	return m.queryRoutedMock.Do(req)
}

func TestCreateEnhancementRequiresActiveRepositoryReadBack(t *testing.T) {
	cases := []struct {
		name string
		kind EnhancementCreateKind
		tool string
		opts CreateEnhancementOptions
	}{
		{"xh", EnhancementCreateXH, "HOOK_IMPL", CreateEnhancementOptions{HostObjectName: "ZSYNTHETIC_HOST", Anchor: `\PR:ZSYNTHETIC_HOST\SE:END\EI`, Source: "WRITE 'SYNTHETIC'."}},
		{"class", EnhancementCreateClass, "CLASENH", CreateEnhancementOptions{ClassName: "ZCL_SYNTHETIC_HOST"}},
		{"badi", EnhancementCreateBAdI, "BADI_IMPL", CreateEnhancementOptions{SpotName: "ZSYNTHETIC_SPOT", BAdIName: "ZBADI_SYNTHETIC", ImplementationName: "ZIM_SYNTHETIC", ImplementationClass: "ZCL_IM_SYNTHETIC"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			created := false
			mock := &createQueryMock{created: &created, toolType: tc.tool}
			mock.byPathBody = map[string]string{"/sap/bc/adt/discovery": "OK", "/sap/bc/adt/core/discovery": "OK"}
			mock.byDdicEntityKeyBody = map[string]string{"TADIR": dataPreviewBody([]string{"DEVCLASS"}, [][]string{{"$TMP"}})}
			cfg := NewConfig("https://sap.example.invalid", "synthetic", "synthetic", WithSafety(DevelopmentSafetyConfig()))
			client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))
			client.rfcFetcherFactory = func(context.Context) (rfcSourceFetcher, error) {
				return &createEnhancementFetcher{created: &created}, nil
			}
			opts := tc.opts
			opts.Kind, opts.Name, opts.Package, opts.Description = tc.kind, "ZSYNTHETIC_ENHO", "$TMP", "Synthetic test"
			result, err := client.CreateEnhancement(context.Background(), opts)
			if err != nil || result == nil || !result.Success || result.ToolType != tc.tool || !created {
				t.Fatalf("expected verified %s creation: result=%+v err=%v created=%v", tc.kind, result, err, created)
			}
		})
	}
}

func TestCreateEnhancementRejectsWrongRepositorySubtype(t *testing.T) {
	created := false
	mock := &createQueryMock{created: &created, toolType: "BADI_IMPL"}
	mock.byPathBody = map[string]string{"/sap/bc/adt/discovery": "OK", "/sap/bc/adt/core/discovery": "OK"}
	mock.byDdicEntityKeyBody = map[string]string{"TADIR": dataPreviewBody([]string{"DEVCLASS"}, [][]string{{"$TMP"}})}
	cfg := NewConfig("https://sap.example.invalid", "synthetic", "synthetic", WithSafety(DevelopmentSafetyConfig()))
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))
	client.rfcFetcherFactory = func(context.Context) (rfcSourceFetcher, error) { return &createEnhancementFetcher{created: &created}, nil }
	result, err := client.CreateEnhancement(context.Background(), CreateEnhancementOptions{
		Kind: EnhancementCreateXH, Name: "ZSYNTHETIC_ENHO", Package: "$TMP", Description: "Synthetic test",
		HostObjectName: "ZSYNTHETIC_HOST", Anchor: `\PR:ZSYNTHETIC_HOST\SE:END\EI`, Source: "WRITE 'SYNTHETIC'.",
	})
	if err != nil || result == nil || result.Success || !strings.Contains(result.Message, "unexpected tool type") {
		t.Fatalf("wrong subtype must not be reported as success: result=%+v err=%v", result, err)
	}
}

func TestNormalizeCreateEnhancementOptions(t *testing.T) {
	t.Run("xh defaults", func(t *testing.T) {
		opts := CreateEnhancementOptions{
			Kind:           "xh",
			Name:           "zvsp_xh",
			Package:        "$tmp",
			Description:    "Synthetic",
			HostObjectName: "zvsp_host",
			Anchor:         `\PR:ZVSP_HOST\SE:END\EI`,
			Source:         "WRITE 'SYNTHETIC'.",
		}
		normalizeCreateEnhancementOptions(&opts)
		if opts.Kind != EnhancementCreateXH || opts.Name != "ZVSP_XH" || opts.Package != "$TMP" {
			t.Fatalf("identity normalization failed: %+v", opts)
		}
		if opts.HostObjectType != "PROG" || opts.HostProgram != "ZVSP_HOST" {
			t.Fatalf("host defaults failed: %+v", opts)
		}
		if opts.MainObjectType != "PROG" || opts.MainObjectName != "ZVSP_HOST" || opts.EnhancementMode != "S" {
			t.Fatalf("main object defaults failed: %+v", opts)
		}
		if err := validateCreateEnhancementOptions(opts); err != nil {
			t.Fatalf("normalized XH should validate: %v", err)
		}
	})

	t.Run("class method body is wrapped", func(t *testing.T) {
		opts := CreateEnhancementOptions{
			Kind:         "class",
			Name:         "zvsp_clenh",
			Package:      "$tmp",
			Description:  "Synthetic",
			ClassName:    "zcl_vsp_host",
			MethodName:   "enh_marker",
			MethodSource: "  DATA lv_marker TYPE string.",
		}
		normalizeCreateEnhancementOptions(&opts)
		if opts.MethodExposure != "PUBLIC" {
			t.Fatalf("expected PUBLIC default, got %q", opts.MethodExposure)
		}
		for _, want := range []string{"METHOD ENH_MARKER.", "DATA lv_marker", "ENDMETHOD."} {
			if !strings.Contains(opts.MethodSource, want) {
				t.Fatalf("wrapped method source missing %q: %s", want, opts.MethodSource)
			}
		}
		if err := validateCreateEnhancementOptions(opts); err != nil {
			t.Fatalf("normalized class enhancement should validate: %v", err)
		}
	})
}

func TestValidateCreateEnhancementOptionsFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		opts CreateEnhancementOptions
		want string
	}{
		{
			name: "unknown kind",
			opts: CreateEnhancementOptions{Kind: "UNKNOWN", Name: "ZVSP_ENH", Package: "$TMP", Description: "Synthetic"},
			want: "kind must be",
		},
		{
			name: "xh requires anchor",
			opts: CreateEnhancementOptions{Kind: EnhancementCreateXH, Name: "ZVSP_ENH", Package: "$TMP", Description: "Synthetic", HostObjectType: "PROG", HostObjectName: "ZVSP_HOST", HostProgram: "ZVSP_HOST", MainObjectType: "PROG", MainObjectName: "ZVSP_HOST", Source: "WRITE 'X'.", EnhancementMode: "S"},
			want: "anchor is required",
		},
		{
			name: "class source requires method",
			opts: CreateEnhancementOptions{Kind: EnhancementCreateClass, Name: "ZVSP_ENH", Package: "$TMP", Description: "Synthetic", ClassName: "ZCL_VSP_HOST", MethodSource: "METHOD X. ENDMETHOD."},
			want: "method_name is required",
		},
		{
			name: "badi requires implementation class",
			opts: CreateEnhancementOptions{Kind: EnhancementCreateBAdI, Name: "ZVSP_ENH", Package: "$TMP", Description: "Synthetic", SpotName: "ZSPOT", BAdIName: "ZBADI", ImplementationName: "ZIMPL"},
			want: "implementation class is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCreateEnhancementOptions(tt.opts)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestResolveEnhancementUsesAuthoritativeHeaderToolType(t *testing.T) {
	const name = "ZVSP_BADI_IMPL"
	searchBody := `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/enhancements/enhoxh/zvsp_badi_impl" adtcore:type="ENHO/XH" adtcore:name="ZVSP_BADI_IMPL" adtcore:packageName="$TMP"/>
</adtcore:objectReferences>`
	mock := &queryRoutedMock{
		byPathBody: map[string]string{
			"/sap/bc/adt/repository/informationsystem/search": searchBody,
			"/sap/bc/adt/discovery":                           "OK",
			"/sap/bc/adt/core/discovery":                      "OK",
		},
		byDdicEntityKeyBody: map[string]string{
			"ENHHEADER": dataPreviewBody([]string{"ENHTOOLTYPE", "VERSION"}, [][]string{{"BADI_IMPL", "A"}}),
			"TADIR":     dataPreviewBody([]string{"DEVCLASS"}, [][]string{{"$TMP"}}),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "u", "p")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	ref, err := client.resolveEnhancement(context.Background(), name)
	if err != nil {
		t.Fatalf("resolveEnhancement failed: %v", err)
	}
	if ref.Kind != "XBD" || ref.ToolType != "BADI_IMPL" {
		t.Fatalf("expected authoritative BADI mapping, got %+v", ref)
	}
	if !strings.Contains(ref.URI, "/enhoxbd/") {
		t.Fatalf("expected BAdI URI, got %q", ref.URI)
	}
}
