package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// trace_execution compares what the code was predicted to call against what it
// actually called. Every field of its result is omitempty, so a run where each
// step failed marshals to almost nothing — and Comparison, which is the whole
// point, is absent both when static and actual agree and when the trace never
// arrived. Those are opposite conclusions with identical output.

func TestATraceRunThatDidNothingSaysSoRatherThanReturningAnEmptyComparison(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-csrf-token", "test-token")
		if r.Method == http.MethodHead || r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/discovery") {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("not authorised"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "user", "pass")
	result, err := client.TraceExecution(context.Background(), &TraceExecutionOptions{
		ObjectURI:     "/sap/bc/adt/oo/classes/zcl_demo_order",
		RunTests:      true,
		TestObjectURI: "/sap/bc/adt/oo/classes/zcl_demo_order_test",
		TraceUser:     "TESTUSER",
		MaxDepth:      3,
	})
	if err != nil {
		t.Fatalf("the call reports what it could not do rather than failing: %v", err)
	}
	if result.Comparison != nil {
		t.Fatal("nothing ran, so there is nothing to compare")
	}
	if len(result.Unsearched) == 0 {
		t.Fatal("every step failed and the result said nothing; an empty comparison " +
			"reads as agreement between prediction and reality")
	}

	var sawStatic, sawTrace bool
	for _, u := range result.Unsearched {
		if u.Reason == "" {
			t.Errorf("a step that did not run must say why, got %+v", u)
		}
		switch {
		case strings.Contains(u.Object, "static call graph"):
			sawStatic = true
		case strings.Contains(u.Object, "runtime trace"):
			sawTrace = true
		}
	}
	if !sawStatic {
		t.Error("the static half could not be built and that is half the comparison")
	}
	if !sawTrace {
		t.Error("no trace means no actual edges; without saying so the answer " +
			"cannot be told from 'the code ran as predicted'")
	}
}
