package adt

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Correlating a dump with the application log is a time join, and a time join
// is where a tool starts lying. Two things happening in the same second is not
// causation, and "the cause was X" printed from proximity will be confidently
// wrong on the first hard case — which is exactly when someone will believe it.
//
// What rescues it is that a log entry records the program that wrote it. If
// that program is the one that died, the connection is structural, not
// coincidental. So time is the filter and the program is the reason, rather
// than the other way round.

// LogMatch is one application log entry offered as related to a dump, with the
// argument for why.
type LogMatch struct {
	Entry AppLogEntry `json:"entry"`
	// Score ranks the match. Higher is stronger; see the constants below.
	Score int `json:"score"`
	// Why is the argument, in words, so a person can overrule the ranking.
	Why string `json:"why"`
	// Offset is how long before the dump the entry was written. Negative means
	// after it.
	Offset time.Duration `json:"-"`
}

// The ladder. The top two rungs are structural: they say the writing program
// was on the path, not merely nearby. The lower ones are the clock, and are
// scored low on purpose.
const (
	// scoreSameProgram: the log was written by the program that dumped.
	scoreSameProgram = 100
	// scoreOnStack: written by a program on the dump's call stack. On the
	// causal path by construction — the stack is what was running.
	scoreOnStack = 80
	// scoreSameUserBefore: same user, shortly before the dump. Weak, and
	// honestly weak: one person doing one thing produces many such rows.
	scoreSameUserBefore = 40
	// scoreSameUserAfter: same user, after the dump. Weaker still, and kept
	// apart deliberately — a log written after the failure is cleanup or error
	// handling, not cause. Sorting purely by distance in seconds loses that.
	scoreSameUserAfter = 20
	// scoreInWindow: nothing but the clock.
	scoreInWindow = 10
)

// CorrelateDump finds application log entries around a dump and ranks them.
//
// The tolerance is a window on both sides: before, because that is where a
// cause would be, and after, because error handling writes there and is worth
// seeing even though it explains nothing.
func (c *Client) CorrelateDump(ctx context.Context, dump Dump, tolerance time.Duration, limit int) ([]LogMatch, error) {
	// The stack is the strongest evidence available without a call graph, and
	// it costs one request. A dump that will not give up its stack is not a
	// reason to refuse the correlation — it only means the top rung is unused.
	var stack []DumpFrame
	if dump.ID != "" {
		if frames, err := c.DumpStack(ctx, dump.ID); err == nil {
			stack = frames
		}
	}
	return c.correlateWithStack(ctx, dump, stack, tolerance, limit)
}

// correlateWithStack is the body, taking the stack as an argument so it can be
// tested without a system.
func (c *Client) correlateWithStack(ctx context.Context, dump Dump, stack []DumpFrame, tolerance time.Duration, limit int) ([]LogMatch, error) {
	if dump.At.IsZero() {
		return nil, fmt.Errorf("this dump carries no timestamp, so there is nothing to correlate against")
	}
	if tolerance <= 0 {
		tolerance = 5 * time.Minute
	}
	if limit <= 0 {
		limit = 20
	}

	from, to := dump.At.Add(-tolerance), dump.At.Add(tolerance)
	// The log is filtered by day because SAP keeps the date and the clock in
	// separate columns; the window itself is applied here, where both are known.
	entries, err := c.ApplicationLog(ctx, AppLogFilter{
		From:  from,
		To:    to,
		Limit: limit * 25,
	})
	if err != nil {
		return nil, err
	}

	matches := make([]LogMatch, 0, len(entries))
	for _, e := range entries {
		if e.At.IsZero() || e.At.Before(from) || e.At.After(to) {
			continue
		}
		match := LogMatch{Entry: e, Offset: dump.At.Sub(e.At)}
		match.Score, match.Why = rankLogAgainstDump(e, dump, stack, match.Offset)
		matches = append(matches, match)
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		// Within a rung, nearer the dump first, measured on the absolute gap.
		return abs(matches[i].Offset) < abs(matches[j].Offset)
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches, nil
}

func rankLogAgainstDump(e AppLogEntry, dump Dump, stack []DumpFrame, offset time.Duration) (int, string) {
	when := "before"
	if offset < 0 {
		when = "after"
	}
	gap := abs(offset).Round(time.Second)

	if e.Program != "" && dump.Program != "" && equalFoldTrim(e.Program, dump.Program) {
		return scoreSameProgram, fmt.Sprintf("written by %s, the program that dumped — %s %s", e.Program, gap, when)
	}
	if frame, on := frameFor(stack, e.Program); on {
		return scoreOnStack, fmt.Sprintf("written by %s, frame %d of the dump stack (%s) — %s %s",
			e.Program, frame.Position, orUnknown(frame.Name), gap, when)
	}
	if e.User != "" && dump.User != "" && equalFoldTrim(e.User, dump.User) {
		if offset >= 0 {
			return scoreSameUserBefore, fmt.Sprintf("same user, %s before", gap)
		}
		return scoreSameUserAfter, fmt.Sprintf("same user, %s after — likely error handling, not cause", gap)
	}
	return scoreInWindow, fmt.Sprintf("only within the window, %s %s", gap, when)
}

func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func equalFoldTrim(a, b string) bool {
	return len(a) > 0 && len(b) > 0 &&
		trimUpper(a) == trimUpper(b)
}

func trimUpper(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// frameFor finds the stack frame a log entry's program belongs to.
func frameFor(stack []DumpFrame, program string) (DumpFrame, bool) {
	if strings.TrimSpace(program) == "" {
		return DumpFrame{}, false
	}
	want := trimUpper(program)
	for _, frame := range stack {
		if trimUpper(frame.Program) == want {
			return frame, true
		}
	}
	return DumpFrame{}, false
}
