package adt

import "testing"

func TestGroupFromFunctionURI(t *testing.T) {
	tests := map[string]string{
		"/sap/bc/adt/functions/groups/zvsp_tmp_fg/fmodules/zvsp_tmp_fm": "ZVSP_TMP_FG",
		"/sap/bc/adt/functions/groups/ZFG/fmodules/ZFM":                 "ZFG",
		// A group's own URI carries no module, and answering with the group
		// would let a lookup for a module silently succeed on the wrong thing.
		"/sap/bc/adt/functions/groups/zfg":     "",
		"/sap/bc/adt/oo/classes/zcl_something": "",
		"":                                     "",
	}
	for uri, want := range tests {
		if got := groupFromFunctionURI(uri); got != want {
			t.Errorf("groupFromFunctionURI(%q) = %q, want %q", uri, got, want)
		}
	}
}

func TestFunctionModuleURL(t *testing.T) {
	// Names reach here from callers who type them however they like; the ADT
	// resource is the same object either way.
	want := "/sap/bc/adt/functions/groups/ZFG/fmodules/ZFM"
	for _, in := range [][2]string{{"ZFG", "ZFM"}, {"zfg", "zfm"}, {"Zfg", "zFm"}} {
		if got := functionModuleURL(in[0], in[1]); got != want {
			t.Errorf("functionModuleURL(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}
