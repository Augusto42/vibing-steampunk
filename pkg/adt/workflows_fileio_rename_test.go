package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Rename had the same lost parameter as deploy, in the other handler. A module
// is addressable only under its group, and here — unlike a deploy — there is no
// filename to read the group from: the caller passes a bare module name. So both
// URLs came back as "function module requires parent function group name", about
// a group the caller was never going to type.
//
// It does not need to be typed. TFDIR maps a module to its group, and the client
// already reads it for other operations.

// The group is resolved through the ADT object search, so that is what the
// fake has to speak — the earlier version of this test answered a table query
// instead, the resolver failed before the code under test was reached, and the
// test passed with the defect reintroduced. A test that cannot fail is worse
// than no test: it reports a guarantee nobody has.
func renameServer(t *testing.T, resolveGroup bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-csrf-token", "test-token")
		if strings.Contains(r.URL.Path, "/informationsystem/search") {
			w.Header().Set("Content-Type", "application/xml")
			if !resolveGroup {
				w.Write([]byte(`<?xml version="1.0"?><adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core"/>`))
				return
			}
			w.Write([]byte(`<?xml version="1.0"?><adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">` +
				`<adtcore:objectReference adtcore:uri="/sap/bc/adt/functions/groups/zdemo_fg/fmodules/z_demo_call" ` +
				`adtcore:type="FUGR/FF" adtcore:name="Z_DEMO_CALL" adtcore:packageName="$ZDEMO"/>` +
				`</adtcore:objectReferences>`))
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
	}))
}

func TestRenamingAFunctionModuleFindsItsGroupInsteadOfDemandingIt(t *testing.T) {
	srv := renameServer(t, true)
	defer srv.Close()

	c := NewClient(srv.URL, "user", "pass")
	_, err := c.RenameObject(context.Background(), ObjectTypeFunctionMod,
		"Z_DEMO_CALL", "ZDEMO_API_CALL", "$ZDEMO", "")

	// The rename itself needs a whole live system to finish; what is under test
	// is that it gets past addressing rather than refusing at the first step.
	if err != nil && strings.Contains(err.Error(), "requires parent function group name") {
		t.Fatalf("the group is derivable from the module name and was not asked for: %v", err)
	}
}

func TestRenamingAModuleWithNoResolvableGroupSaysSo(t *testing.T) {
	srv := renameServer(t, false)
	defer srv.Close()

	c := NewClient(srv.URL, "user", "pass")
	_, err := c.RenameObject(context.Background(), ObjectTypeFunctionMod,
		"Z_DEMO_MISSING", "ZDEMO_API_MISSING", "$ZDEMO", "")
	if err == nil {
		t.Fatal("a module whose group cannot be found is not addressable, and guessing would be worse")
	}
	if !strings.Contains(err.Error(), "Z_DEMO_MISSING") {
		t.Errorf("the failure should name the module it could not place, got %v", err)
	}
}
