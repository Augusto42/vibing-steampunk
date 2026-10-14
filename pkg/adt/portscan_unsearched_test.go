package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// A port that refuses a connection is the answer and stays unrecorded — three
// hundred closed ports in a caveat would bury the one that answered. A name
// that does not resolve is not an answer about any port: every probe fails
// identically, the result is empty, and "Nothing answered on that host" is said
// about a host nobody knocked on.
//
// .invalid is reserved by RFC 2606 and must never resolve, so this needs no
// network and no SAP.
func TestAHostThatDoesNotResolveIsNotAHostWithNothingListening(t *testing.T) {
	result := ScanForADT(context.Background(), "nosuchsystem.invalid", []int{443, 44300}, "100", false)

	if len(result.Findings) != 0 {
		t.Fatalf("findings = %+v, want none", result.Findings)
	}
	if len(result.Unsearched) != 1 {
		t.Fatalf("unsearched = %+v, want the host itself", result.Unsearched)
	}
	if !strings.Contains(result.Unsearched[0].Reason, "does not resolve") {
		t.Fatalf("reason = %q, want it to name resolution", result.Unsearched[0].Reason)
	}
	if result.Unsearched[0].Object != "nosuchsystem.invalid" {
		t.Fatalf("object = %q, want the host", result.Unsearched[0].Object)
	}
}

// A scan that did run carries no caveat, or every clean result would have one
// and readers would learn to skip it. A port that is simply closed is still an
// answer and must stay silent.
func TestAScanThatRanCarriesNoCaveat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}

	result := ScanForADT(context.Background(), u.Hostname(), []int{port}, "100", false)
	if len(result.Unsearched) != 0 {
		t.Fatalf("unsearched = %+v, want none — the sweep reached every port it was given", result.Unsearched)
	}
	if len(result.Findings) == 0 {
		t.Fatal("the listening port should still be found")
	}
}

// A cancelled context makes every probe return nothing, which is
// indistinguishable from every port being closed unless it is said.
func TestACutShortScanSaysSo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A literal address needs no lookup, so the scan gets past resolution and
	// into the sweep, which the dead context then guts.
	result := ScanForADT(ctx, "127.0.0.1", []int{44300, 44301}, "100", false)

	if len(result.Unsearched) == 0 {
		t.Fatal("the scan was cut short and reported an empty result as an answer")
	}
	if !strings.Contains(result.Unsearched[0].Reason, "cut short") {
		t.Fatalf("reason = %q", result.Unsearched[0].Reason)
	}
}
