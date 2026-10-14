package adt

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Callees reads two tables, and either can fail on its own. The dangerous case
// is not both failing — that is already an error — but one succeeding while the
// other does not, because the rows that come back look like a whole answer.
//
// CROSS is the only place a call to a function module is recorded. Lose it and
// the reply becomes "this class calls no function modules", which is a claim
// nobody made and nobody can see is missing.

// crossRefServer answers the freestyle SQL resource, refusing whichever table
// is named in denied.
func crossRefServer(t *testing.T, denied string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The POST is preceded by a token fetch; without this every query
		// fails for a reason that has nothing to do with what is being tested.
		w.Header().Set("x-csrf-token", "test-token")
		if r.Method == http.MethodHead || r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		sql := string(body)
		if denied != "" && strings.Contains(sql, " FROM "+denied+" ") {
			w.WriteHeader(status)
			w.Write([]byte("not authorised for " + denied))
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		switch {
		case strings.Contains(sql, " FROM WBCROSSGT "):
			w.Write([]byte(tableXML(
				col("INCLUDE", "ZCL_DEMO_ORDER===============CM001"),
				col("OTYPE", "TY"),
				col("NAME", "ZCL_DEMO_HELPER"),
				col("DIRECT", "X"),
			)))
		case strings.Contains(sql, " FROM CROSS "):
			w.Write([]byte(tableXML(
				col("INCLUDE", "ZCL_DEMO_ORDER===============CM001"),
				col("TYPE", "F"),
				col("NAME", "Z_DEMO_CALL"),
				col("PROG", ""),
			)))
		default:
			w.Write([]byte(tableXML()))
		}
	}))
}

type xmlCol struct {
	name string
	data []string
}

func col(name string, data ...string) xmlCol { return xmlCol{name: name, data: data} }

func tableXML(cols ...xmlCol) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><dataPreview:tableData xmlns:dataPreview="http://www.sap.com/adt/dataPreview">`)
	for _, c := range cols {
		b.WriteString(`<dataPreview:columns><dataPreview:metadata dataPreview:name="` + c.name + `" dataPreview:type="C" dataPreview:length="30"/><dataPreview:dataSet>`)
		for _, d := range c.data {
			b.WriteString(`<dataPreview:data>` + d + `</dataPreview:data>`)
		}
		b.WriteString(`</dataPreview:dataSet></dataPreview:columns>`)
	}
	b.WriteString(`</dataPreview:tableData>`)
	return b.String()
}

func TestCalleesNamesTheCrossReferenceTableItCouldNotRead(t *testing.T) {
	srv := crossRefServer(t, "CROSS", http.StatusForbidden)
	defer srv.Close()

	client := NewClient(srv.URL, "user", "pass")
	callees, gaps, err := client.Callees(context.Background(), "/sap/bc/adt/oo/classes/zcl_demo_order")
	if err != nil {
		t.Fatalf("one unreadable table must not lose the other: %v", err)
	}
	if len(callees) == 0 {
		t.Fatal("the rows WBCROSSGT did return should still come back")
	}
	if len(gaps) == 0 {
		t.Fatal("CROSS was refused and the answer said nothing about it; " +
			"a short list that looks whole is the defect this guards")
	}
	if gaps[0].Object != "CROSS" {
		t.Errorf("the gap must name the table that is missing, got %q", gaps[0].Object)
	}
	if gaps[0].Reason == "" {
		t.Error("the reason decides the next step — authorisation, timeout or blocked free SQL")
	}
}

func TestCalleesReportsNoGapWhenBothTablesAnswer(t *testing.T) {
	srv := crossRefServer(t, "", 0)
	defer srv.Close()

	client := NewClient(srv.URL, "user", "pass")
	callees, gaps, err := client.Callees(context.Background(), "/sap/bc/adt/oo/classes/zcl_demo_order")
	if err != nil {
		t.Fatalf("both tables answered: %v", err)
	}
	if len(gaps) != 0 {
		t.Errorf("nothing was refused, so nothing should be reported missing: %v", gaps)
	}
	if len(callees) < 2 {
		t.Errorf("both tables contributed a row, got %d", len(callees))
	}
}
