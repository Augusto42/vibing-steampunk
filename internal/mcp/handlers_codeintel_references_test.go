package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// The where-used response is a tree flattened into a list, and most of it is
// scaffolding — packages and function groups carrying isResult="false", present
// only so their children have a parent. Returned whole, one real question
// produced 113 entries and 56,000 characters: more than the agent asking it can
// hold, so the tool that answered was the tool that could not be used.

func referencesXML(hits int, scaffolding int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><usageReferences:referenceResult><referencedObjects>`)
	for i := 0; i < scaffolding; i++ {
		b.WriteString(fmt.Sprintf(
			`<referencedObject uri="/sap/bc/adt/packages/zdemo%d" isResult="false" canHaveChildren="true">`+
				`<adtObject adtcore:name="$ZDEMO%d" adtcore:type="DEVC/K"/></referencedObject>`, i, i))
	}
	for i := 0; i < hits; i++ {
		b.WriteString(fmt.Sprintf(
			`<referencedObject uri="/sap/bc/adt/oo/classes/zcl_demo_caller%d" isResult="true" usageInformation="gradeDirect">`+
				`<adtObject adtcore:name="ZCL_DEMO_CALLER%d" adtcore:type="CLAS/OC"/></referencedObject>`, i, i))
	}
	b.WriteString(`</referencedObjects></usageReferences:referenceResult>`)
	return b.String()
}

func referencesServer(t *testing.T, hits, scaffolding int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-csrf-token", "test-token")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(referencesXML(hits, scaffolding)))
	}))
}

func referencesAnswer(t *testing.T, srv *httptest.Server, args map[string]any) map[string]any {
	t.Helper()
	s := &Server{adtClient: adt.NewClient(srv.URL, "user", "pass")}
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	result, err := s.handleFindReferences(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(toolResultText(t, result)), &out); err != nil {
		t.Fatalf("the answer should be JSON: %v", err)
	}
	return out
}

func TestReferencesDropsScaffoldingAndSaysHowMuch(t *testing.T) {
	srv := referencesServer(t, 3, 7)
	defer srv.Close()

	out := referencesAnswer(t, srv, map[string]any{"object_url": "/sap/bc/adt/oo/classes/zcl_demo_order"})
	if out["total"] != float64(3) {
		t.Errorf("three rows are results, got %v", out["total"])
	}
	if out["scaffolding"] != float64(7) {
		t.Errorf("seven rows are containers and should be counted, not silently dropped, got %v", out["scaffolding"])
	}
	refs, _ := out["references"].([]any)
	if len(refs) != 3 {
		t.Errorf("only the results belong in the list, got %d", len(refs))
	}
}

func TestReferencesCapsAndSaysItCapped(t *testing.T) {
	srv := referencesServer(t, 120, 5)
	defer srv.Close()

	out := referencesAnswer(t, srv, map[string]any{"object_url": "/sap/bc/adt/oo/classes/zcl_demo_order"})
	refs, _ := out["references"].([]any)
	if len(refs) != 50 {
		t.Errorf("the default cap is 50, got %d", len(refs))
	}
	if out["total"] != float64(120) {
		t.Errorf("the total must survive the cap: \"50 of 120\" and \"120\" are different answers, got %v", out["total"])
	}
	if out["truncated"] == nil {
		t.Error("a list that was cut is not a list that ended, and must say so")
	}
}

func TestReferencesSaysWhenEveryRowWasAContainer(t *testing.T) {
	srv := referencesServer(t, 0, 4)
	defer srv.Close()

	out := referencesAnswer(t, srv, map[string]any{"object_url": "/sap/bc/adt/oo/classes/zcl_demo_order"})
	if out["total"] != float64(0) {
		t.Fatalf("no row was a result, got %v", out["total"])
	}
	if out["note"] == nil {
		t.Error("containers only is an answer, and not the same as the resource returning nothing")
	}
}
