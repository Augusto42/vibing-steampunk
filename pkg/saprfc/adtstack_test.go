package saprfc

import (
	"context"
	"strings"
	"testing"
)

// countingTransport answers from a table and remembers what was asked, so a
// test can assert not only the result but how many requests it cost.
type countingTransport struct {
	answers map[string]*ADTResponse
	asked   []string
}

func (c *countingTransport) Do(ctx context.Context, req ADTRequest) (*ADTResponse, error) {
	key := req.Method + " " + strings.Split(req.URI, "?")[0]
	c.asked = append(c.asked, key)
	if res, ok := c.answers[key]; ok {
		return res, nil
	}
	return &ADTResponse{Status: 404, ReasonPhrase: "Not Found"}, nil
}

func (c *countingTransport) count(key string) int {
	n := 0
	for _, asked := range c.asked {
		if asked == key {
			n++
		}
	}
	return n
}

const stackDocument = `<?xml version="1.0" encoding="utf-8"?>` +
	`<dbg:stack isRfc="true" isSameSystem="true" xmlns:dbg="http://www.sap.com/adt/debugger">` +
	`<stackEntry programName="ZVSP_DEBUG_DEMO" includeName="ZVSP_DEBUG_DEMO" line="26" ` +
	`stackPosition="1" stackType="ABAP" eventType="EVENT" eventName="START-OF-SELECTION"/>` +
	`</dbg:stack>`

const (
	stackResourcePath   = "GET /sap/bc/adt/debugger/stack"
	stackDispatcherPath = "POST /sap/bc/adt/debugger"
)

// A release that has no dedicated stack resource still has a stack. 7.50 does
// not expose /sap/bc/adt/debugger/stack — it is absent from discovery and
// answers 404 — and serves the same document from the dispatcher. Before this,
// the first stack read on such a system ended the session, and because the
// catch does a stack read, every caught debuggee was thrown away.
func TestStackFallsBackToTheOlderShape(t *testing.T) {
	transport := &countingTransport{answers: map[string]*ADTResponse{
		stackDispatcherPath: {Status: 200, Body: []byte(stackDocument)},
	}}
	dbg := NewADTDebugger(transport, "TESTUSER")

	res, err := dbg.ADTStack(context.Background())
	if err != nil {
		t.Fatalf("the stack is readable on this release, just not where we looked first: %v", err)
	}
	if !strings.Contains(string(res.Body), "stackEntry") {
		t.Fatalf("should have returned the stack document, got %q", res.Body)
	}
	if transport.count(stackResourcePath) != 1 {
		t.Fatalf("the modern resource should be tried first, asked %d times", transport.count(stackResourcePath))
	}
}

// Discovering the shape costs one 404. Paying it on every step would triple the
// requests of a stepping session on the release that can least afford it.
func TestStackRemembersWhichShapeWorked(t *testing.T) {
	transport := &countingTransport{answers: map[string]*ADTResponse{
		stackDispatcherPath: {Status: 200, Body: []byte(stackDocument)},
	}}
	dbg := NewADTDebugger(transport, "TESTUSER")
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := dbg.ADTStack(ctx); err != nil {
			t.Fatalf("read %d: %v", i+1, err)
		}
	}
	if got := transport.count(stackResourcePath); got != 1 {
		t.Fatalf("the absent resource should be asked for once, not %d times", got)
	}
	if got := transport.count(stackDispatcherPath); got != 3 {
		t.Fatalf("every read should reach the dispatcher, got %d", got)
	}
}

// Where the resource exists, nothing changes: it answers, and the older shape
// is never tried.
func TestStackPrefersTheResourceWhereItExists(t *testing.T) {
	transport := &countingTransport{answers: map[string]*ADTResponse{
		stackResourcePath: {Status: 200, Body: []byte(stackDocument)},
	}}
	dbg := NewADTDebugger(transport, "TESTUSER")

	if _, err := dbg.ADTStack(context.Background()); err != nil {
		t.Fatalf("reading the stack: %v", err)
	}
	if got := transport.count(stackDispatcherPath); got != 0 {
		t.Fatalf("the older shape should not be tried at all, asked %d times", got)
	}
}

// A refusal is not a missing resource. Retrying a 403 or a 500 in another shape
// would hide the reason the caller needs.
func TestStackDoesNotRetryARealFailure(t *testing.T) {
	transport := &countingTransport{answers: map[string]*ADTResponse{
		stackResourcePath: {Status: 403, ReasonPhrase: "Forbidden",
			Body: []byte(`<exc:exception xmlns:exc="x"><message lang="EN">Not authorised</message></exc:exception>`)},
		stackDispatcherPath: {Status: 200, Body: []byte(stackDocument)},
	}}
	dbg := NewADTDebugger(transport, "TESTUSER")

	_, err := dbg.ADTStack(context.Background())
	if err == nil {
		t.Fatal("a refusal must be reported, not worked around")
	}
	if !strings.Contains(err.Error(), "Not authorised") {
		t.Fatalf("the server's reason should reach the caller, got: %v", err)
	}
	if got := transport.count(stackDispatcherPath); got != 0 {
		t.Fatalf("a refusal must not be retried in another shape, asked %d times", got)
	}
}
