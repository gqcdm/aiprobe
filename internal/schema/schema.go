package schema

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Provider string

const (
	ProviderOpenAICompatible Provider = "openai-compatible"
	ProviderAnthropic       Provider = "anthropic"
	ProviderGemini          Provider = "gemini"
	ProviderUnknown         Provider = "unknown"
)

type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

type FailureKind string

const (
	FailureAuthFailed                FailureKind = "auth_failed"
	FailureUnsupportedAPI            FailureKind = "unsupported_api"
	FailureNetworkError              FailureKind = "network_error"
	FailureRateLimited               FailureKind = "rate_limited"
	FailureAmbiguousDetection        FailureKind = "ambiguous_detection"
	FailureReachableNoModelsExposed  FailureKind = "reachable_but_no_models_exposed"
	FailureInvalidResponse           FailureKind = "invalid_response"
	FailureDiagnosticsSkipped        FailureKind = "diagnostics_skipped"
)

type Output struct {
	Input             InputSummary        `json:"input"`
	NormalizedBaseURL string              `json:"normalized_base_url"`
	Detection         DetectionResult     `json:"detection"`
	Models            []Model             `json:"models"`
	Diagnostics       DiagnosticsResult   `json:"diagnostics"`
	ModelDiagnostics  []ModelDiagnostics  `json:"model_diagnostics,omitempty"`
	SampleOutputs     []SampleOutput      `json:"sample_outputs,omitempty"`
	Errors            []ErrorDetail       `json:"errors"`
	Warnings          []string            `json:"warnings"`
}

type InputSummary struct {
	BaseURL       string `json:"base_url"`
	APIKeyPresent bool   `json:"api_key_present"`
	APIKeyHint    string `json:"api_key_hint,omitempty"`
}

type DetectionResult struct {
	Provider           Provider     `json:"provider"`
	APIType            string       `json:"api_type"`
	CompatibilityMode  string       `json:"compatibility_mode,omitempty"`
	Confidence         Confidence   `json:"confidence"`
	CandidateProviders []Provider   `json:"candidate_providers,omitempty"`
	ModelListSource    string       `json:"model_list_source,omitempty"`
	Evidence           []Evidence   `json:"evidence,omitempty"`
}

type Evidence struct {
	Kind    string `json:"kind"`
	Source  string `json:"source"`
	Summary string `json:"summary"`
}

type Model struct {
	ID              string         `json:"id"`
	Label           string         `json:"label,omitempty"`
	Family          string         `json:"family,omitempty"`
	ContextWindow   int            `json:"context_window,omitempty"`
	MaxOutputTokens int            `json:"max_output_tokens,omitempty"`
	Modalities      []string       `json:"modalities,omitempty"`
	Capabilities    []string       `json:"capabilities,omitempty"`
	Retrieved       bool           `json:"retrieved"`
	Inferred        bool           `json:"inferred"`
	Raw             map[string]any `json:"raw,omitempty"`
}

type DiagnosticsResult struct {
	Status       string      `json:"status"`
	Reachable    bool        `json:"reachable"`
	AuthAccepted bool        `json:"auth_accepted"`
	LatencyMS    LatencyMS   `json:"latency_ms"`
	SampleCount  int         `json:"sample_count"`
	StatusCode   int         `json:"status_code,omitempty"`
	FailureKind  FailureKind `json:"failure_kind,omitempty"`
}

type ModelDiagnostics struct {
	ModelID      string      `json:"model_id"`
	Label        string      `json:"label,omitempty"`
	Status       string      `json:"status"`
	Available    bool        `json:"available"`
	TTFTMS       LatencyMS   `json:"ttft_ms"`
	SampleCount  int         `json:"sample_count"`
	SuccessCount int         `json:"success_count,omitempty"`
	FailureKind  FailureKind `json:"failure_kind,omitempty"`
}

type SampleOutput struct {
	ModelID     string      `json:"model_id"`
	Label       string      `json:"label,omitempty"`
	Kind        string      `json:"kind"`
	Status      string      `json:"status"`
	TextReply   string      `json:"text_reply,omitempty"`
	ImagePath   string      `json:"image_path,omitempty"`
	FailureKind FailureKind `json:"failure_kind,omitempty"`
	Warning     string      `json:"warning,omitempty"`
}

type LatencyMS struct {
	Min int64 `json:"min,omitempty"`
	P50 int64 `json:"p50,omitempty"`
	Max int64 `json:"max,omitempty"`
	Avg int64 `json:"avg,omitempty"`
}

type ErrorDetail struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Kind    FailureKind `json:"kind,omitempty"`
}

func NewInputSummary(baseURL, apiKey string) InputSummary {
	return InputSummary{
		BaseURL:       strings.TrimSpace(baseURL),
		APIKeyPresent: strings.TrimSpace(apiKey) != "",
		APIKeyHint:    RedactAPIKey(apiKey),
	}
}

func RedactAPIKey(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	runes := []rune(trimmed)
	if len(runes) <= 4 {
		return "****"
	}

	return fmt.Sprintf("%s****%s", string(runes[:2]), string(runes[len(runes)-2:]))
}

func (o Output) MarshalJSON() ([]byte, error) {
	type alias Output
	cloned := alias(o)
	if cloned.Input.APIKeyHint != "" {
		cloned.Input.APIKeyHint = RedactAPIKey(cloned.Input.APIKeyHint)
	}

	for i := range cloned.Errors {
		cloned.Errors[i].Message = redactMessage(cloned.Errors[i].Message)
	}

	for i := range cloned.Warnings {
		cloned.Warnings[i] = redactMessage(cloned.Warnings[i])
	}

	return json.Marshal(alias(cloned))
}

func redactMessage(message string) string {
	if strings.TrimSpace(message) == "" {
		return message
	}

	for _, field := range []string{"sk-test-123456", "test-secret", "api-key:", "sk-proj-", "gho_"} {
		message = strings.ReplaceAll(message, field, "[redacted]")
	}

	return message
}
