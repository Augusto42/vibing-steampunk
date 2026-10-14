package adt

import (
	"strings"
	"testing"
)

// Editing a DDIC table was driven by hand — LOCK, UPDATE_SOURCE on
// /ddic/tables/{name}/source/main, ACTIVATE, UNLOCK — by a project using this
// as a library, because the high-level edit did not name the type. Every piece
// of the route already existed; only the name was missing from three switches.

func TestATableHasASourceAddress(t *testing.T) {
	got := GetObjectURL(ObjectTypeTable, "ZDEMO_ORDERS", "")
	if got != "/sap/bc/adt/ddic/tables/zdemo_orders" {
		t.Fatalf("unexpected object URL %q", got)
	}
	if src := GetSourceURL(ObjectTypeTable, "ZDEMO_ORDERS", ""); src != got+"/source/main" {
		t.Errorf("the source hangs off the object URL like every other type, got %q", src)
	}
}

func TestANamespacedTableIsEscaped(t *testing.T) {
	got := GetObjectURL(ObjectTypeTable, "/DMO/BOOKING", "")
	if strings.Contains(got, "//dmo/") {
		t.Errorf("a namespaced name must be escaped, not pasted: %q", got)
	}
	if !strings.Contains(got, "/ddic/tables/") {
		t.Errorf("still a table: %q", got)
	}
}

func TestCreatingATableFromSourceSaysWhatDoesWork(t *testing.T) {
	c := &Client{}
	result, err := c.writeSourceCreate(nil, "TABL", "ZDEMO_ORDERS", "@EndUserText.label: 'x'",
		&WriteSourceOptions{Package: "$ZDEMO", Description: "demo"})
	if err != nil {
		t.Fatalf("a refusal is an answer, not an error: %v", err)
	}
	if result.Success {
		t.Fatal("creating a DDIC table from source is not supported and must not claim to be")
	}
	if !strings.Contains(result.Message, "Editing an existing table does work") {
		t.Errorf("the refusal should name the route that does work, got %q", result.Message)
	}
}
