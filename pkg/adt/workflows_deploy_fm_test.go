package adt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A function module is addressable only under its group. The abapGit filename
// carries both — {group}.fugr.{name}.func.abap — and ParseABAPFile reads them
// correctly. The deploy path then dropped the group on the floor twice, so a
// module whose group had just been parsed out of its own filename failed with
// "function module requires parent function group name".
//
// It failed on update rather than create, which is why it looked intermittent:
// DeployFromFile delegates to UpdateFromFile whenever the object already
// exists, so the first deploy of a module could work and every one after it
// could not.

func TestAFunctionModuleFilenameCarriesItsGroup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "zdemo_fg.fugr.z_demo_call.func.abap")
	if err := os.WriteFile(path, []byte("FUNCTION z_demo_call.\nENDFUNCTION.\n"), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	info, err := ParseABAPFile(path)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if info.ObjectType != ObjectTypeFunctionMod {
		t.Fatalf("expected a function module, got %v", info.ObjectType)
	}
	if info.ObjectName != "Z_DEMO_CALL" {
		t.Errorf("module name, got %q", info.ObjectName)
	}
	if info.ParentName != "ZDEMO_FG" {
		t.Errorf("the group is in the filename and must survive parsing, got %q", info.ParentName)
	}
}

func TestTheSourceURLOfAFunctionModuleIncludesItsGroup(t *testing.T) {
	c := &Client{}
	got, err := c.buildSourceURL(ObjectTypeFunctionMod, "Z_DEMO_CALL", "ZDEMO_FG")
	if err != nil {
		t.Fatalf("the group was supplied, so there is nothing to fail on: %v", err)
	}
	for _, want := range []string{"/functions/groups/zdemo_fg", "/fmodules/z_demo_call", "/source/main"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in %q", want, got)
		}
	}
}

func TestAFunctionModuleWithoutAGroupStillRefuses(t *testing.T) {
	c := &Client{}
	if _, err := c.buildSourceURL(ObjectTypeFunctionMod, "Z_DEMO_CALL", ""); err == nil {
		t.Error("a module with no group is not addressable, and guessing one would be worse than refusing")
	}
}

func TestOtherTypesAreUnaffectedByTheParent(t *testing.T) {
	c := &Client{}
	got, err := c.buildSourceURL(ObjectTypeProgram, "ZDEMO_REPORT", "")
	if err != nil {
		t.Fatalf("a program has no parent and never needed one: %v", err)
	}
	if !strings.Contains(got, "/programs/programs/zdemo_report/source/main") {
		t.Errorf("unexpected URL %q", got)
	}
}
