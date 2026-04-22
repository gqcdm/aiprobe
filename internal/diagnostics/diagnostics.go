package diagnostics

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gqcdm/aiprobe/internal/detect"
	"github.com/gqcdm/aiprobe/internal/httpx"
	"github.com/gqcdm/aiprobe/internal/schema"
)

type Input struct {
	BaseURL string
	APIKey  string
	Samples int
}

func Run(adapter detect.Adapter, input Input) schema.DiagnosticsResult {
	if adapter == nil {
		return schema.DiagnosticsResult{Status: "failed", FailureKind: schema.FailureDiagnosticsSkipped}
	}
	latencies := make([]int64, 0, input.Samples)
	status := "ok"
	var failure schema.FailureKind
	reachable := false
	authAccepted := false

	for i := 0; i < input.Samples; i++ {
		started := time.Now()
		result, err := adapter.Probe(input.BaseURL, input.APIKey)
		latencies = append(latencies, time.Since(started).Milliseconds())
		if err != nil {
			status = "failed"
			failure = schema.FailureNetworkError
			continue
		}
		if result.FailureKind == "" || result.FailureKind == schema.FailureReachableNoModelsExposed {
			reachable = true
			authAccepted = true
			continue
		}
		status = "failed"
		failure = result.FailureKind
		if result.FailureKind != schema.FailureAuthFailed {
			reachable = true
		}
	}

	if len(latencies) == 0 {
		return schema.DiagnosticsResult{Status: "failed", FailureKind: schema.FailureNetworkError}
	}

	return schema.DiagnosticsResult{
		Status:       status,
		Reachable:    reachable,
		AuthAccepted: authAccepted,
		LatencyMS:    summarizeLatencies(latencies),
		SampleCount:  input.Samples,
		FailureKind:  clearFailureIfOK(status, failure),
	}
}

func RunModelDiagnostics(provider schema.Provider, baseURL, apiKey string, models []schema.Model, samples int) ([]schema.ModelDiagnostics, []string) {
	if len(models) == 0 {
		return nil, nil
	}
	probe, ok := probeForProvider(provider)
	if !ok {
		results := make([]schema.ModelDiagnostics, 0, len(models))
		for _, model := range models {
			results = append(results, schema.ModelDiagnostics{
				ModelID:     model.ID,
				Label:       model.Label,
				Status:      "skipped",
				Available:   false,
				SampleCount: samples,
				FailureKind: schema.FailureDiagnosticsSkipped,
			})
		}
		return results, []string{fmt.Sprintf("provider %s does not support model-level diagnostics", provider)}
	}

	results := make([]schema.ModelDiagnostics, 0, len(models))
	var warnings []string
	for _, model := range models {
		result, warning := runSingleModelDiagnostics(probe, provider, baseURL, apiKey, model, samples)
		results = append(results, result)
		if warning != "" {
			warnings = append(warnings, warning)
		}
	}
	return results, warnings
}

type streamProbeFunc func(baseURL, apiKey, modelID string) (int64, schema.FailureKind, error)

func probeForProvider(provider schema.Provider) (streamProbeFunc, bool) {
	switch provider {
	case schema.ProviderOpenAICompatible:
		return probeOpenAIModelTTFT, true
	case schema.ProviderAnthropic:
		return probeAnthropicModelTTFT, true
	case schema.ProviderGemini:
		return probeGeminiModelTTFT, true
	default:
		return nil, false
	}
}

func runSingleModelDiagnostics(probe streamProbeFunc, provider schema.Provider, baseURL, apiKey string, model schema.Model, samples int) (schema.ModelDiagnostics, string) {
	label := model.Label
	if label == "" {
		label = model.ID
	}
	result := schema.ModelDiagnostics{
		ModelID:     model.ID,
		Label:       label,
		Status:      "ok",
		Available:   false,
		SampleCount: samples,
	}
	latencies := make([]int64, 0, samples)
	var warning string
	var failure schema.FailureKind

	for i := 0; i < samples; i++ {
		ttft, kind, err := probe(baseURL, apiKey, model.ID)
		if err != nil {
			result.Status = "failed"
			failure = schema.FailureNetworkError
			warning = fmt.Sprintf("model probe failed for %s/%s: %v", provider, model.ID, err)
			continue
		}
		if kind != "" {
			result.Status = "failed"
			failure = kind
			continue
		}
		latencies = append(latencies, ttft)
		result.SuccessCount++
	}

	if len(latencies) > 0 {
		result.Available = true
		result.TTFTMS = summarizeLatencies(latencies)
	}
	if result.SuccessCount == 0 {
		result.Available = false
		if failure == "" {
			failure = schema.FailureNetworkError
		}
	}
	result.FailureKind = clearFailureIfOK(result.Status, failure)
	return result, warning
}

func summarizeLatencies(latencies []int64) schema.LatencyMS {
	if len(latencies) == 0 {
		return schema.LatencyMS{}
	}
	sorted := append([]int64(nil), latencies...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	var total int64
	for _, item := range sorted {
		total += item
	}
	return schema.LatencyMS{
		Min: sorted[0],
		P50: sorted[len(sorted)/2],
		Max: sorted[len(sorted)-1],
		Avg: total / int64(len(sorted)),
	}
}

func clearFailureIfOK(status string, failure schema.FailureKind) schema.FailureKind {
	if status == "ok" {
		return ""
	}
	return failure
}

func probeOpenAIModelTTFT(baseURL, apiKey, modelID string) (int64, schema.FailureKind, error) {
	endpoint, err := httpx.JoinURL(baseURL, "v1", "chat", "completions")
	if err != nil {
		return 0, "", err
	}
	payload := map[string]any{
		"model":       modelID,
		"messages":    []map[string]string{{"role": "user", "content": "Hi"}},
		"stream":      true,
		"max_tokens":  1,
		"temperature": 0,
	}
	headers := map[string]string{
		"Authorization": "Bearer " + apiKey,
		"Accept":        "text/event-stream",
	}
	return probeSSE(endpoint, headers, payload, openAIHasFirstToken)
}

func probeAnthropicModelTTFT(baseURL, apiKey, modelID string) (int64, schema.FailureKind, error) {
	endpoint, err := httpx.JoinURL(baseURL, "v1", "messages")
	if err != nil {
		return 0, "", err
	}
	payload := map[string]any{
		"model":      modelID,
		"messages":   []map[string]string{{"role": "user", "content": "Hi"}},
		"stream":     true,
		"max_tokens": 1,
	}
	headers := map[string]string{
		"x-api-key":         apiKey,
		"anthropic-version": "2023-06-01",
		"Accept":            "text/event-stream",
	}
	return probeSSE(endpoint, headers, payload, anthropicHasFirstToken)
}

func probeGeminiModelTTFT(baseURL, apiKey, modelID string) (int64, schema.FailureKind, error) {
	endpoint, err := httpx.JoinURL(baseURL, "v1beta", "models", modelID+":streamGenerateContent")
	if err != nil {
		return 0, "", err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return 0, "", err
	}
	query := parsed.Query()
	query.Set("alt", "sse")
	parsed.RawQuery = query.Encode()
	payload := map[string]any{
		"contents": []map[string]any{{
			"parts": []map[string]string{{"text": "Hi"}},
		}},
		"generationConfig": map[string]any{
			"maxOutputTokens": 1,
			"temperature":     0,
		},
	}
	headers := map[string]string{
		"x-goog-api-key": apiKey,
		"Accept":         "text/event-stream",
	}
	return probeSSE(parsed.String(), headers, payload, geminiHasFirstToken)
}

func probeSSE(endpoint string, headers map[string]string, payload map[string]any, hasFirstToken func([]byte) bool) (int64, schema.FailureKind, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, "", err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	started := time.Now()
	resp, err := httpx.NewClient(httpx.DefaultDiagnosticsTimeout).Do(req)
	if err != nil {
		return 0, httpx.ClassifyFailure(nil, err), nil
	}
	defer resp.Body.Close()

	if failure := classifyStreamFailure(resp); failure != "" {
		return 0, failure, nil
	}

	reader := bufio.NewReader(resp.Body)
	for {
		dataLine, err := readFirstEventData(reader, started)
		if err != nil {
			return 0, schema.FailureInvalidResponse, nil
		}
		if hasFirstToken([]byte(dataLine)) {
			return time.Since(started).Milliseconds(), "", nil
		}
	}
}

func classifyStreamFailure(resp *http.Response) schema.FailureKind {
	if resp == nil {
		return schema.FailureNetworkError
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return schema.FailureAuthFailed
	case http.StatusTooManyRequests:
		return schema.FailureRateLimited
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return schema.FailureUnsupportedAPI
	}
	if resp.StatusCode >= 400 {
		return schema.FailureInvalidResponse
	}
	return ""
}

func readFirstEventData(reader *bufio.Reader, started time.Time) (string, error) {
	_ = started
	var parts []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			if len(parts) > 0 {
				return strings.Join(parts, "\n"), nil
			}
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		parts = append(parts, payload)
	}
}

func openAIHasFirstToken(data []byte) bool {
	var payload struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return false
	}
	for _, choice := range payload.Choices {
		if strings.TrimSpace(choice.Delta.Content) != "" {
			return true
		}
	}
	return false
}

func anthropicHasFirstToken(data []byte) bool {
	var payload struct {
		Type  string `json:"type"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return false
	}
	return payload.Type == "content_block_delta" && payload.Delta.Type == "text_delta" && strings.TrimSpace(payload.Delta.Text) != ""
}

func geminiHasFirstToken(data []byte) bool {
	var payload struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return false
	}
	for _, candidate := range payload.Candidates {
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				return true
			}
		}
	}
	return false
}
