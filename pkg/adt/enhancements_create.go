package adt

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// EnhancementCreateKind selects the Enhancement Framework tool used to
// create an ENHO. These are intentionally semantic names rather than the
// unreliable subtype reported by older ADT search implementations.
type EnhancementCreateKind string

const (
	EnhancementCreateXH    EnhancementCreateKind = "XH"
	EnhancementCreateClass EnhancementCreateKind = "CLASS"
	EnhancementCreateBAdI  EnhancementCreateKind = "BADI"
)

// CreateEnhancementOptions contains the explicit metadata SAP needs to
// create an enhancement implementation. Fields are grouped by enhancement
// kind; validation rejects incomplete or cross-kind ambiguous requests before
// the WebSocket bridge is opened.
type CreateEnhancementOptions struct {
	Kind        EnhancementCreateKind `json:"kind"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Package     string                `json:"package"`
	Transport   string                `json:"transport,omitempty"`

	// XH / source-code plug-in.
	HostObjectType  string `json:"hostObjectType,omitempty"`
	HostObjectName  string `json:"hostObjectName,omitempty"`
	HostProgram     string `json:"hostProgram,omitempty"`
	MainObjectType  string `json:"mainObjectType,omitempty"`
	MainObjectName  string `json:"mainObjectName,omitempty"`
	Anchor          string `json:"anchor,omitempty"`
	ParentAnchor    string `json:"parentAnchor,omitempty"`
	Spot            string `json:"spot,omitempty"`
	EnhancementMode string `json:"enhancementMode,omitempty"`
	Overwrite       bool   `json:"overwrite,omitempty"`
	HookMethod      bool   `json:"hookMethod,omitempty"`
	Source          string `json:"source,omitempty"`

	// Class enhancement. MethodName is optional; when present, one new method
	// is created. MethodSource may be a full METHOD...ENDMETHOD block or only
	// the method body.
	ClassName         string `json:"className,omitempty"`
	MethodName        string `json:"methodName,omitempty"`
	MethodDescription string `json:"methodDescription,omitempty"`
	MethodExposure    string `json:"methodExposure,omitempty"`
	MethodSource      string `json:"methodSource,omitempty"`

	// BAdI implementation. The implementation class must already exist and
	// implement the interface required by the selected BAdI definition.
	SpotName                  string `json:"spotName,omitempty"`
	BAdIName                  string `json:"badiName,omitempty"`
	ImplementationName        string `json:"implementationName,omitempty"`
	ImplementationClass       string `json:"implementationClass,omitempty"`
	ImplementationDescription string `json:"implementationDescription,omitempty"`
	Inactive                  bool   `json:"inactive,omitempty"`
	DefaultImplementation     bool   `json:"defaultImplementation,omitempty"`
}

// CreateEnhancementResult is returned only after repository metadata confirms
// that the newly created ENHO has an active version of the expected tool type.
type CreateEnhancementResult struct {
	Success    bool                  `json:"success"`
	Name       string                `json:"name"`
	Kind       EnhancementCreateKind `json:"kind"`
	ToolType   string                `json:"toolType,omitempty"`
	Package    string                `json:"package"`
	Transport  string                `json:"transport,omitempty"`
	ObjectURL  string                `json:"objectUrl,omitempty"`
	Activation *ActivationResult     `json:"activation,omitempty"`
	Message    string                `json:"message,omitempty"`
}

type rfcEnhancementCreator interface {
	CreateEnhancement(ctx context.Context, opts CreateEnhancementOptions) error
}

var enhancementIdentifierPattern = regexp.MustCompile(`^[A-Z0-9_/$]+$`)

// CreateEnhancement creates and activates a new ENHO through SAP's own
// Enhancement Framework APIs exposed by the optional ZADT_VSP bridge.
func (c *Client) CreateEnhancement(ctx context.Context, opts CreateEnhancementOptions) (*CreateEnhancementResult, error) {
	normalizeCreateEnhancementOptions(&opts)
	result := &CreateEnhancementResult{
		Name:      opts.Name,
		Kind:      opts.Kind,
		Package:   opts.Package,
		Transport: opts.Transport,
	}

	if err := validateCreateEnhancementOptions(opts); err != nil {
		result.Message = err.Error()
		return result, nil
	}

	mutation := MutationContext{
		Op:        OpWorkflow,
		OpName:    "CreateEnhancement",
		Package:   opts.Package,
		Transport: opts.Transport,
	}
	if err := c.checkMutation(ctx, mutation); err != nil {
		return nil, err
	}
	if err := c.checkPackageTransportRequirement(ctx, opts.Package, opts.Transport, "CreateEnhancement"); err != nil {
		return nil, err
	}

	exists, err := c.enhancementExistsInRepository(ctx, opts.Name)
	if err != nil {
		return nil, fmt.Errorf("checking whether ENHO %s exists: %w", opts.Name, err)
	}
	if exists {
		result.Message = fmt.Sprintf("Enhancement %s already exists", opts.Name)
		return result, nil
	}

	factory := c.rfcFetcherFactory
	if factory == nil {
		factory = c.defaultRFCSourceFetcher
	}
	fetcher, err := factory(ctx)
	if err != nil {
		result.Message = fmt.Sprintf("Creating enhancements requires the ZADT_VSP bridge: %v", err)
		return result, nil
	}
	defer fetcher.Close()

	creator, ok := fetcher.(rfcEnhancementCreator)
	if !ok {
		result.Message = "The installed ZADT_VSP bridge does not support enhancement creation; update ZCL_VSP_RFC_SERVICE and retry"
		return result, nil
	}
	if err := creator.CreateEnhancement(ctx, opts); err != nil {
		result.Message = fmt.Sprintf("Failed to schedule enhancement creation through ZADT_VSP: %v", err)
		return result, nil
	}

	expectedTool := enhancementToolTypeForCreateKind(opts.Kind)
	deadline := time.Now().Add(60 * time.Second)
	for {
		toolType, packageName, active, readErr := c.getEnhancementRepositoryState(ctx, opts.Name)
		if readErr == nil && active && strings.EqualFold(toolType, expectedTool) {
			result.Success = true
			result.ToolType = toolType
			if packageName != "" {
				result.Package = packageName
			}
			result.ObjectURL = enhancementObjectURL(opts.Kind, opts.Name)
			result.Activation = &ActivationResult{Success: true}
			result.Message = "Enhancement created, saved, and activated successfully"
			return result, nil
		}
		if readErr == nil && toolType != "" && !strings.EqualFold(toolType, expectedTool) {
			result.Message = fmt.Sprintf("ENHO %s appeared with unexpected tool type %s (expected %s); success is not reported", opts.Name, toolType, expectedTool)
			return result, nil
		}
		if time.Now().After(deadline) {
			if readErr != nil {
				result.Message = fmt.Sprintf("Enhancement worker was scheduled but repository read-back failed: %v", readErr)
			} else {
				result.Message = "Enhancement worker was scheduled but no matching active ENHO appeared within 60s; creation is not reported as successful"
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
}

func normalizeCreateEnhancementOptions(opts *CreateEnhancementOptions) {
	opts.Kind = EnhancementCreateKind(strings.ToUpper(strings.TrimSpace(string(opts.Kind))))
	opts.Name = strings.ToUpper(strings.TrimSpace(opts.Name))
	opts.Package = strings.ToUpper(strings.TrimSpace(opts.Package))
	opts.Transport = strings.ToUpper(strings.TrimSpace(opts.Transport))
	opts.Description = strings.TrimSpace(opts.Description)

	opts.HostObjectType = strings.ToUpper(strings.TrimSpace(opts.HostObjectType))
	opts.HostObjectName = strings.ToUpper(strings.TrimSpace(opts.HostObjectName))
	opts.HostProgram = strings.ToUpper(strings.TrimSpace(opts.HostProgram))
	opts.MainObjectType = strings.ToUpper(strings.TrimSpace(opts.MainObjectType))
	opts.MainObjectName = strings.ToUpper(strings.TrimSpace(opts.MainObjectName))
	opts.Anchor = strings.TrimSpace(opts.Anchor)
	opts.ParentAnchor = strings.TrimSpace(opts.ParentAnchor)
	opts.Spot = strings.ToUpper(strings.TrimSpace(opts.Spot))
	opts.EnhancementMode = strings.ToUpper(strings.TrimSpace(opts.EnhancementMode))

	opts.ClassName = strings.ToUpper(strings.TrimSpace(opts.ClassName))
	opts.MethodName = strings.ToUpper(strings.TrimSpace(opts.MethodName))
	opts.MethodDescription = strings.TrimSpace(opts.MethodDescription)
	opts.MethodExposure = strings.ToUpper(strings.TrimSpace(opts.MethodExposure))

	opts.SpotName = strings.ToUpper(strings.TrimSpace(opts.SpotName))
	opts.BAdIName = strings.ToUpper(strings.TrimSpace(opts.BAdIName))
	opts.ImplementationName = strings.ToUpper(strings.TrimSpace(opts.ImplementationName))
	opts.ImplementationClass = strings.ToUpper(strings.TrimSpace(opts.ImplementationClass))
	opts.ImplementationDescription = strings.TrimSpace(opts.ImplementationDescription)

	if opts.Kind == EnhancementCreateXH {
		if opts.HostObjectType == "" {
			opts.HostObjectType = "PROG"
		}
		if opts.HostProgram == "" {
			opts.HostProgram = opts.HostObjectName
		}
		if opts.MainObjectType == "" {
			opts.MainObjectType = opts.HostObjectType
		}
		if opts.MainObjectName == "" {
			opts.MainObjectName = opts.HostObjectName
		}
		if opts.EnhancementMode == "" {
			opts.EnhancementMode = "S"
		}
	}
	if opts.Kind == EnhancementCreateClass {
		if opts.MethodDescription == "" {
			opts.MethodDescription = opts.Description
		}
		if opts.MethodExposure == "" {
			opts.MethodExposure = "PUBLIC"
		}
		if opts.MethodName != "" {
			opts.MethodSource = normalizeEnhancedMethodSource(opts.MethodName, opts.MethodSource)
		}
	}
	if opts.Kind == EnhancementCreateBAdI && opts.ImplementationDescription == "" {
		opts.ImplementationDescription = opts.Description
	}
}

func validateCreateEnhancementOptions(opts CreateEnhancementOptions) error {
	if opts.Kind != EnhancementCreateXH && opts.Kind != EnhancementCreateClass && opts.Kind != EnhancementCreateBAdI {
		return fmt.Errorf("kind must be XH, CLASS, or BADI")
	}
	if err := validateEnhancementIdentifier("enhancement name", opts.Name, 30); err != nil {
		return err
	}
	if opts.Package == "" {
		return fmt.Errorf("package is required")
	}
	if opts.Description == "" {
		return fmt.Errorf("description is required")
	}
	if len(opts.Description) > 60 {
		return fmt.Errorf("description must not exceed 60 characters")
	}

	switch opts.Kind {
	case EnhancementCreateXH:
		for label, value := range map[string]string{
			"host object type": opts.HostObjectType,
			"host object name": opts.HostObjectName,
			"host program":     opts.HostProgram,
			"main object type": opts.MainObjectType,
			"main object name": opts.MainObjectName,
		} {
			if err := validateEnhancementIdentifier(label, value, 40); err != nil {
				return err
			}
		}
		if opts.Anchor == "" {
			return fmt.Errorf("anchor is required for XH creation")
		}
		if len(opts.Anchor) > 255 || len(opts.ParentAnchor) > 255 {
			return fmt.Errorf("anchor and parent anchor must not exceed 255 characters")
		}
		if strings.TrimSpace(opts.Source) == "" {
			return fmt.Errorf("source is required for XH creation")
		}
		if len(opts.Source) > 512*1024 {
			return fmt.Errorf("source exceeds the 512 KiB bridge limit")
		}
		if opts.EnhancementMode != "S" && opts.EnhancementMode != "E" && opts.EnhancementMode != "I" {
			return fmt.Errorf("enhancement mode must be S, E, or I")
		}

	case EnhancementCreateClass:
		if len(opts.Name) > 25 {
			return fmt.Errorf("class enhancement name must not exceed 25 characters on classic Enhancement Framework systems")
		}
		if err := validateEnhancementIdentifier("class name", opts.ClassName, 30); err != nil {
			return err
		}
		if opts.MethodName != "" {
			if err := validateEnhancementIdentifier("method name", opts.MethodName, 30); err != nil {
				return err
			}
			if opts.MethodExposure != "PUBLIC" && opts.MethodExposure != "PROTECTED" && opts.MethodExposure != "PRIVATE" {
				return fmt.Errorf("method exposure must be PUBLIC, PROTECTED, or PRIVATE")
			}
			if len(opts.MethodSource) > 512*1024 {
				return fmt.Errorf("method source exceeds the 512 KiB bridge limit")
			}
		} else if strings.TrimSpace(opts.MethodSource) != "" {
			return fmt.Errorf("method_name is required when method source is supplied")
		}

	case EnhancementCreateBAdI:
		for label, value := range map[string]string{
			"enhancement spot":     opts.SpotName,
			"BAdI name":            opts.BAdIName,
			"implementation name":  opts.ImplementationName,
			"implementation class": opts.ImplementationClass,
		} {
			if err := validateEnhancementIdentifier(label, value, 30); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateEnhancementIdentifier(label, value string, maxLen int) error {
	if value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if len(value) > maxLen || !enhancementIdentifierPattern.MatchString(value) {
		return fmt.Errorf("%s contains unsupported characters or exceeds %d characters", label, maxLen)
	}
	return nil
}

func normalizeEnhancedMethodSource(methodName, source string) string {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return fmt.Sprintf("METHOD %s.\nENDMETHOD.", methodName)
	}
	if strings.HasPrefix(strings.ToUpper(trimmed), "METHOD ") {
		return trimmed
	}
	return fmt.Sprintf("METHOD %s.\n%s\nENDMETHOD.", methodName, strings.TrimRight(source, "\n"))
}

func enhancementToolTypeForCreateKind(kind EnhancementCreateKind) string {
	switch kind {
	case EnhancementCreateXH:
		return "HOOK_IMPL"
	case EnhancementCreateClass:
		return "CLASENH"
	case EnhancementCreateBAdI:
		return "BADI_IMPL"
	default:
		return ""
	}
}

func enhancementObjectURL(kind EnhancementCreateKind, name string) string {
	segment := "enhoxh"
	switch kind {
	case EnhancementCreateClass:
		segment = "enhoxc"
	case EnhancementCreateBAdI:
		segment = "enhoxbd"
	}
	return fmt.Sprintf("/sap/bc/adt/enhancements/%s/%s", segment, strings.ToLower(name))
}

func (c *Client) enhancementExistsInRepository(ctx context.Context, name string) (bool, error) {
	toolType, _, _, err := c.getEnhancementRepositoryState(ctx, name)
	if err != nil {
		return false, err
	}
	return toolType != "", nil
}

func (c *Client) getEnhancementRepositoryState(ctx context.Context, name string) (toolType, packageName string, active bool, err error) {
	name = strings.ToUpper(strings.TrimSpace(name))
	headerSQL := fmt.Sprintf(
		"SELECT ENHTOOLTYPE, VERSION FROM ENHHEADER WHERE ENHNAME = '%s'",
		strings.ReplaceAll(name, "'", "''"))
	header, err := c.GetTableContents(ctx, "ENHHEADER", 10, headerSQL)
	if err != nil {
		return "", "", false, err
	}
	for _, row := range header.Rows {
		candidate := strings.ToUpper(strings.TrimSpace(asString(row["ENHTOOLTYPE"])))
		if candidate != "" && toolType == "" {
			toolType = candidate
		}
		if strings.EqualFold(strings.TrimSpace(asString(row["VERSION"])), "A") {
			active = true
			if candidate != "" {
				toolType = candidate
			}
		}
	}
	if toolType == "" {
		return "", "", false, nil
	}

	tadirSQL := fmt.Sprintf(
		"SELECT DEVCLASS FROM TADIR WHERE PGMID = 'R3TR' AND OBJECT = 'ENHO' AND OBJ_NAME = '%s'",
		strings.ReplaceAll(name, "'", "''"))
	tadir, tadirErr := c.GetTableContents(ctx, "TADIR", 2, tadirSQL)
	if tadirErr == nil && len(tadir.Rows) > 0 {
		packageName = strings.ToUpper(strings.TrimSpace(asString(tadir.Rows[0]["DEVCLASS"])))
	}
	return toolType, packageName, active, nil
}
