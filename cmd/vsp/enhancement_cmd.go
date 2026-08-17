package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/spf13/cobra"
)

var enhancementCmd = &cobra.Command{
	Use:   "enhancement",
	Short: "Manage Enhancement Framework implementations",
}

var enhancementCreateCmd = &cobra.Command{
	Use:   "create <xh|class|badi> <name>",
	Short: "Create and activate an ENHO with explicit host metadata",
	Long: `Create a source-code plug-in (XH), class enhancement, or BAdI
implementation through SAP's Enhancement Framework APIs.

For XH, stdin is the raw implementation body. For class enhancements, stdin
is optional and becomes the body of --method. BAdI creation links an existing
implementation class; it does not modify that class.

Examples:
  vsp enhancement create xh ZVSP_XH --host ZVSP_HOST --anchor '\PR:ZVSP_HOST\SE:END\EI' --package '$TMP' --description "Synthetic hook" < body.abap
  vsp enhancement create class ZVSP_CE --class ZCL_VSP_HOST --method ZVSP_METHOD --package '$TMP' --description "Synthetic class enhancement" < method-body.abap
  vsp enhancement create badi ZVSP_BI --spot ZS_VSP_SPOT --badi ZBADI_VSP --implementation ZIM_VSP --implementation-class ZCL_IM_VSP --package '$TMP' --description "Synthetic BAdI implementation"`,
	Args: cobra.ExactArgs(2),
	RunE: runEnhancementCreate,
}

func init() {
	rootCmd.AddCommand(enhancementCmd)
	enhancementCmd.AddCommand(enhancementCreateCmd)

	f := enhancementCreateCmd.Flags()
	f.String("description", "", "Enhancement description (required)")
	f.String("package", "", "Package for the new ENHO (required)")
	f.String("transport", "", "Transport request (required for transportable packages)")

	// XH flags.
	f.String("host-type", "PROG", "XH host repository object type")
	f.String("host", "", "XH host repository object name")
	f.String("program", "", "XH generated/main program (defaults to --host)")
	f.String("main-type", "", "XH main repository object type (defaults to --host-type)")
	f.String("main-name", "", "XH main repository object name (defaults to --host)")
	f.String("anchor", "", "Exact Enhancement Framework FULL_NAME anchor")
	f.String("parent-anchor", "", "Optional parent FULL_NAME anchor")
	f.String("spot", "", "Optional enhancement spot (or BAdI enhancement spot for badi kind)")
	f.String("enhancement-mode", "S", "XH enhancement mode: S, E, or I")
	f.Bool("overwrite", false, "Create an overwrite hook element")
	f.Bool("hook-method", false, "Mark the XH element as a method hook")

	// Class enhancement flags.
	f.String("class", "", "Class enhanced by the ENHO")
	f.String("method", "", "Optional new method to add")
	f.String("method-description", "", "Description of the new method")
	f.String("exposure", "PUBLIC", "New method visibility: PUBLIC, PROTECTED, or PRIVATE")

	// BAdI flags.
	f.String("badi", "", "BAdI definition name")
	f.String("implementation", "", "BAdI implementation name")
	f.String("implementation-class", "", "Existing class implementing the BAdI interface")
	f.String("implementation-description", "", "BAdI implementation description")
	f.Bool("inactive", false, "Create the BAdI implementation entry inactive")
	f.Bool("default", false, "Mark it as the default BAdI implementation")
}

func runEnhancementCreate(cmd *cobra.Command, args []string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}

	kindText := strings.ToUpper(strings.TrimSpace(args[0]))
	if kindText == "CLASENH" {
		kindText = "CLASS"
	}
	if kindText == "XBD" || kindText == "BADI_IMPL" {
		kindText = "BADI"
	}

	readFlag := func(name string) string {
		value, _ := cmd.Flags().GetString(name)
		return value
	}
	readBool := func(name string) bool {
		value, _ := cmd.Flags().GetBool(name)
		return value
	}

	var stdinSource string
	if stat, statErr := os.Stdin.Stat(); statErr == nil && stat.Mode()&os.ModeCharDevice == 0 {
		data, readErr := io.ReadAll(os.Stdin)
		if readErr != nil {
			return fmt.Errorf("reading enhancement source from stdin: %w", readErr)
		}
		stdinSource = string(data)
	}

	opts := adt.CreateEnhancementOptions{
		Kind:        adt.EnhancementCreateKind(kindText),
		Name:        args[1],
		Description: readFlag("description"),
		Package:     readFlag("package"),
		Transport:   readFlag("transport"),

		HostObjectType:  readFlag("host-type"),
		HostObjectName:  readFlag("host"),
		HostProgram:     readFlag("program"),
		MainObjectType:  readFlag("main-type"),
		MainObjectName:  readFlag("main-name"),
		Anchor:          readFlag("anchor"),
		ParentAnchor:    readFlag("parent-anchor"),
		Spot:            readFlag("spot"),
		EnhancementMode: readFlag("enhancement-mode"),
		Overwrite:       readBool("overwrite"),
		HookMethod:      readBool("hook-method"),
		Source:          stdinSource,

		ClassName:         readFlag("class"),
		MethodName:        readFlag("method"),
		MethodDescription: readFlag("method-description"),
		MethodExposure:    readFlag("exposure"),
		MethodSource:      stdinSource,

		SpotName:                  readFlag("spot"),
		BAdIName:                  readFlag("badi"),
		ImplementationName:        readFlag("implementation"),
		ImplementationClass:       readFlag("implementation-class"),
		ImplementationDescription: readFlag("implementation-description"),
		Inactive:                  readBool("inactive"),
		DefaultImplementation:     readBool("default"),
	}

	result, err := client.CreateEnhancement(context.Background(), opts)
	if err != nil {
		return fmt.Errorf("enhancement creation blocked: %w", err)
	}
	output, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(output))
	if !result.Success {
		return fmt.Errorf("enhancement creation failed: %s", result.Message)
	}
	return nil
}
