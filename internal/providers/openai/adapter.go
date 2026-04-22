package openai

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gqcdm/aiprobe/internal/detect"
	"github.com/gqcdm/aiprobe/internal/httpx"
	"github.com/gqcdm/aiprobe/internal/schema"
)

type Adapter struct{}

func New() Adapter { return Adapter{} }

func (Adapter) Name() schema.Provider { return schema.ProviderOpenAICompatible }
func (Adapter) APIType() string       { return "openai-compatible" }

func (Adapter) Probe(baseURL, apiKey string) (detect.ProbeResult, error) {
	endpoint, err := httpx.JoinURL(baseURL, "v1", "models")
	if err != nil {
		return detect.ProbeResult{}, err
	}

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return detect.ProbeResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := httpx.NewClient(httpx.DefaultDetectionTimeout).Do(req)
	if err != nil {
		return detect.ProbeResult{Provider: schema.ProviderOpenAICompatible, APIType: "openai-compatible", Confidence: schema.ConfidenceLow, FailureKind: httpx.ClassifyFailure(nil, err)}, nil
	}
	defer resp.Body.Close()

	result := detect.ProbeResult{
		Provider:        schema.ProviderOpenAICompatible,
		APIType:         "openai-compatible",
		Confidence:      schema.ConfidenceMedium,
		ModelListSource: endpoint,
		Evidence: []schema.Evidence{{Kind: string(detect.FingerprintModelProbe), Source: endpoint, Summary: "queried /v1/models with bearer auth"}},
	}

	if failure := httpx.ClassifyFailure(resp, nil); failure != "" {
		result.FailureKind = failure
		return result, nil
	}

	var payload struct {
		Data []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		result.FailureKind = schema.FailureInvalidResponse
		return result, nil
	}
	if len(payload.Data) == 0 {
		result.FailureKind = schema.FailureReachableNoModelsExposed
		return result, nil
	}

	result.Confidence = schema.ConfidenceHigh
	for _, item := range payload.Data {
		result.Models = append(result.Models, schema.Model{ID: item.ID, Label: item.ID, Family: item.OwnedBy, Retrieved: true, Raw: map[string]any{"object": item.Object, "owned_by": item.OwnedBy}})
	}
	return result, nil
}

var _ detect.Adapter = Adapter{}

func ExampleURL(baseURL string) string {
	url, _ := httpx.JoinURL(baseURL, "v1", "models")
	return url
}

func BearerHeader(key string) string {
	return fmt.Sprintf("Bearer %s", key)
}
