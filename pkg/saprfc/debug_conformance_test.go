//go:build integration

package saprfc

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oisee/open-rfc-go/rfc"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// The debugger is one feature carried by two transports, and the whole point of
// the ADTTransport interface is that neither is privileged. That claim is only
// worth what it is tested against, so this runs the same script — breakpoint,
// listen, attach, stack, variables, release — over each in turn and requires
// them to agree.
//
//	SAP_URL=… SAP_USER=… SAP_PASSWORD=… go test -tags=integration -run Conformance ./pkg/saprfc/
//
// It needs a debuggee to catch. ZADT_DEBUG_LOOP is the one this landscape keeps
// for the purpose: an RFC-enabled module that stops at a known line and, once
// released, leaves a visible trace in TVARVC. Set DEBUG_TARGET/DEBUG_LINE to aim
// somewhere else.

const (
	defaultDebugTarget = "ZADT_DEBUG_LOOP"
	defaultDebugLine   = 9
)

// stop is what one transport observed, reduced to the facts both must agree on.
type stop struct {
	program string
	include string
	line    int
	event   string
	locals  []string
}

func TestConformance_DebuggerAcrossTransports(t *testing.T) {
	dest := integrationDestination(t)
	target, line := debugTarget()

	results := map[string]stop{}
	for _, tc := range []struct {
		name string
		open func(context.Context, *testing.T) (*Debugger, func())
	}{
		{"rfc-tunnel", openTunnelDebugger},
		{"https", openHTTPSDebugger},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()

			dbg, done := tc.open(ctx, t)
			defer done()

			if err := dbg.ADTClearBreakpoints(ctx); err != nil {
				t.Fatalf("clearing breakpoints: %v", err)
			}
			bps, err := dbg.ADTAddBreakpoint(ctx, target, line, "")
			if err != nil {
				t.Fatalf("setting a breakpoint on %s:%d: %v", target, line, err)
			}
			if len(bps) != 1 {
				t.Fatalf("expected one breakpoint, got %d", len(bps))
			}

			// The debuggee has to come from somewhere else: a session cannot
			// catch itself. Fire it once the listener is up.
			fired := make(chan error, 1)
			go func() {
				time.Sleep(5 * time.Second)
				fired <- callInOwnSession(dest, target)
			}()

			who, _, err := dbg.ADTCatch(ctx, dest.User, IDEID, TerminalID, 90)
			if err != nil {
				t.Fatalf("catching a debuggee: %v", err)
			}
			if who == nil {
				t.Fatal("nobody stopped: the breakpoint did not fire within 90s")
			}

			info, err := dbg.StackInfo(ctx)
			if err != nil {
				t.Fatalf("reading the stack: %v", err)
			}
			if len(info.Stack) == 0 {
				t.Fatal("an attached debuggee reported an empty stack")
			}
			top := info.Stack[0]

			vars, err := dbg.Locals(ctx)
			if err != nil {
				t.Fatalf("reading the locals: %v", err)
			}
			var names []string
			for _, v := range vars {
				names = append(names, v.Name)
			}

			if err := dbg.ADTDetach(ctx); err != nil {
				t.Errorf("releasing the debuggee: %v", err)
			}
			// A debuggee that is not released hangs its caller until the RFC
			// timeout, so this is part of the contract, not cleanup.
			select {
			case err := <-fired:
				if err != nil {
					t.Errorf("%s never returned after the release: %v", target, err)
				}
			case <-time.After(60 * time.Second):
				t.Errorf("%s was still suspended a minute after detach", target)
			}

			results[tc.name] = stop{
				program: top.ProgramName, include: top.IncludeName, line: top.Line,
				event: top.EventName, locals: names,
			}
		})
	}

	if len(results) < 2 {
		t.Skip("only one transport ran; nothing to compare")
	}
	a, b := results["rfc-tunnel"], results["https"]
	if a.program != b.program || a.include != b.include || a.line != b.line || a.event != b.event {
		t.Errorf("the transports disagree about where the debuggee stopped:\n  rfc:   %s/%s:%d %s\n  https: %s/%s:%d %s",
			a.program, a.include, a.line, a.event, b.program, b.include, b.line, b.event)
	}
	if strings.Join(a.locals, ",") != strings.Join(b.locals, ",") {
		t.Errorf("the transports disagree about the locals:\n  rfc:   %v\n  https: %v", a.locals, b.locals)
	}
}

// openTunnelDebugger pins an RFC conversation and tunnels ADT through it.
func openTunnelDebugger(ctx context.Context, t *testing.T) (*Debugger, func()) {
	t.Helper()
	dest := integrationDestination(t)
	// The listener occupies the conversation for its whole duration, so the
	// call timeout has to outlast it.
	c, err := OpenWithTimeout(ctx, dest, 5*time.Minute)
	if err != nil {
		t.Skipf("no RFC channel to %s: %v", dest.Host, err)
	}
	dbg, err := NewDebugger(ctx, c, dest.User)
	if err != nil {
		t.Fatalf("pinning a debug session: %v", err)
	}
	return dbg, func() {
		_ = dbg.Close(ctx)
		_ = c.Close(ctx)
	}
}

// openHTTPSDebugger holds one stateful ADT session for the whole loop.
func openHTTPSDebugger(ctx context.Context, t *testing.T) (*Debugger, func()) {
	t.Helper()
	url, user, pass := os.Getenv("SAP_URL"), os.Getenv("SAP_USER"), os.Getenv("SAP_PASSWORD")
	client := os.Getenv("SAP_CLIENT")
	if client == "" {
		client = "001"
	}
	opts := []adt.Option{
		adt.WithClient(client),
		adt.WithLanguage("EN"),
		adt.WithSessionType(adt.SessionStateful),
		adt.WithTimeout(5 * time.Minute),
	}
	if os.Getenv("SAP_INSECURE") == "true" {
		opts = append(opts, adt.WithInsecureSkipVerify())
	}
	cfg := adt.NewConfig(url, user, pass, opts...)
	dbg := NewADTDebugger(HTTPSession(adt.NewTransport(cfg)), user)
	return dbg, func() { _ = dbg.Close(ctx) }
}

// callInOwnSession calls the debuggee over a connection of its own, which is
// what makes it a debuggee: a session cannot stop at its own breakpoint.
func callInOwnSession(dest Params, function string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c, err := OpenWithTimeout(ctx, dest, 3*time.Minute)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close(ctx) }()
	_, err = c.Call(ctx, function, rfc.Params{})
	return err
}

func integrationDestination(t *testing.T) Params {
	t.Helper()
	url, user, pass := os.Getenv("SAP_URL"), os.Getenv("SAP_USER"), os.Getenv("SAP_PASSWORD")
	if url == "" || user == "" || pass == "" {
		t.Skip("SAP_URL, SAP_USER and SAP_PASSWORD are required for the conformance run")
	}
	client := os.Getenv("SAP_CLIENT")
	if client == "" {
		client = "001"
	}
	dest, err := Resolve(Input{URL: url, User: user, Password: pass, Client: client, Language: "EN"})
	if err != nil {
		t.Skipf("resolving an RFC destination from %s: %v", url, err)
	}
	return dest
}

func debugTarget() (string, int) {
	target := os.Getenv("DEBUG_TARGET")
	if target == "" {
		target = defaultDebugTarget
	}
	line := defaultDebugLine
	if s := os.Getenv("DEBUG_LINE"); s != "" {
		if n := atoiSafe(s); n > 0 {
			line = n
		}
	}
	return target, line
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
