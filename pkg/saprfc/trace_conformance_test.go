//go:build integration

package saprfc

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// A trace is ADT REST like everything else here, so it must read identically
// over the RFC tunnel and over HTTPS. Unlike the debugger it needs no session at
// all, which is worth asserting rather than assuming: it is the reason the
// pooled connection is enough and no conversation has to be pinned.
//
//	SAP_URL=… SAP_USER=… SAP_PASSWORD=… go test -tags=integration -run Conformance_Trace ./pkg/saprfc/
func TestConformance_TraceAcrossTransports(t *testing.T) {
	dest := integrationDestination(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	c, err := OpenWithTimeout(ctx, dest, time.Minute)
	if err != nil {
		t.Skipf("no RFC channel to %s: %v", dest.Host, err)
	}
	defer func() { _ = c.Close(ctx) }()
	overRFC := NewTracer(RFCTunnel(c), dest.User, dest.Client)

	url, user, pass := os.Getenv("SAP_URL"), os.Getenv("SAP_USER"), os.Getenv("SAP_PASSWORD")
	client := os.Getenv("SAP_CLIENT")
	if client == "" {
		client = "001"
	}
	opts := []adt.Option{adt.WithClient(client), adt.WithLanguage("EN"), adt.WithTimeout(time.Minute)}
	if os.Getenv("SAP_INSECURE") == "true" {
		opts = append(opts, adt.WithInsecureSkipVerify())
	}
	overHTTP := NewTracer(HTTPSession(adt.NewTransport(adt.NewConfig(url, user, pass, opts...))), user, client)

	rfcTraces, err := overRFC.Traces(ctx)
	if err != nil {
		t.Fatalf("listing traces over the tunnel: %v", err)
	}
	httpTraces, err := overHTTP.Traces(ctx)
	if err != nil {
		t.Fatalf("listing traces over HTTPS: %v", err)
	}
	if len(rfcTraces) != len(httpTraces) {
		t.Fatalf("the transports disagree about how many traces exist: rfc %d, https %d",
			len(rfcTraces), len(httpTraces))
	}
	if len(rfcTraces) == 0 {
		t.Skip("this system holds no traces; run 'vsp trace run <FM> --call' first")
	}

	// A tree, read twice. Pick a non-aggregated one: an aggregated trace has no
	// call hierarchy to compare.
	var id string
	for _, f := range rfcTraces {
		if !f.Aggregated {
			id = f.ID
			break
		}
	}
	if id == "" {
		t.Skip("no non-aggregated trace to compare; arm one with CallTreeParams")
	}

	a, err := overRFC.Tree(ctx, id)
	if err != nil {
		t.Fatalf("reading the tree over the tunnel: %v", err)
	}
	b, err := overHTTP.Tree(ctx, id)
	if err != nil {
		t.Fatalf("reading the tree over HTTPS: %v", err)
	}
	if len(a) != len(b) {
		t.Fatalf("tree %s has %d statements over the tunnel and %d over HTTPS", id, len(a), len(b))
	}
	for i := range a {
		if a[i].CallLevel != b[i].CallLevel || a[i].Kind != b[i].Kind || a[i].Target != b[i].Target {
			t.Fatalf("statement %d differs:\n  rfc:   level %d %s %s\n  https: level %d %s %s",
				i, a[i].CallLevel, a[i].Kind, a[i].Target, b[i].CallLevel, b[i].Kind, b[i].Target)
		}
	}
	t.Logf("%d statements identical over both transports", len(a))
}
