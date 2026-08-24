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
			Oracle: oracleWhereUsed,
		},
		{
			ID: "graph.callees", Capability: "analyze type=callees",
			Why:    "the down direction, read from the cross-reference tables",
			Action: "analyze", Needs: []string{"references"},
			Params: map[string]any{"type": "callees", "object_name": "{references}", "object_type": "CLAS"},
			Oracle: oracleCrossReferences,
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
			Oracle: oracleAlwaysSome("a class always has at least one component"),
		},
		{
			ID: "graph.impact", Capability: "analyze type=impact",
			Why:    "reverse dependencies of an object known to have them",
			Action: "analyze", Needs: []string{"referenced"},
			Params: map[string]any{"type": "impact", "object_name": "{referenced}", "object_type": "CLAS"},
			Oracle: oracleWhereUsed,
		},
		{
			ID: "graph.usage_examples", Capability: "analyze type=usage_examples",
			Why:    "asked CROSS for a two-letter code in a one-character column, and had never returned a row",
			Action: "analyze", Needs: []string{"referenced"},
			Params: map[string]any{"type": "usage_examples", "object_name": "{referenced}", "object_type": "CLAS"},
			Oracle: oracleCrossCallers,
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
			Params:      map[string]any{"type": "check_boundaries", "package": "{package}"},
			EmptyIsFine: true,
		},
		{
			ID: "graph.graph_stats", Capability: "analyze type=graph_stats",
			Why:    "graph counts over supplied source; two statements can never be zero nodes",
			Action: "analyze",
			Params: map[string]any{"type": "graph_stats", "source": "REPORT zdemo.\nPERFORM x.\n"},
			Oracle: oracleAlwaysSome("source with a PERFORM in it has at least one edge"),
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

// oracleCrossReferences counts what the object references, read from the
// cross-reference tables themselves.
func oracleCrossReferences(ctx context.Context, c *adt.Client, t SweepTargets) (int, string, error) {
	return countRows(ctx, c, "WBCROSSGT",
		fmt.Sprintf("SELECT * FROM WBCROSSGT WHERE INCLUDE LIKE '%s%%'", sqlLiteral(t.References)),
		"WBCROSSGT")
}

// oracleCrossCallers counts rows naming the object as a target, which is what
// usage_examples is supposed to turn into snippets.
func oracleCrossCallers(ctx context.Context, c *adt.Client, t SweepTargets) (int, string, error) {
	return countRows(ctx, c, "WBCROSSGT",
		fmt.Sprintf("SELECT * FROM WBCROSSGT WHERE NAME LIKE '%s%%'", sqlLiteral(t.Referenced)),
		"WBCROSSGT")
}

// oracleTableHasRows confirms the probe table is not itself empty, so that an
// empty query result accuses the query path rather than the table.
func oracleTableHasRows(ctx context.Context, c *adt.Client, t SweepTargets) (int, string, error) {
	return countRows(ctx, c, t.Table, "", t.Table)
}

// oraclePackageHasObjects confirms the package the sweep was pointed at holds
// something, so an empty graph is about the graph.
func oraclePackageHasObjects(ctx context.Context, c *adt.Client, t SweepTargets) (int, string, error) {
	return countRows(ctx, c, "TADIR",
		fmt.Sprintf("SELECT * FROM TADIR WHERE DEVCLASS = '%s'", sqlLiteral(t.Package)),
		"TADIR")
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
