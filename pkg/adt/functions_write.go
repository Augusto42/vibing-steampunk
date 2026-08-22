package adt

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// Function modules are addressed by two names, not one: a module lives inside a
// group, and its ADT resource is nested under that group's URL. Everywhere else
// in vsp a caller names an object and gets on with it, and the difference has
// been pushed onto them — read and edit both refused a function module until the
// group was supplied, which is a thing the caller often does not know and never
// chose. These helpers resolve the group instead of demanding it, and give the
// module the same one-call edit that programs and classes already have.

// functionModuleURL builds the ADT resource URL for a module inside a group.
func functionModuleURL(groupName, functionName string) string {
	return fmt.Sprintf("/sap/bc/adt/functions/groups/%s/fmodules/%s",
		url.PathEscape(strings.ToUpper(groupName)),
		url.PathEscape(strings.ToUpper(functionName)))
}

// groupFromFunctionURI extracts the group from a module's ADT URI.
func groupFromFunctionURI(uri string) string {
	const marker = "/functions/groups/"
	i := strings.Index(uri, marker)
	if i < 0 {
		return ""
	}
	rest := uri[i+len(marker):]
	slash := strings.Index(rest, "/")
	if slash <= 0 {
		return ""
	}
	return strings.ToUpper(rest[:slash])
}

// ResolveFunctionGroup finds the group that owns a function module.
//
// The repository search already returns each hit's ADT URI, and a module's URI
// spells out its group — so the answer is one search away and needs no table
// read and no extra privilege.
func (c *Client) ResolveFunctionGroup(ctx context.Context, functionName string) (string, error) {
	functionName = strings.ToUpper(functionName)
	results, err := c.SearchObject(ctx, functionName, 20)
	if err != nil {
		return "", fmt.Errorf("resolving the group of function module %s: %w", functionName, err)
	}
	for _, r := range results {
		if !strings.EqualFold(r.Name, functionName) {
			continue
		}
		// FUGR/FF is a function module; the group itself is FUGR/F.
		if !strings.EqualFold(r.Type, "FUGR/FF") {
			continue
		}
		if group := groupFromFunctionURI(r.URI); group != "" {
			return group, nil
		}
	}
	return "", fmt.Errorf("cannot find function module %s — give its group explicitly if it exists", functionName)
}

// functionGroupFor returns the group to use: the one the caller supplied, or the
// one the system knows.
func (c *Client) functionGroupFor(ctx context.Context, groupName, functionName string) (string, error) {
	if groupName != "" {
		return strings.ToUpper(groupName), nil
	}
	return c.ResolveFunctionGroup(ctx, functionName)
}

// WriteFunctionModuleResult reports what happened to a function module.
type WriteFunctionModuleResult struct {
	Success      bool                `json:"success"`
	GroupName    string              `json:"groupName"`
	FunctionName string              `json:"functionName"`
	ObjectURL    string              `json:"objectUrl"`
	SyntaxErrors []SyntaxCheckResult `json:"syntaxErrors,omitempty"`
	Activation   *ActivationResult   `json:"activation,omitempty"`
	Message      string              `json:"message,omitempty"`
}

// WriteFunctionModule updates a function module's source in one call, running
// the same sequence a person would: check the syntax, lock, write, unlock,
// activate.
//
// groupName may be empty, in which case the group is resolved from the module
// name. The unlock is not left to the happy path — an object still locked after
// a failed write is the sort of thing that outlives the session and blocks the
// next person, so it is released whatever happens.
func (c *Client) WriteFunctionModule(ctx context.Context, groupName, functionName, source, transport string) (*WriteFunctionModuleResult, error) {
	functionName = strings.ToUpper(functionName)

	group, err := c.functionGroupFor(ctx, groupName, functionName)
	if err != nil {
		return nil, err
	}
	objectURL := functionModuleURL(group, functionName)

	if err := c.checkMutation(ctx, MutationContext{
		Op:        OpWorkflow,
		OpName:    "WriteFunctionModule",
		ObjectURL: objectURL,
		Transport: transport,
	}); err != nil {
		return nil, err
	}

	result := &WriteFunctionModuleResult{
		GroupName:    group,
		FunctionName: functionName,
		ObjectURL:    objectURL,
	}

	// Step 1: syntax check before anything is locked or written.
	syntaxErrors, err := c.SyntaxCheck(ctx, objectURL, source)
	if err != nil {
		result.Message = fmt.Sprintf("Syntax check failed: %v", err)
		return result, nil
	}
	for _, se := range syntaxErrors {
		if se.Severity == "E" || se.Severity == "A" || se.Severity == "X" {
			result.SyntaxErrors = syntaxErrors
			result.Message = "Source has syntax errors - not saved"
			return result, nil
		}
	}
	result.SyntaxErrors = syntaxErrors

	// Step 2: lock.
	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock function module: %v", err)
		return result, nil
	}
	unlocked := false
	defer func() {
		if !unlocked {
			c.UnlockObject(ctx, objectURL, lock.LockHandle)
		}
	}()

	// Step 3: write.
	if err := c.UpdateSource(ctx, objectURL+"/source/main", source, lock.LockHandle, transport); err != nil {
		result.Message = fmt.Sprintf("Failed to update source: %v", err)
		return result, nil
	}

	// Step 4: unlock before activating — SAP will not activate a locked object.
	if err := c.UnlockObject(ctx, objectURL, lock.LockHandle); err != nil {
		result.Message = fmt.Sprintf("Failed to unlock function module: %v", err)
		return result, nil
	}
	unlocked = true

	// Step 5: activate.
	activation, err := c.Activate(ctx, objectURL, functionName)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		result.Activation = activation
		return result, nil
	}
	result.Activation = activation
	if activation.Success {
		result.Success = true
		result.Message = "Function module updated and activated successfully"
	} else {
		result.Message = "Activation failed - check activation messages"
	}
	return result, nil
}

// writeSourceFunctionModule adapts WriteFunctionModule to the WriteSource shape,
// so a caller editing a function module uses the same call as for a class.
//
// Creation is deliberately not folded in here. A new module needs an interface —
// parameters, their types, whether it is remote-enabled — and none of that can
// be inferred from a source string alone; the create action takes those and is
// the right door.
func (c *Client) writeSourceFunctionModule(ctx context.Context, name, source string, opts *WriteSourceOptions) (*WriteSourceResult, error) {
	result := &WriteSourceResult{
		ObjectType: "FUNC",
		ObjectName: strings.ToUpper(name),
		Mode:       "updated",
	}

	if opts.Mode == WriteModeCreate {
		result.Message = "Creating a function module needs its interface — use the create action with object_type FUGR/FF"
		return result, nil
	}

	written, err := c.WriteFunctionModule(ctx, opts.Parent, name, source, opts.Transport)
	if err != nil {
		return nil, err
	}

	result.ObjectURL = written.ObjectURL
	result.SyntaxErrors = written.SyntaxErrors
	result.Activation = written.Activation
	result.Success = written.Success
	result.Message = written.Message
	return result, nil
}
