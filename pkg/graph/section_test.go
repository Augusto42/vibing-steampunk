package graph

import "testing"

// The name is not the address. A cross-reference row naming CL_X===========CCAU
// says the reference lives in that class's test include; normalising it to
// "class CL_X" and reading source/main looks up a different part of the same
// object. Measured: every caller of CL_ABAP_UNIT_ASSERT=>ASSERT_EQUALS is a
// CCAU row, and every one read clean and empty.

func TestTheSectionSurvivesNormalisation(t *testing.T) {
	for _, c := range []struct{ include, section string }{
		{"CL_DEMO_ORDER================CCAU", "CCAU"},
		{"CL_DEMO_ORDER================CCIMP", "CCIMP"},
		{"CL_DEMO_ORDER================CCDEF", "CCDEF"},
		{"CL_DEMO_ORDER================CCMAC", "CCMAC"},
		{"CL_DEMO_ORDER================CM001", "CM001"},
		{"CL_DEMO_ORDER================CU", "CU"},
		{"ZIF_DEMO=====================IU", "IU"},
	} {
		if got := SectionOfInclude(c.include); got != c.section {
			t.Errorf("SectionOfInclude(%q) = %q, want %q", c.include, got, c.section)
		}
	}
}

func TestAnIncludeThatIsNotASectionHasNone(t *testing.T) {
	for _, in := range []string{"ZDEMO_REPORT", "SAPLZDEMO_GROUP", "LZDEMO_GROUPTOP", ""} {
		if got := SectionOfInclude(in); got != "" {
			t.Errorf("SectionOfInclude(%q) = %q, want empty — inventing a section is worse than having none", in, got)
		}
	}
}

func TestNormalisationStillAnswersTheSameObject(t *testing.T) {
	// The section is additional, not a replacement: the object a reference
	// belongs to has not changed.
	_, typ, name := NormalizeInclude("CL_DEMO_ORDER================CCAU")
	if typ != "CLAS" || name != "CL_DEMO_ORDER" {
		t.Errorf("got %s %s, want CLAS CL_DEMO_ORDER", typ, name)
	}
}
