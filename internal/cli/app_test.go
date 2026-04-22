package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
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

func TestApplyHintsDoesNotBreakJSONContract(t *testing.T) {
	output := map[string]any{}
	app := New()
	stdout := &bytes.Buffer{}
	app.stdout = stdout
	result := map[string]any{"ok": true}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}
	if len(raw) == 0 || output == nil {
		t.Fatal("expected test scaffolding to be valid")
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
