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
	// The group resolver's output feeds the shared object-URL builder; this
	// pins the shape those two agree on, since a wrong URL here reads as "the
	// module does not exist" rather than as a bug.
	want := "/sap/bc/adt/functions/groups/ZFG/fmodules/ZFM"
	for _, in := range [][2]string{{"ZFG", "ZFM"}, {"zfg", "zfm"}, {"Zfg", "zFm"}} {
		if got := GetObjectURL(ObjectTypeFunctionMod, in[1], in[0]); got != want {
			t.Errorf("GetObjectURL(FUNC, %q, %q) = %q, want %q", in[1], in[0], got, want)
		}
	}
}
