package graph

import (
	"strings"
	"testing"
)

// Two defects on the same statement kind, pulling opposite ways.
//
// CALL FUNCTION with a variable was read as naming a function module after the
// variable, so the graph gained an edge to an object that does not exist — an
// invented dependency, which is worse than a missing one because nothing about
// it looks wrong. And a dynamic method call was classified and then extracted by
// nobody, so it appeared in no answer at all: not as a dependency, not as a
// warning that one exists.

func edgeTargets(edges []*Edge) []string {
	out := make([]string, 0, len(edges))
	for _, e := range edges {
		out = append(out, e.To)
	}
	return out
}

func TestAFunctionModuleNamedByAVariableIsNotInvented(t *testing.T) {
	src := `CALL FUNCTION lv_fm_name.`
	got := edgeTargets(ExtractDepsFromSource(src, "CLAS:ZCL_DEMO"))
	for _, to := range got {
		if strings.Contains(strings.ToUpper(to), "LV_FM_NAME") {
			t.Fatalf("the variable's name was taken for an object's: %v", got)
		}
	}
	if len(got) != 0 {
		t.Errorf("nothing static is knowable here, got %v", got)
	}
}

func TestAQuotedFunctionModuleIsStillAStaticDependency(t *testing.T) {
	src := `CALL FUNCTION 'Z_DEMO_CALL'.`
	got := edgeTargets(ExtractDepsFromSource(src, "CLAS:ZCL_DEMO"))
	if len(got) == 0 {
		t.Fatal("a literal names a real module and must still be found")
	}
	if !strings.Contains(strings.ToUpper(got[0]), "Z_DEMO") {
		t.Errorf("expected the module's group, got %v", got)
	}
}

func TestACallByVariableIsReportedAsDynamic(t *testing.T) {
	for _, src := range []string{
		`CALL METHOD (lv_dyn)=>go.`,
		`CALL METHOD lo_ref->(lv_meth).`,
	} {
		got := edgeTargets(ExtractDynamicCalls(src, "CLAS:ZCL_DEMO"))
		if len(got) == 0 {
			t.Errorf("%s names its target at runtime and that is worth saying, got nothing", src)
			continue
		}
		if !strings.HasPrefix(got[0], "DYNAMIC:") {
			t.Errorf("%s should be dynamic, got %v", src, got)
		}
	}
}

func TestAnOrdinaryMethodCallIsNotMistakenForADynamicOne(t *testing.T) {
	// The parentheses of an argument list are not a runtime name, and reporting
	// them would bury the calls that really are dynamic.
	src := `lo_ref->handle( iv_x = 1 ).
zcl_demo=>go( ).`
	if got := edgeTargets(ExtractDynamicCalls(src, "CLAS:ZCL_DEMO")); len(got) != 0 {
		t.Errorf("nothing here is decided at runtime, got %v", got)
	}
}
