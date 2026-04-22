package providers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gqcdm/aiprobe/internal/detect"
	"github.com/gqcdm/aiprobe/internal/providers/anthropic"
	"github.com/gqcdm/aiprobe/internal/providers/gemini"
	"github.com/gqcdm/aiprobe/internal/providers/openai"
	"github.com/gqcdm/aiprobe/internal/schema"
)

func TestOpenAICompatibleAdapter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[{"id":"gpt-4.1-mini","object":"model","owned_by":"openai"}]}`)
	}))
	defer server.Close()

	result, err := openai.New().Probe(server.URL, "test-key")
	if err != nil {
		t.Fatalf("probe returned error: %v", err)
	}
	if result.Provider != schema.ProviderOpenAICompatible || len(result.Models) != 1 || result.Confidence != schema.ConfidenceHigh {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestOpenAICompatibleAuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"error":"unauthorized"}`)
	}))
	defer server.Close()

	result, err := openai.New().Probe(server.URL, "secret-value")
	if err != nil {
		t.Fatalf("probe returned error: %v", err)
	}
	if result.FailureKind != schema.FailureAuthFailed {
		t.Fatalf("expected auth failure, got %#v", result)
	}
}

func TestAnthropicAdapter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" || r.Header.Get("anthropic-version") == "" {
			t.Fatalf("missing anthropic headers")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[{"id":"claude-3-7-sonnet","display_name":"Claude Sonnet"}],"has_more":false}`)
	}))
	defer server.Close()

	result, err := anthropic.New().Probe(server.URL, "test-key")
	if err != nil {
		t.Fatalf("probe returned error: %v", err)
	}
	if result.Provider != schema.ProviderAnthropic || len(result.Models) != 1 {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestAnthropicMissingVersionHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("anthropic-version") == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"error":"missing version"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[]}`)
	}))
	defer server.Close()

	result, err := anthropic.New().Probe(strings.Replace(server.URL, "http://", "http://", 1), "test-key")
	if err != nil {
		t.Fatalf("probe returned error: %v", err)
	}
	if result.FailureKind != schema.FailureReachableNoModelsExposed {
		t.Fatalf("unexpected failure %#v", result)
	}
}

func TestGeminiAdapter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models" || r.URL.Query().Get("key") != "test-key" {
			t.Fatalf("unexpected gemini request %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"models":[{"name":"models/gemini-1.5-pro","displayName":"Gemini 1.5 Pro","inputTokenLimit":1048576,"outputTokenLimit":8192,"supportedGenerationMethods":["generateContent"]}]}`)
	}))
	defer server.Close()

	result, err := gemini.New().Probe(server.URL, "test-key")
	if err != nil {
		t.Fatalf("probe returned error: %v", err)
	}
	if result.Provider != schema.ProviderGemini || len(result.Models) != 1 {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestGeminiInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"models":`)
	}))
	defer server.Close()

	result, err := gemini.New().Probe(server.URL, "test-key")
	if err != nil {
		t.Fatalf("probe returned error: %v", err)
	}
	if result.FailureKind != schema.FailureInvalidResponse {
		t.Fatalf("expected invalid response, got %#v", result)
	}
}

func TestProviderRegistry(t *testing.T) {
	all := All()
	if len(all) != 3 {
		t.Fatalf("expected 3 adapters, got %d", len(all))
	}
	if ByProvider(schema.ProviderUnknown) != nil {
		t.Fatal("expected nil for unknown provider")
	}
	if ByProvider(schema.ProviderAnthropic) == nil {
		t.Fatal("expected anthropic adapter")
	}
}

func TestDetectionWithAdapters(t *testing.T) {
	engine := detect.NewEngine(All()...)
	if _, err := engine.Detect(detect.Input{BaseURL: "https://example.com/v1", APIKey: "x"}); err != nil {
		t.Fatalf("detect returned error: %v", err)
	}
}
