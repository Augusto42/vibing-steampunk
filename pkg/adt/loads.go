package adt

import (
	"fmt"
	"strings"

	"context"
)

// D010INC over plain ADT free SQL.
//
// This is the compile-time load graph, and it is the one dependency source in
// this client that is not a cross-reference. CROSS and WBCROSSGT record what
// code *names*; D010INC records what has to be loaded for it to run. A program
// split across includes is one compiled unit and the split appears in no
// cross-reference table; a class pool loading another class's type pool is a
// dependency the runtime enforces whether or not a single statement names it.
//
// Two columns matter — MASTER and INCLUDE — and both arrive in the padded form
// the include normaliser already speaks.

// LoadRow is one row of the load table.
type LoadRow struct {
	Master            string
	Include           string
	ObsoleteInVersion int
}

// loadRowLimit caps a lookup. A master with more loads than this is a compiled
// unit nobody was going to read the list of, and the cap exists so one query
// against a kernel program cannot pull the whole table through data preview.
const loadRowLimit = 2000

// Loads reports what a compiled unit pulls in: the down direction.
//
// The name is matched as a prefix because a master arrives padded — a class is
// ZCL_X====CP, a function group SAPLZX — and a caller has the object's name,
// not its pool's. Prefix matching drags in siblings that share it, so the rows
// are filtered afterwards to those whose master really is this object, the same
// way the callee reader guards itself.
func (c *Client) Loads(ctx context.Context, objectName string) ([]LoadRow, []Unsearched, error) {
	name := strings.ToUpper(strings.TrimSpace(objectName))
	if err := checkSQLLiteral(name); err != nil {
		return nil, nil, err
	}

	// A function group's compiled unit is SAPL<group>, a class's is the padded
	// pool, a program's is itself. Asking for all three shapes in one query
	// costs nothing and saves the caller knowing which it has.
	res, err := c.RunQuery(ctx, fmt.Sprintf(
		"SELECT MASTER, INCLUDE, OBSOLETE_IN_VERSION FROM D010INC WHERE MASTER LIKE '%s%%' OR MASTER = 'SAPL%s'",
		name, name), loadRowLimit)
	if err != nil {
		return nil, []Unsearched{{Object: "D010INC", Reason: err.Error()}}, nil
	}
	if res == nil {
		return nil, nil, nil
	}
	return loadRowsFor(res.Rows, name), nil, nil
}

// LoadedBy reports which compiled units pull this one in: the up direction.
//
// It is the question the down direction cannot answer and the cross-reference
// tables cannot answer either. An include that nothing loads is dead in a way
// no where-used list will show, because nothing references it — it is included.
func (c *Client) LoadedBy(ctx context.Context, objectName string) ([]LoadRow, []Unsearched, error) {
	name := strings.ToUpper(strings.TrimSpace(objectName))
	if err := checkSQLLiteral(name); err != nil {
		return nil, nil, err
	}

	res, err := c.RunQuery(ctx, fmt.Sprintf(
		"SELECT MASTER, INCLUDE, OBSOLETE_IN_VERSION FROM D010INC WHERE INCLUDE LIKE '%s%%'", name),
		loadRowLimit)
	if err != nil {
		return nil, []Unsearched{{Object: "D010INC", Reason: err.Error()}}, nil
	}
	if res == nil {
		return nil, nil, nil
	}

	var out []LoadRow
	for _, row := range res.Rows {
		r := loadRowFrom(row)
		if includeBelongsToName(r.Include, name) {
			out = append(out, r)
		}
	}
	return out, nil, nil
}

func loadRowsFor(rows []map[string]interface{}, name string) []LoadRow {
	var out []LoadRow
	for _, row := range rows {
		r := loadRowFrom(row)
		if includeBelongsToName(r.Master, name) {
			out = append(out, r)
		}
	}
	return out
}

func loadRowFrom(row map[string]interface{}) LoadRow {
	r := LoadRow{
		Master:  strings.ToUpper(rowString(row, "MASTER")),
		Include: strings.ToUpper(rowString(row, "INCLUDE")),
	}
	fmt.Sscanf(rowString(row, "OBSOLETE_IN_VERSION"), "%d", &r.ObsoleteInVersion)
	return r
}

// includeBelongsToName reports whether a padded include or master really names
// this object rather than merely starting with the same letters.
//
// ZCL_ORDER and ZCL_ORDER_ITEM share a prefix, and a prefix query returns both.
// The padding is what separates them: a real pool is the name followed by '='
// signs, or by nothing, or — for a function group — by the L/SAPL shapes SAP
// generates. Anything else beginning with the name is a different object.
func includeBelongsToName(include, name string) bool {
	inc := strings.ToUpper(strings.TrimSpace(include))
	switch {
	case inc == name:
		return true
	case inc == "SAPL"+name:
		return true
	case strings.HasPrefix(inc, name+"="):
		return true
	case strings.HasPrefix(inc, "L"+name):
		// A function group's own includes: L<group>TOP, L<group>U01, L<group>$01.
		rest := inc[len("L"+name):]
		return rest != "" && !strings.HasPrefix(rest, "_")
	}
	return false
}
