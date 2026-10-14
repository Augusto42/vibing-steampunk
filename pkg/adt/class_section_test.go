package adt

import "testing"

// The addresses are measured against a live 7.58, not inferred: includes/main,
// includes/localtypes and includes/localimplementations do not answer — 404,
// 404 and 400 — so a path cannot be built from a suffix by pattern.

func TestOnlyTheSectionsWithTheirOwnAddressReportOne(t *testing.T) {
	for _, c := range []struct {
		section string
		want    ClassIncludeType
		own     bool
	}{
		{"CCAU", ClassIncludeTestClasses, true},
		{"CCIMP", ClassIncludeImplementations, true},
		{"CCDEF", ClassIncludeDefinitions, true},
		{"CCMAC", ClassIncludeMacros, true},

		// These live in the main source and have no path of their own. A caller
		// that builds one gets a 404 and reports the object as unreadable.
		{"CU", ClassIncludeMain, false},
		{"CO", ClassIncludeMain, false},
		{"CI", ClassIncludeMain, false},
		{"CP", ClassIncludeMain, false},
		{"CM001", ClassIncludeMain, false},
		{"", ClassIncludeMain, false},
		{"NONSENSE", ClassIncludeMain, false},
	} {
		got, own := ClassIncludeForSection(c.section)
		if got != c.want || own != c.own {
			t.Errorf("ClassIncludeForSection(%q) = %q,%v — want %q,%v", c.section, got, own, c.want, c.own)
		}
	}
}
