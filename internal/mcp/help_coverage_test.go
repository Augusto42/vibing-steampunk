package mcp

import (
	"regexp"
	"strings"
	"testing"
)

// The tool's own description lists the actions it accepts, and `help` documents
// them. Those are two published surfaces and nothing held them together: when
// eleven capabilities were routed under three new actions, the description
// gained them and help did not — so an agent could see that action="i18n"
// exists and had no way to learn what it takes.
//
// This checks each against the other. It is the same shape as the reach pass:
// a claim is only worth what asserts it.
func TestEveryAdvertisedActionIsDocumented(t *testing.T) {
	srv := serverForMode(t, "hyperfocused")
	tools := srv.mcpServer.ListTools()
	sap, ok := tools["SAP"]
	if !ok {
		t.Fatal("the universal tool is not registered")
	}
	desc := sap.Tool.Description

	// The description carries one line naming them: "actions: a, b, c".
	m := regexp.MustCompile(`(?m)^actions:\s*(.+)$`).FindStringSubmatch(desc)
	if m == nil {
		t.Fatal("the tool description no longer names its actions in an 'actions:' line; " +
			"this test reads that line, and a description that stops naming them is itself the defect")
	}
	var actions []string
	for _, a := range strings.Split(m[1], ",") {
		if a = strings.TrimSpace(a); a != "" && a != "help" {
			actions = append(actions, a)
		}
	}
	if len(actions) < 10 {
		t.Fatalf("only %d actions parsed from the description: %v", len(actions), actions)
	}

	// The fallback prints one general overview for anything it does not know,
	// and that overview names every action — so "the text mentions the action"
	// passes for every action whether documented or not. The first version of
	// this test did exactly that and passed while `rfc` had no entry.
	//
	// Comparing against the fallback itself is the check that cannot be fooled.
	fallback := resultText(srv.handleHelpFor("__no_such_topic_exists__"))
	if fallback == "" {
		t.Fatal("help has no fallback text, so this test cannot tell a documented action from an undocumented one")
	}

	for _, action := range actions {
		text := resultText(srv.handleHelpFor(action))
		switch {
		case text == "":
			t.Errorf("action %q is advertised and help returns nothing for it", action)
		case text == fallback:
			t.Errorf("action %q is advertised and has no help entry — it falls through to the overview", action)
		}
	}
}

// And the other direction: help must not document an action nothing routes,
// which would send a reader to call something that answers "no handler found".
func TestHelpDocumentsNothingUnrouted(t *testing.T) {
	srv := serverForMode(t, "hyperfocused")
	desc := srv.mcpServer.ListTools()["SAP"].Tool.Description
	m := regexp.MustCompile(`(?m)^actions:\s*(.+)$`).FindStringSubmatch(desc)
	if m == nil {
		t.Skip("no actions line")
	}
	advertised := map[string]bool{"help": true, "tips": true}
	for _, a := range strings.Split(m[1], ",") {
		advertised[strings.TrimSpace(a)] = true
	}
	// Topics help answers that are not actions are fine when they are sub-topics
	// of one — effects is an analyze type. Named here so the exception is
	// deliberate rather than a hole.
	subTopics := map[string]bool{"effects": true, "history": true}

	for _, c := range srv.caps.All() {
		if !advertised[c.Action] {
			t.Errorf("the registry declares action %q and the tool description does not advertise it", c.Action)
		}
	}
	for _, topic := range helpTopics() {
		if advertised[topic] || subTopics[topic] {
			continue
		}
		t.Errorf("help documents %q, which no action advertises and nothing routes", topic)
	}
}
