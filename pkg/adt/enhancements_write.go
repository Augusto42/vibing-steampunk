package adt

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type rfcEnhancementWriter interface {
	WriteEnhancementSource(ctx context.Context, enhancement, source, transport string) error
}

// Source-only creation is unsafe: host, anchor and subtype cannot be inferred.
// ENHO upsert is intentionally rejected rather than guessing create/update.
func (c *Client) writeSourceEnhancement(ctx context.Context, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	result := &WriteSourceResult{ObjectType: "ENHO", ObjectName: name}
	if opts.Mode != WriteModeUpdate {
		result.Message = "ENHO source writes require mode=update; use CreateEnhancement with explicit host metadata for creation"
		return result, nil
	}
	if opts.ExpectedSourceHash != "" {
		result.Message = "expected_source_hash is not supported for ENHO updates"
		return result, nil
	}
	ref, err := c.resolveEnhancement(ctx, name)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(ref.PackageName) == "" {
		result.Message = "ENHO package could not be verified; no write was attempted"
		return result, nil
	}
	if err := c.checkMutation(ctx, MutationContext{
		Op: OpWorkflow, OpName: "WriteSource", ObjectURL: ref.URI,
		Package: ref.PackageName, Transport: opts.Transport,
	}); err != nil {
		return nil, err
	}
	if err := c.checkPackageTransportRequirement(ctx, ref.PackageName, opts.Transport, "WriteSource"); err != nil {
		return nil, err
	}
	return c.writeEnhancementUpdateByRef(ctx, ref, source, opts.Transport)
}

// writeEnhancementUpdate updates an existing classic source-code enhancement
// implementation. Classic NetWeaver releases expose the ENHO metadata through
// ADT but store the editable body in a generated PROG/I include. Writing that
// include through the ZADT_VSP bridge lets SAP's Enhancement Framework own
// the lock, save, activation, and transport semantics. Success is reported
// only after the active generated include matches the requested source.
//
// Creating an ENHO is deliberately not handled here: source text alone does
// not contain the host object, anchor, enhancement spot, or subtype metadata.
func (c *Client) writeEnhancementUpdate(ctx context.Context, name, source, transport string) (*WriteSourceResult, error) {
	ref, err := c.resolveEnhancement(ctx, name)
	if err != nil {
		return nil, err
	}
	return c.writeEnhancementUpdateByRef(ctx, ref, source, transport)
}

func (c *Client) writeEnhancementUpdateByRef(ctx context.Context, ref *EnhancementRef, source, transport string) (*WriteSourceResult, error) {
	result := &WriteSourceResult{
		ObjectType: "ENHO",
		ObjectName: strings.ToUpper(ref.Name),
		ObjectURL:  ref.URI,
		Mode:       "updated",
	}

	if ref.Kind != EnhancementKind("XH") {
		result.Message = fmt.Sprintf("ENHO/%s update is not supported yet; only source-code plug-in enhancements (ENHO/XH) have a source include", ref.Kind)
		return result, nil
	}
	c.enrichEnhancementRefFromIndex(ctx, ref)
	if strings.TrimSpace(ref.EnhInclude) == "" {
		result.Message = "Could not resolve the classic ENHO source include from ENHINCINX; no write was attempted"
		return result, nil
	}
	if strings.TrimSpace(transport) != "" {
		ownerTask, ownerRequest, ownerErr := c.getEnhancementTransportOwner(ctx, ref.Name)
		if ownerErr != nil {
			result.Message = fmt.Sprintf("Could not verify the ENHO transport owner before mutation: %v", ownerErr)
			return result, nil
		}
		if !transportMatchesCTSOwner(transport, ownerTask, ownerRequest) {
			result.Message = fmt.Sprintf(
				"Transport mismatch before enhancement mutation: caller requested %s but CTS owns the ENHO in request %s (task %s)",
				strings.ToUpper(strings.TrimSpace(transport)), ownerRequest, ownerTask)
			return result, nil
		}
	}

	factory := c.rfcFetcherFactory
	if factory == nil {
		factory = c.defaultRFCSourceFetcher
	}
	fetcher, err := factory(ctx)
	if err != nil {
		result.Message = fmt.Sprintf("Updating classic ENHO source requires the ZADT_VSP bridge: %v", err)
		return result, nil
	}
	defer fetcher.Close()

	readBackLines, readErr := fetcher.ReadSource(ctx, ref.EnhInclude)
	if readErr == nil && normalizeEnhancementSourceForCompare(strings.Join(readBackLines, "\n")) == normalizeEnhancementSourceForCompare(source) {
		result.Success = true
		result.Activation = &ActivationResult{Success: true}
		result.Message = "Enhancement active source already matches; no write was needed"
		return result, nil
	}

	writer, ok := fetcher.(rfcEnhancementWriter)
	if !ok {
		result.Message = "The installed ZADT_VSP bridge does not support ENHO writes; update ZCL_VSP_RFC_SERVICE and retry"
		return result, nil
	}
	if err := writer.WriteEnhancementSource(ctx, ref.Name, source, transport); err != nil {
		result.Message = fmt.Sprintf("Failed to update enhancement through ZADT_VSP: %v", err)
		return result, nil
	}

	deadline := time.Now().Add(45 * time.Second)
	for {
		readBackLines, readErr := fetcher.ReadSource(ctx, ref.EnhInclude)
		if readErr == nil {
			readBack := strings.Join(readBackLines, "\n")
			if normalizeEnhancementSourceForCompare(readBack) == normalizeEnhancementSourceForCompare(source) {
				break
			}
		}
		if time.Now().After(deadline) {
			if readErr != nil {
				result.Message = fmt.Sprintf("Enhancement worker was scheduled but active read-back failed: %v", readErr)
			} else {
				result.Message = "Enhancement worker was scheduled but active read-back did not match within 45s; update is not reported as successful"
			}
			return result, nil
		}
		select {
		case <-ctx.Done():
			result.Message = fmt.Sprintf("Enhancement read-back was canceled: %v", ctx.Err())
			return result, nil
		case <-time.After(500 * time.Millisecond):
		}
	}

	result.Success = true
	result.Activation = &ActivationResult{Success: true}
	result.Message = "Enhancement updated and activated successfully"
	return result, nil
}

func (c *Client) getEnhancementTransportOwner(ctx context.Context, name string) (task, request string, err error) {
	name = strings.ToUpper(strings.TrimSpace(name))
	e071SQL := fmt.Sprintf(
		"SELECT TRKORR FROM E071 WHERE PGMID = 'R3TR' AND OBJECT = 'ENHO' AND OBJ_NAME = '%s' AND LOCKFLAG = 'X'",
		strings.ReplaceAll(name, "'", "''"))
	e071, err := c.GetTableContents(ctx, "E071", 10, e071SQL)
	if err != nil {
		return "", "", err
	}
	for _, row := range e071.Rows {
		candidate := strings.ToUpper(strings.TrimSpace(asString(row["TRKORR"])))
		if candidate == "" {
			continue
		}
		if task != "" && task != candidate {
			return "", "", fmt.Errorf("multiple active CTS owners found for ENHO %s", name)
		}
		task = candidate
	}
	if task == "" {
		return "", "", nil
	}

	e070SQL := fmt.Sprintf("SELECT TRKORR, STRKORR FROM E070 WHERE TRKORR = '%s'", strings.ReplaceAll(task, "'", "''"))
	e070, err := c.GetTableContents(ctx, "E070", 2, e070SQL)
	if err != nil {
		return "", "", err
	}
	request = task
	if len(e070.Rows) > 0 {
		if parent := strings.ToUpper(strings.TrimSpace(asString(e070.Rows[0]["STRKORR"]))); parent != "" {
			request = parent
		}
	}
	return task, request, nil
}

func transportMatchesCTSOwner(requested, task, request string) bool {
	requested = strings.ToUpper(strings.TrimSpace(requested))
	task = strings.ToUpper(strings.TrimSpace(task))
	request = strings.ToUpper(strings.TrimSpace(request))
	return task == "" || requested == task || requested == request
}

func normalizeEnhancementSourceForCompare(source string) string {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")
	lines := strings.Split(source, "\n")
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(strings.ToUpper(line), "ENHANCEMENT ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				line = "ENHANCEMENT " + strings.TrimSuffix(fields[1], ".") + "."
			}
		}
		normalized = append(normalized, line)
	}
	return strings.Join(normalized, "\n")
}

// enrichEnhancementRefFromIndex fills the classic repository fields that ADT
// search omits for HOOK_IMPL enhancements. It is deliberately best-effort:
// modern systems and restricted users still fall through to the existing
// convention-based source lookup when ENHINCINX cannot be read.
func (c *Client) enrichEnhancementRefFromIndex(ctx context.Context, ref *EnhancementRef) {
	if ref == nil || ref.Kind != EnhancementKind("XH") || ref.EnhInclude != "" {
		return
	}

	name := strings.ToUpper(strings.TrimSpace(ref.Name))
	if name == "" {
		return
	}
	// ENHINCINX.ENHNAME is limited to 30 characters even when ENHHEADER keeps
	// the full implementation name.
	indexName := name
	if len(indexName) > 30 {
		indexName = indexName[:30]
	}
	sql := fmt.Sprintf(
		"SELECT ENHNAME, PROGRAMNAME, FULL_NAME, ENHINCLUDE FROM ENHINCINX WHERE ENHNAME = '%s'",
		strings.ReplaceAll(indexName, "'", "''"))

	result, err := c.GetTableContents(ctx, "ENHINCINX", 20, sql)
	if err != nil || result == nil {
		return
	}
	for _, row := range result.Rows {
		enhInclude := strings.TrimSpace(asString(row["ENHINCLUDE"]))
		if enhInclude == "" {
			continue
		}
		ref.EnhInclude = enhInclude
		ref.HostProgram = strings.TrimSpace(asString(row["PROGRAMNAME"]))
		ref.FullName = strings.TrimSpace(asString(row["FULL_NAME"]))
		return
	}
}
