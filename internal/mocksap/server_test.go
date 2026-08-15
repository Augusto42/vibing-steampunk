package mocksap_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/internal/mocksap"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

func TestObjectTypeExpansionEndToEnd(t *testing.T) {
	mock := mocksap.New(mocksap.Options{})
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)

	client := adt.NewClient(
		server.URL,
		mocksap.Username,
		mocksap.Password,
		adt.WithClient(mocksap.Client),
		adt.WithSafety(adt.DevelopmentSafetyConfig()),
	)
	ctx := context.Background()

	enhancement, err := client.GetSource(ctx, "ENHO", mocksap.EnhancementName, nil)
	if err != nil {
		t.Fatalf("GetSource(ENHO) through mock SAP: %v", err)
	}
	if !strings.Contains(enhancement, "ENHANCEMENT 1 zsynthetic_enho") {
		t.Fatalf("unexpected enhancement source: %q", enhancement)
	}

	merged, err := client.GetSource(ctx, "INCL", mocksap.IncludeName, &adt.GetSourceOptions{Merged: true})
	if err != nil {
		t.Fatalf("GetSource(INCL merged) through mock SAP: %v", err)
	}
	if !strings.Contains(merged, "vvv ENHO/XH "+mocksap.EnhancementName) || !strings.Contains(merged, "lv_value = 42") {
		t.Fatalf("merged include did not contain synthetic enhancement:\n%s", merged)
	}

	const updatedSource = `INCLUDE zsynthetic_include.
DATA gv_synthetic_value TYPE i VALUE 7.`
	writeResult, err := client.WriteSource(ctx, "INCL", mocksap.IncludeName, updatedSource, &adt.WriteSourceOptions{Mode: adt.WriteModeUpdate})
	if err != nil {
		t.Fatalf("WriteSource(INCL) through mock SAP: %v", err)
	}
	if !writeResult.Success || writeResult.Mode != "updated" {
		t.Fatalf("WriteSource(INCL) result: %#v", writeResult)
	}
	readBack, err := client.GetSource(ctx, "INCL", mocksap.IncludeName, nil)
	if err != nil {
		t.Fatalf("read-back INCL through mock SAP: %v", err)
	}
	if readBack != updatedSource {
		t.Fatalf("include read-back = %q, want %q", readBack, updatedSource)
	}

	dynproJSON, err := client.GetSource(ctx, "DYNP", mocksap.ProgramName+"/100", nil)
	if err != nil {
		t.Fatalf("GetSource(DYNP) through mock SAP WebSocket: %v", err)
	}
	var dynpro adt.Dynpro
	if err := json.Unmarshal([]byte(dynproJSON), &dynpro); err != nil {
		t.Fatalf("DYNP result is not JSON: %v\n%s", err, dynproJSON)
	}
	if dynpro.Program != mocksap.ProgramName || dynpro.Screen != mocksap.ScreenNumber || len(dynpro.FlowLogic) != 3 {
		t.Fatalf("unexpected DYNP result: %#v", dynpro)
	}

	assertSafeWriteSequence(t, mock.Snapshot().Requests)
}

func TestSyntaxErrorStopsBeforeLockAndWrite(t *testing.T) {
	mock := mocksap.New(mocksap.Options{SyntaxError: true})
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)

	client := adt.NewClient(server.URL, mocksap.Username, mocksap.Password, adt.WithClient(mocksap.Client))
	result, err := client.WriteSource(
		context.Background(),
		"INCL",
		mocksap.IncludeName,
		"INCLUDE zsynthetic_include.\nTHIS IS NOT ABAP.",
		&adt.WriteSourceOptions{Mode: adt.WriteModeUpdate},
	)
	if err != nil {
		t.Fatalf("WriteSource returned transport error: %v", err)
	}
	if result.Success || !strings.Contains(result.Message, "syntax errors") {
		t.Fatalf("expected logical syntax failure, got %#v", result)
	}
	for _, request := range mock.Snapshot().Requests {
		if request.Method == http.MethodPut || queryAction(request.Query) == "LOCK" {
			t.Fatalf("syntax failure must stop before lock/write, saw %#v", request)
		}
	}
}

func TestDynproNonzeroSubrcFailsClosed(t *testing.T) {
	mock := mocksap.New(mocksap.Options{DynproSubrc: 4})
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)

	client := adt.NewClient(server.URL, mocksap.Username, mocksap.Password, adt.WithClient(mocksap.Client))
	_, err := client.GetSource(context.Background(), "DYNP", mocksap.ProgramName+"/0100", nil)
	if err == nil || !strings.Contains(err.Error(), "subrc=4") {
		t.Fatalf("expected fail-closed subrc error, got %v", err)
	}
}

func assertSafeWriteSequence(t *testing.T, requests []mocksap.RequestRecord) {
	t.Helper()
	indices := map[string]int{}
	for index, request := range requests {
		action := queryAction(request.Query)
		switch {
		case request.Method == http.MethodPost && request.Path == "/sap/bc/adt/checkruns":
			indices["check"] = index
		case action == "LOCK":
			indices["lock"] = index
			if !request.Stateful || !request.CSRF {
				t.Fatalf("LOCK was not stateful+CSRF: %#v", request)
			}
		case request.Method == http.MethodPut:
			indices["put"] = index
			if !request.Stateful || !request.CSRF {
				t.Fatalf("PUT was not stateful+CSRF: %#v", request)
			}
		case action == "UNLOCK":
			indices["unlock"] = index
		case request.Method == http.MethodPost && request.Path == "/sap/bc/adt/activation":
			indices["activate"] = index
		}
	}
	for _, step := range []string{"check", "lock", "put", "unlock", "activate"} {
		if _, ok := indices[step]; !ok {
			t.Fatalf("missing %s in request trace: %#v", step, requests)
		}
	}
	if !(indices["check"] < indices["lock"] && indices["lock"] < indices["put"] && indices["put"] < indices["unlock"] && indices["unlock"] < indices["activate"]) {
		t.Fatalf("unsafe write order: %#v", indices)
	}
}

func queryAction(rawQuery string) string {
	values, _ := url.ParseQuery(rawQuery)
	return strings.ToUpper(values.Get("_action"))
}
