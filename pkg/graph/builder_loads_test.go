package graph

import "testing"

// D010INC is the only source in this package that answers "what has to be
// loaded", as against "what does this name". The distinction is the reason the
// table is worth reading at all, so the tests are about what gets kept and what
// gets dropped rather than about counting.

func edgeSet(g *Graph) map[string]EdgeKind {
	out := map[string]EdgeKind{}
	for _, e := range g.Edges() {
		out[e.From+" -> "+e.To] = e.Kind
	}
	return out
}

func TestALoadBetweenTwoObjectsIsAnEdge(t *testing.T) {
	g := BuildD010INCGraph([]D010INCRow{
		{Master: "SAPLZDEMO_GROUP", Include: "CL_ABAP_TYPEDESCR=============CT"},
	})
	edges := edgeSet(g)
	if len(edges) != 1 {
		t.Fatalf("one row between two objects is one edge, got %v", edges)
	}
	for k, kind := range edges {
		if kind != EdgeLoads {
			t.Errorf("%s should be a LOADS edge, not %s — a load is not a call", k, kind)
		}
	}
}

func TestAnObjectLoadingItsOwnPartsIsNotADependency(t *testing.T) {
	// This is most of the table. Every class loads its own methods, and saying
	// so would bury the rows that mean something.
	g := BuildD010INCGraph([]D010INCRow{
		{Master: "ZCL_DEMO_ORDER===============CP", Include: "ZCL_DEMO_ORDER===============CM001"},
		{Master: "ZCL_DEMO_ORDER===============CP", Include: "ZCL_DEMO_ORDER===============CM002"},
	})
	if n := len(g.Edges()); n != 0 {
		t.Errorf("a class containing its own methods is containment, not dependency, got %d edges", n)
	}
}

func TestKernelIncludesAreNotObjects(t *testing.T) {
	g := BuildD010INCGraph([]D010INCRow{
		{Master: "SAPLZDEMO_GROUP", Include: "<SYSINI>"},
		{Master: "SAPLZDEMO_GROUP", Include: "%_CABAP"},
	})
	if n := len(g.Edges()); n != 0 {
		t.Errorf("<SYSINI> stands on every row in the table and is nothing anyone can look up, got %d edges", n)
	}
}

func TestObsoleteRowsAreHistoryNotStructure(t *testing.T) {
	g := BuildD010INCGraph([]D010INCRow{
		{Master: "SAPLZDEMO_GROUP", Include: "CX_ROOT======================CU", ObsoleteInVersion: 740},
	})
	if n := len(g.Edges()); n != 0 {
		t.Errorf("a row marked obsolete describes a release this is not, got %d edges", n)
	}
}

func TestTheLoadedPoolKindSurvivesInTheEdge(t *testing.T) {
	// CT is a class's type pool, IU an interface, a bare name an ordinary
	// include. Normalising the name to an object loses that, and it is the part
	// that says why the load exists.
	g := BuildD010INCGraph([]D010INCRow{
		{Master: "SAPLZDEMO_GROUP", Include: "IF_MESSAGE===================IU"},
	})
	edges := g.Edges()
	if len(edges) != 1 {
		t.Fatalf("expected one edge, got %d", len(edges))
	}
	if edges[0].RefDetail == "" || edges[0].RefDetail == "LOADS:" {
		t.Errorf("the raw include should be kept, got %q", edges[0].RefDetail)
	}
}

// Found in the tool's own output, not by reading: a class's generated companion
// is the *master* of some rows, so filtering only the include side let it
// through as another object loading the class. Names of that shape are in
// neither REPOSRC nor TADIR — nothing can look one up, transport it or break
// it, which is what <SYSINI> is too.

func TestAGeneratedCompanionIsNotAnotherObject(t *testing.T) {
	g := BuildD010INCGraph([]D010INCRow{
		{Master: "~CL_DEMO_ORDER===============HCZ", Include: "ZCL_DEMO_ORDER===============CU"},
		{Master: "~CL_DEMO_ORDER===============HPZ", Include: "ZCL_DEMO_ORDER===============CCDEF"},
	})
	if n := len(g.Edges()); n != 0 {
		t.Errorf("a generated companion is machinery on either side of the row, got %d edges: %v", n, edgeSet(g))
	}
}

func TestARealMasterStillProducesAnEdge(t *testing.T) {
	// The guard must not swallow the rows it exists beside.
	g := BuildD010INCGraph([]D010INCRow{
		{Master: "ZCL_DEMO_CALLER==============CP", Include: "ZCL_DEMO_ORDER===============CU"},
	})
	if n := len(g.Edges()); n != 1 {
		t.Errorf("one object loading another is the whole point, got %d", n)
	}
}
