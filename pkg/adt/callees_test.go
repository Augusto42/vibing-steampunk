package adt

import (
	"context"
	"strings"
	"testing"
)

// Every one of these guards a way the down direction can be quietly wrong
// rather than loudly broken. A callee list that is too long reads as a real
// answer; so does one that is too short. Nothing in the shape of the JSON says
// which happened, so the rules that decide have to be pinned here.

func TestCalleeTargetFromURI(t *testing.T) {
	cases := []struct {
		name string
		uri  string
		want calleeTarget
		ok   bool
	}{
		{
			name: "a class",
			uri:  "/sap/bc/adt/oo/classes/zcl_demo_service",
			want: calleeTarget{Name: "ZCL_DEMO_SERVICE", Type: "CLAS"},
			ok:   true,
		},
		{
			// A method URI is answered with its class rather than refused: the
			// tables are keyed by include, and a class's includes are not one
			// per method in any way this can rely on.
			name: "a method URI falls back to its class",
			uri:  "/sap/bc/adt/oo/classes/zcl_demo_service/source/main#type=CLAS%2FOM;name=RUN",
			want: calleeTarget{Name: "ZCL_DEMO_SERVICE", Type: "CLAS"},
			ok:   true,
		},
		{
			name: "an interface",
			uri:  "/sap/bc/adt/oo/interfaces/zif_demo_service",
			want: calleeTarget{Name: "ZIF_DEMO_SERVICE", Type: "INTF"},
			ok:   true,
		},
		{
			name: "a program",
			uri:  "/sap/bc/adt/programs/programs/zdemo_report",
			want: calleeTarget{Name: "ZDEMO_REPORT", Type: "PROG"},
			ok:   true,
		},
		{
			name: "a function group",
			uri:  "/sap/bc/adt/functions/groups/zdemo_log",
			want: calleeTarget{Name: "ZDEMO_LOG", Type: "FUGR"},
			ok:   true,
		},
		{
			// The module, not the group: the group's includes are every module
			// in it, which is a different and much worse answer.
			name: "a function module keeps its group",
			uri:  "/sap/bc/adt/functions/groups/zdemo_log/fmodules/zdemo_log_write",
			want: calleeTarget{Name: "ZDEMO_LOG_WRITE", Type: "FUNC", Group: "ZDEMO_LOG"},
			ok:   true,
		},
		{
			// Namespaced objects arrive escaped and would otherwise be looked
			// up under a name with a percent sign in it.
			name: "a namespaced program is unescaped",
			uri:  "/sap/bc/adt/programs/programs/%2Fdemo%2Fzreport",
			want: calleeTarget{Name: "/DEMO/ZREPORT", Type: "PROG"},
			ok:   true,
		},
		{
			name: "a table has no include to ask about",
			uri:  "/sap/bc/adt/ddic/tables/zdemo_table",
			ok:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := calleeTargetFromURI(tc.uri)
			if tc.ok != (err == nil) {
				t.Fatalf("calleeTargetFromURI(%q) error = %v, wanted ok=%v", tc.uri, err, tc.ok)
			}
			if !tc.ok {
				return
			}
			if got != tc.want {
				t.Errorf("calleeTargetFromURI(%q) = %+v, want %+v", tc.uri, got, tc.want)
			}
		})
	}
}

// The refusal has to name what it does understand. An error reading "not
// supported" sends the next reader looking for a bug in the query.
func TestCalleeTargetRefusalSaysWhatItKnows(t *testing.T) {
	_, err := calleeTargetFromURI("/sap/bc/adt/ddic/tables/zdemo_table")
	if err == nil {
		t.Fatal("a table URI was accepted")
	}
	for _, want := range []string{"classes", "programs", "function"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q: %v", want, err)
		}
	}
}

func TestIncludePredicate(t *testing.T) {
	c := &Client{}
	cases := []struct {
		name   string
		target calleeTarget
		want   string
	}{
		{"class", calleeTarget{Name: "ZCL_DEMO", Type: "CLAS"}, "INCLUDE LIKE 'ZCL_DEMO%'"},
		{"program", calleeTarget{Name: "ZDEMO_REPORT", Type: "PROG"}, "INCLUDE = 'ZDEMO_REPORT'"},
		{"function group", calleeTarget{Name: "ZDEMO_LOG", Type: "FUGR"}, "INCLUDE LIKE 'LZDEMO_LOG%'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := c.includePredicate(context.Background(), tc.target)
			if err != nil {
				t.Fatalf("includePredicate: %v", err)
			}
			if got != tc.want {
				t.Errorf("includePredicate = %q, want %q", got, tc.want)
			}
		})
	}
}

// The name reaches a freestyle WHERE clause, so anything that could close the
// literal has to be refused before it gets there rather than escaped and
// hoped over.
func TestIncludePredicateRefusesNamesThatAreNotNames(t *testing.T) {
	c := &Client{}
	for _, bad := range []string{"ZCL_X' OR '1'='1", "ZCL_X; DROP", "ZCL X", ""} {
		if _, err := c.includePredicate(context.Background(), calleeTarget{Name: bad, Type: "CLAS"}); err == nil {
			t.Errorf("%q was put into a query", bad)
		}
	}
}

// The LIKE pattern is the reason this exists: 'ZCL_DEMO%' also matches the
// includes of ZCL_DEMO_HELPER, and quietly reporting another class's
// dependencies as this one's is the kind of wrong answer nobody checks.
func TestIncludeBelongsTo(t *testing.T) {
	class := calleeTarget{Name: "ZCL_DEMO", Type: "CLAS"}
	if !includeBelongsTo("ZCL_DEMO=====================CM001", class) {
		t.Error("a class's own method include was rejected")
	}
	if !includeBelongsTo("ZCL_DEMO=====================CI", class) {
		t.Error("a class's own private-section include was rejected")
	}
	if includeBelongsTo("ZCL_DEMO_HELPER==============CM001", class) {
		t.Error("another class's include was accepted because the names share a prefix")
	}

	group := calleeTarget{Name: "ZDEMO_LOG", Type: "FUGR"}
	if !includeBelongsTo("LZDEMO_LOGU01", group) {
		t.Error("a function pool section was rejected")
	}
	if includeBelongsTo("LZDEMO_LOGGINGU01", group) {
		t.Error("another group's include was accepted because the names share a prefix")
	}
}

func TestSplitCrossName(t *testing.T) {
	cases := []struct {
		raw           string
		name, compone string
	}{
		{"ZCL_DEMO_UTILS", "ZCL_DEMO_UTILS", ""},
		{"ZCL_DEMO_UTILS\\ME:JSON_STR", "ZCL_DEMO_UTILS", "JSON_STR"},
		// A parameter of a method you call is not a separate callee.
		{"ZCL_DEMO_UTILS\\ME:BUILD\\DA:IV_DATA", "ZCL_DEMO_UTILS", "BUILD"},
		{"ZIF_DEMO_SERVICE\\TY:TY_RESPONSE", "ZIF_DEMO_SERVICE", "TY_RESPONSE"},
	}
	for _, tc := range cases {
		name, component := splitCrossName(tc.raw)
		if name != tc.name || component != tc.compone {
			t.Errorf("splitCrossName(%q) = %q, %q; want %q, %q", tc.raw, name, component, tc.name, tc.compone)
		}
	}
}

// CROSS-TYPE is one character. Two-letter codes are not a near miss: the data
// preview resource rejects them with 400, and that error was being read as
// "nothing found".
func TestCrossKind(t *testing.T) {
	if kind, calls := crossKind("F"); kind != "function module" || !calls {
		t.Errorf(`crossKind("F") = %q, %v`, kind, calls)
	}
	if kind, calls := crossKind("U"); kind != "subroutine" || !calls {
		t.Errorf(`crossKind("U") = %q, %v`, kind, calls)
	}
	if _, calls := crossKind("Z"); calls {
		t.Error("an unknown code was reported as a call")
	}
}

func TestWBCrossCalleesFiltersRows(t *testing.T) {
	target := calleeTarget{Name: "ZCL_DEMO", Type: "CLAS"}
	rows := []map[string]interface{}{
		{"INCLUDE": "ZCL_DEMO=====================CM001", "OTYPE": "ME", "NAME": "ZCL_DEMO_UTILS\\ME:JSON_STR", "DIRECT": "X"},
		// Indirect: a type implied by a type the code named. Reporting these
		// makes every class look like it depends on half of DDIC.
		{"INCLUDE": "ZCL_DEMO=====================CM001", "OTYPE": "TY", "NAME": "SYST_DATUM", "DIRECT": "", "INDIRECT": "X"},
		// The class describing itself.
		{"INCLUDE": "ZCL_DEMO=====================CM001", "OTYPE": "TY", "NAME": "ZCL_DEMO", "DIRECT": "X"},
		// Another class whose include shares the prefix.
		{"INCLUDE": "ZCL_DEMO_HELPER==============CM001", "OTYPE": "ME", "NAME": "ZCL_OTHER\\ME:RUN", "DIRECT": "X"},
	}

	got := wbCrossCallees(rows, target)
	if len(got) != 1 {
		t.Fatalf("wbCrossCallees kept %d rows, want 1: %+v", len(got), got)
	}
	if got[0].Name != "ZCL_DEMO_UTILS" || got[0].Component != "JSON_STR" || !got[0].Calls {
		t.Errorf("wbCrossCallees = %+v", got[0])
	}
}

func TestCrossCalleesSwapsFormAndProgram(t *testing.T) {
	target := calleeTarget{Name: "ZDEMO_REPORT", Type: "PROG"}
	rows := []map[string]interface{}{
		{"INCLUDE": "ZDEMO_REPORT", "TYPE": "F", "NAME": "Z_DEMO_LOG_WRITE", "PROG": ""},
		// A PERFORM: the program is what somebody can open, the form is which
		// part of it.
		{"INCLUDE": "ZDEMO_REPORT", "TYPE": "U", "NAME": "BUILD_OUTPUT", "PROG": "ZDEMO_FORMS"},
	}

	got := crossCallees(rows, target)
	if len(got) != 2 {
		t.Fatalf("crossCallees returned %d rows, want 2", len(got))
	}
	if got[0].Name != "Z_DEMO_LOG_WRITE" || got[0].Kind != "function module" {
		t.Errorf("the function module row is %+v", got[0])
	}
	if got[1].Name != "ZDEMO_FORMS" || got[1].Component != "BUILD_OUTPUT" {
		t.Errorf("the PERFORM row is %+v, want the program named and the form as its component", got[1])
	}
}

// One object reached five ways is one dependency, and if any of those ways was
// a call then the object is called — a type reference to a class you also call
// tells the reader nothing.
func TestMergeCalleesPrefersTheCall(t *testing.T) {
	got := mergeCallees([]Callee{
		{Name: "ZCL_DEMO_UTILS", Kind: "type", Source: "WBCROSSGT"},
		{Name: "ZCL_DEMO_UTILS", Kind: "method", Component: "JSON_STR", Calls: true, Source: "WBCROSSGT"},
		{Name: "ZCL_DEMO_UTILS", Kind: "method", Component: "JSON_OBJ", Calls: true, Source: "WBCROSSGT"},
		{Name: "ZCL_DEMO_OTHER", Kind: "type", Source: "WBCROSSGT"},
	})

	if len(got) != 2 {
		t.Fatalf("mergeCallees returned %d entries, want 2: %+v", len(got), got)
	}
	// Calls sort first, or a list opening with forty DDIC types buries them.
	if got[0].Name != "ZCL_DEMO_UTILS" || !got[0].Calls || got[0].Kind != "method" {
		t.Errorf("the merged entry is %+v", got[0])
	}
	if !strings.Contains(got[0].Component, "JSON_STR") || !strings.Contains(got[0].Component, "JSON_OBJ") {
		t.Errorf("both methods should survive the merge: %q", got[0].Component)
	}
}

func TestFunctionModuleIncludePadsTheSection(t *testing.T) {
	// TFDIR keeps the section as a number; the include wants two digits, so a
	// module in section 5 is LZDEMO_LOGU05 and not LZDEMO_LOGU5.
	if got := poolIncludeFor("ZDEMO_LOG", "5"); got != "LZDEMO_LOGU05" {
		t.Errorf("poolIncludeFor(section 5) = %q", got)
	}
	if got := poolIncludeFor("ZDEMO_LOG", "15"); got != "LZDEMO_LOGU15" {
		t.Errorf("poolIncludeFor(section 15) = %q", got)
	}
}
