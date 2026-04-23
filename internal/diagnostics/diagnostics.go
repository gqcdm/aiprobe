package diagnostics

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"path/filepath"
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

func RunSampleOutputs(provider schema.Provider, baseURL, apiKey string, models []schema.Model) ([]schema.SampleOutput, []string) {
	if len(models) == 0 {
		return nil, nil
	}

	outputs := make([]schema.SampleOutput, 0, len(models))
	var warnings []string
	for _, model := range models {
		result := schema.SampleOutput{
			ModelID: model.ID,
			Label:   firstNonEmpty(model.Label, model.ID),
			Status:  "skipped",
		}

		if isImageModel(model) {
			result.Kind = "image"
			path, warning, failure := generateImageSample(provider, baseURL, apiKey, model.ID)
			if path != "" {
				result.Status = "ok"
				result.ImagePath = path
			} else {
				result.Status = "failed"
				result.FailureKind = failure
				result.Warning = warning
			}
			if warning != "" {
				warnings = append(warnings, warning)
			}
			outputs = append(outputs, result)
			continue
		}

		result.Kind = "text"
		text, warning, failure := generateTextSample(provider, baseURL, apiKey, model.ID)
		if text != "" {
			result.Status = "ok"
			result.TextReply = text
		} else {
			result.Status = "failed"
			result.FailureKind = failure
			result.Warning = warning
		}
		if warning != "" {
			warnings = append(warnings, warning)
		}
		outputs = append(outputs, result)
	}

	return outputs, warnings
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
	endpoint, err := httpx.JoinVersionedURL(baseURL, "v1", "chat", "completions")
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
	endpoint, err := httpx.JoinVersionedURL(baseURL, "v1", "messages")
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
	endpoint, err := httpx.JoinVersionedURL(baseURL, "v1beta", "models", modelID+":streamGenerateContent")
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

func generateTextSample(provider schema.Provider, baseURL, apiKey, modelID string) (string, string, schema.FailureKind) {
	if provider != schema.ProviderOpenAICompatible {
		warning := fmt.Sprintf("sample text generation is not implemented for provider %s/%s", provider, modelID)
		return "", warning, schema.FailureDiagnosticsSkipped
	}

	endpoint, err := httpx.JoinVersionedURL(baseURL, "v1", "chat", "completions")
	if err != nil {
		warning := fmt.Sprintf("sample text generation failed for %s: %v", modelID, err)
		return "", warning, schema.FailureNetworkError
	}

	payload := map[string]any{
		"model":       modelID,
		"messages":    []map[string]string{{"role": "user", "content": "Reply in one short sentence to confirm the model works."}},
		"max_tokens":  60,
		"temperature": 0,
	}
	headers := map[string]string{
		"Authorization": "Bearer " + apiKey,
		"Accept":        "application/json",
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	body, failure, err := postJSON(endpoint, headers, payload)
	if err != nil {
		warning := fmt.Sprintf("sample text generation failed for %s: %v", modelID, err)
		return "", warning, schema.FailureNetworkError
	}
	if failure != "" {
		warning := fmt.Sprintf("sample text generation failed for %s: %s", modelID, failure)
		return "", warning, failure
	}
	if err := json.Unmarshal(body, &response); err != nil {
		warning := fmt.Sprintf("sample text generation returned invalid response for %s", modelID)
		return "", warning, schema.FailureInvalidResponse
	}
	for _, choice := range response.Choices {
		content := strings.TrimSpace(choice.Message.Content)
		if content != "" {
			return content, "", ""
		}
	}
	warning := fmt.Sprintf("sample text generation returned no content for %s", modelID)
	return "", warning, schema.FailureInvalidResponse
}

func generateImageSample(provider schema.Provider, baseURL, apiKey, modelID string) (string, string, schema.FailureKind) {
	if provider != schema.ProviderOpenAICompatible {
		warning := fmt.Sprintf("sample image generation is not implemented for provider %s/%s", provider, modelID)
		return "", warning, schema.FailureDiagnosticsSkipped
	}

	endpoint, err := httpx.JoinVersionedURL(baseURL, "v1", "images", "generations")
	if err != nil {
		warning := fmt.Sprintf("sample image generation failed for %s: %v", modelID, err)
		return "", warning, schema.FailureNetworkError
	}

	payload := map[string]any{
		"model":  modelID,
		"prompt": "Generate a simple blue square on a white background.",
		"size":   "256x256",
	}
	headers := map[string]string{
		"Authorization": "Bearer " + apiKey,
		"Accept":        "application/json",
	}

	var response struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	body, failure, err := postJSONWithTimeout(endpoint, headers, payload, 60*time.Second)
	if err != nil {
		warning := fmt.Sprintf("sample image generation failed for %s: %v", modelID, err)
		return "", warning, schema.FailureNetworkError
	}
	if failure != "" {
		if failure == schema.FailureInvalidResponse {
			payload["response_format"] = "b64_json"
			body, failure, err = postJSONWithTimeout(endpoint, headers, payload, 60*time.Second)
			if err != nil {
				warning := fmt.Sprintf("sample image generation failed for %s: %v", modelID, err)
				return "", warning, schema.FailureNetworkError
			}
			if failure == "" {
				goto parseImageResponse
			}
		}
		if path, ok := tryResponsesImageFallback(baseURL, apiKey, modelID); ok {
			return path, "", ""
		}
		warning := fmt.Sprintf("sample image generation failed for %s: %s", modelID, failure)
		return "", warning, failure
	}

parseImageResponse:
	if err := json.Unmarshal(body, &response); err != nil {
		warning := fmt.Sprintf("sample image generation returned invalid response for %s", modelID)
		return "", warning, schema.FailureInvalidResponse
	}
	if len(response.Data) == 0 {
		warning := fmt.Sprintf("sample image generation returned no data for %s", modelID)
		return "", warning, schema.FailureInvalidResponse
	}

	if strings.TrimSpace(response.Data[0].B64JSON) != "" {
		decoded, err := base64.StdEncoding.DecodeString(response.Data[0].B64JSON)
		if err != nil {
			warning := fmt.Sprintf("sample image generation returned invalid base64 for %s", modelID)
			return "", warning, schema.FailureInvalidResponse
		}
		path, err := writeTempImageFile(modelID, decoded)
		if err != nil {
			warning := fmt.Sprintf("sample image generation failed to write temp file for %s: %v", modelID, err)
			return "", warning, schema.FailureNetworkError
		}
		return path, "", ""
	}

	imageURL := firstNonEmpty(response.Data[0].URL)
	if strings.TrimSpace(imageURL) != "" {
		path, err := downloadTempImageFile(modelID, imageURL)
		if err != nil {
			warning := fmt.Sprintf("sample image download failed for %s: %v", modelID, err)
			return "", warning, schema.FailureNetworkError
		}
		return path, "", ""
	}

	if path, ok := tryResponsesImageFallback(baseURL, apiKey, modelID); ok {
		return path, "", ""
	}

	warning := fmt.Sprintf("sample image generation returned no image artifact for %s", modelID)
	return "", warning, schema.FailureInvalidResponse
}

func tryResponsesImageFallback(baseURL, apiKey, modelID string) (string, bool) {
	endpoint, err := httpx.JoinVersionedURL(baseURL, "v1", "responses")
	if err != nil {
		return "", false
	}
	payload := map[string]any{
		"model": modelID,
		"input": "Generate a simple blue square on a white background.",
		"tools": []map[string]string{{"type": "image_generation"}},
	}
	headers := map[string]string{
		"Authorization": "Bearer " + apiKey,
		"Accept":        "application/json",
	}
	body, failure, err := postJSONWithTimeout(endpoint, headers, payload, 60*time.Second)
	if err != nil || failure != "" {
		return "", false
	}
	b64, imageURL := extractResponsesImageArtifact(body)
	if b64 != "" {
		decoded, decodeErr := base64.StdEncoding.DecodeString(b64)
		if decodeErr != nil {
			return "", false
		}
		path, writeErr := writeTempImageFile(modelID, decoded)
		if writeErr != nil {
			return "", false
		}
		return path, true
	}
	if imageURL != "" {
		path, downloadErr := downloadTempImageFile(modelID, imageURL)
		if downloadErr != nil {
			return "", false
		}
		return path, true
	}
	return "", false
}

func extractResponsesImageArtifact(body []byte) (string, string) {
	var payload struct {
		Output []struct {
			Type     string `json:"type"`
			Result   string `json:"result"`
			B64JSON  string `json:"b64_json"`
			URL      string `json:"url"`
			ImageURL string `json:"image_url"`
		} `json:"output"`
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", ""
	}
	for _, item := range payload.Output {
		if strings.TrimSpace(item.Type) == "image_generation_call" {
			if strings.TrimSpace(item.Result) != "" {
				return strings.TrimSpace(item.Result), ""
			}
			if strings.TrimSpace(item.B64JSON) != "" {
				return strings.TrimSpace(item.B64JSON), ""
			}
			if url := firstNonEmpty(item.URL, item.ImageURL); strings.TrimSpace(url) != "" {
				return "", strings.TrimSpace(url)
			}
		}
	}
	for _, item := range payload.Data {
		if strings.TrimSpace(item.B64JSON) != "" {
			return strings.TrimSpace(item.B64JSON), ""
		}
		if strings.TrimSpace(item.URL) != "" {
			return "", strings.TrimSpace(item.URL)
		}
	}
	return "", ""
}

func postJSON(endpoint string, headers map[string]string, payload map[string]any) ([]byte, schema.FailureKind, error) {
	return postJSONWithTimeout(endpoint, headers, payload, httpx.DefaultDiagnosticsTimeout)
}

func postJSONWithTimeout(endpoint string, headers map[string]string, payload map[string]any, timeout time.Duration) ([]byte, schema.FailureKind, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := httpx.NewClient(timeout).Do(req)
	if err != nil {
		return nil, httpx.ClassifyFailure(nil, err), err
	}
	defer resp.Body.Close()

	data, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, schema.FailureNetworkError, readErr
	}
	if failure := httpx.ClassifyFailure(resp, nil); failure != "" {
		return data, failure, nil
	}
	return data, "", nil
}

func writeTempImageFile(modelID string, data []byte) (string, error) {
	file, err := os.CreateTemp("", tempImagePattern(modelID))
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return "", err
	}
	return file.Name(), nil
}

func downloadTempImageFile(modelID, rawURL string) (string, error) {
	resp, err := httpx.NewClient(httpx.DefaultDiagnosticsTimeout).Get(rawURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("download status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return writeTempImageFile(modelID, data)
}

func tempImagePattern(modelID string) string {
	cleaned := strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(modelID)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		cleaned = "sample"
	}
	return fmt.Sprintf("aiprobe-%s-*.png", filepath.Base(cleaned))
}

func isImageModel(model schema.Model) bool {
	for _, item := range append(append([]string{}, model.Modalities...), model.Capabilities...) {
		value := strings.ToLower(strings.TrimSpace(item))
		if strings.Contains(value, "image") || strings.Contains(value, "vision") {
			return true
		}
	}
	identity := strings.ToLower(firstNonEmpty(model.Label, model.ID))
	for _, token := range []string{"image", "picture", "dall", "imagen", "flux", "vision"} {
		if strings.Contains(identity, token) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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
