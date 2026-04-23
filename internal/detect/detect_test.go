package detect

import (
	"testing"

	"github.com/gqcdm/aiprobe/internal/schema"
)

type stubAdapter struct {
	provider schema.Provider
	apiType  string
	result   ProbeResult
	err      error
	called   *int
}

func (s stubAdapter) Name() schema.Provider { return s.provider }
func (s stubAdapter) APIType() string       { return s.apiType }
func (s stubAdapter) Probe(baseURL, apiKey string) (ProbeResult, error) {
	if s.called != nil {
		*s.called = *s.called + 1
	}
	return s.result, s.err
}

func TestDetectionPrecedence(t *testing.T) {
	openAICalled := 0
	geminiCalled := 0
	engine := NewEngine(
		stubAdapter{
			provider: schema.ProviderOpenAICompatible,
			apiType:  "openai-compatible",
			called:   &openAICalled,
			result: ProbeResult{
				Provider:        schema.ProviderOpenAICompatible,
				APIType:         "openai-compatible",
				Confidence:      schema.ConfidenceMedium,
				ModelListSource: "probe",
			},
		},
		stubAdapter{
			provider: schema.ProviderGemini,
			apiType:  "gemini",
			called:   &geminiCalled,
			result: ProbeResult{
				Provider:        schema.ProviderGemini,
				APIType:         "gemini",
				Confidence:      schema.ConfidenceHigh,
				ModelListSource: "probe",
			},
		},
	)

	output, err := engine.Detect(Input{BaseURL: "https://generativelanguage.googleapis.com", APIKey: "test"})
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}

	if output.Detection.Provider != schema.ProviderGemini {
		t.Fatalf("expected gemini provider, got %q", output.Detection.Provider)
	}
	if geminiCalled != 1 || openAICalled != 0 {
		t.Fatalf("expected strong static fingerprint to narrow probing to gemini only, got openai=%d gemini=%d", openAICalled, geminiCalled)
	}
	if output.Diagnostics.Status != "skipped" {
		t.Fatalf("expected diagnostics to stay skipped during detect, got %q", output.Diagnostics.Status)
	}
	if len(output.Detection.Evidence) == 0 {
		t.Fatal("expected detection evidence to be recorded")
	}
	if output.Detection.ModelListSource != "probe" {
		t.Fatalf("expected probe model source, got %q", output.Detection.ModelListSource)
	}
}

func TestAmbiguousDetection(t *testing.T) {
	engine := NewEngine()

	output, err := engine.Detect(Input{BaseURL: "https://anthropic.example.com/gemini", APIKey: "test"})
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}

	if output.Detection.Provider != schema.ProviderUnknown {
		t.Fatalf("expected unknown provider for ambiguous fingerprints, got %q", output.Detection.Provider)
	}
	if len(output.Detection.CandidateProviders) < 2 {
		t.Fatalf("expected multiple candidate providers, got %#v", output.Detection.CandidateProviders)
	}
	if len(output.Errors) == 0 || output.Errors[0].Kind != schema.FailureAmbiguousDetection {
		t.Fatalf("expected ambiguous detection error, got %#v", output.Errors)
	}
	if output.Diagnostics.Status != "skipped" {
		t.Fatalf("expected diagnostics to remain skipped, got %q", output.Diagnostics.Status)
	}
}

func TestProbeResultsPreferOpenAIWhenOthersReturnInvalidResponses(t *testing.T) {
	engine := NewEngine(
		stubAdapter{
			provider: schema.ProviderOpenAICompatible,
			apiType:  "openai-compatible",
			result: ProbeResult{
				Provider:        schema.ProviderOpenAICompatible,
				APIType:         "openai-compatible",
				Confidence:      schema.ConfidenceHigh,
				ModelListSource: "https://example.com/v1/models",
				Models:          []schema.Model{{ID: "gpt-4.1-mini", Retrieved: true}},
			},
		},
		stubAdapter{
			provider: schema.ProviderAnthropic,
			apiType:  "anthropic",
			result: ProbeResult{
				Provider:    schema.ProviderAnthropic,
				APIType:     "anthropic",
				Confidence:  schema.ConfidenceMedium,
				FailureKind: schema.FailureInvalidResponse,
			},
		},
		stubAdapter{
			provider: schema.ProviderGemini,
			apiType:  "gemini",
			result: ProbeResult{
				Provider:    schema.ProviderGemini,
				APIType:     "gemini",
				Confidence:  schema.ConfidenceMedium,
				FailureKind: schema.FailureInvalidResponse,
			},
		},
	)

	output, err := engine.Detect(Input{BaseURL: "https://example.com", APIKey: "test"})
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if output.Detection.Provider != schema.ProviderOpenAICompatible {
		t.Fatalf("expected openai-compatible provider, got %q", output.Detection.Provider)
	}
	if len(output.Models) != 1 {
		t.Fatalf("expected detected models, got %#v", output.Models)
	}
}
