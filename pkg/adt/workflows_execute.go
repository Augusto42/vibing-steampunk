package adt

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// --- Execute ABAP Code via Unit Test ---

// ExecuteABAPResult represents the result of executing ABAP code via unit test.
type ExecuteABAPResult struct {
	Success       bool            `json:"success"`
	ProgramName   string          `json:"programName"`
	Output        []string        `json:"output"`              // Values returned via assertion messages
	RawAlerts     []UnitTestAlert `json:"rawAlerts,omitempty"` // Full alert details for debugging
	ExecutionTime float64         `json:"executionTime"`       // Execution time in seconds
	Message       string          `json:"message,omitempty"`
	CleanedUp     bool            `json:"cleanedUp"`

	// Failure is what stopped the code before it finished, when something did.
	Failure *ExecuteFailure `json:"failure,omitempty"`
}

// ExecuteFailure is a run that ended in the middle of the caller's code.
//
// ABAP Unit catches whatever the payload raises and reports it as an alert, so
// this evidence was always in the response — it was simply thrown away, and a
// division by zero came back as "Executed successfully (no output captured)".
// Note that ABAP Unit catching it also means there is no dump in ST22 to find
// afterwards: for this class of failure the response is the only witness.
type ExecuteFailure struct {
	// Kind and Severity are ABAP Unit's own words: "exception" / "critical" for
	// a runtime error, "failedAssertion" for an assert the payload made itself.
	Kind     string `json:"kind"`
	Severity string `json:"severity,omitempty"`
	// Title is SAP's summary, and for a runtime error it names the error:
	// "Exception Error <COMPUTE_INT_ZERODIVIDE>".
	Title   string   `json:"title"`
	Details []string `json:"details,omitempty"`
	// Line is the line of the *caller's* code, counting from one. The wrapper
	// is a generated report and its line numbers are meaningless to whoever
	// wrote the payload, so they are translated back. Zero when the stack did
	// not place the failure in the wrapper at all.
	Line int `json:"line,omitempty"`
}

// execResultMarker prefixes the assertion message that carries a value back out
// of the executed code. Every run that gets that far ends in a deliberate
// cl_abap_unit_assert=>fail — that is the only way a value leaves a test method
// — so "the test failed" says nothing here, and this marker is the whole
// difference between the failure we asked for and one we did not.
const execResultMarker = "EXEC_RESULT:"

// payloadStartMarker is the line the generated wrapper puts immediately before
// the caller's code, and it is what makes a line number in the wrapper
// translatable back into a line number in what the caller actually wrote.
const payloadStartMarker = `" === USER CODE START ===`

// ExecuteABAPOptions configures ExecuteABAP behavior.
type ExecuteABAPOptions struct {
	// RiskLevel controls what operations the code can perform:
	// - "harmless" (default): No DB writes, no external calls
	// - "dangerous": Can write to DB, call external services
	// - "critical": Full system access (use with caution!)
	RiskLevel string

	// ReturnVariable is the name of the variable to return via assertion.
	// The code should set this variable, and its value will be returned.
	// If empty, uses "lv_result" by default.
	ReturnVariable string

	// KeepProgram prevents cleanup of the temp program (for debugging).
	KeepProgram bool

	// ProgramPrefix is the prefix for the temp program name.
	// Default is "ZTEMP_EXEC_".
	ProgramPrefix string
}

// ExecuteABAP executes arbitrary ABAP code via a temporary unit test wrapper.
//
// This is a powerful tool that allows executing any ABAP code on the SAP system.
// The code is wrapped in a test class and executed via RunUnitTests.
// Return values are extracted from assertion messages.
//
// Workflow:
// 1. Generate unique temp program name
// 2. Create program with test class wrapper
// 3. Inject user code into test method
// 4. Activate program
// 5. Run unit tests
// 6. Parse assertion messages for return values
// 7. Delete temp program (unless KeepProgram=true)
//
// Example:
//
//	result, err := client.ExecuteABAP(ctx, `
//	  DATA(lv_msg) = |Hello from SAP at { sy-datum } { sy-uzeit }|.
//	  DATA(lv_user) = sy-uname.
//	  lv_result = |{ lv_msg } by { lv_user }|.
//	`, nil)
//	// result.Output contains the assertion message with lv_result value
//
// Security: This is gated by OpWorkflow safety check.
func (c *Client) ExecuteABAP(ctx context.Context, code string, opts *ExecuteABAPOptions) (*ExecuteABAPResult, error) {
	// Safety check for workflow operations
	if err := c.checkSafety(OpWorkflow, "ExecuteABAP"); err != nil {
		return nil, err
	}

	if opts == nil {
		opts = &ExecuteABAPOptions{}
	}
	if opts.RiskLevel == "" {
		opts.RiskLevel = "harmless"
	}
	if opts.ReturnVariable == "" {
		opts.ReturnVariable = "lv_result"
	}
	if opts.ProgramPrefix == "" {
		opts.ProgramPrefix = "ZTEMP_EXEC_"
	}

	result := &ExecuteABAPResult{
		Output: []string{},
	}

	// Generate unique program name using timestamp
	timestamp := fmt.Sprintf("%d", time.Now().UnixNano()/1000000)                     // milliseconds
	programName := strings.ToUpper(opts.ProgramPrefix + timestamp[len(timestamp)-8:]) // Last 8 digits
	result.ProgramName = programName
	objectURL := fmt.Sprintf("/sap/bc/adt/programs/programs/%s", url.PathEscape(programName))

	// Build the test class wrapper source
	riskLevelABAP := "RISK LEVEL HARMLESS"
	switch strings.ToLower(opts.RiskLevel) {
	case "dangerous":
		riskLevelABAP = "RISK LEVEL DANGEROUS"
	case "critical":
		riskLevelABAP = "RISK LEVEL CRITICAL"
	}

	source := executeWrapperSource(programName, riskLevelABAP, opts.ReturnVariable, code)

	// Step 1: Create the temp program
	err := c.CreateObject(ctx, CreateObjectOptions{
		ObjectType:  ObjectTypeProgram,
		Name:        programName,
		Description: "Temp program for ExecuteABAP",
		PackageName: "$TMP",
	})
	if err != nil {
		result.Message = fmt.Sprintf("Failed to create temp program: %v", err)
		return result, nil
	}

	// Ensure cleanup on any error (unless KeepProgram is set)
	defer func() {
		if !opts.KeepProgram {
			// Try to delete the program
			lock, lockErr := c.LockObject(ctx, objectURL, "MODIFY")
			if lockErr == nil {
				_ = c.DeleteObject(ctx, objectURL, lock.LockHandle, "")
				result.CleanedUp = true
			}
		}
	}()

	// Step 2: Lock and update source
	lock, err := c.LockObject(ctx, objectURL, "MODIFY")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to lock temp program: %v", err)
		return result, nil
	}

	sourceURL := objectURL + "/source/main"
	err = c.UpdateSource(ctx, sourceURL, source, lock.LockHandle, "")
	if err != nil {
		_ = c.UnlockObject(ctx, objectURL, lock.LockHandle)
		result.Message = fmt.Sprintf("Failed to update source: %v", err)
		return result, nil
	}

	// Step 3: Unlock
	err = c.UnlockObject(ctx, objectURL, lock.LockHandle)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to unlock: %v", err)
		return result, nil
	}

	// Step 4: Activate
	_, err = c.Activate(ctx, objectURL, programName)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to activate: %v", err)
		return result, nil
	}

	// Step 5: Run unit tests
	flags := UnitTestRunFlags{
		Harmless:  true,
		Dangerous: strings.ToLower(opts.RiskLevel) == "dangerous" || strings.ToLower(opts.RiskLevel) == "critical",
		Critical:  strings.ToLower(opts.RiskLevel) == "critical",
		Short:     true,
		Medium:    true,
		Long:      false,
	}

	testResult, err := c.RunUnitTests(ctx, objectURL, &flags)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to run unit tests: %v", err)
		return result, nil
	}

	// Step 6: Parse results - extract assertion messages
	for _, class := range testResult.Classes {
		// A failure while the test class itself is being set up lands on the
		// class rather than on the method, and a payload that dies in a
		// CLASS-CONSTRUCTOR or a DATA declaration dies exactly there.
		result.RawAlerts = append(result.RawAlerts, class.Alerts...)
		for _, method := range class.TestMethods {
			result.ExecutionTime += method.ExecutionTime
			for _, alert := range method.Alerts {
				result.RawAlerts = append(result.RawAlerts, alert)

				// Look for our EXEC_RESULT marker in the alert title
				if output, found := execResult(alert.Title); found {
					result.Output = append(result.Output, output)
				}

				// Also check details for additional output
				for _, detail := range alert.Details {
					if output, found := execResult(detail); found {
						result.Output = append(result.Output, output)
					}
				}
			}
		}
	}

	if alert := PayloadFailure(result.RawAlerts); alert != nil {
		result.Failure = &ExecuteFailure{
			Kind:     alert.Kind,
			Severity: alert.Severity,
			Title:    alert.Title,
			Details:  alert.Details,
			Line:     payloadLine(*alert, programName, payloadOffset(source)),
		}
	}

	result.Success = result.Failure == nil
	switch {
	case result.Failure != nil:
		result.Message = fmt.Sprintf("The code did not finish: %s", result.Failure.Title)
	case len(result.Output) > 0:
		result.Message = fmt.Sprintf("Executed successfully, %d output(s) returned", len(result.Output))
	default:
		result.Message = "Executed successfully (no output captured)"
	}

	return result, nil
}

// executeWrapperSource builds the throwaway report that the payload runs inside.
//
// It is a function rather than an inline literal because two other things
// depend on its exact shape — the marker that ends the run and hands a value
// back, and the line the payload starts on — and a template nobody can call is
// a template nobody can test.
func executeWrapperSource(programName, riskLevel, returnVariable, code string) string {
	return fmt.Sprintf(`REPORT %s.

*&---------------------------------------------------------------------*
*& Auto-generated program for code execution via unit test
*& Generated by vsp ExecuteABAP workflow
*&---------------------------------------------------------------------*

CLASS ltc_executor DEFINITION FOR TESTING %s DURATION SHORT.
  PUBLIC SECTION.
    METHODS execute_payload FOR TESTING.
ENDCLASS.

CLASS ltc_executor IMPLEMENTATION.
  METHOD execute_payload.
    DATA %s TYPE string.

    %s
%s
    " === USER CODE END ===

    " Return result via assertion message
    cl_abap_unit_assert=>fail( msg = |%s{ %s }| ).
  ENDMETHOD.
ENDCLASS.
`, programName, riskLevel, returnVariable, payloadStartMarker, code, execResultMarker, returnVariable)
}

// PayloadFailure picks the alert that says the executed code died rather than
// finished, and returns nil when nothing did.
//
// The deliberate closing assertion is skipped by its marker; what is left is
// what ABAP Unit caught on the way through — an uncaught exception, a zero
// divide, an assert the payload made itself. Warnings are left alone: they are
// things ABAP Unit noticed, not things that stopped the code.
func PayloadFailure(alerts []UnitTestAlert) *UnitTestAlert {
	var fallback *UnitTestAlert
	for i := range alerts {
		alert := alerts[i]
		if carriesExecResult(alert) {
			continue
		}
		// An exception is unambiguous, so it wins outright. Anything else has to
		// be severe enough to have ended the run, which rules out the
		// "tolerable" and "tolerant" severities SAP uses for the rest.
		if strings.EqualFold(alert.Kind, "exception") {
			return &alerts[i]
		}
		if fallback == nil && (strings.EqualFold(alert.Severity, "critical") || strings.EqualFold(alert.Severity, "fatal")) {
			fallback = &alerts[i]
		}
	}
	return fallback
}

// carriesExecResult reports whether an alert is the closing assertion that
// hands a value back, rather than a failure.
func carriesExecResult(alert UnitTestAlert) bool {
	if _, found := execResult(alert.Title); found {
		return true
	}
	for _, detail := range alert.Details {
		if _, found := execResult(detail); found {
			return true
		}
	}
	return false
}

// execResult pulls the returned value out of an assertion message.
//
// The marker is looked for anywhere in the text rather than at the start,
// because SAP does not hand back the message that was passed to it: it wraps it
// in a sentence of its own and quotes it, so |EXEC_RESULT:42| comes back as
// "Critical Assertion Error: 'EXEC_RESULT:42'". Matching on a prefix is why
// every successful run used to report "no output captured" — the value was
// there the whole time, three words to the right of where anyone looked.
func execResult(text string) (string, bool) {
	before, value, found := strings.Cut(text, execResultMarker)
	if !found {
		return "", false
	}
	// The closing quote belongs to SAP's sentence, not to the value, and only
	// exists when the opening one does.
	if strings.HasSuffix(strings.TrimSpace(before), "'") {
		value = strings.TrimSuffix(value, "'")
	}
	return value, true
}

// payloadOffset returns the line of the generated wrapper that holds the first
// line of the caller's code.
//
// Deriving it from the source rather than counting the template by hand means
// an edit to the wrapper cannot silently start reporting line numbers that are
// off by three.
func payloadOffset(source string) int {
	for i, line := range strings.Split(source, "\n") {
		if strings.Contains(line, payloadStartMarker) {
			// i is zero-based and the payload begins on the next line, so the
			// first line of the caller's code is line i+2 of the wrapper.
			return i + 2
		}
	}
	return 0
}

// payloadLine translates a line in the generated wrapper into a line in what
// the caller wrote.
//
// The stack entry points at the wrapper, because that is what SAP compiled, and
// its uri carries the position as a fragment: ".../source/main#start=18,0".
// Only entries naming the wrapper are usable — a failure raised deeper in SAP
// standard code has frames whose line numbers belong to other programs
// entirely, and reporting one of those as "your line 4" would be a lie.
func payloadLine(alert UnitTestAlert, programName string, offset int) int {
	if offset <= 0 {
		return 0
	}
	for _, frame := range alert.Stack {
		if !strings.EqualFold(frame.Name, programName) {
			continue
		}
		_, fragment, found := strings.Cut(frame.URI, "#start=")
		if !found {
			continue
		}
		number, _, _ := strings.Cut(fragment, ",")
		line, err := strconv.Atoi(strings.TrimSpace(number))
		if err != nil || line < offset {
			continue
		}
		return line - offset + 1
	}
	return 0
}

// ExecuteABAPMultiple executes ABAP code and returns multiple results via chained assertions.
// Each call to RETURN_VALUE( ) in the code adds a value to the output.
//
// Example:
//
//	result, err := client.ExecuteABAPMultiple(ctx, `
//	  SELECT * FROM t000 INTO TABLE @DATA(lt_clients) UP TO 5 ROWS.
//	  LOOP AT lt_clients INTO DATA(ls_client).
//	    RETURN_VALUE( |Client { ls_client-mandt }: { ls_client-mtext }| ).
//	  ENDLOOP.
//	`, nil)
//	// result.Output contains one entry per client
func (c *Client) ExecuteABAPMultiple(ctx context.Context, code string, opts *ExecuteABAPOptions) (*ExecuteABAPResult, error) {
	// Wrap the code with a macro that chains assertions
	wrappedCode := `
    DATA lt_exec_results TYPE string_table.

    DEFINE RETURN_VALUE.
      APPEND &1 TO lt_exec_results.
    END-OF-DEFINITION.

    ` + code + `

    " Output all collected results
    DATA lv_idx TYPE i.
    LOOP AT lt_exec_results INTO DATA(lv_exec_result).
      lv_idx = lv_idx + 1.
      cl_abap_unit_assert=>fail( msg = |EXEC_RESULT:{ lv_exec_result }| ).
    ENDLOOP.

    " Mark completion
    lv_result = |Completed with { lines( lt_exec_results ) } results|.
`

	return c.ExecuteABAP(ctx, wrappedCode, opts)
}
