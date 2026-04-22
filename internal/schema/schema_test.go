package schema

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONContract(t *testing.T) {
	output := Output{
		Input:             NewInputSummary(" https://example.com ", "sk-test-123456"),
		NormalizedBaseURL: "https://example.com",
		Detection: DetectionResult{
			Provider:          ProviderOpenAICompatible,
			APIType:           "openai-compatible",
			CompatibilityMode: "native",
			Confidence:        ConfidenceHigh,
			ModelListSource:   "probe",
			Evidence: []Evidence{{Kind: "header", Source: "response", Summary: "openai-style response"}},
		},
		Models: []Model{{ID: "gpt-4.1", Retrieved: true, Inferred: false}},
		Diagnostics: DiagnosticsResult{
			Status:       "skipped",
			Reachable:    false,
			AuthAccepted: false,
			LatencyMS:    LatencyMS{},
			SampleCount:  0,
			FailureKind:  FailureDiagnosticsSkipped,
		},
		ModelDiagnostics: []ModelDiagnostics{{ModelID: "gpt-4.1", Status: "ok", Available: true, TTFTMS: LatencyMS{Min: 10, P50: 10, Max: 10, Avg: 10}, SampleCount: 1, SuccessCount: 1}},
		Errors:   []ErrorDetail{{Code: "none", Message: "", Kind: ""}},
		Warnings: []string{"safe warning"},
	}

	raw, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	serialized := string(raw)
	for _, field := range []string{"\"input\"", "\"normalized_base_url\"", "\"detection\"", "\"models\"", "\"diagnostics\"", "\"model_diagnostics\"", "\"errors\"", "\"warnings\""} {
		if !strings.Contains(serialized, field) {
			t.Fatalf("expected serialized output to contain %s, got %s", field, serialized)
		}
	}

	if output.Detection.Confidence != ConfidenceHigh && output.Detection.Confidence != ConfidenceMedium && output.Detection.Confidence != ConfidenceLow {
		t.Fatalf("unexpected confidence enum: %q", output.Detection.Confidence)
	}

	if output.Detection.Provider != ProviderOpenAICompatible && output.Detection.Provider != ProviderAnthropic && output.Detection.Provider != ProviderGemini && output.Detection.Provider != ProviderUnknown {
		t.Fatalf("unexpected provider enum: %q", output.Detection.Provider)
	}
}

func TestRedactsAPIKey(t *testing.T) {
	output := Output{
		Input:             NewInputSummary("https://example.com", "sk-test-123456"),
		NormalizedBaseURL: "https://example.com",
		Detection: DetectionResult{
			Provider:   ProviderUnknown,
			APIType:    "unknown",
			Confidence: ConfidenceLow,
		},
		Diagnostics: DiagnosticsResult{Status: "failed", FailureKind: FailureAuthFailed},
		Errors:      []ErrorDetail{{Code: "auth_failed", Message: "upstream rejected key sk-test-123456", Kind: FailureAuthFailed}},
		Warnings:    []string{"api-key: sk-test-123456 should never be printed", "test-secret should not appear"},
	}

	raw, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	serialized := string(raw)
	for _, secret := range []string{"sk-test-123456", "test-secret"} {
		if strings.Contains(serialized, secret) {
			t.Fatalf("expected serialized output to redact %q, got %s", secret, serialized)
		}
	}

	if !strings.Contains(serialized, "****") && !strings.Contains(serialized, "[redacted]") {
		t.Fatalf("expected serialized output to include redaction markers, got %s", serialized)
	}
}
