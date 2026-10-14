//go:build integration

package saprfc

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/oisee/open-rfc-go/rfc"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// What one debugger round trip costs, on each transport.
//
// This is the number the stepping recorder lives or dies by. Stepping a region
// from outside the debuggee costs at least one round trip per statement, plus
// one per stack read and one per variable read, so "can we step the whole area
// of interest" is entirely a question of milliseconds per operation. Measuring
// it beats guessing, and the answer differs per transport and per network.
//
//	SAP_URL=… SAP_USER=… SAP_PASSWORD=… go test -tags=integration -run StepCost -v ./pkg/saprfc/
func TestStepCost(t *testing.T) {
	dest := integrationDestination(t)
	target, line := debugTarget()

	for _, tc := range []struct {
		name string
		open func(context.Context, *testing.T) (*Debugger, func())
	}{
		{"rfc-tunnel", openTunnelDebugger},
		{"https", openHTTPSDebugger},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			dbg, done := tc.open(ctx, t)
			defer done()

			if err := dbg.ADTClearBreakpoints(ctx); err != nil {
				t.Fatalf("clearing breakpoints: %v", err)
			}
			if _, err := dbg.ADTAddBreakpoint(ctx, target, line, ""); err != nil {
				t.Fatalf("setting the breakpoint: %v", err)
			}
			fired := make(chan error, 1)
			go func() {
				time.Sleep(4 * time.Second)
				fired <- callInOwnSession(dest, target)
			}()
			who, _, err := dbg.ADTCatch(ctx, dest.User, IDEID, TerminalID, 60)
			if err != nil || who == nil {
				t.Fatalf("catching a debuggee: %v", err)
			}

			// The debuggee stays where it is for these, so the only thing being
			// measured is the round trip.
			const rounds = 20
			start := time.Now()
			for i := 0; i < rounds; i++ {
				if _, err := dbg.StackInfo(ctx); err != nil {
					t.Fatalf("stack read %d: %v", i, err)
				}
			}
			perStack := time.Since(start) / rounds

			start = time.Now()
			for i := 0; i < rounds; i++ {
				if _, err := dbg.Locals(ctx); err != nil {
					t.Fatalf("locals read %d: %v", i, err)
				}
			}
			perLocals := time.Since(start) / rounds

			// Steps are bounded by the debuggee: this one is a handful of
			// statements long, so count what succeeds rather than demanding N.
			steps := 0
			start = time.Now()
			for i := 0; i < rounds; i++ {
				if _, err := dbg.ADTStep(ctx, "stepInto"); err != nil {
					break
				}
				steps++
			}
			var perStep time.Duration
			if steps > 0 {
				perStep = time.Since(start) / time.Duration(steps)
			}

			// The same three things as one multipart request, which is what a
			// capture mode actually issues.
			batched := 0
			start = time.Now()
			for i := 0; i < rounds; i++ {
				if _, err := dbg.CaptureStep(ctx, ""); err != nil {
					t.Fatalf("batched capture %d: %v", i, err)
				}
				batched++
			}
			perBatch := time.Since(start) / time.Duration(batched)

			t.Logf("%s: batched stack+locals %v/op — %.0f stops/minute",
				tc.name, perBatch.Round(time.Millisecond), 60.0/perBatch.Seconds())
			t.Logf("%s: stack %v/op · locals %v/op (2 calls) · step %v/op over %d steps",
				tc.name, perStack.Round(time.Millisecond), perLocals.Round(time.Millisecond),
				perStep.Round(time.Millisecond), steps)
			t.Logf("%s: a step+stack+locals record costs about %v — %.0f statements/minute",
				tc.name, (perStep + perStack + perLocals).Round(time.Millisecond),
				60.0/(perStep+perStack+perLocals).Seconds())

			_ = dbg.ADTDetach(ctx)
			select {
			case <-fired:
			case <-time.After(30 * time.Second):
			}
		})
	}
}

var _ = adt.DebugVariable{}
var _ = rfc.Params{}
var _ = os.Getenv
