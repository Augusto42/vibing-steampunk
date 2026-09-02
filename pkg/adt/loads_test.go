package adt

import "testing"

// A prefix query is the only way to find a padded master, and a prefix drags in
// every sibling that shares it. ZCL_ORDER and ZCL_ORDER_ITEM are different
// objects; the padding is what says so, and reading it wrong attributes one
// object's loads to another.

func TestAPoolBelongsToTheObjectItIsPaddedFrom(t *testing.T) {
	for _, c := range []struct {
		include, name string
		want          bool
	}{
		{"ZCL_DEMO_ORDER===============CP", "ZCL_DEMO_ORDER", true},
		{"ZCL_DEMO_ORDER===============CM001", "ZCL_DEMO_ORDER", true},
		{"ZDEMO_REPORT", "ZDEMO_REPORT", true},
		{"SAPLZDEMO_GROUP", "ZDEMO_GROUP", true},
		{"LZDEMO_GROUPTOP", "ZDEMO_GROUP", true},
		{"LZDEMO_GROUP$01", "ZDEMO_GROUP", true},

		// The sibling trap: same prefix, different object.
		{"ZCL_DEMO_ORDER_ITEM==========CP", "ZCL_DEMO_ORDER", false},
		{"ZDEMO_REPORT_V2", "ZDEMO_REPORT", false},
		{"ZCL_DEMO_ORDERING============CP", "ZCL_DEMO_ORDER", false},
	} {
		if got := includeBelongsToName(c.include, c.name); got != c.want {
			t.Errorf("includeBelongsToName(%q, %q) = %v, want %v", c.include, c.name, got, c.want)
		}
	}
}

func TestLoadRowsAreFilteredToTheObjectAsked(t *testing.T) {
	rows := []map[string]interface{}{
		{"MASTER": "ZCL_DEMO_ORDER===============CP", "INCLUDE": "CL_ABAP_TYPEDESCR=============CT", "OBSOLETE_IN_VERSION": "0"},
		{"MASTER": "ZCL_DEMO_ORDER_ITEM==========CP", "INCLUDE": "CX_ROOT======================CU", "OBSOLETE_IN_VERSION": "0"},
	}
	got := loadRowsFor(rows, "ZCL_DEMO_ORDER")
	if len(got) != 1 {
		t.Fatalf("only one of these masters is the object asked about, got %d", len(got))
	}
	if got[0].Include != "CL_ABAP_TYPEDESCR=============CT" {
		t.Errorf("the wrong row survived: %+v", got[0])
	}
}

func TestTheObsoleteMarkerIsRead(t *testing.T) {
	rows := []map[string]interface{}{
		{"MASTER": "ZDEMO_REPORT", "INCLUDE": "ZDEMO_INCL", "OBSOLETE_IN_VERSION": "740"},
	}
	got := loadRowsFor(rows, "ZDEMO_REPORT")
	if len(got) != 1 {
		t.Fatalf("the row belongs to this object and the builder decides what to do with it, got %d", len(got))
	}
	if got[0].ObsoleteInVersion != 740 {
		t.Errorf("a row kept for an older release must arrive saying so, got %d", got[0].ObsoleteInVersion)
	}
}
