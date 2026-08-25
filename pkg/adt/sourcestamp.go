package adt

// Knowing what changed, without asking about each object.
//
// A package scan costs one source fetch per object — 167 of them took 18.8
// seconds against a live 7.58 system, and the April plan measured 227 at about
// a minute. Nearly all of that is round trips, so the way to make it fast is not
// to fetch faster but to fetch less.
//
// That needs a validity signal, and the honest question is what invalidates a
// cached source. Measured, rather than assumed:
//
//   - ADT returns ETag and Last-Modified on a source read, so per-object
//     validation is possible. But a conditional request still costs a round
//     trip, and 167 round trips is the whole cost. 304 does not help.
//   - REPOSRC carries PROGNAME, UDAT and UTIME, and answers for a whole
//     package prefix in 0.4 seconds. **One** round trip tells you which
//     objects changed.
//
// So this reads stamps in bulk, and the caller fetches only what moved.
//
// The rule that keeps it safe is the one this codebase has been learning all
// week: **a missing stamp is not an unchanged stamp.** An object whose stamp
// could not be read is reported as absent, and a caller that treats absence as
// "unchanged" serves stale source and analyses code that is not there. Every
// path here is built so that the cheap wrong answer is unavailable.
//
// # What the stamp is, and why it is sound
//
// Settled by a controlled experiment rather than by inspecting standard
// objects, whose history nobody can reconstruct: a class was created, its
// method body changed twice and its public section changed once, on a live
// 7.58 system. Four changes, four different source hashes, and at every step
//
//	ETag(source/main) == max(REPOSRC over this object's includes, excluding CS)
//
// exactly. Note that CU did not move on a method-body change and CM001 did, so
// no single include is the signal; the maximum is.
//
// **CS has to be excluded.** It is regenerated, it moves without the source
// changing — several standard classes carry today's timestamp on it — and
// including it drops agreement across ten standard classes from eight to zero.
//
// Of those ten, eight match the ETag exactly and two are **later** than it. That
// direction is the one that matters: a stamp never older than what ADT serves
// can only invalidate too often, never too rarely. Over-invalidation costs a
// fetch; under-invalidation serves code that is not there, under a verdict
// somebody acts on.
//
// The earlier conclusion here was that no sound signal existed. It was wrong,
// and it was wrong for an avoidable reason: the probe that produced it read 40
// rows of a class with 63 includes, and the include that carried the answer was
// past the cut. A rule inferred from four unexplained examples, when a fifth
// would have explained them.

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SourceStamp is when an object's source last changed, as the repository
// records it.
type SourceStamp struct {
	// Object is the repository name asked about, not the include it was found
	// through — a class is one object however many includes carry its source.
	Object string `json:"object"`
	Type   string `json:"type"`
	// At is the latest change across everything that makes up this object. A
	// class whose test include moved has changed, because its source read
	// returns different bytes.
	At time.Time `json:"at"`
	// Includes is how many source units backed the answer, which is the number
	// a reader needs to judge whether the answer is plausible.
	Includes int `json:"includes"`
}

// includePattern returns the SQL LIKE pattern that selects exactly this
// object's own source units, and nothing belonging to a neighbour.
//
// This is the third time this week a prefix has had to be defended, so it is
// stated plainly: a class's includes are the name **padded with '='** to thirty
// characters and then a section suffix. `LIKE 'ZCL_ORDER%'` therefore also
// matches every include of ZCL_ORDER_ITEM, and attributing one object's changes
// to another is invention, not imprecision. The padding character is the only
// thing that separates them, so the pattern includes it.
func includePattern(objType, name string) (string, bool) {
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "" {
		return "", false
	}
	switch strings.ToUpper(objType) {
	case "CLAS", "INTF":
		// The '=' is load-bearing. Without it the pattern swallows siblings.
		return name + "=%", true
	case "PROG":
		// A program is its own source unit; no pattern, an exact name.
		return name, true
	case "FUGR":
		// A function group's units are L<name>U01, L<name>TOP and so on.
		return "L" + name + "%", true
	}
	return "", false
}

// StampRef names one object to stamp. It is deliberately not PackageObject:
// this needs a type and a name and nothing else, and a narrow input is what
// lets a caller with a different object list use it without converting.
type StampRef struct {
	Type string
	Name string
}

// SourceStamps reads the last-change time of each named object in as few
// requests as it can.
//
// Objects it could not stamp are returned in `unstamped` rather than omitted,
// because a caller comparing against a cache must be able to tell "unchanged"
// from "unknown", and the two look identical in a map that simply lacks a key.
func (c *Client) SourceStamps(ctx context.Context, objects []StampRef) (map[string]SourceStamp, []Unsearched, error) {
	stamps := make(map[string]SourceStamp, len(objects))
	var unstamped []Unsearched

	// Group by the pattern shape so one query serves many objects. Programs are
	// exact names and go in an IN list; the padded kinds each need their own
	// LIKE, which SAP's freestyle parser will not accept more than one of per
	// WHERE — the OR-LIKE limit that this project has now learned twice.
	var programs []string
	var patterned []StampRef
	for _, obj := range objects {
		pattern, ok := includePattern(obj.Type, obj.Name)
		if !ok {
			unstamped = append(unstamped, Unsearched{
				Object: obj.Type + " " + obj.Name,
				Reason: "no source unit naming rule is known for " + obj.Type + ", so its change time cannot be read in bulk",
			})
			continue
		}
		if strings.ContainsAny(pattern, "%") {
			patterned = append(patterned, obj)
		} else {
			programs = append(programs, pattern)
		}
	}

	if len(programs) > 0 {
		rows, err := c.stampRows(ctx, "PROGNAME IN ("+quoteList(programs)+")")
		if err != nil {
			for _, p := range programs {
				unstamped = append(unstamped, Unsearched{Object: "PROG " + p, Reason: err.Error()})
			}
		} else {
			for name, at := range rows {
				stamps[stampKey("PROG", name)] = SourceStamp{Object: name, Type: "PROG", At: at, Includes: 1}
			}
		}
	}

	for _, obj := range patterned {
		pattern, _ := includePattern(obj.Type, obj.Name)
		rows, err := c.stampRows(ctx, fmt.Sprintf("PROGNAME LIKE '%s'", sqlLike(pattern)))
		if err != nil {
			unstamped = append(unstamped, Unsearched{Object: obj.Type + " " + obj.Name, Reason: err.Error()})
			continue
		}
		if len(rows) == 0 {
			unstamped = append(unstamped, Unsearched{
				Object: obj.Type + " " + obj.Name,
				Reason: "the repository holds no source unit for it, so there is no change time to compare against",
			})
			continue
		}
		var latest time.Time
		counted := 0
		for include, at := range rows {
			if isRegeneratedSection(obj.Name, include) {
				continue
			}
			counted++
			if at.After(latest) {
				latest = at
			}
		}
		if counted == 0 {
			unstamped = append(unstamped, Unsearched{
				Object: obj.Type + " " + obj.Name,
				Reason: "every source unit found for it is a regenerated section, which moves without the source changing",
			})
			continue
		}
		stamps[stampKey(obj.Type, obj.Name)] = SourceStamp{
			Object: obj.Name, Type: strings.ToUpper(obj.Type), At: latest, Includes: counted,
		}
	}

	// Anything asked about and not answered is named, never dropped.
	for _, obj := range objects {
		if _, ok := stamps[stampKey(obj.Type, obj.Name)]; ok {
			continue
		}
		if namedIn(unstamped, obj.Type+" "+obj.Name) {
			continue
		}
		unstamped = append(unstamped, Unsearched{
			Object: obj.Type + " " + obj.Name,
			Reason: "the repository returned no change time for it",
		})
	}
	return stamps, unstamped, nil
}

// stampRows reads PROGNAME, UDAT and UTIME for one WHERE clause.
func (c *Client) stampRows(ctx context.Context, where string) (map[string]time.Time, error) {
	res, err := c.GetTableContents(ctx, "REPOSRC", 5000,
		"SELECT PROGNAME, UDAT, UTIME FROM REPOSRC WHERE "+where)
	if err != nil {
		return nil, err
	}
	// Keyed by the raw PROGNAME, deliberately. The exact-name branch asks about
	// programs, where PROGNAME *is* the object name, so the key maps straight
	// across. The pattern branch asks one object at a time and takes the latest
	// of whatever comes back, so it never looks at the key at all.
	//
	// Deriving the object name from an include name here would be a fourth
	// place in this codebase that has to know how a class is padded, and the
	// first version of this function got it wrong in the usual way: it trimmed
	// '=' from the right, where an include name ends in its section rather than
	// in padding, so the trim did nothing and every key was the include.
	out := make(map[string]time.Time, len(res.Rows))
	for _, row := range res.Rows {
		name := strings.TrimSpace(rowString(row, "PROGNAME"))
		at := parseRepoTime(rowString(row, "UDAT"), rowString(row, "UTIME"))
		if name == "" || at.IsZero() {
			continue
		}
		if prev, ok := out[name]; !ok || at.After(prev) {
			out[name] = at
		}
	}
	return out, nil
}

// StampKey is the map key SourceStamps uses, exported so a caller can look one
// up without reconstructing the convention.
func StampKey(objType, name string) string { return stampKey(objType, name) }

func stampKey(objType, name string) string {
	return strings.ToUpper(strings.TrimSpace(objType)) + " " + strings.ToUpper(strings.TrimSpace(name))
}

// parseRepoTime turns REPOSRC's separate date and time columns into one moment.
//
// UDAT is YYYYMMDD and UTIME is HHMMSS, both in the system's own timezone. They
// are compared only against each other — a cached stamp against a fresh one —
// so the zone never enters the comparison, and pretending to know it would be
// the invention.
func parseRepoTime(udat, utime string) time.Time {
	udat = strings.TrimSpace(udat)
	if len(udat) != 8 {
		return time.Time{}
	}
	utime = strings.TrimSpace(utime)
	for len(utime) < 6 {
		utime = "0" + utime
	}
	if len(utime) > 6 {
		utime = utime[:6]
	}
	at, err := time.Parse("20060102150405", udat+utime)
	if err != nil {
		return time.Time{}
	}
	return at
}

func quoteList(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, "'"+sqlLike(n)+"'")
	}
	return strings.Join(quoted, ",")
}

// sqlLike makes a repository name safe inside the quotes of a freestyle SELECT.
func sqlLike(s string) string {
	return strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(s)), "'", "''")
}

func namedIn(list []Unsearched, object string) bool {
	for _, u := range list {
		if u.Object == object {
			return true
		}
	}
	return false
}

// isRegeneratedSection reports whether an include is one the system rewrites on
// its own, so its timestamp says nothing about the source.
//
// CS is the one that matters and it matters a lot: on a live system several
// standard classes carry today's date on it, hours after nobody edited them.
// Including it in the maximum makes every object look changed on every run,
// which is a cache that never hits and a scan that is slower than no cache at
// all.
func isRegeneratedSection(object, include string) bool {
	object = strings.ToUpper(strings.TrimSpace(object))
	include = strings.ToUpper(strings.TrimSpace(include))
	if !strings.HasPrefix(include, object) {
		return false
	}
	section := strings.TrimLeft(include[len(object):], "=")
	return section == "CS"
}
