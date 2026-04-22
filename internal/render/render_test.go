package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gqcdm/aiprobe/internal/schema"
)

func TestRenderJSONAndText(t *testing.T) {
	output := schema.Output{NormalizedBaseURL: "https://example.com", Detection: schema.DetectionResult{Provider: schema.ProviderUnknown, APIType: "unknown", Confidence: schema.ConfidenceLow}, Diagnostics: schema.DiagnosticsResult{Status: "skipped"}}
	text := &bytes.Buffer{}
	if err := Write(text, output, "text"); err != nil {
		t.Fatalf("text render failed: %v", err)
	}
	if !strings.Contains(text.String(), "Provider:") {
		t.Fatalf("unexpected text output %q", text.String())
	}
	jsonBuffer := &bytes.Buffer{}
	if err := Write(jsonBuffer, output, "json"); err != nil {
		t.Fatalf("json render failed: %v", err)
	}
	if !strings.Contains(jsonBuffer.String(), "normalized_base_url") {
		t.Fatalf("unexpected json output %q", jsonBuffer.String())
	}
}
