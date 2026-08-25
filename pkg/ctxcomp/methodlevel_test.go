package ctxcomp

import (
	"strings"
	"testing"
)

const wideContract = `interface IF_DEMO_WIDE
  public .
  types ty_key type string .
  constants co_max type i value 10 .
  METHODS get_used
    IMPORTING iv_id TYPE ty_key
    RETURNING VALUE(rv) TYPE string .
  METHODS never_called
    IMPORTING iv_x TYPE i .
  METHODS also_used .
  METHODS another_unused
    RETURNING VALUE(rv) TYPE i .
endinterface.`

// A wide interface used narrowly is mostly noise: measured on a live system,
// IF_ATO_DB_ACCESS carries 56 methods and its caller uses nine.
func TestOnlyCalledMethodsSurvive(t *testing.T) {
	narrowed, total, kept := NarrowContract(wideContract, []string{"GET_USED", "ALSO_USED"})
	if total != 4 {
		t.Errorf("total = %d, want 4", total)
	}
	if kept != 2 {
		t.Errorf("kept = %d, want 2", kept)
	}
	for _, want := range []string{"GET_USED", "ALSO_USED"} {
		if !strings.Contains(strings.ToUpper(narrowed), want) {
			t.Errorf("%s is called and was dropped", want)
		}
	}
	for _, gone := range []string{"NEVER_CALLED", "ANOTHER_UNUSED"} {
		if strings.Contains(strings.ToUpper(narrowed), gone) {
			t.Errorf("%s is not called and survived", gone)
		}
	}
	// The parameter lines of a kept method must come with it, or the signature
	// is a name without a shape.
	if !strings.Contains(narrowed, "IMPORTING iv_id") {
		t.Error("a kept method lost its parameters")
	}
	// Types and constants are what the surviving signatures are written in
	// terms of, and they stay whatever happens.
	for _, keep := range []string{"ty_key", "co_max", "interface IF_DEMO_WIDE"} {
		if !strings.Contains(narrowed, keep) {
			t.Errorf("%s was dropped; the kept signatures are written in terms of it", keep)
		}
	}
}

// Knowing nothing is not the same as knowing none are called. A dependency
// reached only through a declaration has no call sites to narrow on, and
// narrowing on no information would be guessing.
func TestNoKnownCallsLeavesTheContractWhole(t *testing.T) {
	narrowed, total, kept := NarrowContract(wideContract, nil)
	if narrowed != wideContract {
		t.Error("a contract with nothing known about its use was narrowed anyway")
	}
	if total != kept {
		t.Errorf("total=%d kept=%d; with nothing to narrow on, everything is shown", total, kept)
	}
}

// METHODS: a, b, c declares several at once. Removing one of them from the
// chain produces text that is not ABAP, so the chain is left whole.
func TestAChainedDeclarationIsLeftWhole(t *testing.T) {
	const chained = `interface IF_DEMO
  public .
  METHODS: first, second, third .
endinterface.`
	narrowed, _, _ := NarrowContract(chained, []string{"FIRST"})
	for _, want := range []string{"first", "second", "third"} {
		if !strings.Contains(narrowed, want) {
			t.Errorf("%s was cut out of a chained declaration, leaving invalid ABAP", want)
		}
	}
}

// A receiver whose type is not declared in this source is skipped rather than
// guessed: attributing a method to the wrong class would put a signature into
// the context that does not exist there, which is worse than a contract that
// is too wide.
func TestAnUndeclaredReceiverIsNotAttributed(t *testing.T) {
	const src = `CLASS zcl_probe IMPLEMENTATION.
  METHOD run.
    DATA mo_known TYPE REF TO zif_demo_known.
    mo_known->real_call( ).
    mo_mystery->invented_call( ).
    zcl_static=>static_call( ).
    zif_intf~interface_call( ).
  ENDMETHOD.
ENDCLASS.`
	got := MethodsCalledOn(src)
	if !contains(got["ZIF_DEMO_KNOWN"], "REAL_CALL") {
		t.Errorf("a call on a declared receiver was missed: %v", got["ZIF_DEMO_KNOWN"])
	}
	if !contains(got["ZCL_STATIC"], "STATIC_CALL") {
		t.Errorf("a static call was missed: %v", got["ZCL_STATIC"])
	}
	if !contains(got["ZIF_INTF"], "INTERFACE_CALL") {
		t.Errorf("an interface call was missed: %v", got["ZIF_INTF"])
	}
	for owner, methods := range got {
		if contains(methods, "INVENTED_CALL") {
			t.Errorf("a call on an undeclared receiver was attributed to %s", owner)
		}
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
