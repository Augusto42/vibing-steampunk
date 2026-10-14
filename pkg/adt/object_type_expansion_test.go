//nolint:bodyclose // The transport under test owns all synthetic responses.
package adt

import (
	"context"
	"net/http"
	"testing"
)

func TestProgramIncludeURLRemainsFirstClassObject(t *testing.T) {
	const includeURL = "/sap/bc/adt/programs/includes/ZSYNTHETIC_INCL/source/main"
	if got, want := normalizeObjectURLForPackageCheck(includeURL), "/sap/bc/adt/programs/includes/ZSYNTHETIC_INCL"; got != want {
		t.Fatalf("normalizeObjectURLForPackageCheck() = %q, want %q", got, want)
	}

	const classIncludeURL = "/sap/bc/adt/oo/classes/ZCL_SYNTHETIC/includes/testclasses"
	if got, want := normalizeObjectURLForPackageCheck(classIncludeURL), "/sap/bc/adt/oo/classes/ZCL_SYNTHETIC"; got != want {
		t.Fatalf("class include normalization = %q, want %q", got, want)
	}
}

func TestSyntaxCheckUsesCorrectIncludeArtifact(t *testing.T) {
	tests := []struct {
		name      string
		objectURL string
		want      string
	}{
		{
			name:      "program include has source main",
			objectURL: "/sap/bc/adt/programs/includes/ZSYNTHETIC_INCL",
			want:      "/sap/bc/adt/programs/includes/ZSYNTHETIC_INCL/source/main",
		},
		{
			name:      "class include is its own artifact",
			objectURL: "/sap/bc/adt/oo/classes/ZCL_SYNTHETIC/includes/testclasses",
			want:      "/sap/bc/adt/oo/classes/ZCL_SYNTHETIC/includes/testclasses",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sourceArtifactURIForSyntaxCheck(tt.objectURL); got != tt.want {
				t.Fatalf("sourceArtifactURIForSyntaxCheck() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriteSourceUpdatesProgramInclude(t *testing.T) {
	const emptyCheckRun = `<?xml version="1.0" encoding="UTF-8"?><chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun"/>`
	mock := &methodPathMock{routes: []routedResponse{
		resp("", "discovery", http.StatusOK, ""),
		resp(http.MethodGet, "/programs/includes/ZSYNTHETIC_INCL/source/main", http.StatusOK, "INCLUDE zsynthetic_incl."),
		resp(http.MethodGet, "/repository/informationsystem/search", http.StatusOK, `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/includes/zsynthetic_incl" adtcore:type="PROG/I" adtcore:name="ZSYNTHETIC_INCL" adtcore:packageName="$TMP"/>
</adtcore:objectReferences>`),
		resp(http.MethodPost, "/checkruns", http.StatusOK, emptyCheckRun),
		resp(http.MethodPost, "/programs/includes/ZSYNTHETIC_INCL", http.StatusOK, syntheticLocalLockXML),
		resp(http.MethodPut, "/programs/includes/ZSYNTHETIC_INCL/source/main", http.StatusOK, ""),
		resp(http.MethodPost, "/activation", http.StatusOK, ""),
	}}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	result, err := client.WriteSource(
		context.Background(),
		"INCL",
		"ZSYNTHETIC_INCL",
		"INCLUDE zsynthetic_incl.\nDATA gv_value TYPE i.",
		&WriteSourceOptions{Mode: WriteModeUpdate},
	)
	if err != nil {
		t.Fatalf("WriteSource(INCL) failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("WriteSource(INCL) result = %#v", result)
	}
	if result.ObjectURL != "/sap/bc/adt/programs/includes/ZSYNTHETIC_INCL" {
		t.Fatalf("ObjectURL = %q", result.ObjectURL)
	}

	indices := map[string]int{}
	for i, call := range mock.calls {
		switch {
		case call.method == http.MethodPost && call.path == "/sap/bc/adt/checkruns":
			indices["check"] = i
		case call.method == http.MethodPost && call.query.Get("_action") == "LOCK":
			indices["lock"] = i
		case call.method == http.MethodPut:
			indices["put"] = i
			if got := call.query.Get("lockHandle"); got != "SYNTHETIC-HANDLE" {
				t.Fatalf("PUT lockHandle = %q", got)
			}
		case call.method == http.MethodPost && call.query.Get("_action") == "UNLOCK":
			indices["unlock"] = i
		case call.method == http.MethodPost && call.path == "/sap/bc/adt/activation":
			indices["activate"] = i
		}
	}
	for _, step := range []string{"check", "lock", "put", "unlock", "activate"} {
		if _, ok := indices[step]; !ok {
			t.Fatalf("workflow did not execute %s: calls=%#v", step, mock.calls)
		}
	}
	if !(indices["check"] < indices["lock"] && indices["lock"] < indices["put"] && indices["put"] < indices["unlock"] && indices["unlock"] < indices["activate"]) {
		t.Fatalf("unsafe workflow order: %#v", indices)
	}
}
