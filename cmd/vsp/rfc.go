package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

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

var rfcProbeCmd = &cobra.Command{
	Use:   "probe",
	Short: "Fingerprint the system over RFC (release, components, helpers, authorizations)",
	Long: `Gather what you want to know before trusting a system with real work: what it
is, which components are installed, whether the vsp/abapGit helpers are present, and
which function modules this user is actually authorized to call — the last of which
ADT cannot answer. Read-only: nothing is executed or written.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return withRFCDest(cmd, func(ctx context.Context, c *rfc.Client, dest saprfc.Params) error {
			probe, err := saprfc.RunProbe(ctx, c, dest)
			if err != nil {
				return err
			}
			if format, _ := cmd.Flags().GetString("format"); format == "json" {
				return emitRFC(probe)
			}
			fmt.Print(probe.Text())
			return nil
		})
	},
}

var rfcExportCmd = &cobra.Command{
	Use:   "export <PACKAGE>",
	Short: "Serialize a package to an abapGit ZIP over RFC",
	Long: `Serialize an ABAP package into an abapGit ZIP with a single RFC call to
abapGit's own Z_ABAPGIT_SERIALIZE_PACKAGE. Needs abapGit installed on the system;
it needs no vsp helper, and no HTTP.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		out, _ := cmd.Flags().GetString("output")
		if out == "" {
			out = strings.ToLower(strings.TrimPrefix(args[0], "$")) + ".zip"
		}
		opts := saprfc.ExportOptions{}
		opts.FolderLogic, _ = cmd.Flags().GetString("folder-logic")
		opts.MainLanguageOnly, _ = cmd.Flags().GetBool("main-lang-only")
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			zip, err := saprfc.ExportPackage(ctx, c, args[0], opts)
			if err != nil {
				return err
			}
			if err := os.WriteFile(out, zip, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "wrote %s (%d bytes)\n", out, len(zip))
			return nil
		})
	},
}

var rfcRunCmd = &cobra.Command{
	Use:   "run <REPORT>",
	Short: "Run an ABAP report as a background job over RFC",
	Long: `Schedule a report as a background job (SUBST_START_REPORT_IN_BATCH), optionally
wait for it to finish, and optionally fetch its spool. This is the thing the ADT
WebSocket path cannot do — APC forbids SUBMIT — and it needs no helper on the system.

  vsp rfc run RSPARAM --wait 60
  vsp rfc run ZMY_REPORT -p P_WERKS=1000 -p S_MATNR=M1 --wait 120 --spool`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		raw, _ := cmd.Flags().GetStringArray("param")
		var params []saprfc.ReportParam
		for _, kv := range raw {
			name, value, found := strings.Cut(kv, "=")
			if !found {
				return fmt.Errorf("parameter %q must be NAME=VALUE", kv)
			}
			params = append(params, saprfc.ReportParam{Name: name, Low: value})
		}
		jobName, _ := cmd.Flags().GetString("job-name")
		waitSecs, _ := cmd.Flags().GetInt("wait")
		wantSpool, _ := cmd.Flags().GetBool("spool")

		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			run, err := saprfc.RunReport(ctx, c, args[0], jobName, params, time.Duration(waitSecs)*time.Second)
			if err != nil {
				return err
			}
			if wantSpool && run.Status == "F" {
				spool, serr := saprfc.ReadSpool(ctx, c, run.JobName, run.JobCount)
				if serr != nil {
					fmt.Fprintln(os.Stderr, "spool unavailable:", serr)
				} else {
					run.Spool = spool
				}
			}
			return emitRFC(run)
		})
	},
}

var rfcSpoolCmd = &cobra.Command{
	Use:   "spool <JOBNAME> <JOBCOUNT>",
	Short: "Read a background job's spool list over RFC",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		step, _ := cmd.Flags().GetInt("step")
		return withRFC(cmd, func(ctx context.Context, c *rfc.Client) error {
			out, err := saprfc.ReadSpoolStep(ctx, c, args[0], args[1], step)
			if err != nil {
				return err
			}
			if out == "" {
				fmt.Fprintln(os.Stderr, "the job produced no spool list")
				return nil
			}
			fmt.Print(out)
			return nil
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
	return withRFCDest(cmd, func(ctx context.Context, c *rfc.Client, _ saprfc.Params) error {
		return fn(ctx, c)
	})
}

// withRFCDest is withRFC for callers that also need the resolved destination.
func withRFCDest(cmd *cobra.Command, fn func(context.Context, *rfc.Client, saprfc.Params) error) error {
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
	return fn(ctx, c, dest)
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

	rfcProbeCmd.Flags().String("format", "text", "Output format: text or json")
	rfcExportCmd.Flags().StringP("output", "o", "", "Write the ZIP here (default: <package>.zip)")
	rfcExportCmd.Flags().String("folder-logic", "", "abapGit folder logic: FULL or PREFIX")
	rfcExportCmd.Flags().Bool("main-lang-only", false, "Serialize the main language only")
	rfcRunCmd.Flags().StringArrayP("param", "p", nil, "Selection parameter NAME=VALUE (repeatable)")
	rfcRunCmd.Flags().String("job-name", "", "Background job name (default: VSP_<REPORT>)")
	rfcRunCmd.Flags().Int("wait", 0, "Seconds to wait for the job to finish (0 = do not wait)")
	rfcRunCmd.Flags().Bool("spool", false, "Fetch the spool list once the job has finished")
	rfcSpoolCmd.Flags().Int("step", 1, "Job step number")
	rfcCmd.AddCommand(rfcInfoCmd, rfcPingCmd, rfcProbeCmd, rfcExportCmd, rfcRunCmd, rfcSpoolCmd, rfcDescribeCmd, rfcCallCmd, rfcSearchCmd, rfcReadTableCmd)
	rootCmd.AddCommand(rfcCmd)
}
