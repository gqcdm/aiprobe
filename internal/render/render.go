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
		_, err := fmt.Fprintf(w, "Provider: %s\nAPI Type: %s\nConfidence: %s\nNormalized URL: %s\nModels: %d\nDiagnostics: %s\n", output.Detection.Provider, output.Detection.APIType, output.Detection.Confidence, output.NormalizedBaseURL, len(output.Models), output.Diagnostics.Status)
		return err
	}
	if format != "json" {
		return fmt.Errorf("unsupported format %q", format)
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
