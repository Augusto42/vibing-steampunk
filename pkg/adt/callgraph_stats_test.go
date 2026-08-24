package adt

import "testing"

// The callee side of a call graph is built from cross-reference rows. Those name
// an object and carry no ADT path, so every child arrives with an empty URI.
// Deduplicating on that key folded twenty-seven distinct callees into one, and
// the answer reported two nodes directly beside its own list of twenty-seven
// edges — a contradiction visible in a single response.

func TestCalleesWithoutURIsAreCountedAsDistinctNodes(t *testing.T) {
	root := &CallGraphNode{
		URI: "/sap/bc/adt/oo/classes/zcl_demo_order", Name: "ZCL_DEMO_ORDER", Type: "CLAS",
	}
	for _, n := range []string{"CL_ABAP_ZIP", "CL_ABAP_CODEPAGE", "ZCL_DEMO_HELPER", "ABAP_BOOL"} {
		root.Children = append(root.Children, CallGraphNode{Name: n, Type: "method"})
	}

	stats := AnalyzeCallGraph(root)
	if stats.TotalEdges != 4 {
		t.Fatalf("four children, got %d edges", stats.TotalEdges)
	}
	if stats.TotalNodes != 5 {
		t.Errorf("the root and four distinct callees are five nodes, got %d — "+
			"an empty URI is not evidence that two objects are the same one", stats.TotalNodes)
	}
	if len(stats.UniqueNodes) != 5 {
		t.Errorf("every distinct callee should be named, got %v", stats.UniqueNodes)
	}
}

func TestTheSameCalleeTwiceIsStillOneNode(t *testing.T) {
	root := &CallGraphNode{URI: "/x", Name: "ROOT", Type: "CLAS"}
	root.Children = []CallGraphNode{
		{Name: "CL_ABAP_ZIP", Type: "method"},
		{Name: "CL_ABAP_ZIP", Type: "method"},
	}
	stats := AnalyzeCallGraph(root)
	if stats.TotalEdges != 2 {
		t.Errorf("two references are two edges, got %d", stats.TotalEdges)
	}
	if stats.TotalNodes != 2 {
		t.Errorf("the root and one callee are two nodes, got %d — dedup still has to work", stats.TotalNodes)
	}
}

func TestNodesWithRealURIsStillDedupeByURI(t *testing.T) {
	root := &CallGraphNode{URI: "/a", Name: "ROOT", Type: "CLAS"}
	root.Children = []CallGraphNode{
		{URI: "/b", Name: "SAME", Type: "CLAS"},
		{URI: "/b", Name: "SAME_RENDERED_DIFFERENTLY", Type: "CLAS"},
	}
	stats := AnalyzeCallGraph(root)
	if stats.TotalNodes != 2 {
		t.Errorf("one URI is one object whatever it is called, got %d nodes", stats.TotalNodes)
	}
}

// Coverage is executed invocations over recorded invocations. Most static edges
// of an ordinary class are type and data references — ABAP_BOOL, SYST, TADIR —
// and counting those as paths that were never taken produced 0.037 for a class
// where the only thing callable had in fact been called.

func TestCoverageIgnoresReferencesNothingCouldExecute(t *testing.T) {
	static := []CallGraphEdge{
		{CallerName: "ZCL_DEMO_ORDER", CalleeName: "CL_ABAP_ZIP", CalleeKind: "method"},
		{CallerName: "ZCL_DEMO_ORDER", CalleeName: "Z_DEMO_FM", CalleeKind: "function module"},
		{CallerName: "ZCL_DEMO_ORDER", CalleeName: "ABAP_BOOL", CalleeKind: "type"},
		{CallerName: "ZCL_DEMO_ORDER", CalleeName: "SYST", CalleeKind: "type"},
		{CallerName: "ZCL_DEMO_ORDER", CalleeName: "SY", CalleeKind: "data"},
	}
	actual := []CallGraphEdge{
		{CallerName: "ZCL_DEMO_ORDER", CalleeName: "CL_ABAP_ZIP", CalleeKind: "method"},
		{CallerName: "ZCL_DEMO_ORDER", CalleeName: "Z_DEMO_FM", CalleeKind: "function module"},
	}

	comp := CompareCallGraphs(static, actual)
	if comp.ExecutableEdges != 2 {
		t.Fatalf("two of the five edges are invocations, got %d", comp.ExecutableEdges)
	}
	if comp.CoverageRatio != 1.0 {
		t.Errorf("everything callable was called, so coverage is 1.0, got %v — "+
			"three type and data references are not paths that went untaken", comp.CoverageRatio)
	}
}

func TestCoverageIsNotZeroWhenThereWasNothingCallable(t *testing.T) {
	static := []CallGraphEdge{
		{CallerName: "ZCL_DEMO_TYPES", CalleeName: "ABAP_BOOL", CalleeKind: "type"},
	}
	comp := CompareCallGraphs(static, nil)
	if comp.CoverageRatio != -1 {
		t.Errorf("nothing callable was recorded, so there is no coverage to report; "+
			"a zero would read as 'none of it ran', got %v", comp.CoverageRatio)
	}
}
