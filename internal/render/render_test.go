package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gqcdm/aiprobe/internal/schema"
)

func TestRenderJSONAndText(t *testing.T) {
	output := schema.Output{
		NormalizedBaseURL: "https://example.com",
		Detection:         schema.DetectionResult{Provider: schema.ProviderUnknown, APIType: "unknown", Confidence: schema.ConfidenceLow},
		Diagnostics:       schema.DiagnosticsResult{Status: "ok", SampleCount: 1, LatencyMS: schema.LatencyMS{Min: 12, P50: 12, Max: 12, Avg: 12}},
		ModelDiagnostics: []schema.ModelDiagnostics{{ModelID: "gpt-4.1-mini", Status: "ok", TTFTMS: schema.LatencyMS{Min: 120, P50: 120, Max: 120, Avg: 120}}},
		SampleOutputs:    []schema.SampleOutput{{ModelID: "gpt-4.1-mini", Kind: "text", Status: "ok", TextReply: "hello from sample output"}},
	}
	text := &bytes.Buffer{}
	if err := Write(text, output, "text"); err != nil {
		t.Fatalf("text render failed: %v", err)
	}
	if !strings.Contains(text.String(), "Provider:") || !strings.Contains(text.String(), "Endpoint Latency") || !strings.Contains(text.String(), "Sample Outputs:") {
		t.Fatalf("unexpected text output %q", text.String())
	}
	jsonBuffer := &bytes.Buffer{}
	if err := Write(jsonBuffer, output, "json"); err != nil {
		t.Fatalf("json render failed: %v", err)
	}
	if !strings.Contains(jsonBuffer.String(), "normalized_base_url") || !strings.Contains(jsonBuffer.String(), "sample_outputs") {
		t.Fatalf("unexpected json output %q", jsonBuffer.String())
	}
}
