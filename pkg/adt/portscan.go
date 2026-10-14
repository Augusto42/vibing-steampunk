package adt

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// A landscape file says nothing about HTTP. It describes SAP GUI connectivity —
// dispatcher ports at 3200+nn, message servers at 3600+nn — because SAP GUI
// speaks DIAG and never needs an HTTP port. Eclipse ADT does not derive one
// either: it asks the person setting up the project. So for a host reached
// through single sign-on, where there is no password to fall back on and no
// gateway route, nothing on the machine knows where to knock.
//
// The convention — HTTPS at 443nn, HTTP at 80nn — is a starting guess and often
// wrong: a system behind a web dispatcher answers on 443, and one measured here
// answers on 8422, which follows no rule at all. So the port is found by asking.

// PortFinding is what one port turned out to be.
type PortFinding struct {
	Port int    `json:"port"`
	URL  string `json:"url"`
	// Kind says how far the probe got, which is the difference between "wrong
	// port" and "right port, and now an authorization problem".
	Kind PortKind `json:"kind"`
	// Status is the HTTP status the ADT path answered with, when it answered.
	Status int `json:"status,omitempty"`
	// Detail carries what a reader needs and cannot guess — a certificate for
	// another name, most often.
	Detail string `json:"detail,omitempty"`
	// CertHost is the name a mismatched certificate was issued for. It is not
	// an error detail but a lead: that is the name this port is served under,
	// and reaching it is a matter of using it.
	CertHost string `json:"certHost,omitempty"`
	// Secure reports whether the answer came over TLS.
	Secure bool `json:"secure"`
}

// PortKind is how far a probe of one port got.
type PortKind string

const (
	// PortADT means the ADT resource answered: this is the port to configure.
	PortADT PortKind = "adt"
	// PortSAPNoADT means SAP answered but not on the ADT path — the ICF node
	// is likely inactive, which is a different conversation with basis.
	PortSAPNoADT PortKind = "sap-without-adt"
	// PortHTTP means something HTTP is listening that is not recognisably SAP.
	PortHTTP PortKind = "http"
	// PortTLSMismatch means a server is there and its certificate is for
	// another name. The port is right; the hostname used to reach it is not.
	PortTLSMismatch PortKind = "tls-name-mismatch"
	// PortOpen means the TCP port accepted a connection and nothing more.
	PortOpen PortKind = "open"
)

// PortScanResult is everything found on a host.
type PortScanResult struct {
	Host     string        `json:"host"`
	Findings []PortFinding `json:"findings"`
	// Unsearched is what stopped the sweep from being a sweep.
	//
	// Per port, a dial that is refused or times out is the answer and is
	// deliberately not recorded — noting three hundred closed ports would bury
	// the one that answered. But a name that does not resolve is not an answer
	// about any port: every probe then fails identically and the scan reports
	// an empty result, which reads as "nothing is listening on that host" when
	// what happened is that nobody ever knocked.
	Unsearched []Unsearched `json:"unsearched,omitempty"`
}

// Best returns the port to configure, if the scan found one.
//
// Among ports that answered, TLS wins. A session cookie is the whole credential
// on a single sign-on system, and sending it in clear because a plain port
// happened to sort first is not a trade worth making silently.
func (r *PortScanResult) Best() *PortFinding {
	for _, kind := range []PortKind{PortADT, PortSAPNoADT, PortTLSMismatch, PortHTTP} {
		for _, secure := range []bool{true, false} {
			for i := range r.Findings {
				if r.Findings[i].Kind == kind && r.Findings[i].Secure == secure {
					return &r.Findings[i]
				}
			}
		}
	}
	return nil
}

// CertificateLead returns a name some port is served under, when the scan met a
// certificate for a host other than the one asked about.
//
// A mismatch is not a dead end: it says the service is there and names where it
// lives. On a system fronted by a web dispatcher the application server presents
// the dispatcher's certificate, and that name is the HTTPS address the caller
// was looking for.
func (r *PortScanResult) CertificateLead() string {
	for _, f := range r.Findings {
		if f.CertHost != "" {
			return f.CertHost
		}
	}
	return ""
}

// CandidatePorts returns the ports worth trying for a host, most likely first.
//
// The instance number, when known, contributes the conventional pair. The rest
// are the ports these systems are actually found on: the default HTTPS port
// where a dispatcher or load balancer terminates, and the handful of defaults
// SAP installs pick.
func CandidatePorts(instanceNr string) []int {
	ports := []int{443}
	if instanceNr != "" {
		var nr int
		if _, err := fmt.Sscanf(instanceNr, "%d", &nr); err == nil && nr >= 0 && nr <= 99 {
			ports = append(ports, 44300+nr, 8000+nr)
		}
	}
	// Instance 00 and 01 cover most single-instance systems; 50000/50001 are
	// what a developer edition and an older stack answer on.
	ports = append(ports, 44300, 44301, 8000, 8001, 50000, 50001, 80, 8080, 8443)

	seen := map[int]bool{}
	out := ports[:0:0]
	for _, p := range ports {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// ExhaustivePorts returns every port an ABAP stack could plausibly serve HTTP
// on: the full conventional range for all hundred instance numbers, plus the
// defaults an installation picks.
//
// This is what to use when the shortlist found nothing and the answer has to be
// somewhere — one system measured here answers on 8422, which belongs to no
// convention and would never appear in a shortlist.
func ExhaustivePorts() []int {
	ports := make([]int, 0, 320)
	ports = append(ports, 443, 80, 8080, 8443)
	for nr := 0; nr <= 99; nr++ {
		// The conventional pair, and the band an ICM is often moved to when
		// 443nn is taken: one system measured here serves HTTP on 8022 and
		// HTTPS on 8422, for an instance the landscape records as 20.
		ports = append(ports, 44300+nr, 8000+nr, 8400+nr)
	}
	ports = append(ports, 50000, 50001, 50100, 50101)

	seen := map[int]bool{}
	out := ports[:0:0]
	for _, p := range ports {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// scanConcurrency bounds the sweep. Two hundred sockets opened at once is the
// kind of thing that reads as a port scan to whatever sits between here and the
// host, and a bounded sweep finishes just as fast.
const scanConcurrency = 32

// ScanForADT finds which port on a host serves ADT.
//
// Ports are tried concurrently: a closed one costs the full connect timeout, and
// a dozen of those in sequence is a minute of waiting for an answer that takes
// two seconds to obtain.
func ScanForADT(ctx context.Context, host string, ports []int, client string, insecure bool) *PortScanResult {
	result := &PortScanResult{Host: host}
	if host == "" || len(ports) == 0 {
		return result
	}

	// Resolved once for the whole sweep rather than implicitly per port: the
	// answer is identical for every one of them, and asking here is what
	// separates a host that refuses connections from a host that does not
	// exist. A literal address resolves to itself, so this costs nothing there.
	if _, err := net.DefaultResolver.LookupHost(ctx, host); err != nil {
		result.Unsearched = append(result.Unsearched, Unsearched{
			Object: host,
			Reason: "the name does not resolve: " + err.Error(),
		})
		return result
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, scanConcurrency)
	for _, port := range ports {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			if finding := probePort(ctx, host, port, client, insecure); finding != nil {
				mu.Lock()
				result.Findings = append(result.Findings, *finding)
				mu.Unlock()
			}
		}(port)
	}
	wg.Wait()

	// A cancelled or expired context makes every probe return nothing, which is
	// indistinguishable from every port being closed unless it is said.
	if err := ctx.Err(); err != nil {
		result.Unsearched = append(result.Unsearched, Unsearched{
			Object: fmt.Sprintf("%s (%d ports)", host, len(ports)),
			Reason: "the scan was cut short: " + err.Error(),
		})
	}

	// Most useful first, and stable, so two runs read the same.
	rank := map[PortKind]int{PortADT: 0, PortSAPNoADT: 1, PortTLSMismatch: 2, PortHTTP: 3, PortOpen: 4}
	sort.Slice(result.Findings, func(i, j int) bool {
		if rank[result.Findings[i].Kind] != rank[result.Findings[j].Kind] {
			return rank[result.Findings[i].Kind] < rank[result.Findings[j].Kind]
		}
		return result.Findings[i].Port < result.Findings[j].Port
	})
	return result
}

// probePort reports what is listening on one port, or nothing at all.
func probePort(ctx context.Context, host string, port int, client string, insecure bool) *PortFinding {
	address := net.JoinHostPort(host, fmt.Sprint(port))

	dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", address)
	if err != nil {
		return nil
	}
	conn.Close()

	// A port that is open but serves nothing recognisable is still worth
	// reporting: it is the difference between "the host is firewalled" and
	// "the host is there and this is the wrong port".
	finding := &PortFinding{Port: port, Kind: PortOpen}

	for _, scheme := range schemesFor(port) {
		base := fmt.Sprintf("%s://%s", scheme, address)
		kind, status, detail, certHost := probeADTPath(ctx, base, client, insecure)
		if kind == "" {
			continue
		}
		finding.Kind, finding.Status, finding.Detail = kind, status, detail
		finding.URL, finding.CertHost = base, certHost
		finding.Secure = scheme == "https"
		if kind == PortADT {
			return finding
		}
	}
	return finding
}

// schemesFor puts the likely scheme first: the plain HTTP ports are 80nn and
// 80/8080, everything else is TLS more often than not.
func schemesFor(port int) []string {
	switch {
	case port == 80 || port == 8080 || (port >= 8000 && port <= 8099):
		return []string{"http", "https"}
	default:
		return []string{"https", "http"}
	}
}

// probeADTPath asks the ADT discovery resource and classifies the answer.
func probeADTPath(ctx context.Context, base, client string, insecure bool) (kind PortKind, status int, detail, certHost string) {
	httpClient := &http.Client{
		Timeout: 6 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
			Proxy:           http.ProxyFromEnvironment,
		},
		// Following a redirect to an identity provider proves nothing about
		// the port and costs a round trip to somewhere else entirely.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	url := base + "/sap/bc/adt/discovery"
	if client != "" {
		url += "?sap-client=" + client
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", 0, "", ""
	}
	// SAP hands Negotiate and the ADT resources to recognised clients only on
	// some systems; asking as Eclipse avoids a refusal that is about the
	// user agent rather than about the port.
	req.Header.Set("User-Agent", "Eclipse/4.39.0 (win32; x86_64) ADT/3.56.0 (devedition)")

	resp, err := httpClient.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "certificate") {
			// The server is there and answering TLS; the name used to reach it
			// is not one the certificate covers. That is a hostname problem,
			// not a port problem, and the certificate names the host to use.
			return PortTLSMismatch, 0, certificateDetail(err.Error()), certificateHost(err.Error())
		}
		return "", 0, "", ""
	}
	defer resp.Body.Close()

	sapResponse := resp.Header.Get("server") != "" && strings.Contains(
		strings.ToLower(resp.Header.Get("server")), "sap")
	switch {
	case resp.StatusCode == http.StatusOK:
		return PortADT, resp.StatusCode, "", ""
	case resp.StatusCode == http.StatusUnauthorized:
		// ADT is there and wants credentials, which is the port answering.
		return PortADT, resp.StatusCode, "wants authentication", ""
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		// A redirect from an ADT path is the single sign-on handshake starting.
		return PortADT, resp.StatusCode, "redirects to sign in", ""
	case resp.StatusCode == http.StatusNotFound && sapResponse:
		return PortSAPNoADT, resp.StatusCode, "SAP answers; the ADT node looks inactive", ""
	case sapResponse:
		return PortSAPNoADT, resp.StatusCode, "", ""
	default:
		return PortHTTP, resp.StatusCode, "", ""
	}
}

// certificateHost pulls the name a certificate was issued for out of a TLS
// error, so a mismatch becomes a lead rather than a complaint.
func certificateHost(msg string) string {
	const marker = "certificate is valid for "
	i := strings.Index(msg, marker)
	if i < 0 {
		return ""
	}
	rest := msg[i+len(marker):]
	// The message reads "valid for a, b, not c" — the first name is enough,
	// and everything from ", not " onward is the name that failed.
	if j := strings.Index(rest, ", not "); j >= 0 {
		rest = rest[:j]
	}
	if j := strings.IndexAny(rest, ",\""); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}

// certificateDetail pulls the readable part out of a TLS error.
func certificateDetail(msg string) string {
	if i := strings.Index(msg, "x509:"); i >= 0 {
		msg = msg[i:]
	}
	if len(msg) > 110 {
		msg = msg[:107] + "..."
	}
	return strings.TrimSpace(msg)
}
