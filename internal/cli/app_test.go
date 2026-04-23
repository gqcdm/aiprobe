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
	app := New()
	app.stdout = stdout
	app.stderr = stderr

	if err := app.Run([]string{"--help"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "detect") || !strings.Contains(output, "test") || !strings.Contains(output, "completion") {
		t.Fatalf("expected help output to mention detect, test, and completion, got %q", output)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
}

func TestDetectHelp(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := New()
	app.stdout = stdout
	app.stderr = stderr

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
	app := New()
	app.stdout = stdout
	app.stderr = stderr

	err := app.Run([]string{"nope"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected cobra with SilenceUsage/SilenceErrors to avoid direct output, got stdout=%q stderr=%q", stdout.String(), stderr.String())
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
	app.root = app.newRootCmd()

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

func TestCompletionCommand(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := New()
	app.stdout = stdout
	app.stderr = stderr

	err := app.Run([]string{"completion", "fish"})
	if err != nil {
		t.Fatalf("completion command failed: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "complete -c aiprobe") {
		t.Fatalf("expected fish completion output, got %q", output)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestTestCommandIncludesModelDiagnostics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			if got := r.Header.Get("Authorization"); got == "Bearer test-key" {
				_, _ = fmt.Fprint(w, `{"data":[{"id":"gpt-4.1-mini","object":"model","owned_by":"openai"}]}`)
				return
			}
			if got := r.Header.Get("x-api-key"); got == "test-key" {
				_, _ = fmt.Fprint(w, `{"data":[]}`)
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"error":"unauthorized"}`)
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("expected flusher")
			}
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
			flusher.Flush()
		case "/v1beta/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"models":[]}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := New()
	app.stdout = stdout
	app.stderr = stderr

	err := app.Run([]string{"test", "--base-url", server.URL, "--api-key", "test-key", "--samples", "1", "--format", "json"})
	if err != nil {
		t.Fatalf("test command failed: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, `"model_diagnostics"`) || !strings.Contains(output, `"model_id": "gpt-4.1-mini"`) {
		t.Fatalf("expected model diagnostics output, got %s", output)
	}
	if !strings.Contains(output, `"ttft_ms"`) {
		t.Fatalf("expected ttft output, got %s", output)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestTestShortcutUsesPositionalBaseURLAndAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			if got := r.Header.Get("Authorization"); got == "Bearer test-key" {
				_, _ = fmt.Fprint(w, `{"data":[{"id":"gpt-4.1-mini","object":"model","owned_by":"openai"}]}`)
				return
			}
			if got := r.Header.Get("x-api-key"); got == "test-key" {
				_, _ = fmt.Fprint(w, `{"data":[]}`)
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"error":"unauthorized"}`)
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("expected flusher")
			}
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
			flusher.Flush()
		case "/v1beta/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"models":[]}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := New()
	app.stdout = stdout
	app.stderr = stderr

	err := app.Run([]string{"-t", server.URL, "test-key", "--samples", "1", "--format", "json"})
	if err != nil {
		t.Fatalf("shortcut test command failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, `"model_diagnostics"`) || !strings.Contains(output, `"ttft_ms"`) {
		t.Fatalf("expected model diagnostics ttft output, got %s", output)
	}
	if !strings.Contains(output, `"api_key_hint": "te****ey"`) {
		t.Fatalf("expected redacted api key hint, got %s", output)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestTestShortcutRequiresPositionalValues(t *testing.T) {
	app := New()
	err := app.Run([]string{"-t"})
	if err == nil {
		t.Fatal("expected -t shortcut to require values")
	}
	if ExitCode(err) != 1 {
		t.Fatalf("expected exit code 1, got %d", ExitCode(err))
	}
	if !strings.Contains(err.Error(), "test requires --base-url and --api-key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetectUnknownProvider(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := &App{stdout: stdout, stderr: stderr, engine: detect.NewEngine()}
	app.root = app.newRootCmd()

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
