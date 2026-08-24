package main

import "testing"

// The first version of this trimmed padding and section letters from the right,
// which ate real characters and handed the sweep a generated include as an
// object name. The handler refused it and the sweep filed the refusal as a
// defect — a false finding, which is the one failure this tool must not have.
func TestRepositoryNameFromInclude(t *testing.T) {
	cases := map[string]string{
		"CL_ABAP_TYPEDESCR=============CP": "CL_ABAP_TYPEDESCR",
		"ZCL_VSP_GIT_SERVICE===========CU": "ZCL_VSP_GIT_SERVICE",
		"/IWBEP/CL_MGW_REQUEST=========CP": "/IWBEP/CL_MGW_REQUEST",
		"ZVSP_ENQUEUE_RESET":               "ZVSP_ENQUEUE_RESET",

		// Not repository objects, and each one previously produced a name.
		"%_CCRMB":  "",
		"%_T00001": "",
		"":         "",
		"CP":       "",
	}
	for include, want := range cases {
		if got := repositoryNameFromInclude(include); got != want {
			t.Errorf("repositoryNameFromInclude(%q) = %q, want %q", include, got, want)
		}
	}
}

// A name ending in one of the section letters must survive intact. This is the
// case the right-trimming version got wrong.
func TestSectionLettersAreNotTrimmedFromTheName(t *testing.T) {
	for _, name := range []string{"ZCL_DEMO_ACT", "ZCL_DEMO_TOP", "ZCL_DEMO_CU"} {
		include := name + "=============CP"
		if got := repositoryNameFromInclude(include); got != name {
			t.Errorf("repositoryNameFromInclude(%q) = %q, want %q", include, got, name)
		}
	}
}
