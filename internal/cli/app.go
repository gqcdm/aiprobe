package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
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
	a.root.SetOut(a.stdout)
	a.root.SetErr(a.stderr)
	a.root.SetArgs(args)
	return a.root.Execute()
}

func (a *App) newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           app.Name,
		Short:         "Auto-detect AI API providers and diagnostics",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.CompletionOptions.DisableDefaultCmd = false
	cmd.AddCommand(a.newDetectCmd(), a.newTestCmd())
	return cmd
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

			output, err := a.engine.Detect(detect.Input{BaseURL: baseURL, APIKey: apiKey})
			if err != nil {
				return err
			}

			adapter := providers.ByProvider(output.Detection.Provider)
			if adapter == nil {
				output.Diagnostics = schema.DiagnosticsResult{
					Status:      "failed",
					FailureKind: schema.FailureDiagnosticsSkipped,
				}
			} else {
				output.Diagnostics = diagnostics.Run(adapter, diagnostics.Input{BaseURL: baseURL, APIKey: apiKey, Samples: samples})
				modelDiagnostics, warnings := diagnostics.RunModelDiagnostics(output.Detection.Provider, baseURL, apiKey, output.Models, samples)
				output.ModelDiagnostics = modelDiagnostics
				output.Warnings = append(output.Warnings, warnings...)
			}

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
