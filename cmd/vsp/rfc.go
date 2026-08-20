package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/oisee/open-rfc-go/rfc"
	"github.com/oisee/vibing-steampunk/pkg/config"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
	"github.com/spf13/cobra"
)

var rfcCmd = &cobra.Command{
	Use:   "rfc",
	Short: "Call SAP function modules over classic RFC (SDK-free)",
	Long: `Call ABAP function modules over classic RFC, against the same system vsp
uses for ADT. The gateway host defaults to the system URL's host and the port to
3300 + system number; override with --rfc-host / --sysnr / --port, or per system
with rfc_host / rfc_sysnr / rfc_port in .vsp.json.

RFC logon uses rfc_user/rfc_password (which fall back to SAP_USER/SAP_PASSWORD),
otherwise the system's own user/password.`,
}

var rfcInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "RFC system info (RFC_SYSTEM_INFO)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			r, err := c.Call(ctx, "RFC_SYSTEM_INFO", nil)
			if err != nil {
				return err
			}
			return emitRFC(r.Get("RFCSI_EXPORT"))
		})
	},
}

var rfcPingCmd = &cobra.Command{
	Use:   "ping",
	Short: "RFC connection test (RFC_PING)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			if _, err := c.Call(ctx, "RFC_PING", nil); err != nil {
				return err
			}
			fmt.Println("ok")
			return nil
		})
	},
}

var rfcDescribeCmd = &cobra.Command{
	Use:   "describe <FUNCTION_MODULE>",
	Short: "Describe an FM interface as an MCP-tool JSON Schema",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			tool, err := c.DescribeTool(ctx, strings.ToUpper(args[0]))
			if err != nil {
				return err
			}
			return emitRFC(tool)
		})
	},
}

var rfcCallCmd = &cobra.Command{
	Use:   "call <FUNCTION_MODULE> [json]",
	Short: "Call a function module with JSON parameters",
	Long: `Call any RFC-enabled function module. Parameters are a JSON object, given
inline, with --file, or on stdin; values are coerced to each parameter's type.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		raw := ""
		if len(args) > 1 {
			raw = args[1]
		}
		if file, _ := cmd.Flags().GetString("file"); file != "" {
			b, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			raw = string(b)
		}
		if stdin, _ := cmd.Flags().GetBool("stdin"); stdin {
			b, err := readAllStdin()
			if err != nil {
				return err
			}
			raw = string(b)
		}
		params := rfc.Params{}
		if strings.TrimSpace(raw) != "" {
			dec := json.NewDecoder(strings.NewReader(raw))
			dec.UseNumber()
			var obj map[string]any
			if err := dec.Decode(&obj); err != nil {
				return fmt.Errorf("parameters must be a JSON object: %w", err)
			}
			params = obj
		}
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			r, err := c.Call(ctx, strings.ToUpper(args[0]), params)
			if err != nil {
				return err
			}
			return emitRFC(r)
		})
	},
}

var rfcSearchCmd = &cobra.Command{
	Use:   "search <pattern>",
	Short: "Find RFC-enabled function modules (name mask, * wildcard)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		like := strings.ReplaceAll(strings.ToUpper(args[0]), "*", "%")
		if !strings.Contains(like, "%") {
			like = "%" + like + "%"
		}
		where := "FUNCNAME LIKE '" + like + "'"
		if all, _ := cmd.Flags().GetBool("all"); !all {
			where += " AND FMODE = 'R'"
		}
		top, _ := cmd.Flags().GetInt("top")
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			rows, err := saprfc.ReadTable(ctx, c, "TFDIR", where, []string{"FUNCNAME", "PNAME"}, top)
			if err != nil {
				return err
			}
			return emitRFC(rows)
		})
	},
}

var rfcReadTableCmd = &cobra.Command{
	Use:   "read-table <table>",
	Short: "Read a table over RFC (RFC_READ_TABLE)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		where, _ := cmd.Flags().GetString("where")
		top, _ := cmd.Flags().GetInt("top")
		var fields []string
		if f, _ := cmd.Flags().GetString("fields"); f != "" {
			for _, x := range strings.Split(f, ",") {
				if x = strings.TrimSpace(x); x != "" {
					fields = append(fields, strings.ToUpper(x))
				}
			}
		}
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			rows, err := saprfc.ReadTable(ctx, c, strings.ToUpper(args[0]), where, fields, top)
			if err != nil {
				return err
			}
			return emitRFC(rows)
		})
	},
}

// withRFC resolves the RFC destination for the selected system and runs fn.
func withRFC(cmd *cobra.Command, fn func(context.Context, *rfc.Client) error) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return err
	}
	in := saprfc.Input{
		URL: params.URL, User: params.User, Password: params.Password,
		Client: params.Client, Language: params.Language,
	}
	// Per-system RFC settings, when the system came from .vsp.json.
	if params.Name != "" {
		if cfg, _, cerr := config.LoadSystems(); cerr == nil && cfg != nil {
			if sys, serr := cfg.GetSystem(params.Name); serr == nil {
				in.RFCHost, in.RFCSysnr, in.RFCPort = sys.RFCHost, sys.RFCSysnr, sys.RFCPort
				in.RFCUser, in.RFCPassword = sys.RFCUser, sys.RFCPassword
			}
		}
	} else {
		in.RFCUser, in.RFCPassword = os.Getenv("SAP_USER"), os.Getenv("SAP_PASSWORD")
	}
	in.HostFlag, _ = cmd.Flags().GetString("rfc-host")
	in.SysnrFlag, _ = cmd.Flags().GetString("sysnr")
	in.PortFlag, _ = cmd.Flags().GetInt("port")
	in.UserFlag, _ = cmd.Flags().GetString("rfc-user")

	dest, err := saprfc.Resolve(in)
	if err != nil {
		return err
	}
	if verbose, _ := cmd.Flags().GetBool("verbose"); verbose {
		fmt.Fprintf(os.Stderr, "[INFO] RFC %s:%d (sysnr %s) client %s user %s\n", dest.Host, dest.Port, dest.Sysnr, dest.Client, dest.User)
	}
	ctx := context.Background()
	c, err := saprfc.Open(ctx, dest)
	if err != nil {
		return fmt.Errorf("RFC logon to %s:%d failed: %w", dest.Host, dest.Port, err)
	}
	defer c.Close(ctx)
	return fn(ctx, c)
}

func emitRFC(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func readAllStdin() ([]byte, error) {
	var b []byte
	buf := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(buf)
		b = append(b, buf[:n]...)
		if err != nil {
			if err.Error() == "EOF" {
				return b, nil
			}
			return b, nil
		}
	}
}

func init() {
	rfcCmd.PersistentFlags().String("rfc-host", "", "RFC gateway host (default: host from the system URL)")
	rfcCmd.PersistentFlags().String("sysnr", "", "SAP system number, 00..99 (default: derived from the URL port)")
	rfcCmd.PersistentFlags().Int("port", 0, "RFC gateway port (default: 3300 + system number)")
	rfcCmd.PersistentFlags().String("rfc-user", "", "RFC logon user (default: rfc_user / SAP_USER / the system's user)")

	rfcCallCmd.Flags().String("file", "", "read JSON parameters from a file")
	rfcCallCmd.Flags().Bool("stdin", false, "read JSON parameters from stdin")
	rfcSearchCmd.Flags().Bool("all", false, "include function modules that are not RFC-enabled")
	rfcSearchCmd.Flags().Int("top", 100, "maximum rows")
	rfcReadTableCmd.Flags().String("where", "", "WHERE clause")
	rfcReadTableCmd.Flags().String("fields", "", "comma-separated column list")
	rfcReadTableCmd.Flags().Int("top", 0, "maximum rows (0 = all)")

	rfcCmd.AddCommand(rfcInfoCmd, rfcPingCmd, rfcDescribeCmd, rfcCallCmd, rfcSearchCmd, rfcReadTableCmd)
	rootCmd.AddCommand(rfcCmd)
}
