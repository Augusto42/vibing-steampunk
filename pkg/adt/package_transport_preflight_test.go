package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestEnhancementPackageTransportPreflight(t *testing.T) {
	cases := []struct {
		name      string
		pkg       string
		transport string
		xml       string
		wantError bool
	}{
		{
			name: "transportable requires request", pkg: "ZSYNTHETIC", wantError: true,
			xml: `<pak:package xmlns:pak="http://www.sap.com/adt/packages" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="ZSYNTHETIC"><pak:attributes pak:recordChanges="true"/><pak:transport><pak:softwareComponent pak:name="HOME"/></pak:transport></pak:package>`,
		},
		{
			name: "transportable with request", pkg: "ZSYNTHETIC", transport: "SYNK900001",
			xml: `<pak:package xmlns:pak="http://www.sap.com/adt/packages" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="ZSYNTHETIC"><pak:attributes pak:recordChanges="true"/><pak:transport><pak:softwareComponent pak:name="HOME"/></pak:transport></pak:package>`,
		},
		{
			name: "named local", pkg: "ZLOCAL",
			xml: `<pak:package xmlns:pak="http://www.sap.com/adt/packages" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="ZLOCAL"><pak:attributes pak:recordChanges="false"/><pak:transport><pak:softwareComponent pak:name="LOCAL"/></pak:transport></pak:package>`,
		},
		{
			name: "unknown transport metadata fails closed", pkg: "ZUNKNOWN", wantError: true,
			xml: `<pak:package xmlns:pak="http://www.sap.com/adt/packages" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="ZUNKNOWN"/>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockTransportClient{responses: map[string]*http.Response{
				"/sap/bc/adt/packages/" + tc.pkg: newTestResponse(tc.xml),
				"discovery":                      newTestResponse("OK"),
			}}
			cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
			client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))
			err := client.checkPackageTransportRequirement(context.Background(), tc.pkg, tc.transport, "CreateEnhancement")
			if tc.wantError && (err == nil || !strings.Contains(err.Error(), "blocked before mutation")) {
				t.Fatalf("expected fail-closed preflight, got %v", err)
			}
			if !tc.wantError && err != nil {
				t.Fatalf("unexpected preflight error: %v", err)
			}
		})
	}
}
