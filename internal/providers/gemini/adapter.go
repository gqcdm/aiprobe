package gemini

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/gqcdm/aiprobe/internal/detect"
	"github.com/gqcdm/aiprobe/internal/httpx"
	"github.com/gqcdm/aiprobe/internal/schema"
)

type Adapter struct{}

func New() Adapter { return Adapter{} }

func (Adapter) Name() schema.Provider { return schema.ProviderGemini }
func (Adapter) APIType() string       { return "gemini" }

func (Adapter) Probe(baseURL, apiKey string) (detect.ProbeResult, error) {
	endpoint, err := httpx.JoinURL(baseURL, "v1beta", "models")
	if err != nil {
		return detect.ProbeResult{}, err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return detect.ProbeResult{}, err
	}
	query := parsed.Query()
	query.Set("key", apiKey)
	parsed.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return detect.ProbeResult{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpx.NewClient(httpx.DefaultDetectionTimeout).Do(req)
	if err != nil {
		return detect.ProbeResult{Provider: schema.ProviderGemini, APIType: "gemini", Confidence: schema.ConfidenceLow, FailureKind: httpx.ClassifyFailure(nil, err)}, nil
	}
	defer resp.Body.Close()

	result := detect.ProbeResult{
		Provider:        schema.ProviderGemini,
		APIType:         "gemini",
		Confidence:      schema.ConfidenceMedium,
		ModelListSource: parsed.String(),
		Evidence: []schema.Evidence{{Kind: string(detect.FingerprintModelProbe), Source: parsed.Path, Summary: "queried /v1beta/models with query api key"}},
	}
	if failure := httpx.ClassifyFailure(resp, nil); failure != "" {
		result.FailureKind = failure
		return result, nil
	}

	var payload struct {
		Models []struct {
			Name                       string   `json:"name"`
			DisplayName                string   `json:"displayName"`
			Description                string   `json:"description"`
			InputTokenLimit            int      `json:"inputTokenLimit"`
			OutputTokenLimit           int      `json:"outputTokenLimit"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		result.FailureKind = schema.FailureInvalidResponse
		return result, nil
	}
	if len(payload.Models) == 0 {
		result.FailureKind = schema.FailureReachableNoModelsExposed
		return result, nil
	}

	result.Confidence = schema.ConfidenceHigh
	for _, item := range payload.Models {
		id := strings.TrimPrefix(item.Name, "models/")
		label := item.DisplayName
		if label == "" {
			label = id
		}
		result.Models = append(result.Models, schema.Model{ID: id, Label: label, Family: "gemini", ContextWindow: item.InputTokenLimit, MaxOutputTokens: item.OutputTokenLimit, Capabilities: item.SupportedGenerationMethods, Retrieved: true, Raw: map[string]any{"description": item.Description, "next_page_token": payload.NextPageToken}})
	}
	return result, nil
}

var _ detect.Adapter = Adapter{}
