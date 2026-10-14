package graph

import "testing"

// The reverse mapping — an include back to the object that owns it — is what
// upward tracing rests on: a cross-reference row says which include holds the
// reference, and the caller is whatever object that include belongs to.
//
// It used to look for one of a fixed set of section prefixes anywhere in the
// name. That matches U01 and F15 and misses U27, so a function group with more
// than a couple of dozen includes of one kind came back as a *program* named
// after its own include. Nothing failed. The object was simply the wrong kind
// with the wrong name, which is the worse outcome.
func TestFunctionPoolIncludesResolveWhateverTheirNumber(t *testing.T) {
	cases := []struct{ include, want string }{
		{"LZDEMO_FGU01", "ZDEMO_FG"},
		{"LZDEMO_FGU27", "ZDEMO_FG"}, // the one that used to fall through
		{"LZDEMO_FGU99", "ZDEMO_FG"},
		{"LZDEMO_FGF15", "ZDEMO_FG"},
		{"LZDEMO_FGTOP", "ZDEMO_FG"},
		{"LZDEMO_FGUXX", "ZDEMO_FG"},
		{"LZDEMO_FGI01", "ZDEMO_FG"},
		{"LSBAL_DB_CONVERTU04", "SBAL_DB_CONVERT"},
	}
	for _, tc := range cases {
		_, typ, name := NormalizeInclude(tc.include)
		if typ != "FUGR" || name != tc.want {
			t.Errorf("%s resolved to %s %s, want FUGR %s", tc.include, typ, name, tc.want)
		}
	}
}

func TestTheOtherKindsStillResolve(t *testing.T) {
	cases := []struct{ include, wantType, wantName string }{
		{"SAPLZDEMO_FG", "FUGR", "ZDEMO_FG"},
		{"ZCL_DEMO=======================CM003", "CLAS", "ZCL_DEMO"},
		{"ZIF_DEMO=======================IP", "INTF", "ZIF_DEMO"},
		{"ZDEMO_REPORT", "PROG", "ZDEMO_REPORT"},
	}
	for _, tc := range cases {
		_, typ, name := NormalizeInclude(tc.include)
		if typ != tc.wantType || name != tc.wantName {
			t.Errorf("%s resolved to %s %s, want %s %s", tc.include, typ, name, tc.wantType, tc.wantName)
		}
	}
}

// A program whose name happens to start with L must not be mistaken for a
// function pool.
func TestAProgramStartingWithLIsStillAProgram(t *testing.T) {
	for _, inc := range []string{"LEGACY_REPORT", "ZLEGACY"} {
		_, typ, _ := NormalizeInclude(inc)
		if typ == "FUGR" {
			t.Errorf("%s should not be read as a function pool", inc)
		}
	}
}
