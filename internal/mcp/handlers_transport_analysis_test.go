package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
)

// A transport boundary report answers "is this transport self-consistent", and
// an object whose source would not load contributes no dependencies at all —
// which is exactly what a self-consistent object looks like. The gaps have to
// travel inside the JSON, because that document is the whole of what an agent
// sees.
func TestTransportBoundaryGapsRideInsideTheDocument(t *testing.T) {
	envelope := transportBoundaryResult{
		TransportBoundaryReport: &graph.TransportBoundaryReport{},
		Notes: []string{
			"objects in this transport scope could not be read, so their dependencies are missing from the analysis below.\n" +
				adt.UnsearchedNote([]adt.Unsearched{{Object: "CLAS ZCL_DEMO", Reason: "not authorised"}}, 4, "object"),
		},
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("the payload must stay parseable: %v", err)
	}
	if _, ok := parsed["notes"]; !ok {
		t.Fatalf("the gaps must be a field of the report, not prose beside it:\n%s", doc)
	}
	for _, want := range []string{"ZCL_DEMO", "not authorised", "1 of 4"} {
		if !strings.Contains(doc, want) {
			t.Fatalf("the payload is missing %q:\n%s", want, doc)
		}
	}
}

// A transport whose objects all loaded must not grow an empty notes array, or
// the caveat stops being a signal.
func TestACleanTransportScopeCarriesNoNotes(t *testing.T) {
	data, err := json.Marshal(transportBoundaryResult{TransportBoundaryReport: &graph.TransportBoundaryReport{}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "notes") {
		t.Fatalf("nothing was missed, so nothing should be said:\n%s", data)
	}
}
