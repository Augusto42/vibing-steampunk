package adt

import (
	"context"
	"strings"
	"time"
)

// Nothing that runs ABAP through ADT is told that the ABAP died. A unit test
// wrapper, an RFC call, a SUBMIT of a report that writes a list in a session
// with no screen — the request comes back 200 and the runtime error is written
// somewhere else entirely, into ST22, by the work process that terminated. The
// caller is left saying "executed successfully" about code that produced a
// dump. The only way to connect the two from this side is to look at the dump
// feed before and after the run and see what appeared in between.
//
// The obvious way to do that — remember the newest dump's timestamp and treat
// anything later as new — does not survive contact with a real system. Dumps
// are stamped by SAP's clock and the comparison would be made against ours, and
// on the system this was written against the two sat six seconds apart: a dump
// caused at 23:58:51 by our clock arrived stamped 23:58:45, five seconds
// *before* the run that caused it. A timestamp test misses that dump every
// single time, and misses it silently, which is the failure mode we are here to
// remove.
//
// Dump ids do not have that problem. Each one is unique — SAP builds it from
// the timestamp, the host, the user, the client and a counter — so remembering
// the ids the feed already held and subtracting them afterwards is exact and
// needs no agreement between the clocks at all. Time is then used for one thing
// only: bounding how much feed we ask for, where being a few minutes generous
// costs nothing.

// DumpWatch remembers which runtime errors were already there before something
// ran, so that the ones that were not can be recognised afterwards.
type DumpWatch struct {
	// User, when set, is the only author whose dumps count. Left empty — under
	// single sign-on there is frequently no user name on this side at all — a
	// dump by anyone counts, and the caller has to be correspondingly careful
	// about what it claims.
	User string
	// From bounds both readings of the feed. The unbounded feed is megabytes on
	// a system that has been up for a while (2 MB on a quiet one); a window of
	// minutes is a few kilobytes.
	From time.Time

	seen map[string]bool
}

// dumpWatchWindow is how far back the two feed readings reach. It has to
// outlast the code being watched plus any disagreement between our clock and
// SAP's, and the only cost of being generous is a slightly larger feed.
const dumpWatchWindow = 15 * time.Minute

// dumpWatchPoll is how often the feed is re-read while waiting for a dump to
// surface. ST22 is written by the dying work process after our request has
// already been answered, so the dump can be a moment behind us.
const dumpWatchPoll = 500 * time.Millisecond

// dumpWatchLimit caps each reading. Higher than anything a single short run
// could plausibly produce, low enough that a system in a dump storm does not
// turn one execute into a report.
const dumpWatchLimit = 200

// NewDumpWatch records what the feed already held.
//
// The reading is the caller's, not this function's, which is what makes the
// interesting half of dump watching testable without a system.
func NewDumpWatch(user string, before []Dump) *DumpWatch {
	w := &DumpWatch{User: user, seen: make(map[string]bool, len(before))}
	for _, d := range before {
		w.seen[d.ID] = true
	}
	return w
}

// Unseen returns the dumps in a later reading that the earlier one did not
// have, newest first, as the feed orders them.
func (w *DumpWatch) Unseen(after []Dump) []Dump {
	var fresh []Dump
	for _, d := range after {
		if w.seen[d.ID] {
			continue
		}
		// The feed read is already filtered by author, but a caller that reads
		// it some other way should not be able to turn somebody else's dump
		// into ours by accident.
		if w.User != "" && !strings.EqualFold(strings.TrimSpace(w.User), strings.TrimSpace(d.User)) {
			continue
		}
		fresh = append(fresh, d)
	}
	return fresh
}

// WatchDumps reads the runtime error feed as it stands now, so that a later
// reading can be compared against it. Call it before running anything whose
// failure would land in ST22 rather than in the response.
func (c *Client) WatchDumps(ctx context.Context, user string) (*DumpWatch, error) {
	from := time.Now().Add(-dumpWatchWindow)
	before, err := c.Dumps(ctx, DumpFilter{From: from, User: user, Limit: dumpWatchLimit})
	if err != nil {
		return nil, err
	}
	w := NewDumpWatch(user, before)
	w.From = from
	return w, nil
}

// AwaitDumps reads the feed again and returns the runtime errors that appeared
// while the watch was open.
//
// It keeps asking for `settle` because the dump is written after the request
// that caused it was answered — on the system this was developed against the
// dump showed up about a second later, but that is a measurement, not a
// guarantee, so the wait is the caller's to choose. It returns as soon as
// something new appears, so the wait is only paid in full by a run that did not
// dump at all.
func (c *Client) AwaitDumps(ctx context.Context, w *DumpWatch, settle time.Duration) ([]Dump, error) {
	if w == nil {
		return nil, nil
	}
	deadline := time.Now().Add(settle)
	for {
		after, err := c.Dumps(ctx, DumpFilter{From: w.From, User: w.User, Limit: dumpWatchLimit})
		if err != nil {
			return nil, err
		}
		if fresh := w.Unseen(after); len(fresh) > 0 {
			return fresh, nil
		}
		if !time.Now().Before(deadline) {
			return nil, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(dumpWatchPoll):
		}
	}
}
