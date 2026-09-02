package adt

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestCandidatePortsPutTheDefaultFirst(t *testing.T) {
	// 443 leads because that is where a landscape fronted by a dispatcher
	// answers, which is most of them.
	ports := CandidatePorts("20")
	if ports[0] != 443 {
		t.Errorf("first port = %d, want 443", ports[0])
	}
	// The instance contributes the conventional pair, which is right on a
	// directly exposed ICM.
	var has44320, has8020 bool
	for _, p := range ports {
		has44320 = has44320 || p == 44320
		has8020 = has8020 || p == 8020
	}
	if !has44320 || !has8020 {
		t.Errorf("ports %v, want the conventional pair for instance 20", ports)
	}
}

func TestCandidatePortsWithoutAnInstance(t *testing.T) {
	ports := CandidatePorts("")
	if len(ports) == 0 || ports[0] != 443 {
		t.Fatalf("ports = %v", ports)
	}
	seen := map[int]bool{}
	for _, p := range ports {
		if seen[p] {
			t.Errorf("port %d appears twice; each one costs a connect timeout", p)
		}
		seen[p] = true
	}
}

func TestCandidatePortsIgnoreNonsense(t *testing.T) {
	// A junk instance must not produce a junk port; scanning 44300+999 wastes
	// a timeout on a port that cannot exist.
	for _, nr := range []string{"abc", "999", "-1", "  "} {
		for _, p := range CandidatePorts(nr) {
			if p < 1 || p > 65535 {
				t.Errorf("instance %q produced port %d", nr, p)
			}
		}
	}
}

func TestSchemesForPutsTheLikelyOneFirst(t *testing.T) {
	if got := schemesFor(8020); got[0] != "http" {
		t.Errorf("port 8020 tries %q first, want http", got[0])
	}
	if got := schemesFor(44300); got[0] != "https" {
		t.Errorf("port 44300 tries %q first, want https", got[0])
	}
	// Both are always tried: a system can serve either on any port it likes.
	if len(schemesFor(443)) != 2 {
		t.Error("only one scheme is attempted")
	}
}

func TestCertificateDetailKeepsTheUsefulPart(t *testing.T) {
	msg := `Get "https://host/x": tls: failed to verify certificate: x509: certificate is valid for other.example, not host`
	got := certificateDetail(msg)
	if !strings.Contains(got, "other.example") {
		t.Errorf("got %q, want the name the certificate carries — that is where to go instead", got)
	}
	if strings.HasPrefix(got, "Get ") {
		t.Errorf("got %q, want the request noise dropped", got)
	}
}

func TestBestPrefersADT(t *testing.T) {
	r := &PortScanResult{Findings: []PortFinding{
		{Port: 80, Kind: PortHTTP},
		{Port: 8000, Kind: PortSAPNoADT},
		{Port: 443, Kind: PortADT},
	}}
	if best := r.Best(); best == nil || best.Kind != PortADT {
		t.Errorf("best = %+v, want the ADT one", best)
	}
}

func TestBestFallsBackAndThenGivesUp(t *testing.T) {
	r := &PortScanResult{Findings: []PortFinding{{Port: 8000, Kind: PortSAPNoADT}}}
	if best := r.Best(); best == nil || best.Kind != PortSAPNoADT {
		t.Errorf("best = %+v, want SAP without ADT", best)
	}
	// An open port that served nothing is not a suggestion.
	empty := &PortScanResult{Findings: []PortFinding{{Port: 22, Kind: PortOpen}}}
	if best := empty.Best(); best != nil {
		t.Errorf("best = %+v, want none", best)
	}
}

// scanOne runs the scanner against a test server and returns what it made of it.
func scanOne(t *testing.T, handler http.HandlerFunc) PortFinding {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	host, portText, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portText)

	result := ScanForADT(context.Background(), host, []int{port}, "", false)
	if len(result.Findings) != 1 {
		t.Fatalf("findings = %+v, want one", result.Findings)
	}
	return result.Findings[0]
}

func TestScanClassifiesAnAnsweringADT(t *testing.T) {
	got := scanOne(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/sap/bc/adt/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	if got.Kind != PortADT {
		t.Errorf("kind = %q, want adt", got.Kind)
	}
}

func TestScanReadsAChallengeAsTheRightPort(t *testing.T) {
	// A 401 from the ADT path is the port answering; the credentials are a
	// separate conversation and not a reason to keep scanning.
	got := scanOne(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	if got.Kind != PortADT {
		t.Errorf("kind = %q, want adt", got.Kind)
	}
}

func TestScanSeparatesSAPWithoutADT(t *testing.T) {
	// SAP is there and the ADT node is not — a different problem from a wrong
	// port, and one basis has to fix.
	got := scanOne(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("server", "SAP NetWeaver Application Server")
		w.WriteHeader(http.StatusNotFound)
	})
	if got.Kind != PortSAPNoADT {
		t.Errorf("kind = %q, want sap-without-adt", got.Kind)
	}
}

func TestScanReportsSomethingElseListening(t *testing.T) {
	got := scanOne(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	if got.Kind != PortHTTP {
		t.Errorf("kind = %q, want http", got.Kind)
	}
}

func TestScanFindsNothingOnAClosedPort(t *testing.T) {
	// Port 1 is not listening anywhere this test runs; a closed port must
	// produce no finding at all rather than an entry to read past.
	result := ScanForADT(context.Background(), "127.0.0.1", []int{1}, "", false)
	if len(result.Findings) != 0 {
		t.Errorf("findings = %+v, want none", result.Findings)
	}
}

func TestExhaustivePortsCoverTheBandsSeenInPractice(t *testing.T) {
	ports := ExhaustivePorts()
	index := map[int]bool{}
	for _, p := range ports {
		index[p] = true
	}

	// The conventional pair for every instance, the default, and the band an
	// ICM is moved to when 443nn is taken — 8422 was measured on a live system
	// whose landscape records instance 20, and no shortlist would reach it.
	for _, want := range []int{443, 44300, 44399, 8000, 8099, 8400, 8422, 8499, 50000} {
		if !index[want] {
			t.Errorf("port %d is not in the exhaustive set", want)
		}
	}

	seen := map[int]bool{}
	for _, p := range ports {
		if seen[p] {
			t.Errorf("port %d appears twice; each repeat is a wasted connect", p)
		}
		seen[p] = true
		if p < 1 || p > 65535 {
			t.Errorf("port %d is not a port", p)
		}
	}
}

func TestExhaustiveIsWiderThanTheShortlist(t *testing.T) {
	// If it were not, the flag would promise something it does not do.
	if len(ExhaustivePorts()) <= len(CandidatePorts("20")) {
		t.Error("the exhaustive set is no larger than the shortlist")
	}
}

func TestBestPrefersTLS(t *testing.T) {
	// On a single sign-on system the session cookie is the whole credential.
	// Choosing a plain port because it sorted first would send it in clear.
	r := &PortScanResult{Findings: []PortFinding{
		{Port: 8020, Kind: PortADT, Secure: false, URL: "http://h:8020"},
		{Port: 443, Kind: PortADT, Secure: true, URL: "https://h:443"},
	}}
	best := r.Best()
	if best == nil || !best.Secure {
		t.Fatalf("best = %+v, want the TLS one", best)
	}
}

func TestBestTakesPlainWhenThereIsNoTLS(t *testing.T) {
	// Preferring TLS must not mean refusing to answer when none is offered.
	r := &PortScanResult{Findings: []PortFinding{
		{Port: 8020, Kind: PortADT, Secure: false, URL: "http://h:8020"},
	}}
	if best := r.Best(); best == nil || best.Port != 8020 {
		t.Errorf("best = %+v, want the plain one", best)
	}
}

func TestCertificateHostIsALead(t *testing.T) {
	tests := map[string]string{
		`x509: certificate is valid for web.example, not app.example`:          "web.example",
		`x509: certificate is valid for a.example, b.example, not app.example`: "a.example",
		`x509: certificate signed by unknown authority`:                        "",
		``: "",
	}
	for msg, want := range tests {
		if got := certificateHost(msg); got != want {
			t.Errorf("certificateHost(%q) = %q, want %q", msg, got, want)
		}
	}
}

func TestCertificateLeadFindsTheOtherName(t *testing.T) {
	// A mismatch is not a dead end: on a system fronted by a web dispatcher the
	// application server presents the dispatcher's certificate, and that name
	// is the HTTPS address the caller was looking for.
	r := &PortScanResult{Findings: []PortFinding{
		{Port: 8020, Kind: PortADT, Secure: false},
		{Port: 443, Kind: PortTLSMismatch, CertHost: "web.example"},
	}}
	if got := r.CertificateLead(); got != "web.example" {
		t.Errorf("lead = %q, want web.example", got)
	}

	none := &PortScanResult{Findings: []PortFinding{{Port: 8020, Kind: PortADT}}}
	if got := none.CertificateLead(); got != "" {
		t.Errorf("lead = %q, want none", got)
	}
}
