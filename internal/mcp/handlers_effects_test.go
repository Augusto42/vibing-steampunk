package mcp

import (
	"strings"
	"testing"
)

// The classification is the answer, and the label is not the answer. "owner"
// means nothing to a reader who does not know the model; what they need is that
// this unit ends their transaction.
func TestEachLUWClassSaysWhatItMeansForTheCaller(t *testing.T) {
	cases := map[string]string{
		"safe":        "intact",
		"participant": "caller",
		"owner":       "ends its caller",
		"unsafe":      "part of what it queues",
	}
	for class, want := range cases {
		if got := luwConsequence(class); !strings.Contains(got, want) {
			t.Errorf("luwConsequence(%q) = %q, expected it to mention %q", class, got, want)
		}
	}
	if luwConsequence("something-new") == "" {
		t.Error("an unknown classification must still say something rather than nothing")
	}
}

// The case worth catching: a unit that both commits and defers. Part of what it
// queues is committed by its own COMMIT and part by the caller's, and neither
// "owner" nor "participant" describes it.
func TestCommitPlusDeferredWorkIsUnsafe(t *testing.T) {
	const src = `METHOD save.
  UPDATE zdemo_orders SET status = 'X'.
  CALL FUNCTION 'ZDEMO_AUDIT' IN UPDATE TASK.
  COMMIT WORK.
ENDMETHOD.`
	a := analyseEffects("ZCL_DEMO", src)
	if a.LUW != "unsafe" {
		t.Errorf("LUW = %q, want unsafe", a.LUW)
	}
	if a.Pure {
		t.Error("a unit that writes and commits is not pure")
	}
	if !contains(a.Effects, "COMMIT WORK") {
		t.Errorf("effects do not mention the commit: %v", a.Effects)
	}
	if !contains(a.WritesTables, "ZDEMO_ORDERS") {
		t.Errorf("writes = %v, expected the updated table", a.WritesTables)
	}
}

// A unit that only defers is a participant: its writes land in somebody else's
// transaction, and that is the fact the caller needs.
func TestDeferredWorkAloneIsParticipant(t *testing.T) {
	const src = `METHOD queue.
  CALL FUNCTION 'ZDEMO_AUDIT' IN UPDATE TASK.
ENDMETHOD.`
	a := analyseEffects("ZCL_DEMO", src)
	if a.LUW != "participant" {
		t.Errorf("LUW = %q, want participant", a.LUW)
	}
	if !strings.Contains(a.Consequence, "somebody else") {
		t.Errorf("the consequence does not say whose transaction it lands in: %q", a.Consequence)
	}
}

// The limit must travel with the answer. A reader who takes a local "safe" for
// a transitive one has been misled by the report rather than by the code — and
// the steering plan that designed this named exactly that as its first risk.
func TestTheAnswerStatesThatItIsLocal(t *testing.T) {
	a := analyseEffects("ZCL_DEMO", "METHOD m.\nENDMETHOD.")
	var said bool
	for _, n := range a.Notes {
		if strings.Contains(n, "this source only") {
			said = true
		}
	}
	if !said {
		t.Errorf("the answer does not say the analysis is local: %v", a.Notes)
	}
	if !a.Pure {
		t.Error("an empty method has no detectable effect")
	}
	// And "pure" must not be allowed to read as transitively pure.
	var qualified bool
	for _, n := range a.Notes {
		if strings.Contains(n, "not that the unit is pure transitively") {
			qualified = true
		}
	}
	if !qualified {
		t.Error("pure was reported without the qualification that makes it true")
	}
}

// Reached through the dispatch an agent uses, in the mode that could not reach
// anything until today.
func TestEffectsIsRoutedFromTheUniversalTool(t *testing.T) {
	srv := serverForMode(t, "focused")
	result, err := srv.handleUniversalTool(t.Context(), newRequest(map[string]any{
		"action": "analyze",
		"params": map[string]any{"type": "effects", "source": "METHOD m. COMMIT WORK. ENDMETHOD."},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(result)
	if strings.Contains(text, "No handler found") {
		t.Fatalf("analyze type=effects is not routed:\n%s", text)
	}
	if !strings.Contains(text, "owner") {
		t.Errorf("a method containing COMMIT WORK was not classified as an owner:\n%s", text)
	}
}

// Called with neither source nor object, it must name what it needs — not deny
// itself, which is the rule the query and grep defect established.
func TestEffectsWithoutInputNamesWhatItNeeds(t *testing.T) {
	srv := serverForMode(t, "expert")
	result, err := srv.handleUniversalTool(t.Context(), newRequest(map[string]any{
		"action": "analyze",
		"params": map[string]any{"type": "effects"},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(result)
	if strings.Contains(text, "No handler found") {
		t.Fatalf("the action denied its own existence:\n%s", text)
	}
	if !strings.Contains(text, "source") {
		t.Errorf("it does not say what it needs:\n%s", text)
	}
}

func contains(items []string, want string) bool {
	for _, i := range items {
		if strings.Contains(i, want) {
			return true
		}
	}
	return false
}
