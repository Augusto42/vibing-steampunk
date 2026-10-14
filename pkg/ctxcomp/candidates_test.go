package ctxcomp

import "testing"

// The budget decides what a reader sees, and line order was deciding it. A
// superclass declared on line two ranked below a type used once, because the
// only tiebreaker was custom-versus-standard and then position.
func TestObligationsOutrankEverything(t *testing.T) {
	const src = `CLASS zcl_probe DEFINITION
  INHERITING FROM cl_apc_wsp_ext_stateful_base.
  PUBLIC SECTION.
    INTERFACES if_apc_wsp_extension.
    METHODS run IMPORTING io_ctx TYPE REF TO zif_vsp_service.
  PRIVATE SECTION.
    DATA mo_git TYPE REF TO zcl_vsp_git_service.
    DATA mo_rfc TYPE REF TO zcl_vsp_rfc_service.
ENDCLASS.
CLASS zcl_probe IMPLEMENTATION.
  METHOD run.
    mo_git->push( ).
    mo_git->pull( ).
    mo_git->status( ).
    TRY.
        mo_rfc->call( ).
      CATCH cx_root.
    ENDTRY.
  ENDMETHOD.
ENDCLASS.`
	deps := []Dependency{
		{Name: "CX_ROOT", Kind: KindClass, Line: 16},
		{Name: "ZCL_VSP_GIT_SERVICE", Kind: KindClass, Line: 7},
		{Name: "CL_APC_WSP_EXT_STATEFUL_BASE", Kind: KindClass, Line: 2},
		{Name: "IF_APC_WSP_EXTENSION", Kind: KindInterface, Line: 4},
		{Name: "ZIF_VSP_SERVICE", Kind: KindInterface, Line: 5},
		{Name: "ZCL_VSP_RFC_SERVICE", Kind: KindClass, Line: 8},
	}
	ranked := RankCandidates(src, deps)

	roleOf := map[string]DepRole{}
	posOf := map[string]int{}
	for i, r := range ranked {
		roleOf[r.Name] = r.Role
		posOf[r.Name] = i
	}

	for _, want := range []string{"CL_APC_WSP_EXT_STATEFUL_BASE", "IF_APC_WSP_EXTENSION"} {
		if roleOf[want] != RoleObligation {
			t.Errorf("%s is a superclass or implemented interface; classified %s", want, roleOf[want])
		}
	}
	if roleOf["ZIF_VSP_SERVICE"] != RoleSignature {
		t.Errorf("ZIF_VSP_SERVICE appears in the public signature; classified %s", roleOf["ZIF_VSP_SERVICE"])
	}
	if roleOf["CX_ROOT"] != RoleException {
		t.Errorf("CX_ROOT classified %s", roleOf["CX_ROOT"])
	}

	// The exception is last: it is the first thing to drop when the budget runs
	// out, and under the old order it could outrank the superclass.
	if posOf["CX_ROOT"] != len(ranked)-1 {
		t.Errorf("CX_ROOT is at position %d of %d; it should be last", posOf["CX_ROOT"], len(ranked))
	}
	if posOf["CL_APC_WSP_EXT_STATEFUL_BASE"] > posOf["ZCL_VSP_GIT_SERVICE"] {
		t.Error("a collaborator outranked the superclass")
	}
}

// Within a band, how often something is used is the honest proxy for how
// central it is. Both are custom collaborators; one is used three times.
func TestMostUsedCollaboratorComesFirst(t *testing.T) {
	const src = `CLASS zcl_probe IMPLEMENTATION.
  METHOD run.
    zcl_busy=>a( ). zcl_busy=>b( ). zcl_busy=>c( ).
    zcl_quiet=>once( ).
  ENDMETHOD.
ENDCLASS.`
	ranked := RankCandidates(src, []Dependency{
		{Name: "ZCL_QUIET", Kind: KindClass, Line: 4},
		{Name: "ZCL_BUSY", Kind: KindClass, Line: 3},
	})
	if ranked[0].Name != "ZCL_BUSY" {
		t.Errorf("ranked %s first; ZCL_BUSY is used three times and ZCL_QUIET once", ranked[0].Name)
	}
	if ranked[0].Uses <= ranked[1].Uses {
		t.Errorf("use counts came out %d and %d", ranked[0].Uses, ranked[1].Uses)
	}
}

// Custom code outranks standard within a band: a reader plausibly knows
// CL_ABAP_TYPEDESCR and cannot know ZCL_VSP_GIT_SERVICE.
func TestCustomOutranksStandardWithinABand(t *testing.T) {
	const src = `CLASS zcl_probe IMPLEMENTATION.
  METHOD run.
    cl_abap_typedescr=>describe_by_data( ).
    zcl_vsp_git_service=>push( ).
  ENDMETHOD.
ENDCLASS.`
	ranked := RankCandidates(src, []Dependency{
		{Name: "CL_ABAP_TYPEDESCR", Kind: KindClass, Line: 3},
		{Name: "ZCL_VSP_GIT_SERVICE", Kind: KindClass, Line: 4},
	})
	if ranked[0].Name != "ZCL_VSP_GIT_SERVICE" {
		t.Errorf("ranked %s first; the custom class is the one a reader cannot know", ranked[0].Name)
	}
}

func TestExceptionClassesAreRecognisedIncludingNamespaced(t *testing.T) {
	for _, yes := range []string{"CX_ROOT", "CX_SY_ZERODIVIDE", "ZCX_DEMO_ERROR", "YCX_OTHER", "/BOBF/CX_FRW"} {
		if !isExceptionClass(yes) {
			t.Errorf("%s is an exception class and was not recognised", yes)
		}
	}
	// Names that merely contain CX_, and a class whose name starts with C.
	for _, no := range []string{"CL_ABAP_TYPEDESCR", "ZCL_CX_HELPER", "CONTEXT", "/BOBF/CL_FRW"} {
		if isExceptionClass(no) {
			t.Errorf("%s is not an exception class and was treated as one", no)
		}
	}
}
