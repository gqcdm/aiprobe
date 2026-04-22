package cli

import (
	"errors"
	"flag"
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
)

type App struct {
	stdout io.Writer
	stderr io.Writer
	engine *detect.Engine
}

func New() *App {
	return &App{
		stdout: app.Stdout,
		stderr: app.Stderr,
		engine: detect.NewEngine(providers.All()...),
	}
}

func (a *App) Run(args []string) error {
	if len(args) == 0 {
		a.printHelp()
		return nil
	}

	switch args[0] {
	case "-h", "--help", "help":
		a.printHelp()
		return nil
	case "detect":
		return a.runDetect(args[1:])
	case "test":
		return a.runTest(args[1:])
	default:
		a.printHelp()
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func (a *App) runDetect(args []string) error {
	if wantsHelp(args) {
		fmt.Fprint(a.stdout, detectHelp)
		return nil
	}

	cmd := flag.NewFlagSet("detect", flag.ContinueOnError)
	cmd.SetOutput(a.stderr)
	baseURL := cmd.String("base-url", "", "API base URL")
	apiKey := cmd.String("api-key", "", "API key")
	format := cmd.String("format", "text", "Output format: text or json")
	if err := cmd.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*baseURL) == "" || strings.TrimSpace(*apiKey) == "" {
		return errors.New("detect requires --base-url and --api-key")
	}

	output, err := a.engine.Detect(detect.Input{BaseURL: *baseURL, APIKey: *apiKey})
	if err != nil {
		return err
	}

	return render.Write(a.stdout, output, *format)
}

func (a *App) runTest(args []string) error {
	if wantsHelp(args) {
		fmt.Fprint(a.stdout, testHelp)
		return nil
	}

	cmd := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetOutput(a.stderr)
	baseURL := cmd.String("base-url", "", "API base URL")
	apiKey := cmd.String("api-key", "", "API key")
	format := cmd.String("format", "text", "Output format: text or json")
	samples := cmd.Int("samples", 3, "Latency sample count")
	if err := cmd.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*baseURL) == "" || strings.TrimSpace(*apiKey) == "" {
		return errors.New("test requires --base-url and --api-key")
	}
	if *samples <= 0 {
		return errors.New("test requires --samples > 0")
	}

	output, err := a.engine.Detect(detect.Input{BaseURL: *baseURL, APIKey: *apiKey})
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
		result := diagnostics.Run(adapter, diagnostics.Input{BaseURL: *baseURL, APIKey: *apiKey, Samples: *samples})
		output.Diagnostics = result
	}

	if err := render.Write(a.stdout, output, *format); err != nil {
		return err
	}

	if output.Diagnostics.Status == "failed" {
		if exitErr, ok := errWithCode(3).(*exitCodeError); ok {
			return exitErr
		}
	}

	return nil
}

func (a *App) printHelp() {
	fmt.Fprint(a.stdout, rootHelp)
}

func wantsHelp(args []string) bool {
	for _, arg := range args {
		if strings.TrimSpace(arg) == "-h" || strings.TrimSpace(arg) == "--help" {
			return true
		}
	}

	return false
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
	if strings.Contains(err.Error(), "requires --") || strings.Contains(err.Error(), "flag provided") {
		return 1
	}
	return 2
}

func init() {
	_ = os.Setenv("AIPROBE_CLI", "1")
}
