package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/gqcdm/aiprobe/internal/app"
	"github.com/gqcdm/aiprobe/internal/detect"
	"github.com/gqcdm/aiprobe/internal/diagnostics"
	"github.com/gqcdm/aiprobe/internal/providers"
	"github.com/gqcdm/aiprobe/internal/render"
	"github.com/gqcdm/aiprobe/internal/schema"
	"github.com/spf13/cobra"
)

type App struct {
	stdout io.Writer
	stderr io.Writer
	engine *detect.Engine
	root   *cobra.Command
}

func New() *App {
	a := &App{
		stdout: app.Stdout,
		stderr: app.Stderr,
		engine: detect.NewEngine(providers.All()...),
	}
	a.root = a.newRootCmd()
	return a
}

func (a *App) Run(args []string) error {
	args = rewriteShortcutArgs(args)
	a.root.SetOut(a.stdout)
	a.root.SetErr(a.stderr)
	a.root.SetArgs(args)
	return a.root.Execute()
}

func (a *App) newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           app.Name,
		Short:         "Auto-detect AI API providers and diagnostics",
		Long:          "Auto-detect AI API providers and diagnostics. Use `aiprobe -t <base-url> <api-key>` for a one-shot test that lists available models and first-token latency.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().BoolP("test", "t", false, "Shortcut for `test` with positional <base-url> <api-key>")
	cmd.CompletionOptions.DisableDefaultCmd = false
	cmd.AddCommand(a.newDetectCmd(), a.newTestCmd())
	return cmd
}

func rewriteShortcutArgs(args []string) []string {
	if len(args) == 0 || !hasTestShortcut(args) || hasExplicitCommand(args) {
		return args
	}

	rewritten := []string{"test"}
	remaining := make([]string, 0, len(args)-1)
	positionals := make([]string, 0, 2)

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-t" || arg == "--test" {
			continue
		}

		if consumesValue(arg) {
			remaining = append(remaining, arg)
			if i+1 < len(args) {
				i++
				remaining = append(remaining, args[i])
			}
			continue
		}

		if strings.HasPrefix(arg, "--base-url=") || strings.HasPrefix(arg, "--api-key=") || strings.HasPrefix(arg, "--format=") || strings.HasPrefix(arg, "--samples=") {
			remaining = append(remaining, arg)
			continue
		}

		if strings.HasPrefix(arg, "-") {
			remaining = append(remaining, arg)
			continue
		}

		if len(positionals) < 2 {
			positionals = append(positionals, arg)
			continue
		}

		remaining = append(remaining, arg)
	}

	if len(positionals) > 0 && !hasLongFlag(remaining, "--base-url") {
		rewritten = append(rewritten, "--base-url", positionals[0])
	}
	if len(positionals) > 1 && !hasLongFlag(remaining, "--api-key") {
		rewritten = append(rewritten, "--api-key", positionals[1])
	}

	return append(rewritten, remaining...)
}

func hasTestShortcut(args []string) bool {
	return slices.Contains(args, "-t") || slices.Contains(args, "--test")
}

func hasExplicitCommand(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return arg == "detect" || arg == "test" || arg == "completion"
	}
	return false
}

func consumesValue(arg string) bool {
	return arg == "--base-url" || arg == "--api-key" || arg == "--format" || arg == "--samples"
}

func hasLongFlag(args []string, name string) bool {
	prefix := name + "="
	for _, arg := range args {
		if arg == name || strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

func (a *App) newDetectCmd() *cobra.Command {
	var baseURL string
	var apiKey string
	var format string

	cmd := &cobra.Command{
		Use:   "detect",
		Short: "Detect provider, API type, and model list metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(apiKey) == "" {
				return errors.New("detect requires --base-url and --api-key")
			}

			output, err := a.engine.Detect(detect.Input{BaseURL: baseURL, APIKey: apiKey})
			if err != nil {
				return err
			}

			return render.Write(a.stdout, output, format)
		},
	}
	cmd.Flags().StringVar(&baseURL, "base-url", "", "API base URL")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key")
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text or json")
	return cmd
}

func (a *App) newTestCmd() *cobra.Command {
	var baseURL string
	var apiKey string
	var format string
	var samples int

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run detection plus diagnostics and latency checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(apiKey) == "" {
				return errors.New("test requires --base-url and --api-key")
			}
			if samples <= 0 {
				return errors.New("test requires --samples > 0")
			}

			showProgress := shouldShowTestProgress(format)
			a.writeTestProgress(showProgress, "[1/4] Detecting provider and models...")

			output, err := a.engine.Detect(detect.Input{BaseURL: baseURL, APIKey: apiKey})
			if err != nil {
				return err
			}
			a.writeTestProgress(showProgress, "[1/4] Detected %s (%d models found)", output.Detection.Provider, len(output.Models))

			adapter := providers.ByProvider(output.Detection.Provider)
			if adapter == nil {
				a.writeTestProgress(showProgress, "[2/4] Skipping endpoint diagnostics (provider unresolved)")
				output.Diagnostics = schema.DiagnosticsResult{
					Status:      "failed",
					FailureKind: schema.FailureDiagnosticsSkipped,
				}
			} else {
				a.writeTestProgress(showProgress, "[2/4] Running endpoint diagnostics (samples=%d)...", samples)
				output.Diagnostics = diagnostics.Run(adapter, diagnostics.Input{BaseURL: baseURL, APIKey: apiKey, Samples: samples})
				a.writeTestProgress(showProgress, "[2/4] Endpoint diagnostics %s", output.Diagnostics.Status)

				if len(output.Models) == 0 {
					a.writeTestProgress(showProgress, "[3/4] Skipping model diagnostics (no models found)")
				} else {
					a.writeTestProgress(showProgress, "[3/4] Running model diagnostics (%d models, samples=%d)...", len(output.Models), samples)
				}
				modelDiagnostics, warnings := diagnostics.RunModelDiagnostics(output.Detection.Provider, baseURL, apiKey, output.Models, samples)
				output.ModelDiagnostics = modelDiagnostics
				output.Warnings = append(output.Warnings, warnings...)
				if len(output.Models) > 0 {
					a.writeTestProgress(showProgress, "[3/4] Model diagnostics finished (%d results)", len(output.ModelDiagnostics))
				}
			}

			a.writeTestProgress(showProgress, "[4/4] Rendering result...")

			if err := render.Write(a.stdout, output, format); err != nil {
				return err
			}

			if output.Diagnostics.Status == "failed" {
				return errWithCode(3)
			}

			return nil
		},
	}
	cmd.Flags().StringVar(&baseURL, "base-url", "", "API base URL")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key")
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text or json")
	cmd.Flags().IntVar(&samples, "samples", 3, "Latency sample count")
	return cmd
}

func shouldShowTestProgress(format string) bool {
	return strings.TrimSpace(strings.ToLower(format)) != "json"
}

func (a *App) writeTestProgress(enabled bool, format string, args ...any) {
	if !enabled {
		return
	}
	_, _ = fmt.Fprintf(a.stderr, format+"\n", args...)
}

type exitCodeError struct {
	code int
}

func (e *exitCodeError) Error() string {
	return fmt.Sprintf("command exited with code %d", e.code)
}

func errWithCode(code int) error {
	return &exitCodeError{code: code}
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.code
	}
	if strings.Contains(err.Error(), "requires --") || strings.Contains(err.Error(), "unknown command") || strings.Contains(err.Error(), "accepts") {
		return 1
	}
	return 2
}

func init() {
	_ = os.Setenv("AIPROBE_CLI", "1")
}
