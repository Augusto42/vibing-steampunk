package deps

import (
	"strings"
	"testing"
)

func TestGetDependencyZIP(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantData bool
	}{
		{name: "standalone-lowercase", input: "abapgit-standalone", wantData: true},
		{name: "standalone-uppercase", input: "ABAPGIT-STANDALONE", wantData: true},
		{name: "full", input: "abapgit-full", wantData: true},
		{name: "dev-alias-trimmed", input: "  abapgit-dev  ", wantData: true},
		{name: "unknown", input: "does-not-exist", wantData: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetDependencyZIP(tt.input)
			if tt.wantData && got == nil {
				t.Fatalf("expected ZIP data for %q, got nil", tt.input)
			}
			if !tt.wantData && got != nil {
				t.Fatalf("expected nil for %q, got %d bytes", tt.input, len(got))
			}
		})
	}
}

// The archives are embedded at build time, so a build can know a dependency and
// still carry zero bytes of it — which is the state this repository is in for
// abapGit. A nil check let that through, the caller unpacked nothing, and the
// install reported success.
func TestRequireDependencyZIPRejectsAnEmptyArchive(t *testing.T) {
	for _, name := range []string{"abapgit-standalone", "abapgit-full"} {
		if len(GetDependencyZIP(name)) > 0 {
			t.Skipf("%s is embedded in this build, nothing to prove", name)
		}
		_, err := RequireDependencyZIP(name)
		if err == nil {
			t.Fatalf("%s is empty and must be refused", name)
		}
		if !strings.Contains(err.Error(), "vsp rfc export") {
			t.Fatalf("the error must say how to produce the archive, got: %v", err)
		}
	}
}

func TestRequireDependencyZIPRejectsAnUnknownName(t *testing.T) {
	if _, err := RequireDependencyZIP("not-a-dependency"); err == nil {
		t.Fatal("an unknown dependency must be refused")
	}
}
