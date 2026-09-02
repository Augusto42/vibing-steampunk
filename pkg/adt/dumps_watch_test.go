package adt

import (
	"testing"
	"time"
)

func at(clock string) time.Time {
	t, err := time.Parse(time.RFC3339, clock)
	if err != nil {
		panic(err)
	}
	return t
}

// The whole point of watching by id rather than by clock. On the system this
// was written against SAP's clock ran six seconds behind ours, so the dump
// caused by a run that started at 23:58:51 came back stamped 23:58:45 — before
// the run that caused it. Anything comparing timestamps declares that run
// clean.
func TestDumpWatchSeesADumpStampedBeforeTheRunStarted(t *testing.T) {
	before := []Dump{{ID: "old", User: "TESTUSER", At: at("2026-08-22T23:57:03Z")}}
	after := []Dump{
		{ID: "new", User: "TESTUSER", At: at("2026-08-22T23:58:45Z"), ErrorType: "DYNPRO_SEND_IN_BACKGROUND"},
		{ID: "old", User: "TESTUSER", At: at("2026-08-22T23:57:03Z")},
	}

	watch := NewDumpWatch("TESTUSER", before)
	watch.From = at("2026-08-22T23:58:51Z") // our clock, later than the dump's

	fresh := watch.Unseen(after)
	if len(fresh) != 1 {
		t.Fatalf("expected the one new dump, got %d", len(fresh))
	}
	if fresh[0].ID != "new" {
		t.Fatalf("wrong dump: %q", fresh[0].ID)
	}
}

func TestDumpWatchIgnoresWhatWasAlreadyThere(t *testing.T) {
	existing := []Dump{
		{ID: "a", User: "TESTUSER"},
		{ID: "b", User: "TESTUSER"},
	}
	if fresh := NewDumpWatch("TESTUSER", existing).Unseen(existing); len(fresh) != 0 {
		t.Fatalf("nothing changed, yet %d dumps were called new", len(fresh))
	}
}

// A shared system dumps for other people while ours runs, and blaming our code
// for those would make the check worse than not having one.
func TestDumpWatchIgnoresOtherUsers(t *testing.T) {
	watch := NewDumpWatch("TESTUSER", nil)
	fresh := watch.Unseen([]Dump{
		{ID: "theirs", User: "OTHERUSER"},
		{ID: "ours", User: "testuser"}, // case is SAP's business, not ours
	})
	if len(fresh) != 1 || fresh[0].ID != "ours" {
		t.Fatalf("expected only our own dump, got %+v", fresh)
	}
}

// Under single sign-on there is often no user name on this side at all. Then
// every dump counts, and the caller has to hedge its wording instead.
func TestDumpWatchWithoutAUserTakesEverything(t *testing.T) {
	fresh := NewDumpWatch("", nil).Unseen([]Dump{{ID: "a", User: "SOMEONE"}, {ID: "b"}})
	if len(fresh) != 2 {
		t.Fatalf("expected both dumps, got %d", len(fresh))
	}
}

// The feed hands them over newest first and the caller reports them in that
// order, so the first line printed is the most recent failure.
func TestDumpWatchKeepsFeedOrder(t *testing.T) {
	fresh := NewDumpWatch("", nil).Unseen([]Dump{
		{ID: "newer", At: at("2026-08-22T23:58:45Z")},
		{ID: "older", At: at("2026-08-22T23:57:03Z")},
	})
	if len(fresh) != 2 || fresh[0].ID != "newer" {
		t.Fatalf("order was not preserved: %+v", fresh)
	}
}
