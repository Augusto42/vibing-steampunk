package adt

import (
	"testing"
	"time"
)

// A caller who quotes a full dump id was handed a Dump carrying nothing but
// that id — no time, no program, no error type — and explain_dump then failed
// with "this dump carries no timestamp". The timestamp is the first fourteen
// characters of the id it was given.
func TestDumpTimeIsRecoveredFromTheID(t *testing.T) {
	const id = "/sap/bc/adt/vit/runtime/dumps/20260824012009devsys_A4H_00" +
		"%20%20%20%20%20TESTUSER%20%20%20001%20%20%20%203"
	got := DumpTimeFromID(id)
	want := time.Date(2026, 8, 24, 1, 20, 9, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("DumpTimeFromID = %s, want %s", got, want)
	}
	if got.Location() != time.UTC {
		t.Errorf("recovered time is %s, not UTC; a correlation window would be shifted", got.Location())
	}
}

// Recovery must be distinguishable from invention: an id with no timestamp
// returns the zero time so the caller can say so rather than correlate against
// the epoch.
func TestAnIDWithoutATimestampRecoversNothing(t *testing.T) {
	for _, id := range []string{
		"",
		"/sap/bc/adt/vit/runtime/dumps/",
		"/sap/bc/adt/vit/runtime/dumps/notatimestamp0",
		"/sap/bc/adt/vit/runtime/dumps/2026082401", // too short
		"latest",
	} {
		if got := DumpTimeFromID(id); !got.IsZero() {
			t.Errorf("DumpTimeFromID(%q) = %s, want the zero time", id, got)
		}
	}
}

// A bare id with no path in front of it is what a caller pastes most often.
func TestABareIDWorksToo(t *testing.T) {
	got := DumpTimeFromID("20260101235959devsys_A4H_00 TESTUSER 001 1")
	if got.IsZero() {
		t.Fatal("a bare id was not recognised")
	}
	if got.Year() != 2026 || got.Hour() != 23 || got.Second() != 59 {
		t.Errorf("parsed as %s", got)
	}
}
