package mcp

import (
	"strings"
	"testing"
)

// A parameter's documentation may contain commas — "CLAS, PROG, INTF, FUGR" is
// the natural way to write the allowed values — and the tag is comma-separated.
// The first parser split on every comma and kept the last piece, so that list
// rendered as "FUGR" and the documentation said something false.
func TestDocumentationMayContainCommas(t *testing.T) {
	type p struct {
		Type string `vsp:"object_type,required,CLAS, PROG, INTF, FUGR"`
		Name string `vsp:"name,object name"`
		Bare string `vsp:"bare"`
		Skip string `vsp:"-"`
		None string
	}
	got := Capability{Params: p{}}.ParamList()
	if len(got) != 3 {
		t.Fatalf("parsed %d params, want 3 (untagged and '-' are skipped): %+v", len(got), got)
	}
	if got[0].Name != "object_type" || !got[0].Required {
		t.Errorf("first param parsed as %+v", got[0])
	}
	if got[0].Doc != "CLAS, PROG, INTF, FUGR" {
		t.Errorf("documentation lost its commas: %q", got[0].Doc)
	}
	if got[1].Required {
		t.Error("a param without `required` was marked required")
	}
	if got[2].Doc != "" || got[2].Name != "bare" {
		t.Errorf("a name-only tag parsed as %+v", got[2])
	}
}

// The point of the registry is that nothing else holds the same list. These
// assert the derivations agree with the declaration by construction.
func TestEverythingIsDerivedFromOneDeclaration(t *testing.T) {
	srv := serverForMode(t, "hyperfocused")

	for _, c := range srv.caps.All() {
		// Routed.
		if got, ok := srv.caps.Lookup(c.Action, c.Op); !ok || got.Key() != c.Key() {
			t.Errorf("%s is declared and not routable", c.Key())
		}
		// Documented, with its parameters named.
		help := resultText(srv.handleHelpFor(c.Action))
		if !strings.Contains(help, c.Summary) {
			t.Errorf("%s: help does not carry its summary", c.Key())
		}
		for _, p := range c.ParamList() {
			if !strings.Contains(help, p.Name) {
				t.Errorf("%s: help does not name parameter %q", c.Key(), p.Name)
			}
		}
		// Every capability carries a working call, because a parameter list is
		// not documentation.
		if len(c.Examples) == 0 {
			t.Errorf("%s has no example", c.Key())
		}
		if c.Handler == nil {
			t.Errorf("%s has no handler", c.Key())
		}
	}
}

// The operations named in an error must be operations that exist, or the
// message sends a caller to something that answers "no handler found".
func TestTheOpsNamedInAnErrorAreRouted(t *testing.T) {
	srv := serverForMode(t, "hyperfocused")
	for _, action := range srv.caps.Actions() {
		for _, op := range srv.caps.Ops(action) {
			if _, ok := srv.caps.Lookup(action, op); !ok {
				t.Errorf("%s names op %q and does not route it", action, op)
			}
		}
	}
}

// A write is declared once. The sweep reads that declaration rather than a
// second list of action names, which is how the first version of the read-only
// guard was written.
func TestWritesAreDeclaredNotListed(t *testing.T) {
	srv := serverForMode(t, "hyperfocused")
	var writes int
	for _, c := range srv.caps.All() {
		if c.Writes {
			writes++
			if !strings.Contains(resultText(srv.handleHelpFor(c.Action)), "changes the system") {
				t.Errorf("%s writes and its help does not say so", c.Key())
			}
		}
	}
	if writes == 0 {
		t.Error("no declared capability writes; the two i18n write operations do")
	}
}
