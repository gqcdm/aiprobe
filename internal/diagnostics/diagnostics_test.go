package diagnostics

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gqcdm/aiprobe/internal/schema"
)

func TestRunModelDiagnosticsOpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"data":[{"id":"gpt-4.1-mini","object":"model","owned_by":"openai"}]}`)
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("expected flusher")
			}
			_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
			flusher.Flush()
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	result, warnings := RunModelDiagnostics(schema.ProviderOpenAICompatible, server.URL, "test-key", []schema.Model{{ID: "gpt-4.1-mini", Label: "gpt-4.1-mini"}}, 2)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 model result, got %d", len(result))
	}
	if !result[0].Available || result[0].Status != "ok" {
		t.Fatalf("unexpected diagnostics %#v", result[0])
	}
	if result[0].SampleCount != 2 || result[0].SuccessCount != 2 {
		t.Fatalf("unexpected counts %#v", result[0])
	}
}

func TestRunModelDiagnosticsUnsupportedProvider(t *testing.T) {
	result, warnings := RunModelDiagnostics(schema.ProviderUnknown, "https://example.com", "test-key", []schema.Model{{ID: "x"}}, 1)
	if len(result) != 1 {
		t.Fatalf("expected 1 model result, got %d", len(result))
	}
	if result[0].FailureKind != schema.FailureDiagnosticsSkipped || result[0].Status != "skipped" {
		t.Fatalf("unexpected result %#v", result[0])
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "does not support model-level diagnostics") {
		t.Fatalf("unexpected warnings %v", warnings)
	}
}

func TestReadFirstEventData(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("event: message\ndata: {\"x\":1}\n\n"))
	line, err := readFirstEventData(reader, time.Now())
	if err != nil {
		t.Fatalf("readFirstEventData returned error: %v", err)
	}
	if line != `{"x":1}` {
		t.Fatalf("unexpected line %q", line)
	}
}
