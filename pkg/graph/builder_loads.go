package graph

import "strings"

// The load graph, from D010INC.
//
// Every other source in this package answers "what does this object *refer to*".
// D010INC answers a different question — "what has to be loaded for this to
// run" — and it is the only source here that does. The two are not the same
// relation and the difference is not academic:
//
//   - A reference recorded at activation says the code names something. A load
//     says the runtime cannot start the program without it. A program can name
//     an object it never loads (dead code) and load one it never names (an
//     include pulled in for its declarations).
//   - The INCLUDE statement produces a dependency that appears in no
//     cross-reference table at all. A report split across five includes is one
//     compiled unit; CROSS and WBCROSSGT record what the *includes* reference
//     and say nothing about the split itself.
//
// The table is two useful columns, MASTER and INCLUDE, both in the padded form
// the rest of this package already reads. Checked live on 7.58:
//
//	MASTER                             INCLUDE
//	ZCL_VSP_GIT_SERVICE===========CP   ZCL_VSP_GIT_SERVICE===========CM001
//	SAPLZDEMO_GROUP                    LZDEMO_GROUPTOP
//	SAPLZDEMO_GROUP                    CL_ABAP_TYPEDESCR=============CT
//	SAPLZDEMO_GROUP                    <SYSINI>
//
// Three kinds of row, and only one of them is a dependency between objects.

// D010INCRow is one load relationship: MASTER loads INCLUDE.
type D010INCRow struct {
	// Master is the compiled unit — a program, or the pool of a class or
	// function group, in its padded form.
	Master string
	// Include is what the load pulls in.
	Include string
	// ObsoleteInVersion is non-zero for rows kept for older releases. They are
	// history rather than current structure, and are dropped.
	ObsoleteInVersion int
}

// BuildD010INCGraph turns load rows into LOADS edges between objects.
//
// It reports edges between *different* objects only. A class pool loading its
// own method includes is the overwhelming majority of this table and says
// nothing a caller did not know: every class contains its methods. Reporting
// those would bury the rows that matter under a hundred times their number.
func BuildD010INCGraph(rows []D010INCRow) *Graph {
	g := New()

	for _, row := range rows {
		master := strings.TrimSpace(row.Master)
		include := strings.TrimSpace(row.Include)
		if master == "" || include == "" || row.ObsoleteInVersion != 0 {
			continue
		}
		// Both sides, not just the include. A generated companion appears as
		// the *master* of a row and would otherwise be reported as another
		// object loading this one.
		if isGenerated(include) || isGenerated(master) {
			continue
		}

		fromID, fromType, fromName := NormalizeInclude(master)
		toID, toType, toName := NormalizeInclude(include)
		if fromName == "" || toName == "" {
			continue
		}
		// An object loading its own parts is containment, not dependency.
		if strings.EqualFold(fromName, toName) {
			continue
		}

		g.AddNode(&Node{ID: fromID, Name: fromName, Type: fromType, Includes: []string{master}})
		g.AddNode(&Node{ID: toID, Name: toName, Type: toType, Includes: []string{include}})
		g.AddEdge(&Edge{
			From:   fromID,
			To:     toID,
			Kind:   EdgeLoads,
			Source: SourceD010INC,
			// The raw include is kept because the padded suffix says what kind
			// of pool was loaded — CT a class's types, IU an interface, and a
			// bare name an ordinary include — and that distinction is lost once
			// the name is normalised.
			RefDetail: "LOADS:" + include,
		})
	}

	return g
}

// isGenerated reports whether a name is machinery rather than an object.
//
// Three shapes, and the third was found in the tool's own output rather than by
// reading. <SYSINI> stands on nearly every row and %_CABAP on most; neither is
// anything a caller can look up.
//
// The third is a tilde prefix — ~CL_VSP_GIT_SERVICE===========HCZ appeared as
// the *master* of a row, so a class's own generated companion was reported as
// another object loading it. Checked rather than assumed: names of that shape
// are in neither REPOSRC nor TADIR, so they are not programs with source and
// not repository objects. Something exists at load time that nothing can look
// up, transport or break, which is the same class of thing as <SYSINI>.
//
// What HCZ and HPZ stand for is not recorded here, because nothing consulted
// says: the filter rests on the tilde and on the absence from both catalogues,
// not on a reading of the suffix.
func isGenerated(name string) bool {
	switch {
	case strings.HasPrefix(name, "<"):
		return true
	case strings.HasPrefix(name, "%_"):
		return true
	case strings.HasPrefix(name, "~"):
		return true
	}
	return false
}
