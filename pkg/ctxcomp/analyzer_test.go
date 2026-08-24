package ctxcomp

import (
	"context"
	"testing"
)

func TestAnalyzerOffline(t *testing.T) {
	// The comment below uses " and not an indented *, because an indented * is
	// not a comment in ABAP — it is a syntax error, and source like it cannot
	// be read from an activated object. The fixture had one, the old extractor
	// was lenient about it, and the statement parser is not. Strictness loses
	// nothing real here and the leniency was hiding that this layer was not
	// using the parser it was named for.
	source := `CLASS zcl_demo DEFINITION PUBLIC.
  PUBLIC SECTION.
    DATA mo_log TYPE REF TO zif_logger.
    METHODS run IMPORTING io_helper TYPE REF TO zcl_helper.
ENDCLASS.
CLASS zcl_demo IMPLEMENTATION.
  METHOD run.
    DATA(lo) = NEW zcl_factory( ).
    zcl_util=>do_stuff( ).
    CALL FUNCTION 'Z_GET_DATA'.
    " comment: zcl_fake_comment=>nope
    WRITE 'zcl_fake_string=>nope'.
  ENDMETHOD.
ENDCLASS.`

	analyzer := NewAnalyzer(nil) // offline mode
	result := analyzer.Analyze(context.Background(), source, "ZCL_DEMO")

	t.Logf("Layers used: %v", result.Layers)
	t.Logf("Duration: %v", result.Duration)
	t.Logf("True deps: %d, False positives: %d", result.TrueDeps, result.FalsePositives)
	t.Logf("")

	for _, dep := range result.Dependencies {
		status := "TRUE"
		if dep.InString || dep.InComment {
			status = "FALSE POSITIVE"
		}
		layers := ""
		for _, l := range dep.FoundBy {
			layers += l.String() + "+"
		}
		t.Logf("  %-25s conf=%.1f  %s  [%s]", dep.Name, dep.Confidence, status, layers)
	}

	// Verify false positives detected
	for _, dep := range result.Dependencies {
		if dep.Name == "ZCL_FAKE_STRING" || dep.Name == "ZCL_FAKE_COMMENT" {
			if !dep.InString && !dep.InComment {
				t.Errorf("%s should be marked as false positive", dep.Name)
			}
		}
	}

	// Verify real deps found
	realDeps := map[string]bool{"ZIF_LOGGER": false, "ZCL_HELPER": false, "ZCL_FACTORY": false, "ZCL_UTIL": false, "Z_GET_DATA": false}
	for _, dep := range result.Dependencies {
		if _, ok := realDeps[dep.Name]; ok {
			realDeps[dep.Name] = true
		}
	}
	for name, found := range realDeps {
		if !found {
			t.Errorf("Missing real dependency: %s", name)
		}
	}
}
