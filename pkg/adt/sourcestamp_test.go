package adt

import (
	"strings"
	"testing"
	"time"
)

// The prefix defence, for the third time this week. A class's includes are the
// name padded with '=' and then a section, so LIKE 'ZCL_ORDER%' also matches
// every include of ZCL_ORDER_ITEM. Attributing one object's change time to
// another is invention: the cache would then believe an object unchanged
// because its neighbour was, or refetch everything because its neighbour moved.
func TestIncludePatternDoesNotSwallowNeighbours(t *testing.T) {
	got, ok := includePattern("CLAS", "ZCL_ORDER")
	if !ok {
		t.Fatal("CLAS has no pattern")
	}
	if got != "ZCL_ORDER=%" {
		t.Fatalf("pattern = %q; without the '=' it matches ZCL_ORDER_ITEM too", got)
	}

	// Spelled out as the property, not the string: the pattern must accept the
	// object's own includes and reject the neighbour's.
	mine := "ZCL_ORDER=====================CM001"
	theirs := "ZCL_ORDER_ITEM================CM001"
	if !likeMatch(got, mine) {
		t.Errorf("the pattern rejects the object's own include %q", mine)
	}
	if likeMatch(got, theirs) {
		t.Errorf("the pattern accepts a neighbour's include %q", theirs)
	}
}

// likeMatch is SQL LIKE for the one wildcard these patterns use.
func likeMatch(pattern, s string) bool {
	if len(pattern) > 0 && pattern[len(pattern)-1] == '%' {
		prefix := pattern[:len(pattern)-1]
		return len(s) >= len(prefix) && s[:len(prefix)] == prefix
	}
	return pattern == s
}

func TestIncludePatternPerObjectKind(t *testing.T) {
	cases := []struct {
		typ, name, want string
		ok              bool
	}{
		{"CLAS", "ZCL_DEMO", "ZCL_DEMO=%", true},
		{"INTF", "ZIF_DEMO", "ZIF_DEMO=%", true},
		{"PROG", "ZDEMO_REPORT", "ZDEMO_REPORT", true}, // exact, no wildcard
		{"FUGR", "ZVSP_GIT", "LZVSP_GIT%", true},
		{"TABL", "ZDEMO_T", "", false}, // no source units — must be refused, not guessed
		{"CLAS", "", "", false},
	}
	for _, c := range cases {
		got, ok := includePattern(c.typ, c.name)
		if ok != c.ok || got != c.want {
			t.Errorf("includePattern(%q,%q) = (%q,%v), want (%q,%v)", c.typ, c.name, got, ok, c.want, c.ok)
		}
	}
}

// A kind with no naming rule must be reported as unstampable rather than
// pattern-matched on a guess. A guessed pattern that matches nothing looks
// exactly like an unchanged object.
func TestAnUnknownKindIsRefusedRatherThanGuessed(t *testing.T) {
	if _, ok := includePattern("DDLS", "ZDEMO_CDS"); ok {
		t.Error("a CDS view was given a source-unit pattern; it has none")
	}
}

func TestRepoTimeCombinesDateAndTime(t *testing.T) {
	got := parseRepoTime("20260824", "012009")
	want := time.Date(2026, 8, 24, 1, 20, 9, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseRepoTime = %s, want %s", got, want)
	}
	// SAP writes UTIME without leading zeros in some paths; midnight past is
	// the case that breaks a naive parse.
	if got := parseRepoTime("20260824", "1209"); got.Hour() != 0 || got.Minute() != 12 || got.Second() != 9 {
		t.Errorf("a short UTIME parsed as %s", got)
	}
}

// A stamp that cannot be read must be the zero time, so a caller can tell it
// apart from a real one. Returning "now" or the epoch would make every
// comparison silently wrong in one direction or the other.
func TestAnUnreadableStampIsZero(t *testing.T) {
	for _, c := range [][2]string{{"", ""}, {"notadate", "012009"}, {"2026082", "012009"}, {"20261332", "012009"}} {
		if got := parseRepoTime(c[0], c[1]); !got.IsZero() {
			t.Errorf("parseRepoTime(%q,%q) = %s, want the zero time", c[0], c[1], got)
		}
	}
}

func TestStampKeyIsCaseAndSpaceInsensitive(t *testing.T) {
	if StampKey("clas", " zcl_demo ") != StampKey("CLAS", "ZCL_DEMO") {
		t.Error("the same object produced two different keys")
	}
}

// CS is regenerated: on a live system several standard classes carry today's
// date on it, hours after nobody edited them. Including it in the maximum makes
// every object look changed on every run — a cache that never hits, and a scan
// slower than no cache at all. Across ten standard classes, including CS drops
// agreement with the ETag from eight to zero.
func TestTheRegeneratedSectionIsExcluded(t *testing.T) {
	const obj = "CL_HTTP_CLIENT"
	pad := obj + "================" // padded to 30
	cases := map[string]bool{
		pad + "CS":    true,  // regenerated
		pad + "CP":    false, // class pool — real
		pad + "CU":    false, // public section — real
		pad + "CM00Q": false, // a method — real, and the one that carried the answer
		pad + "CCIMP": false,
		pad + "CT":    false,
	}
	for include, want := range cases {
		if got := isRegeneratedSection(obj, include); got != want {
			t.Errorf("isRegeneratedSection(%q) = %v, want %v", include, got, want)
		}
	}
}

// A name that merely ends in CS is not the CS section. ZCL_DEMO_CS is an object
// in its own right, and treating its includes as regenerated would drop the
// only stamp it has — which reads as "unchanged" and serves stale source.
func TestAnObjectWhoseNameEndsInCSIsNotASection(t *testing.T) {
	const obj = "ZCL_DEMO_CS"
	pad := obj + strings.Repeat("=", 30-len(obj))
	if isRegeneratedSection(obj, pad+"CM001") {
		t.Error("a method include of ZCL_DEMO_CS was taken for a regenerated section")
	}
	if !isRegeneratedSection(obj, pad+"CS") {
		t.Error("the actual CS section of ZCL_DEMO_CS was not recognised")
	}
	// And an unrelated include must not be judged at all.
	if isRegeneratedSection(obj, "ZCL_OTHER=====================CS") {
		t.Error("another object's section was attributed to this one")
	}
}
