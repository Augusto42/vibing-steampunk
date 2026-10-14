package saprfc

import (
	"testing"
)

// A table small enough to read whole is read whole. Sampling a table of three
// rows would be a strange thing to do to somebody.
func TestSmallTablesAreReadWhole(t *testing.T) {
	for _, lines := range []int{1, 2, 17, 99} {
		rows := tableRowSample(lines, 99)
		if len(rows) != lines {
			t.Fatalf("a table of %d rows should be read whole, got %d rows", lines, len(rows))
		}
		for i, row := range rows {
			if row != i+1 {
				t.Fatalf("rows should be 1..%d in order, got %v", lines, rows)
			}
		}
	}
}

// The point of sampling. Reading the first N of a large table is the obvious
// choice and the least informative: the head is where the least surprising data
// lives, and anything that went wrong late is invisible from there.
func TestLargeTablesAreSampledHeadMiddleAndEnd(t *testing.T) {
	const lines = 1_000_000
	rows := tableRowSample(lines, 99)

	if len(rows) > 99 {
		t.Fatalf("the budget is 99 rows, the sample asks for %d", len(rows))
	}
	if len(rows) < 90 {
		t.Fatalf("the budget should be close to spent, got %d rows", len(rows))
	}

	if rows[0] != 1 {
		t.Fatalf("the sample should start at the first row, got %d", rows[0])
	}
	if last := rows[len(rows)-1]; last != lines {
		t.Fatalf("the sample should reach the last row, got %d", last)
	}

	var middle int
	for _, row := range rows {
		if row > lines/4 && row < lines*3/4 {
			middle++
		}
	}
	if middle == 0 {
		t.Fatal("nothing was read from the middle; head and end alone cannot show whether the table is homogeneous")
	}
}

// Ordering matters to the caller: rows come back to be matched against a
// listing, and an unsorted sample reads as corruption.
func TestSampleIsOrderedAndFreeOfDuplicates(t *testing.T) {
	for _, lines := range []int{100, 101, 150, 297, 1000} {
		rows := tableRowSample(lines, 99)
		seen := map[int]bool{}
		for i, row := range rows {
			if row < 1 || row > lines {
				t.Fatalf("lines=%d: row %d is outside the table", lines, row)
			}
			if seen[row] {
				t.Fatalf("lines=%d: row %d asked for twice: %v", lines, row, rows)
			}
			seen[row] = true
			if i > 0 && row <= rows[i-1] {
				t.Fatalf("lines=%d: rows out of order: %v", lines, rows)
			}
		}
	}
}

// The windows overlap when the table is only a little larger than the budget.
// Overlapping is fine; asking for the same row twice is not, and neither is
// coming back with fewer rows than a smaller table would have given.
func TestSampleHandlesTablesJustOverTheBudget(t *testing.T) {
	rows := tableRowSample(100, 99)
	if len(rows) == 0 {
		t.Fatal("a table of 100 rows should still be sampled")
	}
	if rows[len(rows)-1] != 100 {
		t.Fatalf("the last row should be reached, got %d", rows[len(rows)-1])
	}
}

func TestSampleOfNothingIsNothing(t *testing.T) {
	if rows := tableRowSample(0, 99); rows != nil {
		t.Fatalf("an empty table has no rows to read, got %v", rows)
	}
	if rows := tableRowSample(10, 0); rows != nil {
		t.Fatalf("no budget means no rows, got %v", rows)
	}
}

// What the reader is told. "33 of 1000000 rows: 1-33, …" is the difference
// between a sample and a lie.
func TestRowRangesReadAsRanges(t *testing.T) {
	cases := []struct {
		rows []int
		want string
	}{
		{nil, "none"},
		{[]int{1}, "1"},
		{[]int{1, 2, 3}, "1-3"},
		{[]int{1, 2, 3, 50, 51, 99}, "1-3, 50-51, 99"},
	}
	for _, tc := range cases {
		if got := FormatRowRanges(tc.rows); got != tc.want {
			t.Fatalf("%v should render as %q, got %q", tc.rows, tc.want, got)
		}
	}
}

// A sample of everything is not partial, and saying so would put a misleading
// note under every small table.
func TestWholeTablesAreNotReportedAsPartial(t *testing.T) {
	whole := &TableSample{Lines: 2, Rows: []int{1, 2}}
	if whole.Partial() {
		t.Fatal("a table read whole must not be reported as sampled")
	}
	part := &TableSample{Lines: 1000, Rows: []int{1, 2}}
	if !part.Partial() {
		t.Fatal("a sampled table must be reported as sampled")
	}
	var none *TableSample
	if none.Partial() {
		t.Fatal("no expansion means nothing to report")
	}
}
