package main

import (
	"strings"
	"testing"
)

// A system declared read_only was writable from every CLI subcommand, because
// only the MCP server ever handed a safety configuration to its client. The
// setting said one thing and the tool did another, which is the worst kind of
// safety feature.
func TestGetClientHonoursDeclaredSafety(t *testing.T) {
	client, err := getClient(&systemParams{
		URL: "https://sap.example:44300", User: "TESTER", Password: "secret",
		Client: "001", Language: "EN", ReadOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if safety := client.Safety(); safety == nil || !safety.ReadOnly {
		t.Fatalf("a read_only system must reach the client as read-only, got %+v", safety)
	}
}

// And a system with an allowed-package list must carry it too, or the list is
// advice rather than a restriction.
func TestGetClientCarriesAllowedPackages(t *testing.T) {
	client, err := getClient(&systemParams{
		URL: "https://sap.example:44300", User: "TESTER", Password: "secret",
		Client: "001", Language: "EN", AllowedPackages: []string{"Z*"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if safety := client.Safety(); safety == nil || strings.Join(safety.AllowedPackages, ",") != "Z*" {
		t.Fatalf("allowed packages did not reach the client: %+v", safety)
	}
}

func TestGetClientUnrestrictedByDefault(t *testing.T) {
	client, err := getClient(&systemParams{
		URL: "https://sap.example:44300", User: "TESTER", Password: "secret",
		Client: "001", Language: "EN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if safety := client.Safety(); safety != nil && safety.ReadOnly {
		t.Fatal("an unrestricted system must not arrive read-only")
	}
}

func TestSplitListIgnoresBlanks(t *testing.T) {
	got := splitList(" Z*, ,Y* ")
	if strings.Join(got, "|") != "Z*|Y*" {
		t.Fatalf("got %v", got)
	}
}
