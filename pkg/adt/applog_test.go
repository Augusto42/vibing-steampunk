package adt

import (
	"strings"
	"testing"
	"time"
)

// The whole point of this filter is ALPROG: a log entry written by the program
// that dumped, or by one on its call stack, is connected to that dump
// structurally. A timestamp near the dump is not.
func TestAppLogFilterNarrowsOnTheWritingProgram(t *testing.T) {
	where := appLogWhere(AppLogFilter{Program: "ZCL_DEMO_ORDER"})
	if !strings.Contains(where, "alprog = 'ZCL_DEMO_ORDER'") {
		t.Fatalf("the program must reach the query, got %q", where)
	}
}

// Names arrive from whatever the caller was handed — a dump, a stack, an
// argument — and this is free SQL.
func TestAppLogFilterEscapesQuotes(t *testing.T) {
	where := appLogWhere(AppLogFilter{Object: "IT'S"})
	if !strings.Contains(where, "'IT''S'") {
		t.Fatalf("a quote in a value must be doubled, got %q", where)
	}
	if strings.Count(where, "'")%2 != 0 {
		t.Fatalf("the clause should still be balanced, got %q", where)
	}
}

// Users and log objects are upper case in SAP; a lower-case argument that
// silently matched nothing would read as "there are no such logs".
func TestAppLogFilterUppercasesWhereSAPDoes(t *testing.T) {
	where := appLogWhere(AppLogFilter{User: "testuser", Object: "zdemo_log"})
	if !strings.Contains(where, "'TESTUSER'") || !strings.Contains(where, "'ZDEMO_LOG'") {
		t.Fatalf("user and object should be upper-cased, got %q", where)
	}
	// The program is not: ALPROG holds class pool names as SAP wrote them, and
	// those are lower case in the table.
	where = appLogWhere(AppLogFilter{Program: "cl_rstt_trace"})
	if !strings.Contains(where, "'cl_rstt_trace'") {
		t.Fatalf("the program name must be left as given, got %q", where)
	}
}

func TestAppLogFilterWithNothingSetIsUnbounded(t *testing.T) {
	if where := appLogWhere(AppLogFilter{}); where != "" {
		t.Fatalf("an empty filter should add no clause, got %q", where)
	}
}

func TestAppLogFilterBoundsDates(t *testing.T) {
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	where := appLogWhere(AppLogFilter{From: from, To: to})
	if !strings.Contains(where, "aldate >= '20260801'") || !strings.Contains(where, "aldate <= '20260823'") {
		t.Fatalf("dates should be bounded in SAP's own format, got %q", where)
	}
}

// SAP keeps the date and the clock in separate columns.
func TestSAPStampJoinsDateAndTime(t *testing.T) {
	at := parseSAPStamp("20260409", "190900")
	if at.IsZero() {
		t.Fatal("a valid pair should parse")
	}
	if got := at.Format("2006-01-02 15:04:05"); got != "2026-04-09 19:09:00" {
		t.Fatalf("parsed to %s", got)
	}
}

// A strange timestamp must not lose the row: an entry with an unreadable date
// is still an entry, and dropping it would hide the very thing being looked for.
func TestSAPStampSurvivesRubbish(t *testing.T) {
	for _, tc := range []struct{ date, clock string }{
		{"", ""},
		{"not-a-date", "190900"},
		{"20260409", ""},
		{"20261301", "999999"},
	} {
		at := parseSAPStamp(tc.date, tc.clock)
		_ = at // the point is that it returns rather than panicking
	}
	if at := parseSAPStamp("20260409", ""); at.IsZero() {
		t.Fatal("a missing clock should still give the date")
	}
	if at := parseSAPStamp("", "190900"); !at.IsZero() {
		t.Fatal("no date means no timestamp")
	}
}

// The query layer hands back whatever it parsed; column names have come back
// both cased.
func TestCellReadsEitherCasing(t *testing.T) {
	row := map[string]interface{}{"ALPROG": "  SAPMHTTP  ", "aluser": "TESTUSER"}
	if got := cell(row, "ALPROG"); got != "SAPMHTTP" {
		t.Fatalf("value should be trimmed, got %q", got)
	}
	if got := cell(row, "ALUSER"); got != "TESTUSER" {
		t.Fatalf("a lower-case column should still be found, got %q", got)
	}
	if got := cell(row, "MISSING"); got != "" {
		t.Fatalf("an absent column is empty, got %q", got)
	}
}
