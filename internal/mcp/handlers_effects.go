package mcp

// Side-effect and LUW analysis, reachable at last.
//
// `pkg/graph.ExtractEffects` has been implemented, tested and described in the
// README since April, and until this file nothing called it. Not a CLI command,
// not an MCP action — 250 lines of correct Go that no user could invoke, while
// two README sections said the capability was there. It was a library described
// as a feature, and the steering plan written the same day as the
// implementation had already named the risk: "semantic overclaiming".
//
// What it answers is worth the wiring. The interesting effects in ABAP are not
// database writes but **LUW effects**, because ABAP lets a unit defer a write so
// that it lands inside somebody else's transaction:
//
//	A method that calls CALL FUNCTION … IN UPDATE TASK is not writing yet — it
//	is deferring. Whoever calls COMMIT WORK higher up triggers every deferred
//	write. That is invisible coupling, and nothing in SAP's toolchain reports it.
//
// So the answer leads with the classification and says what it means for the
// caller, because "LUW-owner" is a label and "this commits, so it ends its
// caller's transaction" is a fact somebody can act on.

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/graph"
)

// effectsAnswer is what a caller gets back.
type effectsAnswer struct {
	Object string `json:"object,omitempty"`
	Lines  int    `json:"lines"`

	// LUW is the classification, and Consequence is what it means for whoever
	// calls this code. The label alone has repeatedly been read as a severity,
	// which it is not: an owner is not worse than a participant, it is
	// different, and the difference is what the caller has to plan around.
	LUW         string `json:"luw"`
	Consequence string `json:"consequence"`
	Pure        bool   `json:"pure"`

	ReadsTables  []string `json:"readsTables,omitempty"`
	WritesTables []string `json:"writesTables,omitempty"`
	RFCTargets   []string `json:"rfcDestinations,omitempty"`

	// Effects lists what was detected, in the caller's vocabulary rather than
	// as a struct of booleans most of which are false.
	Effects []string `json:"effects,omitempty"`

	Notes []string `json:"notes,omitempty"`
}

// luwConsequence turns the classification into the sentence a caller needs.
func luwConsequence(class string) string {
	switch class {
	case "safe":
		return "this unit neither commits nor registers deferred work, so it leaves its caller's transaction intact"
	case "participant":
		return "this unit registers work that runs when somebody else commits, so its writes land inside the caller's transaction and are invisible here"
	case "owner":
		return "this unit contains COMMIT WORK, so it ends its caller's transaction — every caller above it loses atomicity"
	case "unsafe":
		return "this unit both commits and registers deferred work, so part of what it queues may be committed by its own COMMIT and part by the caller's"
	}
	return "the classification is not one this build knows about"
}

// effectList renders the detected effects as phrases.
func effectList(e *graph.EffectInfo) []string {
	var out []string
	add := func(cond bool, phrase string) {
		if cond {
			out = append(out, phrase)
		}
	}
	add(e.HasCommit, "COMMIT WORK")
	add(e.HasRollback, "ROLLBACK WORK")
	add(e.UpdateTask, "registers work IN UPDATE TASK")
	add(e.BackgroundTask, "registers work IN BACKGROUND TASK")
	add(e.UpdateTaskLocal, "SET UPDATE TASK LOCAL")
	add(e.AsyncRFC, "calls asynchronously (STARTING NEW TASK)")
	add(e.BackgroundJob, "submits a background job")
	add(e.SubmitAndReturn, "SUBMIT AND RETURN")
	add(e.HTTPCall, "opens an HTTP client")
	add(e.APCPush, "pushes over APC/WebSocket")
	add(e.ReadsState, "reads instance or class state")
	add(e.WritesState, "writes instance or class state")
	add(e.RaisesExc, "raises an exception")
	add(e.RaisesMessage, "issues MESSAGE type E/A/X")
	add(e.LeavesContext, "leaves the program or the transaction")
	return out
}

// analyseEffects builds the answer from source.
func analyseEffects(object, source string) effectsAnswer {
	e := graph.ExtractEffects(source)
	class := e.ClassifyLUW()
	answer := effectsAnswer{
		Object:       object,
		Lines:        strings.Count(source, "\n") + 1,
		LUW:          class,
		Consequence:  luwConsequence(class),
		Pure:         e.IsPure(),
		ReadsTables:  e.ReadsDB,
		WritesTables: e.WritesDB,
		RFCTargets:   e.SyncRFC,
		Effects:      effectList(e),
	}

	// The limits, stated with the answer rather than in documentation nobody
	// reads next to it. This analysis is local: it reads one unit's own source
	// and nothing it calls, so a method that looks safe may call one that
	// commits. Saying so is the difference between a summary and a claim.
	answer.Notes = append(answer.Notes,
		"this is local analysis: it reads this source only, so an effect inside something it calls is not counted here")
	if answer.Pure {
		answer.Notes = append(answer.Notes,
			"pure here means no effect was detected in this source, not that the unit is pure transitively")
	}
	if len(e.WritesDB) > 0 && class == "participant" {
		answer.Notes = append(answer.Notes,
			"the writes listed are issued directly; the deferred ones are not visible in this source at all")
	}
	return answer
}

// handleAnalyzeEffects answers analyze type=effects.
func (s *Server) handleAnalyzeEffects(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	source := firstParam(args, "source")
	objectType := strings.ToUpper(firstParam(args, "object_type"))
	name := strings.ToUpper(firstParam(args, "name", "object_name"))

	if source == "" {
		if objectType == "" || name == "" {
			return needParams("analyze type=effects", args,
				[]string{"source", "object_type plus name"},
				`SAP(action="analyze", params={"type": "effects", "object_type": "CLAS", "name": "ZCL_DEMO"})
  SAP(action="analyze", params={"type": "effects", "source": "METHOD m. COMMIT WORK. ENDMETHOD."})`), nil
		}
		var err error
		source, err = s.adtClient.GetSource(ctx, objectType, name, nil)
		if err != nil {
			return newToolResultError(fmt.Sprintf("Failed to fetch source for %s %s: %v", objectType, name, err)), nil
		}
	}
	if name == "" {
		name = "(supplied source)"
	}
	return newToolResultJSON(analyseEffects(name, source)), nil
}
