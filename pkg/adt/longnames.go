package adt

import (
	"context"
	"fmt"
	"strings"
)

// A reference whose full name does not fit WBCROSSGT's NAME column — CHAR(120)
// — is not truncated there. SAP stores a SHA-1 of the name instead, and keeps
// the name itself in WBCROSSGTX ("Index for Global Types - Management of Long
// Names"), which pairs that hash with a LONG_NAME of type STRG.
//
// Read WBCROSSGT alone and the hash arrives looking exactly like a name: forty
// hex characters, no backslash, so nothing in the callee filters removes it and
// it is reported as the thing an object references. That is worse than the
// silences fixed elsewhere in this package — a silence withholds an answer,
// this invents one. On a stock A4H there are at least five hundred such rows.
//
// SAP's own where-used reader does this lookup; CL_RIS_METHOD_DATA hashes the
// name before it searches, which is the same fact from the writing side.

// longNameHashLength is SHA-1 rendered as uppercase hex.
const longNameHashLength = 40

// looksLikeLongNameHash reports whether a NAME could be a stored hash rather
// than a name. It is only a filter for what is worth asking about: the lookup
// in WBCROSSGTX is what decides, because a name is only a hash if that table
// says so. Real ABAP object names cannot collide with this in practice —
// nothing addressable is forty characters of hex — but the check exists to keep
// the query small, not to be the authority.
func looksLikeLongNameHash(name string) bool {
	if len(name) != longNameHashLength || strings.Contains(name, "\\") {
		return false
	}
	for _, r := range name {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// longNameBatch caps one IN list. WBCROSSGTX is keyed by NAME, so a batch is a
// plain lookup and the limit is the length SAP accepts in a WHERE clause rather
// than anything about the data.
const longNameBatch = 20

// ResolveLongNames replaces hashed NAMEs in cross-reference rows with the names
// they stand for, in place. It returns what it could not resolve: a hash left
// in the output would be reported as an object name, so the caller has to know
// rather than discover it in a result somebody is reading.
func (c *Client) ResolveLongNames(ctx context.Context, rows []map[string]interface{}) []Unsearched {
	pending := map[string][]map[string]interface{}{}
	for _, row := range rows {
		name := rowString(row, "NAME")
		if looksLikeLongNameHash(name) {
			pending[strings.ToUpper(name)] = append(pending[strings.ToUpper(name)], row)
		}
	}
	if len(pending) == 0 {
		return nil
	}

	hashes := make([]string, 0, len(pending))
	for h := range pending {
		hashes = append(hashes, h)
	}

	var gaps []Unsearched
	for start := 0; start < len(hashes); start += longNameBatch {
		end := start + longNameBatch
		if end > len(hashes) {
			end = len(hashes)
		}
		batch := hashes[start:end]

		quoted := make([]string, 0, len(batch))
		for _, h := range batch {
			if err := checkSQLLiteral(h); err != nil {
				gaps = append(gaps, Unsearched{Object: h, Reason: err.Error()})
				continue
			}
			quoted = append(quoted, "'"+h+"'")
		}
		if len(quoted) == 0 {
			continue
		}

		res, err := c.RunQuery(ctx,
			fmt.Sprintf("SELECT NAME, LONG_NAME FROM WBCROSSGTX WHERE NAME IN (%s)", strings.Join(quoted, ",")),
			len(quoted)*2)
		if err != nil || res == nil {
			for _, h := range batch {
				gaps = append(gaps, Unsearched{Object: h, Reason: errOrEmpty(err,
					"WBCROSSGTX returned nothing for this hash")})
			}
			continue
		}

		found := map[string]bool{}
		for _, row := range res.Rows {
			hash := strings.ToUpper(rowString(row, "NAME"))
			long := rowString(row, "LONG_NAME")
			if hash == "" || long == "" {
				continue
			}
			found[hash] = true
			for _, target := range pending[hash] {
				target["NAME"] = long
			}
		}
		for _, h := range batch {
			if !found[h] {
				// Not in WBCROSSGTX, so it was never a hash: a name that only
				// looked like one. Left exactly as it was found.
				delete(pending, h)
			}
		}
	}
	return gaps
}

func errOrEmpty(err error, fallback string) string {
	if err != nil {
		return err.Error()
	}
	return fallback
}
