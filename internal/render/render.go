package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gqcdm/aiprobe/internal/schema"
)

func Write(w io.Writer, output schema.Output, format string) error {
	format = strings.TrimSpace(strings.ToLower(format))
	if format == "" || format == "text" {
		if _, err := fmt.Fprintf(w, "Provider: %s\nAPI Type: %s\nConfidence: %s\nNormalized URL: %s\nModels: %d\nDiagnostics: %s\nModel Diagnostics: %d\n", output.Detection.Provider, output.Detection.APIType, output.Detection.Confidence, output.NormalizedBaseURL, len(output.Models), output.Diagnostics.Status, len(output.ModelDiagnostics)); err != nil {
			return err
		}
		if output.Diagnostics.SampleCount > 0 {
			if _, err := fmt.Fprintf(w, "Endpoint Latency (ms): min=%d p50=%d max=%d avg=%d\n", output.Diagnostics.LatencyMS.Min, output.Diagnostics.LatencyMS.P50, output.Diagnostics.LatencyMS.Max, output.Diagnostics.LatencyMS.Avg); err != nil {
				return err
			}
		}
		if len(output.ModelDiagnostics) > 0 {
			if _, err := fmt.Fprintln(w, "Model Latencies:"); err != nil {
				return err
			}
			for _, item := range output.ModelDiagnostics {
				label := firstNonEmpty(item.Label, item.ModelID)
				if item.Status == "ok" {
					if _, err := fmt.Fprintf(w, "- %s: ttft_ms min=%d p50=%d max=%d avg=%d\n", label, item.TTFTMS.Min, item.TTFTMS.P50, item.TTFTMS.Max, item.TTFTMS.Avg); err != nil {
						return err
					}
					continue
				}
				if _, err := fmt.Fprintf(w, "- %s: %s (%s)\n", label, item.Status, item.FailureKind); err != nil {
					return err
				}
			}
		}
		if len(output.SampleOutputs) > 0 {
			if _, err := fmt.Fprintln(w, "Sample Outputs:"); err != nil {
				return err
			}
			for _, item := range output.SampleOutputs {
				label := firstNonEmpty(item.Label, item.ModelID)
				switch item.Kind {
				case "image":
					if item.Status == "ok" {
						if _, err := fmt.Fprintf(w, "- %s [image]: %s\n", label, item.ImagePath); err != nil {
							return err
						}
					} else {
						if _, err := fmt.Fprintf(w, "- %s [image]: %s (%s)\n", label, item.Status, item.FailureKind); err != nil {
							return err
						}
					}
				default:
					if item.Status == "ok" {
						if _, err := fmt.Fprintf(w, "- %s [text]: %s\n", label, truncate(item.TextReply, 120)); err != nil {
							return err
						}
					} else {
						if _, err := fmt.Fprintf(w, "- %s [text]: %s (%s)\n", label, item.Status, item.FailureKind); err != nil {
							return err
						}
					}
				}
			}
		}
		_, err := fmt.Fprintln(w)
		return err
	}
	if format != "json" {
		return fmt.Errorf("unsupported format %q", format)
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "..."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
