package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gqcdm/aiprobe/internal/detect"
	"github.com/gqcdm/aiprobe/internal/schema"
)

func TestRunRootHelp(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := &App{stdout: stdout, stderr: stderr}

	if err := app.Run([]string{"--help"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "detect") || !strings.Contains(output, "test") {
		t.Fatalf("expected help output to mention detect and test, got %q", output)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
}

func TestDetectHelp(t *testing.T) {
	stdout := &bytes.Buffer{}
	app := &App{stdout: stdout, stderr: &bytes.Buffer{}, engine: nil}

	if err := app.Run([]string{"detect", "--help"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "--base-url") {
		t.Fatalf("expected detect help, got %q", stdout.String())
	}
}

func TestDetectRequiresFlags(t *testing.T) {
	app := New()
	err := app.Run([]string{"detect"})
	if err == nil {
		t.Fatal("expected detect to require flags")
	}
	if ExitCode(err) != 1 {
		t.Fatalf("expected exit code 1, got %d", ExitCode(err))
	}
}

func TestUnknownSubcommandFailsCleanly(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := &App{stdout: stdout, stderr: stderr}

	err := app.Run([]string{"nope"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown subcommand") {
		t.Fatalf("expected unknown subcommand error, got %v", err)
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("expected help output on stdout, got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr writes from App.Run, got %q", stderr.String())
	}
}

func TestDetectCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[{"id":"gpt-4.1-mini","object":"model","owned_by":"openai"}]}`)
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := &App{stdout: stdout, stderr: stderr, engine: detect.NewEngine(stubCLIAdapter{})}

	err := app.Run([]string{"detect", "--base-url", server.URL, "--api-key", "test-key", "--format", "json"})
	if err != nil {
		t.Fatalf("detect command failed: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, `"provider": "openai-compatible"`) {
		t.Fatalf("expected openai-compatible provider, got %s", output)
	}
	if !strings.Contains(output, `"id": "gpt-4.1-mini"`) {
		t.Fatalf("expected model id in output, got %s", output)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestDetectUnknownProvider(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := &App{stdout: stdout, stderr: stderr, engine: detect.NewEngine()}

	err := app.Run([]string{"detect", "--base-url", "https://example.invalid", "--api-key", "test-key", "--format", "json"})
	if err != nil {
		t.Fatalf("detect command failed: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, `"provider": "unknown"`) {
		t.Fatalf("expected unknown provider output, got %s", output)
	}
}

type stubCLIAdapter struct{}

func (stubCLIAdapter) Name() schema.Provider { return schema.ProviderOpenAICompatible }
func (stubCLIAdapter) APIType() string       { return "openai-compatible" }
func (stubCLIAdapter) Probe(baseURL, apiKey string) (detect.ProbeResult, error) {
	return detect.ProbeResult{
		Provider:        schema.ProviderOpenAICompatible,
		APIType:         "openai-compatible",
		Confidence:      schema.ConfidenceHigh,
		ModelListSource: baseURL + "/v1/models",
		Evidence:        []schema.Evidence{{Kind: "model_probe", Source: "/v1/models", Summary: "stub probe"}},
		Models:          []schema.Model{{ID: "gpt-4.1-mini", Label: "gpt-4.1-mini", Retrieved: true}},
	}, nil
}
