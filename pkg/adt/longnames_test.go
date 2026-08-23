package adt

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// WBCROSSGT stores a SHA-1 in NAME when the real name does not fit CHAR(120).
// Forty hex characters pass every callee filter — no backslash to split on, no
// match against the object's own name — so without the WBCROSSGTX lookup the
// hash is reported as the thing the object references. That is an invented
// answer, not a missing one, which is why an undecodable row is dropped and
// declared rather than shown.

const demoHash = "A15C4C18AE006254E67B73937D5149766FD922C9"

func longNameServer(t *testing.T, denyLongNames bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-csrf-token", "test-token")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		sql := string(body)
		switch {
		case strings.Contains(sql, " FROM WBCROSSGTX "):
			if denyLongNames {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte("not authorised for WBCROSSGTX"))
				return
			}
			w.Write([]byte(tableXML(
				col("NAME", demoHash),
				col("LONG_NAME", `ZCL_DEMO_HELPER\DA:GT_CACHE`),
			)))
		case strings.Contains(sql, " FROM WBCROSSGT "):
			w.Write([]byte(tableXML(
				col("INCLUDE", "ZCL_DEMO_ORDER===============CM001"),
				col("OTYPE", "DA"),
				col("NAME", demoHash),
				col("DIRECT", "X"),
			)))
		default:
			w.Write([]byte(tableXML()))
		}
	}))
}

func TestALongNameIsDecodedRatherThanReportedAsItsHash(t *testing.T) {
	srv := longNameServer(t, false)
	defer srv.Close()

	client := NewClient(srv.URL, "user", "pass")
	callees, gaps, err := client.Callees(context.Background(), "/sap/bc/adt/oo/classes/zcl_demo_order")
	if err != nil {
		t.Fatalf("the row decodes, so nothing should fail: %v", err)
	}
	if len(gaps) != 0 {
		t.Errorf("WBCROSSGTX answered, so nothing is missing: %v", gaps)
	}
	if len(callees) != 1 {
		t.Fatalf("expected the one decoded reference, got %d: %+v", len(callees), callees)
	}
	if callees[0].Name != "ZCL_DEMO_HELPER" {
		t.Errorf("the name should come from LONG_NAME, got %q", callees[0].Name)
	}
	if callees[0].Component != "GT_CACHE" {
		t.Errorf("the component is packed into LONG_NAME too, got %q", callees[0].Component)
	}
}

func TestAnUndecodableLongNameIsDeclaredRatherThanShown(t *testing.T) {
	srv := longNameServer(t, true)
	defer srv.Close()

	client := NewClient(srv.URL, "user", "pass")
	callees, gaps, err := client.Callees(context.Background(), "/sap/bc/adt/oo/classes/zcl_demo_order")
	if err != nil {
		t.Fatalf("one unreadable lookup must not fail the whole answer: %v", err)
	}
	for _, c := range callees {
		if c.Name == demoHash {
			t.Fatalf("a hash reached the answer as an object name: %+v", c)
		}
	}
	if len(gaps) == 0 {
		t.Fatal("the row was dropped and nothing said so; a shorter list that " +
			"looks whole is the defect this guards")
	}
	if gaps[0].Object != demoHash {
		t.Errorf("the gap should name the hash it could not decode, got %q", gaps[0].Object)
	}
}

func TestANameThatMerelyLooksLikeAHashIsLeftAlone(t *testing.T) {
	for _, name := range []string{
		"ZCL_DEMO_HELPER",
		strings.Repeat("A", 39),
		strings.Repeat("A", 41),
		"A15C4C18AE006254E67B73937D5149766FD922CZ", // Z is not hex
		`ZCL_X\DA:GT_SERVICES`,
	} {
		if looksLikeLongNameHash(name) {
			t.Errorf("%q is not a stored hash and must not be looked up", name)
		}
	}
	if !looksLikeLongNameHash(demoHash) {
		t.Errorf("%q is the shape WBCROSSGT stores", demoHash)
	}
}
