package ctxcomp

import (
	"context"
	"strings"
	"testing"
)

// The rule used to be "found by the regex layer and not by the parser layer,
// therefore in a string or a comment". That assumes the parser is a superset of
// the regex, and it is not: the parser layer is the graph's edge extractor and
// reads calls, while the regex reads declarations. They answer different
// questions, so their disagreement is evidence about neither.
//
// Measured on this repo's own ABAP, the rule dismissed the most important name
// in almost every file — including, in the class whose whole job is to dispatch
// to them, every service it dispatches to.
func TestADeclaredDependencyIsNotAFalsePositive(t *testing.T) {
	const src = `CLASS zcl_vsp_apc_handler DEFINITION
  INHERITING FROM cl_apc_wsp_ext_stateful_base.
  PUBLIC SECTION.
    INTERFACES if_apc_wsp_extension.
  PRIVATE SECTION.
    DATA mo_git TYPE REF TO zcl_vsp_git_service.
ENDCLASS.`
	res := NewAnalyzer(nil).Analyze(context.Background(), src, "ZCL_VSP_APC_HANDLER")
	for _, want := range []string{"CL_APC_WSP_EXT_STATEFUL_BASE", "IF_APC_WSP_EXTENSION", "ZCL_VSP_GIT_SERVICE"} {
		var found bool
		for _, d := range res.Dependencies {
			if d.Name != want {
				continue
			}
			found = true
			if d.InString || d.InComment {
				t.Errorf("%s is declared in code and was reported as a string or comment", want)
			}
			if d.Confidence < 0.5 {
				t.Errorf("%s is declared in code and was given confidence %.2f", want, d.Confidence)
			}
		}
		if !found {
			t.Errorf("%s was not reported as a dependency at all", want)
		}
	}
	if res.FalsePositives != 0 {
		t.Errorf("nothing here is in a string or a comment, yet %d were reported as such", res.FalsePositives)
	}
}

// The field says "in a string or a comment", so it must mean that. A name that
// really does occur only inside quotes or after a comment marker is the case it
// was invented for, and it must still be caught.
func TestANameOnlyInQuotesOrCommentsIsMarked(t *testing.T) {
	const src = `CLASS zcl_probe IMPLEMENTATION.
  METHOD run.
*   old code called zcl_demo_retired here
    DATA(msg) = 'call zcl_demo_quoted to continue'. " and zcl_demo_trailing
    DATA lo_real TYPE REF TO zcl_demo_real.
  ENDMETHOD.
ENDCLASS.`
	res := NewAnalyzer(nil).Analyze(context.Background(), src, "ZCL_PROBE")
	state := map[string]bool{}
	for _, d := range res.Dependencies {
		state[d.Name] = d.InString || d.InComment
	}
	for _, quoted := range []string{"ZCL_DEMO_RETIRED", "ZCL_DEMO_QUOTED", "ZCL_DEMO_TRAILING"} {
		if seen, ok := state[quoted]; ok && !seen {
			t.Errorf("%s occurs only inside a comment or a literal and was not marked", quoted)
		}
	}
	if state["ZCL_DEMO_REAL"] {
		t.Error("ZCL_DEMO_REAL is declared in code and was marked as quoted")
	}
}

// One real occurrence is enough. Dropping a genuine dependency from a reader's
// context is worse than carrying an extra one, so a name that appears in a
// comment *and* in code is code.
func TestOneRealOccurrenceOutweighsAnyNumberOfQuotedOnes(t *testing.T) {
	const src = `CLASS zcl_probe IMPLEMENTATION.
  METHOD run.
*   zcl_demo_both was the old name
    DATA(x) = 'zcl_demo_both'.
    DATA lo TYPE REF TO zcl_demo_both.
  ENDMETHOD.
ENDCLASS.`
	res := NewAnalyzer(nil).Analyze(context.Background(), src, "ZCL_PROBE")
	for _, d := range res.Dependencies {
		if d.Name == "ZCL_DEMO_BOTH" && (d.InString || d.InComment) {
			t.Error("a name used in real code was dismissed because it also appears quoted")
		}
	}
}

// The quote walker is the whole of the evidence, so it is tested on its own —
// ABAP uses " for comments and ' for literals, and ” escapes inside a literal.
func TestQuoteWalking(t *testing.T) {
	cases := []struct {
		line   string
		name   string
		quoted bool
	}{
		{`    DATA lo TYPE REF TO zcl_x.`, "ZCL_X", false},
		{`    DATA(x) = 'zcl_x'.`, "ZCL_X", true},
		{`    DATA lo TYPE REF TO zcl_y. " see zcl_x`, "ZCL_X", true},
		{`*   zcl_x was here`, "ZCL_X", true},
		{`    DATA(x) = 'it''s zcl_x'.`, "ZCL_X", true},
		{"    DATA(x) = `zcl_x`.", "ZCL_X", true},
	}
	for _, c := range cases {
		got := occurrenceIsQuotedOrCommented(c.line, strings.ToUpper(c.line), c.name)
		if got != c.quoted {
			t.Errorf("%q: quoted=%v, want %v", c.line, got, c.quoted)
		}
	}
}

// A function module is named in a string literal by construction — CALL
// FUNCTION 'SSFC_BASE64_DECODE' — so "every occurrence is inside quotes" is
// true of every real one. The direct check has to know that, or it makes the
// same mistake as the rule it replaced, in the other direction: measured on
// this repo's own ABAP it turned four genuine function-module dependencies into
// false positives before this exemption existed.
func TestAFunctionModuleIsNotQuotedAway(t *testing.T) {
	const src = `CLASS zcl_probe IMPLEMENTATION.
  METHOD run.
    CALL FUNCTION 'SSFC_BASE64_DECODE'
      EXPORTING b64data = iv_in
      IMPORTING bindata = ev_out.
  ENDMETHOD.
ENDCLASS.`
	res := NewAnalyzer(nil).Analyze(context.Background(), src, "ZCL_PROBE")
	var found bool
	for _, d := range res.Dependencies {
		if d.Name != "SSFC_BASE64_DECODE" {
			continue
		}
		found = true
		if d.InString || d.InComment {
			t.Error("a called function module was dismissed as a string literal")
		}
	}
	if !found {
		t.Error("the called function module was not reported at all")
	}
	if res.FalsePositives != 0 {
		t.Errorf("%d false positives in source that has none", res.FalsePositives)
	}
}
