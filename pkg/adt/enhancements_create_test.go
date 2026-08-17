package adt

import (
	"context"
	"strings"
	"testing"
)

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
