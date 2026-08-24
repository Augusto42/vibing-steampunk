package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// A boundary verdict rests on having read the package. Asking GetSource without
// an object type failed for every object — it switches on the type and has no
// branch for the empty string — so the graph came out empty, CheckBoundaries
// found nothing to violate anything, and the answer was
// "Total dependencies: 0, CLEAN" for a package with three crossings in it.
//
// Two guards, because the first alone is not enough: pass the type, and refuse
// to call a package clean when nothing in it was read.

const boundaryPackageXML = `<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA>
<TREE_CONTENT>
 <SEU_ADT_REPOSITORY_OBJ_NODE>
  <OBJECT_TYPE>CLAS/OC</OBJECT_TYPE><OBJECT_NAME>ZCL_DEMO_ORDER</OBJECT_NAME>
  <OBJECT_URI>/sap/bc/adt/oo/classes/zcl_demo_order</OBJECT_URI>
 </SEU_ADT_REPOSITORY_OBJ_NODE>
</TREE_CONTENT>
</DATA></asx:values></asx:abap>`

const boundaryClassSource = `CLASS zcl_demo_order DEFINITION PUBLIC.
  PUBLIC SECTION.
    METHODS run.
ENDCLASS.
CLASS zcl_demo_order IMPLEMENTATION.
  METHOD run.
    DATA lo TYPE REF TO zcl_demo_foreign.
    zcl_demo_foreign=>go( ).
  ENDMETHOD.
ENDCLASS.`

func boundaryServer(t *testing.T, sourceStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-csrf-token", "test-token")
		switch {
		case strings.Contains(r.URL.Path, "/repository/nodestructure"):
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(boundaryPackageXML))
		case strings.Contains(r.URL.Path, "/oo/classes/") && strings.HasSuffix(r.URL.Path, "/source/main"):
			if sourceStatus != http.StatusOK {
				w.WriteHeader(sourceStatus)
				w.Write([]byte("not authorised"))
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(boundaryClassSource))
		default:
			// TADIR lookups and anything else: empty, not an error. The point
			// under test is the source read, not package resolution.
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0"?><x/>`))
		}
	}))
}

func boundaryRequest(pkg string) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = map[string]any{"package": pkg}
	return req
}

func TestAPackageWhoseObjectsCouldNotBeReadIsNotCalledClean(t *testing.T) {
	srv := boundaryServer(t, http.StatusForbidden)
	defer srv.Close()

	s := &Server{adtClient: adt.NewClient(srv.URL, "user", "pass")}
	result, err := s.handleCheckBoundaries(context.Background(), boundaryRequest("$ZDEMO"))
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	text := toolResultText(t, result)
	if strings.Contains(text, "CLEAN") {
		t.Fatalf("nothing was read, so there is no verdict to give:\n%s", text)
	}
	if !strings.Contains(text, "ZCL_DEMO_ORDER") {
		t.Errorf("the answer must name the object it could not read:\n%s", text)
	}
	if !strings.Contains(strings.ToUpper(text), "$ZDEMO") {
		t.Errorf("the answer must name the package it did not analyse:\n%s", text)
	}
}

func TestAReadablePackageIsActuallyRead(t *testing.T) {
	srv := boundaryServer(t, http.StatusOK)
	defer srv.Close()

	s := &Server{adtClient: adt.NewClient(srv.URL, "user", "pass")}
	result, err := s.handleCheckBoundaries(context.Background(), boundaryRequest("$ZDEMO"))
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	text := toolResultText(t, result)
	if strings.Contains(text, "Total dependencies: 0") {
		t.Fatalf("the class references ZCL_DEMO_FOREIGN; a zero here means the "+
			"source was never read:\n%s", text)
	}
	if !strings.Contains(strings.ToUpper(text), "ZCL_DEMO_FOREIGN") {
		t.Errorf("the dependency the source declares should appear:\n%s", text)
	}
}
