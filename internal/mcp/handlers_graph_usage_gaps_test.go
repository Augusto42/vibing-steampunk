package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
)

// usage_examples asks two cross-reference tables for candidate callers, and
// either can fail on its own. CROSS is where a call from procedural code is
// recorded; lose it while WBCROSSGT answers and the reply becomes "these are
// the callers" when it is only the OO half of them. total_callers counts the
// half and reads as the whole.

func usageSQLServer(t *testing.T, denied string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-csrf-token", "test-token")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		sql := string(body)
		if denied != "" && strings.Contains(sql, " FROM "+denied+" ") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("not authorised for " + denied))
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>` +
			`<dataPreview:tableData xmlns:dataPreview="http://www.sap.com/adt/dataPreview">` +
			`<dataPreview:columns><dataPreview:metadata dataPreview:name="INCLUDE" dataPreview:type="C" dataPreview:length="40"/>` +
			`<dataPreview:dataSet><dataPreview:data>ZCL_DEMO_CALLER===============CM001</dataPreview:data></dataPreview:dataSet></dataPreview:columns>` +
			`<dataPreview:columns><dataPreview:metadata dataPreview:name="OTYPE" dataPreview:type="C" dataPreview:length="2"/>` +
			`<dataPreview:dataSet><dataPreview:data>TY</dataPreview:data></dataPreview:dataSet></dataPreview:columns>` +
			`<dataPreview:columns><dataPreview:metadata dataPreview:name="NAME" dataPreview:type="C" dataPreview:length="30"/>` +
			`<dataPreview:dataSet><dataPreview:data>ZCL_DEMO_TARGET</dataPreview:data></dataPreview:dataSet></dataPreview:columns>` +
			`</dataPreview:tableData>`))
	}))
}

func TestUsageCandidatesNameTheTableTheyCouldNotRead(t *testing.T) {
	srv := usageSQLServer(t, "CROSS")
	defer srv.Close()

	s := &Server{adtClient: adt.NewClient(srv.URL, "user", "pass")}
	target := graph.UsageTarget{ObjectType: "CLAS", ObjectName: "ZCL_DEMO_TARGET"}

	cands, gaps, err := s.fetchUsageCandidatesFallback(context.Background(), target, 12)
	if err != nil {
		t.Fatalf("one refused table must not lose the other: %v", err)
	}
	if len(cands) == 0 {
		t.Fatal("WBCROSSGT answered, so its candidates should still come back")
	}
	if len(gaps) == 0 {
		t.Fatal("CROSS was refused and the answer said nothing about it; " +
			"the procedural half of the callers is missing and nobody can tell")
	}
	if gaps[0].Object != "CROSS" {
		t.Errorf("the gap must name the table that is missing, got %q", gaps[0].Object)
	}
	if !strings.Contains(gaps[0].Reason, "403") && !strings.Contains(gaps[0].Reason, "not authorised") {
		t.Errorf("the reason decides the next step, got %q", gaps[0].Reason)
	}
}

func TestUsageCandidatesReportNoGapWhenBothTablesAnswer(t *testing.T) {
	srv := usageSQLServer(t, "")
	defer srv.Close()

	s := &Server{adtClient: adt.NewClient(srv.URL, "user", "pass")}
	target := graph.UsageTarget{ObjectType: "CLAS", ObjectName: "ZCL_DEMO_TARGET"}

	_, gaps, err := s.fetchUsageCandidatesFallback(context.Background(), target, 12)
	if err != nil {
		t.Fatalf("both tables answered: %v", err)
	}
	if len(gaps) != 0 {
		t.Errorf("nothing was refused, so nothing should be reported missing: %v", gaps)
	}
}

func TestTableOfQueryNamesTheHalfThatIsMissing(t *testing.T) {
	cases := map[string]string{
		"SELECT INCLUDE, OTYPE, NAME FROM WBCROSSGT WHERE NAME LIKE 'Z%'":     "WBCROSSGT",
		"SELECT INCLUDE, TYPE AS OTYPE, NAME FROM CROSS WHERE NAME LIKE 'Z%'": "CROSS",
		"SELECT FUNCNAME, PNAME FROM TFDIR WHERE FUNCNAME IN ('Z_A')":         "TFDIR",
		"SELECT SOMETHING FROM ZWHATEVER WHERE X = 'Y'":                       "cross-reference table",
	}
	for sql, want := range cases {
		if got := tableOfQuery(sql); got != want {
			t.Errorf("tableOfQuery(%q) = %q, want %q", sql, got, want)
		}
	}
}
