package adt

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/datacluster"
)

// W3MI is a binary object in the MIME repository — what SMW0 shows and what
// abapGit serialises as a .w3mi.xml plus a .w3mi.data.* file beside it.
//
// The bytes do not live in a column. SMW0 writes them into WWWDATA as an
// INDX-style data cluster, LZH-compressed, split across as many rows as it
// takes, and inside that cluster they are a table of fixed 255-byte lines. The
// last line is padded with zeroes, so the cluster alone cannot say where the
// file ends: the true length is the `filesize` parameter in WWWPARAMS, and
// truncating to it is the only thing that turns 255-byte lines back into a file.
type W3MI struct {
	// Name is the OBJID in WWWDATA/WWWPARAMS, e.g. "ZORK-MINI.Z3".
	Name string
	// Data is the file, already joined and truncated to Size.
	Data []byte
	// Size is the filesize parameter — the authoritative length.
	Size int
	// MimeType, Extension and Version are the other WWWPARAMS entries.
	MimeType  string
	Extension string
	Version   string
	// Params is every WWWPARAMS row, so nothing read is silently dropped.
	//
	// NOTE: a `filename` parameter holds the path the file was uploaded FROM,
	// on whoever's workstation did the upload. It is a live identifier — a user
	// name, a host, a directory layout — and must not be written into a tracked
	// file. Callers that serialise this out are responsible for leaving it out.
	Params map[string]string
	// Padding is how many zero bytes the final 255-byte line carried past Size.
	// Kept because a non-zero byte in there would mean the length is wrong.
	Padding int
	// Lines is how many 255-byte rows the cluster held.
	Lines int
}

// w3miLineLength is the width of one row of the W3MIME line table. It is a
// property of the DDIC structure SMW0 writes, not of any one file.
const w3miLineLength = 255

// GetW3MI reads one MIME-repository object and reassembles the file.
//
// Two reads: WWWPARAMS for the metadata, WWWDATA for the cluster. Both are
// ordinary table reads, so this needs nothing installed on the system — no
// abapGit, no RFC, no ZADT_VSP.
func (c *Client) GetW3MI(ctx context.Context, objID string) (*W3MI, error) {
	objID = strings.ToUpper(strings.TrimSpace(objID))
	if objID == "" {
		return nil, fmt.Errorf("w3mi: object id is required")
	}
	if strings.ContainsAny(objID, "'\"") {
		return nil, fmt.Errorf("w3mi: object id %q contains a quote", objID)
	}

	out := &W3MI{Name: objID, Params: map[string]string{}}

	params, err := c.RunQuery(ctx,
		fmt.Sprintf("SELECT NAME, VALUE FROM WWWPARAMS WHERE RELID = 'MI' AND OBJID = '%s'", objID), 100)
	if err != nil {
		return nil, fmt.Errorf("w3mi: reading WWWPARAMS for %s: %w", objID, err)
	}
	for _, row := range params.Rows {
		name := strings.ToLower(strings.TrimSpace(fmt.Sprint(row["NAME"])))
		value := strings.TrimSpace(fmt.Sprint(row["VALUE"]))
		if name == "" {
			continue
		}
		out.Params[name] = value
		switch name {
		case "filesize":
			if n, convErr := strconv.Atoi(value); convErr == nil {
				out.Size = n
			}
		case "mimetype":
			out.MimeType = value
		case "fileextension":
			out.Extension = value
		case "version":
			out.Version = value
		}
	}
	if len(out.Params) == 0 {
		return nil, fmt.Errorf("w3mi: no WWWPARAMS rows for %s — no such MIME object", objID)
	}
	if out.Size <= 0 {
		return nil, fmt.Errorf("w3mi: %s has no usable filesize parameter (got %q); "+
			"without it the 255-byte padding cannot be told from the file", objID, out.Params["filesize"])
	}

	records, err := c.ReadClusterRecords(ctx, "WWWDATA",
		fmt.Sprintf("RELID = 'MI' AND OBJID = '%s'", objID), 0)
	if err != nil {
		return nil, fmt.Errorf("w3mi: reading the WWWDATA cluster for %s: %w", objID, err)
	}
	if records == nil || len(records.Records) == 0 {
		return nil, fmt.Errorf("w3mi: %s has WWWPARAMS but no WWWDATA rows", objID)
	}

	var buf []byte
	for i := range records.Records {
		cluster, parseErr := datacluster.Parse(records.Records[i].Blob)
		if parseErr != nil {
			return nil, fmt.Errorf("w3mi: parsing the %s cluster: %w", objID, parseErr)
		}
		for oi := range cluster.Objects {
			obj := &cluster.Objects[oi]
			for _, row := range obj.Rows {
				if len(row) == 0 {
					continue
				}
				line, lineErr := w3miRowBytes(row[0])
				if lineErr != nil {
					return nil, fmt.Errorf("w3mi: %s row %d: %w", objID, out.Lines, lineErr)
				}
				buf = append(buf, line...)
				out.Lines++
			}
		}
	}

	if len(buf) < out.Size {
		return nil, fmt.Errorf("w3mi: %s is short — WWWPARAMS says %d bytes, the cluster holds %d "+
			"across %d lines; the object is truncated on the system, not here",
			objID, out.Size, len(buf), out.Lines)
	}

	// Everything past filesize must be the zero padding of the final line. A
	// non-zero byte there means filesize and the cluster disagree, and guessing
	// which one is right would be worse than saying so.
	for i := out.Size; i < len(buf); i++ {
		if buf[i] != 0 {
			return nil, fmt.Errorf("w3mi: %s has a non-zero byte at offset %d, past the %d-byte filesize — "+
				"the padding is not padding, so filesize and the cluster disagree", objID, i, out.Size)
		}
	}
	out.Padding = len(buf) - out.Size
	out.Data = buf[:out.Size]
	return out, nil
}

// w3miRowBytes turns one parsed cluster row value into its bytes. A raw field
// comes back hex-encoded; a byte slice is taken as it is.
func w3miRowBytes(v any) ([]byte, error) {
	switch t := v.(type) {
	case []byte:
		return t, nil
	case string:
		b, err := hex.DecodeString(strings.TrimSpace(t))
		if err != nil {
			return nil, fmt.Errorf("value is neither bytes nor hex: %w", err)
		}
		return b, nil
	default:
		return nil, fmt.Errorf("unexpected row value of type %T", v)
	}
}

// ListW3MI lists MIME-repository objects, optionally filtered by an SQL LIKE
// pattern on the object id.
func (c *Client) ListW3MI(ctx context.Context, pattern string, maxRows int) ([]W3MI, error) {
	where := "RELID = 'MI'"
	if p := strings.TrimSpace(pattern); p != "" {
		if strings.ContainsAny(p, "'\"") {
			return nil, fmt.Errorf("w3mi: pattern %q contains a quote", p)
		}
		where += fmt.Sprintf(" AND OBJID LIKE '%s'", strings.ToUpper(p))
	}
	if maxRows <= 0 {
		maxRows = 1000
	}

	res, err := c.RunQuery(ctx,
		fmt.Sprintf("SELECT OBJID, NAME, VALUE FROM WWWPARAMS WHERE %s", where), maxRows)
	if err != nil {
		return nil, fmt.Errorf("w3mi: listing WWWPARAMS: %w", err)
	}

	byID := map[string]*W3MI{}
	var order []string
	for _, row := range res.Rows {
		id := strings.TrimSpace(fmt.Sprint(row["OBJID"]))
		if id == "" {
			continue
		}
		obj, seen := byID[id]
		if !seen {
			obj = &W3MI{Name: id, Params: map[string]string{}}
			byID[id] = obj
			order = append(order, id)
		}
		name := strings.ToLower(strings.TrimSpace(fmt.Sprint(row["NAME"])))
		value := strings.TrimSpace(fmt.Sprint(row["VALUE"]))
		obj.Params[name] = value
		switch name {
		case "filesize":
			if n, convErr := strconv.Atoi(value); convErr == nil {
				obj.Size = n
			}
		case "mimetype":
			obj.MimeType = value
		case "fileextension":
			obj.Extension = value
		case "version":
			obj.Version = value
		}
	}

	out := make([]W3MI, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}
