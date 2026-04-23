package anthropic

import (
	"encoding/json"
	"net/http"

	"github.com/gqcdm/aiprobe/internal/detect"
	"github.com/gqcdm/aiprobe/internal/httpx"
	"github.com/gqcdm/aiprobe/internal/schema"
)

const versionHeader = "2023-06-01"

type Adapter struct{}

func New() Adapter { return Adapter{} }

func (Adapter) Name() schema.Provider { return schema.ProviderAnthropic }
func (Adapter) APIType() string       { return "anthropic" }

func (Adapter) Probe(baseURL, apiKey string) (detect.ProbeResult, error) {
	endpoint, err := httpx.JoinVersionedURL(baseURL, "v1", "models")
	if err != nil {
		return detect.ProbeResult{}, err
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return detect.ProbeResult{}, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", versionHeader)
	req.Header.Set("Accept", "application/json")

	resp, err := httpx.NewClient(httpx.DefaultDetectionTimeout).Do(req)
	if err != nil {
		return detect.ProbeResult{Provider: schema.ProviderAnthropic, APIType: "anthropic", Confidence: schema.ConfidenceLow, FailureKind: httpx.ClassifyFailure(nil, err)}, nil
	}
	defer resp.Body.Close()

	result := detect.ProbeResult{
		Provider:        schema.ProviderAnthropic,
		APIType:         "anthropic",
		Confidence:      schema.ConfidenceMedium,
		ModelListSource: endpoint,
		Evidence: []schema.Evidence{{Kind: string(detect.FingerprintModelProbe), Source: endpoint, Summary: "queried /v1/models with anthropic headers"}},
	}
	if failure := httpx.ClassifyFailure(resp, nil); failure != "" {
		result.FailureKind = failure
		return result, nil
	}

	var payload struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
		FirstID string `json:"first_id"`
		LastID  string `json:"last_id"`
		HasMore bool   `json:"has_more"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		result.FailureKind = schema.FailureInvalidResponse
		return result, nil
	}
	if len(payload.Data) == 0 {
		result.FailureKind = schema.FailureReachableNoModelsExposed
		return result, nil
	}
	if payload.FirstID == "" && payload.LastID == "" {
		result.FailureKind = schema.FailureInvalidResponse
		return result, nil
	}

	result.Confidence = schema.ConfidenceHigh
	for _, item := range payload.Data {
		label := item.DisplayName
		if label == "" {
			label = item.ID
		}
		result.Models = append(result.Models, schema.Model{ID: item.ID, Label: label, Family: "anthropic", Retrieved: true, Raw: map[string]any{"first_id": payload.FirstID, "last_id": payload.LastID, "has_more": payload.HasMore}})
	}
	return result, nil
}

var _ detect.Adapter = Adapter{}
