package diagnostics

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestRunModelDiagnosticsOpenAIWithVersionedBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
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

	result, warnings := RunModelDiagnostics(schema.ProviderOpenAICompatible, server.URL+"/v1", "test-key", []schema.Model{{ID: "gpt-4.1-mini", Label: "gpt-4.1-mini"}}, 1)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(result) != 1 || !result[0].Available || result[0].Status != "ok" {
		t.Fatalf("unexpected diagnostics %#v", result)
	}
}

func TestRunSampleOutputsOpenAITextAndImage(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Test reply from model."}}]}`)
		case "/v1/images/generations":
			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(r.URL.RawQuery, "variant=url") {
				_, _ = fmt.Fprint(w, `{"data":[{"url":"`+server.URL+`/img.png"}]}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"data":[{"b64_json":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a4c8AAAAASUVORK5CYII="}]}`)
		case "/img.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	outputs, warnings := RunSampleOutputs(schema.ProviderOpenAICompatible, server.URL, "test-key", []schema.Model{
		{ID: "gpt-4.1-mini", Label: "gpt-4.1-mini"},
		{ID: "gpt-image-2", Label: "gpt-image-2"},
	})
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(outputs) != 2 {
		t.Fatalf("expected 2 sample outputs, got %d", len(outputs))
	}
	if outputs[0].Kind != "text" || outputs[0].Status != "ok" || !strings.Contains(outputs[0].TextReply, "Test reply") {
		t.Fatalf("unexpected text sample %#v", outputs[0])
	}
	if outputs[1].Kind != "image" || outputs[1].Status != "ok" || outputs[1].ImagePath == "" {
		t.Fatalf("unexpected image sample %#v", outputs[1])
	}
	if _, err := os.Stat(outputs[1].ImagePath); err != nil {
		t.Fatalf("expected image file to exist: %v", err)
	}
	_ = os.Remove(outputs[1].ImagePath)
}

func TestGenerateImageSampleWithURLArtifact(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/generations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"data":[{"url":"`+server.URL+`/artifact.png"}]}`)
		case "/artifact.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	path, warning, failure := generateImageSample(schema.ProviderOpenAICompatible, server.URL, "test-key", "gpt-image-2")
	if warning != "" || failure != "" {
		t.Fatalf("expected success, got warning=%q failure=%q", warning, failure)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected image file to exist: %v", err)
	}
	_ = os.Remove(path)
}

func TestGenerateImageSampleFallsBackToResponsesResult(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/generations":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"error":"unsupported"}`)
		case "/v1/responses":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"output":[{"type":"image_generation_call","result":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a4c8AAAAASUVORK5CYII="}]}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	path, warning, failure := generateImageSample(schema.ProviderOpenAICompatible, server.URL, "test-key", "gpt-image-2")
	if warning != "" || failure != "" {
		t.Fatalf("expected fallback success, got warning=%q failure=%q", warning, failure)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected image file to exist: %v", err)
	}
	_ = os.Remove(path)
}

func TestGenerateImageSampleFallsBackToResponsesURL(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/generations":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"error":"unsupported"}`)
		case "/v1/responses":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"output":[{"type":"image_generation_call","image_url":"`+server.URL+`/resp.png"}]}`)
		case "/resp.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	path, warning, failure := generateImageSample(schema.ProviderOpenAICompatible, server.URL, "test-key", "gpt-image-2")
	if warning != "" || failure != "" {
		t.Fatalf("expected fallback success, got warning=%q failure=%q", warning, failure)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected image file to exist: %v", err)
	}
	_ = os.Remove(path)
}

func TestRunSampleOutputsUnsupportedProvider(t *testing.T) {
	outputs, warnings := RunSampleOutputs(schema.ProviderAnthropic, "https://example.com", "test-key", []schema.Model{{ID: "claude-3-7-sonnet"}})
	if len(outputs) != 1 {
		t.Fatalf("expected 1 sample output, got %d", len(outputs))
	}
	if outputs[0].Status != "failed" || outputs[0].FailureKind != schema.FailureDiagnosticsSkipped {
		t.Fatalf("unexpected unsupported provider output %#v", outputs[0])
	}
	if len(warnings) != 1 {
		t.Fatalf("expected warning for unsupported provider, got %v", warnings)
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

func TestPostJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()

	body, failure, err := postJSON(server.URL, map[string]string{"Authorization": "Bearer test"}, map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("postJSON returned error: %v", err)
	}
	if failure != "" {
		t.Fatalf("expected no failure, got %s", failure)
	}
	var payload map[string]bool
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("expected valid body, got error: %v", err)
	}
	if !payload["ok"] {
		t.Fatalf("unexpected payload %#v", payload)
	}
}
