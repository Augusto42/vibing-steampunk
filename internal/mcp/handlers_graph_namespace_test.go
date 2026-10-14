package mcp

import (
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// A namespaced object name carries slashes, and a path built from it unescaped
// has an empty segment where the name should be. Every reader downstream then
// behaves as though no object was named — which is what `analyze type=callees`
// did for every namespaced object, from the day it shipped, while the same
// question through the CLI answered correctly.
//
// Found by `vsp sweep`, on a target it had chosen out of WBCROSSGT rather than
// been handed.
func TestNamespacedObjectSurvivesTheURI(t *testing.T) {
	cases := []struct{ objType, name string }{
		{"CLAS", "/BOBF/CL_CONF_ADT_RESOURCE"},
		{"INTF", "/IWBEP/IF_MGW_APPL_SRV_RUNTIME"},
		{"PROG", "/SDF/RSORAVSH"},
		{"FUGR", "/BOBF/CONF_UI"},
	}
	for _, c := range cases {
		uri := buildADTObjectURL(c.objType, c.name)
		if uri == "" {
			t.Fatalf("%s %s produced no URI at all", c.objType, c.name)
		}
		// The name must not appear as raw slashes: that is the defect.
		if strings.Contains(uri, "//"+strings.ToLower(strings.Trim(c.name, "/"))[:4]) {
			t.Errorf("%s: name went into the path unescaped: %s", c.name, uri)
		}
		// And it must come back out intact, which is what the callee lookup
		// does with it.
		got, err := adt.CalleeTargetNameFromURI(uri)
		if err != nil {
			t.Errorf("%s: the URI %s cannot be read back: %v", c.name, uri, err)
			continue
		}
		if got != c.name {
			t.Errorf("%s: round-tripped through %s as %q", c.name, uri, got)
		}
	}
}

// The ordinary case must keep working exactly as before.
func TestPlainObjectNameIsUnchanged(t *testing.T) {
	if got := buildADTObjectURL("CLAS", "ZCL_VSP_DEMO"); got != "/sap/bc/adt/oo/classes/zcl_vsp_demo" {
		t.Errorf("buildADTObjectURL(CLAS, ZCL_VSP_DEMO) = %q", got)
	}
}
