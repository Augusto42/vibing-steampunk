package adt

import (
	"context"
	"runtime"
	"testing"
)

// Parallels not being installed is the ordinary case on every machine that is
// not a Mac, and it is an answer: there are no guests. Only prlctl being there
// and refusing to answer is a failure — the distinction matters because
// `landscape scan` used to swallow both, and a Mac whose Parallels was not
// running was told there is no landscape anywhere, when the file that carries
// the company's shared <Include> was inside the guest that could not be listed.
func TestNoParallelsIsAnAnswerNotAFailure(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("this machine may actually have Parallels, which is a different case")
	}
	guests, err := ParallelsGuests(context.Background())
	if err != nil {
		t.Fatalf("no Parallels here is not an error: %v", err)
	}
	if len(guests) != 0 {
		t.Fatalf("guests = %v, want none", guests)
	}

	paths, err := ParallelsLandscapeFiles(context.Background(), "Windows 11")
	if err != nil {
		t.Fatalf("no Parallels here is not an error: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("paths = %v, want none", paths)
	}
}

// And with nothing to discover, the scan reports no phantom source. A row with
// an Err is only added when a discovery step actually failed.
func TestScanAddsNoParallelsRowWhenThereIsNoParallels(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("this machine may actually have Parallels, which is a different case")
	}
	for _, src := range ScanLandscapeSources(context.Background()) {
		if src.Kind == "parallels" {
			t.Fatalf("unexpected parallels source %+v", src)
		}
	}
}
