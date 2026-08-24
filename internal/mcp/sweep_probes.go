package mcp

// The probe table.
//
// Each entry is one advertised capability, an input that has an answer on a
// stock system, and — where an empty answer would otherwise be unfalsifiable —
// a second route that says whether there was anything to find.
//
// Choosing the input is most of the work, and it is where a sweep quietly
// becomes useless. Ask for the callers of a class nobody calls and the empty
// answer is true, the probe passes, and the dead capability behind it stays
// dead. So the targets that matter are not "any class" but "a class that is
// referenced" and "a class that references", resolved before the sweep runs
// and reported as skipped when they cannot be.

import (
	"context"
	"fmt"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/ctxcomp"
)

// SweepProbes returns every probe, in the order a reader would want them.
func SweepProbes() []Probe {
	var out []Probe
	out = append(out, coreActionProbes()...)
	out = append(out, graphProbes()...)
	out = append(out, postMortemProbes()...)
	out = append(out, contextProbes()...)
	return out
}

// coreActionProbes cover the surface every agent touches first. If any of
// these is dead nothing else matters, which is why they come first and why
// several carry oracles even though they look unimpeachable.
func coreActionProbes() []Probe {
	return []Probe{
		{
			ID: "core.help", Capability: "action=help",
			Why:    "the documentation an agent reads before anything else",
			Action: "help", MustContain: "action",
		},
		{
			ID: "core.system", Capability: "action=system",
			Why: "release and database, which every route decision depends on",
			// The sub-operation goes in the target, not in params — which the
			// tool's own description contradicts. See sweep findings.
			Action: "system", Target: "INFO",
		},
		{
			ID: "core.read", Capability: "action=read",
			Why:    "reading a class is the single most used capability",
			Action: "read", Target: "CLAS {class}", Needs: []string{"class"},
			MustContain: "class",
		},
		{
			ID: "core.read.program", Capability: "action=read (PROG)",
			Why:    "programs take a different ADT path from classes",
			Action: "read", Target: "PROG {program}", Needs: []string{"program"},
		},
		{
			ID: "core.search", Capability: "action=search",
			Why:    "object search; an empty answer here is never true for 'CL_*'",
			Action: "search", Target: "CL_*",
			Oracle: oracleAlwaysSome("SAP ships thousands of classes named CL_*"),
		},
		{
			ID: "core.query", Capability: "action=query",
			Why:    "free SQL, the route under most of the analysis surface",
			Action: "query", Needs: []string{"table"},
			Params: map[string]any{"sql": "SELECT * FROM {table}"},
			Oracle: oracleTableHasRows,
		},
		{
			ID: "core.grep", Capability: "action=grep",
			Why:    "source search across a package",
			Action: "grep", Needs: []string{"package"},
			Params:      map[string]any{"pattern": "METHOD", "package": "{package}"},
			EmptyIsFine: true,
		},
	}
}

// graphProbes cover the analysis surface. Four of these — callers, callees,
// call_graph, object_structure — were built on an ADT namespace that exists on
// no release, and returned an empty graph for a year. Every one of them
// therefore carries an oracle.
func graphProbes() []Probe {
	return []Probe{
		{
			ID: "graph.callers", Capability: "analyze type=callers",
			Why:    "the up direction; empty means nothing calls this object",
			Action: "analyze", Needs: []string{"referenced"},
			Params: map[string]any{"type": "callers", "object_name": "{referenced}", "object_type": "CLAS"},
			Oracle: oracleCrossOrLongName,
		},
		{
			ID: "graph.callees", Capability: "analyze type=callees",
			Why:    "the down direction, read from the cross-reference tables",
			Action: "analyze", Needs: []string{"references"},
			Params: map[string]any{"type": "callees", "object_name": "{references}", "object_type": "CLAS"},
			Oracle: oracleParserDeps,
		},
		{
			ID: "graph.call_graph", Capability: "analyze type=call_graph",
			Why:    "the combined graph; it once answered with a root and no children",
			Action: "analyze", Needs: []string{"referenced"},
			Params: map[string]any{"type": "call_graph", "object_name": "{referenced}", "object_type": "CLAS", "direction": "callers"},
			Oracle: oracleWhereUsed,
		},
		{
			ID: "graph.object_structure", Capability: "analyze type=object_structure",
			Why:    "a class always has components; an empty structure is never true",
			Action: "analyze", Needs: []string{"class"},
			Params: map[string]any{"type": "object_structure", "object_name": "{class}", "object_type": "CLAS"},
			Oracle: oracleSourceHasMethods,
		},
		{
			ID: "graph.impact", Capability: "analyze type=impact",
			Why:    "reverse dependencies of an object known to have them",
			Action: "analyze", Needs: []string{"referenced"},
			Params: map[string]any{"type": "impact", "object_name": "{referenced}", "object_type": "CLAS"},
			Oracle: oracleCrossOrLongName,
		},
		{
			ID: "graph.usage_examples", Capability: "analyze type=usage_examples",
			Why:    "asked CROSS for a two-letter code in a one-character column, and had never returned a row",
			Action: "analyze", Needs: []string{"referenced"},
			Params: map[string]any{"type": "usage_examples", "object_name": "{referenced}", "object_type": "CLAS"},
			Oracle: oracleCrossOrLongName,
		},
		{
			ID: "graph.where_used_config", Capability: "analyze type=where_used_config",
			Why:         "filtered on a value that is not a value of that column",
			Action:      "analyze",
			Params:      map[string]any{"type": "where_used_config", "variable": "ZDEMO_PARAM"},
			EmptyIsFine: true,
		},
		{
			ID: "graph.check_boundaries", Capability: "analyze type=check_boundaries",
			Why:    "directional package crossings",
			Action: "analyze", Needs: []string{"package"},
			Params: map[string]any{"type": "check_boundaries", "package": "{package}"},
			// Not EmptyIsFine. This answered CLEAN on a package it had not
			// opened a single file of, and "no crossings" is the same sentence
			// either way.
			Oracle: oraclePackageHasReadableSource,
		},
		{
			ID: "graph.graph_stats", Capability: "analyze type=graph_stats",
			Why:    "graph counts over supplied source; two statements can never be zero nodes",
			Action: "analyze",
			Params: map[string]any{"type": "graph_stats", "source": "REPORT zdemo.\nPERFORM x.\n"},
			Oracle: oracleAlwaysSome("source with a PERFORM in it has at least one edge"),
		},
		{
			ID: "graph.graph_stats.object", Capability: "analyze type=graph_stats (object)",
			Why:    "the same counts asked about a repository object, which the type refused to do until 2026-08-25 while its name promised otherwise",
			Action: "analyze", Needs: []string{"references"},
			Params: map[string]any{"type": "graph_stats", "object_type": "CLAS", "object_name": "{references}"},
			// The object was chosen because the cross-reference tables have rows
			// for it, so its source cannot be free of dependencies either.
			Oracle: oracleCrossOrLongName,
		},
		{
			ID: "graph.graph_stats.package", Capability: "analyze type=graph_stats (package)",
			Why:    "counts over a whole package, through the same scanner the boundary check uses",
			Action: "analyze", Needs: []string{"package"},
			Params: map[string]any{"type": "graph_stats", "package": "{package}"},
			Oracle: oraclePackageHasReadableSource,
		},
		{
			ID: "graph.health", Capability: "analyze type=health",
			Why:    "the report that once said GOOD over a scan that could not run",
			Action: "analyze", Needs: []string{"package"},
			Params:      map[string]any{"type": "health", "package": "{package}"},
			MustContain: "package",
		},
		{
			ID: "graph.co_change", Capability: "analyze type=co_change",
			Why:         "objects that travel together in transports",
			Action:      "analyze",
			Needs:       []string{"referenced"},
			Params:      map[string]any{"type": "co_change", "object_type": "CLAS", "object_name": "{referenced}"},
			EmptyIsFine: true,
		},
		{
			ID: "graph.cr_history", Capability: "analyze type=cr_history",
			Why:         "change-request grouping over transport attributes",
			Action:      "analyze",
			Needs:       []string{"referenced"},
			Params:      map[string]any{"type": "cr_history", "object_type": "CLAS", "object_name": "{referenced}"},
			EmptyIsFine: true,
		},
		{
			ID: "graph.analyze_call_graph", Capability: "analyze type=analyze_call_graph",
			Why:    "the call graph with its statistics; the statistics counted two nodes for twenty-seven edges until 2026-08-24",
			Action: "analyze", Needs: []string{"references"},
			Params: map[string]any{"type": "analyze_call_graph", "object_type": "CLAS",
				"object_name": "{references}", "direction": "callees"},
			Oracle:      oracleCrossOrLongName,
			MustContain: "edges",
		},
		{
			ID: "graph.compare_call_graphs", Capability: "analyze type=compare_call_graphs",
			Why:    "static prediction against a supplied trace; the trace is ours, so only the static half is under test",
			Action: "analyze", Needs: []string{"references"},
			Params: map[string]any{"type": "compare_call_graphs",
				"object_uri": "{references_uri}",
				// One synthetic edge, because the comparison needs something to
				// compare against and a real trace is not available to a sweep.
				// What is being checked is that the static side has edges at
				// all: for an object the tables have rows for, zero is a death.
				"trace_data": `[{"caller_name":"{references}","callee_name":"NOTHING_AT_ALL"}]`},
			Oracle:      oracleCrossOrLongName,
			MustContain: "static_edges",
		},
		{
			ID: "graph.trace_execution", Capability: "analyze type=trace_execution",
			Why:    "predicted against actual; with no trace recorded the static half must still answer, and the absence must be declared",
			Action: "analyze", Needs: []string{"references"},
			Params: map[string]any{"type": "trace_execution",
				"object_uri": "{references_uri}"},
			// Not "comparison": that is legitimately absent when nothing ran.
			// The static statistics are not, and this returned neither along
			// with no word that anything was missing until 2026-08-24.
			MustContain: "static_stats",
			Oracle:      oracleCrossOrLongName,
		},
		{
			ID: "graph.cr_boundaries", Capability: "analyze type=cr_boundaries",
			Why:    "change requests grouped across transports; needs the CR attribute this landscape uses",
			Action: "analyze", Requires: []string{"cr_attribute"},
			Params: map[string]any{"type": "cr_boundaries", "cr_id": "CR-EXAMPLE"},
			// A landscape with no CR attribute configured cannot be asked this,
			// and the handler says so clearly — which is the capability working,
			// not failing. The first version of this probe asserted the answer
			// would name the CR, was told about configuration instead, and
			// reported a working capability as broken. Requiring the target
			// turns that into a skip with a reason, which is what it is.
			EmptyIsFine: true,
		},
		{
			ID: "graph.tr_boundaries", Capability: "analyze type=tr_boundaries",
			Why:         "a transport holding nothing was once reported SELF-CONSISTENT",
			Action:      "analyze",
			Params:      map[string]any{"type": "tr_boundaries", "transports": "TR-EXAMPLE"},
			EmptyIsFine: true,
		},
	}
}

// postMortemProbes cover the dump and log surface. Its filters were decoration
// and its feed parser read fields that are empty on every row, so the probes
// ask for detail rather than for a count.
func postMortemProbes() []Probe {
	return []Probe{
		{
			ID: "pm.list_dumps", Capability: "analyze type=list_dumps",
			Why:    "ST22 over ADT; a quiet system genuinely has none",
			Action: "analyze", Params: map[string]any{"type": "list_dumps", "max_results": 5},
			EmptyIsFine: true,
		},
		{
			ID: "pm.group_dumps", Capability: "analyze type=group_dumps",
			Why:    "grouping by what keeps failing",
			Action: "analyze", Params: map[string]any{"type": "group_dumps"},
			EmptyIsFine: true,
		},
		{
			ID: "pm.application_log", Capability: "analyze type=application_log",
			Why:    "SLG1 read as an ordinary table, because BAL_DB_SEARCH is not remote-enabled",
			Action: "analyze", Params: map[string]any{"type": "application_log", "max_results": 5},
			EmptyIsFine: true,
		},
		{
			ID: "pm.list_traces", Capability: "analyze type=list_traces",
			Why:    "SAT traces recorded on this system",
			Action: "analyze", Params: map[string]any{"type": "list_traces"},
			EmptyIsFine: true,
		},
		{
			ID: "pm.get_dump", Capability: "analyze type=get_dump",
			Why:    "one runtime error in full; the id is resolved before the sweep so an empty feed reads as skipped",
			Action: "analyze", Needs: []string{"dump"},
			Params:      map[string]any{"type": "get_dump", "dump_id": "{dump}"},
			MustContain: "dump",
		},
		{
			ID: "pm.explain_dump", Capability: "analyze type=explain_dump",
			Why:    "the dump joined to what the application log said around it",
			Action: "analyze", Needs: []string{"dump"},
			Params: map[string]any{"type": "explain_dump", "dump_id": "{dump}"},
			// Log matches are genuinely often absent; the dump itself is not.
			MustContain: "dump",
		},
		{
			ID: "pm.similar_dumps", Capability: "analyze type=similar_dumps",
			Why:    "a dump is always similar to itself, so an empty answer here cannot be true",
			Action: "analyze", Needs: []string{"dump"},
			Params: map[string]any{"type": "similar_dumps", "dump_id": "{dump}"},
			Oracle: oracleAlwaysSome("the dump the question is about exists, and nothing is more like it than itself"),
		},
		{
			ID: "pm.dump_impact", Capability: "analyze type=dump_impact",
			Why:    "what the failing program reaches; the blast radius rung",
			Action: "analyze", Needs: []string{"dump"},
			Params:      map[string]any{"type": "dump_impact", "dump_id": "{dump}"},
			MustContain: "dump",
		},
		{
			ID: "pm.get_trace", Capability: "analyze type=get_trace",
			Why:    "one recorded trace read back; skipped when nothing was ever recorded",
			Action: "analyze", Needs: []string{"trace"},
			Params: map[string]any{"type": "get_trace", "trace_id": "{trace}"},
			// The id came from the system's own listing a moment earlier, so
			// there is nothing for an empty answer to be true about. Without
			// saying this the probe would be the third unfalsifiable one, and
			// the table's own test refuses to let that pass — correctly.
			Oracle: oracleAlwaysSome("the trace id was read from this system's trace listing, so that trace exists"),
		},
		{
			ID: "pm.list_sql_traces", Capability: "analyze type=list_sql_traces",
			// Unprobed until its sibling turned out to be dead. A quiet system
			// genuinely has no traces; what this catches is the 406 that made
			// the call fail whatever the system held.
			Why:    "ST05 records; a system nobody traced has none",
			Action: "analyze", Params: map[string]any{"type": "list_sql_traces", "max_results": 5},
			EmptyIsFine: true,
		},
		{
			ID: "pm.sql_trace_state", Capability: "analyze type=sql_trace_state",
			Why:    "ST05 state always has an answer, on or off",
			Action: "analyze", Params: map[string]any{"type": "sql_trace_state"},
			Oracle: oracleAlwaysSome("the SQL trace is either on or off, so there is always a state"),
		},
	}
}

// contextProbes cover the compression surface an agent depends on for reads.
func contextProbes() []Probe {
	return []Probe{
		{
			ID: "ctx.context", Capability: "analyze type=context",
			Why:    "dependency contracts appended to a read",
			Action: "analyze", Needs: []string{"class"},
			Params: map[string]any{"type": "context", "name": "{class}", "object_type": "CLAS"},
			Oracle: oracleAlwaysSome("a class that reads has a source, so it has a context"),
		},
		{
			ID: "ctx.parse_abap", Capability: "analyze type=parse_abap",
			Why:    "the offline parser; it needs no system and must never be empty",
			Action: "analyze",
			Params: map[string]any{"type": "parse_abap", "source": "REPORT zdemo.\nWRITE 'x'.\n"},
			Oracle: oracleAlwaysSome("two statements were handed to the parser"),
		},
		{
			ID: "ctx.analyze_deps", Capability: "analyze type=analyze_deps",
			Why:    "dependency extraction from source",
			Action: "analyze", Needs: []string{"class"},
			Params:      map[string]any{"type": "analyze_deps", "name": "{class}", "object_type": "CLAS"},
			EmptyIsFine: true,
		},
	}
}

// --- oracles --------------------------------------------------------------
//
// An oracle answers one question only: could an empty answer be true? It is
// never used as the answer itself. Two of these read the cross-reference
// tables directly, which is the second route the graph handlers are supposed
// to be a convenient front for — if the table has rows and the handler has
// none, the handler is the problem.

// oracleAlwaysSome is for capabilities where an empty answer cannot be true by
// construction, and the reason is worth stating rather than assuming.
func oracleAlwaysSome(why string) Oracle {
	return func(context.Context, *adt.Client, SweepTargets) (int, string, error) {
		return 1, why, nil
	}
}

// oracleWhereUsed asks the where-used list, which is a different resource from
// the graph handlers and answers on every release.
func oracleWhereUsed(ctx context.Context, c *adt.Client, t SweepTargets) (int, string, error) {
	uri := "/sap/bc/adt/oo/classes/" + strings.ToLower(t.Referenced)
	callers, err := c.WhereUsed(ctx, uri)
	if err != nil {
		return 0, "the where-used list", err
	}
	return len(callers), "the where-used list", nil
}

// oracleParserDeps is the strongest oracle in the table, because it does not
// share a source with what it checks.
//
// `callees` reads the cross-reference tables. This reads the object's source
// and parses it. Two different routes to the same fact, so agreement is
// evidence and disagreement names which side failed: if the parser finds
// dependencies in the text and the tables report none, the tables are dead —
// which is exactly the shape the callee defect had.
func oracleParserDeps(ctx context.Context, c *adt.Client, t SweepTargets) (int, string, error) {
	src, err := c.GetClassSource(ctx, t.References)
	if err != nil {
		return 0, "the parser over the object's own source", err
	}
	deps := ctxcomp.ExtractDependencies(src)
	return len(deps), "the parser over the object's own source", nil
}

// oracleCrossOrLongName counts references to an object by name, and then by
// hash.
//
// A name too long for CHAR(120) is not in WBCROSSGT under its own name at all:
// it is stored as a SHA-1, with the readable form in WBCROSSGTX. An oracle that
// looked only at the first table would report zero for exactly the objects the
// long-name defect was about, and a dead capability would pass on the strength
// of it.
func oracleCrossOrLongName(ctx context.Context, c *adt.Client, t SweepTargets) (int, string, error) {
	const how = "WBCROSSGT, and WBCROSSGTX for long names"
	n, _, err := countRows(ctx, c, "WBCROSSGT",
		fmt.Sprintf("SELECT * FROM WBCROSSGT WHERE NAME LIKE '%s%%'", sqlLiteral(t.Referenced)), how)
	if err != nil {
		return 0, how, err
	}
	if n > 0 {
		return n, how, nil
	}
	long, _, err := countRows(ctx, c, "WBCROSSGTX",
		fmt.Sprintf("SELECT * FROM WBCROSSGTX WHERE LONG_NAME LIKE '%s%%'", sqlLiteral(t.Referenced)), how)
	if err != nil {
		// The first table answered and found nothing; say so rather than
		// turning a failed second lookup into a claim about the object.
		return 0, how, err
	}
	return long, how, nil
}

// oraclePackageHasReadableSource confirms the package holds at least one object
// whose source can be read.
//
// Without it, `check_boundaries` answering "Total dependencies: 0" is
// indistinguishable from a clean package — which is the defect it had: it
// reported CLEAN without opening a single file.
func oraclePackageHasReadableSource(ctx context.Context, c *adt.Client, t SweepTargets) (int, string, error) {
	const how = "the package's own object list"
	n, _, err := countRows(ctx, c, "TADIR",
		fmt.Sprintf("SELECT * FROM TADIR WHERE DEVCLASS = '%s'", sqlLiteral(t.Package)), how)
	return n, how, err
}

// oracleSourceHasMethods reads the class's own source and counts method
// definitions. A class whose source declares methods cannot honestly answer
// with an empty structure.
func oracleSourceHasMethods(ctx context.Context, c *adt.Client, t SweepTargets) (int, string, error) {
	const how = "METHODS declarations in the class's own source"
	src, err := c.GetClassSource(ctx, t.Class)
	if err != nil {
		return 0, how, err
	}
	n := 0
	for _, line := range strings.Split(src, "\n") {
		f := strings.Fields(strings.ToUpper(strings.TrimSpace(line)))
		if len(f) > 0 && (f[0] == "METHODS" || f[0] == "CLASS-METHODS") {
			n++
		}
	}
	return n, how, nil
}

// oracleTableHasRows confirms the probe table is not itself empty, so that an
// empty query result accuses the query path rather than the table.
func oracleTableHasRows(ctx context.Context, c *adt.Client, t SweepTargets) (int, string, error) {
	return countRows(ctx, c, t.Table, "", t.Table)
}

func countRows(ctx context.Context, c *adt.Client, table, sql, name string) (int, string, error) {
	res, err := c.GetTableContents(ctx, table, 10, sql)
	if err != nil {
		return 0, name, err
	}
	if res == nil {
		return 0, name, nil
	}
	return len(res.Rows), name, nil
}

// sqlLiteral makes a value safe to place inside the single quotes of a
// freestyle SELECT. ADT rejects most of what could be smuggled through, but
// the sweep builds these strings from names it read off a live system and a
// quote in one of them would produce a confusing 400 rather than a finding.
func sqlLiteral(s string) string {
	return strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(s)), "'", "''")
}
