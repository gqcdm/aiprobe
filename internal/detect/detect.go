package detect

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gqcdm/aiprobe/internal/httpx"
	"github.com/gqcdm/aiprobe/internal/schema"
)

type Input struct {
	BaseURL string
	APIKey  string
}

type FingerprintKind string

const (
	FingerprintHostPath    FingerprintKind = "host_path"
	FingerprintHeaderBody  FingerprintKind = "header_body"
	FingerprintModelProbe  FingerprintKind = "model_probe"
	FingerprintCapability  FingerprintKind = "capability_probe"
)

type Fingerprint struct {
	Provider   schema.Provider
	APIType    string
	Confidence schema.Confidence
	Kind       FingerprintKind
	Source     string
	Summary    string
}

type Adapter interface {
	Name() schema.Provider
	APIType() string
	Probe(baseURL, apiKey string) (ProbeResult, error)
}

type ProbeResult struct {
	Provider          schema.Provider
	APIType           string
	Confidence        schema.Confidence
	Evidence          []schema.Evidence
	Models            []schema.Model
	ModelListSource   string
	FailureKind       schema.FailureKind
	HeadersInspected  bool
	CapabilitiesTried bool
}

type Engine struct {
	adapters []Adapter
}

func NewEngine(adapters ...Adapter) *Engine {
	return &Engine{adapters: adapters}
}

func (e *Engine) Detect(input Input) (schema.Output, error) {
	normalized, err := httpx.NormalizeBaseURL(input.BaseURL)
	if err != nil {
		return schema.Output{}, err
	}

	output := schema.Output{
		Input:             schema.NewInputSummary(input.BaseURL, input.APIKey),
		NormalizedBaseURL: normalized,
		Detection: schema.DetectionResult{
			Provider:   schema.ProviderUnknown,
			APIType:    "unknown",
			Confidence: schema.ConfidenceLow,
		},
		Diagnostics: schema.DiagnosticsResult{
			Status:      "skipped",
			FailureKind: schema.FailureDiagnosticsSkipped,
		},
	}

	static := staticFingerprints(normalized)
	if resolved, ambiguous := resolveFingerprints(static); ambiguous {
		output.Detection.CandidateProviders = candidateProviders(static)
		output.Detection.Evidence = append(output.Detection.Evidence, toEvidence(static)...)
		output.Detection.Provider = schema.ProviderUnknown
		output.Detection.APIType = "unknown"
		output.Errors = append(output.Errors, schema.ErrorDetail{
			Code:    string(schema.FailureAmbiguousDetection),
			Message: "conflicting static fingerprints detected",
			Kind:    schema.FailureAmbiguousDetection,
		})
		return output, nil
	} else if resolved.Provider != schema.ProviderUnknown {
		output.Detection.Provider = resolved.Provider
		output.Detection.APIType = resolved.APIType
		output.Detection.Confidence = resolved.Confidence
		output.Detection.Evidence = append(output.Detection.Evidence, resolved.Evidence...)
		output.Detection.CandidateProviders = []schema.Provider{resolved.Provider}
	}

	candidates := adaptersForProviders(e.adapters, candidateProviders(static))
	if len(candidates) == 0 {
		candidates = e.adapters
	}

	results := make([]ProbeResult, 0, len(candidates))
	for _, adapter := range candidates {
		result, probeErr := adapter.Probe(normalized, input.APIKey)
		if probeErr != nil {
			output.Warnings = append(output.Warnings, fmt.Sprintf("probe for %s failed: %v", adapter.Name(), probeErr))
			continue
		}
		results = append(results, result)
	}

	resolved, ambiguous := resolveProbeResults(results)
	if ambiguous {
		output.Detection.CandidateProviders = candidateProvidersFromResults(results)
		output.Detection.Evidence = append(output.Detection.Evidence, collectEvidence(results)...)
		output.Detection.Provider = schema.ProviderUnknown
		output.Detection.APIType = "unknown"
		output.Errors = append(output.Errors, schema.ErrorDetail{
			Code:    string(schema.FailureAmbiguousDetection),
			Message: "conflicting probe evidence detected",
			Kind:    schema.FailureAmbiguousDetection,
		})
		return output, nil
	}

	if resolved.Provider == schema.ProviderUnknown {
		output.Detection.Evidence = append(output.Detection.Evidence, collectEvidence(results)...)
		if output.Detection.Provider != schema.ProviderUnknown {
			return output, nil
		}
		return output, nil
	}

	output.Detection.Provider = resolved.Provider
	output.Detection.APIType = resolved.APIType
	output.Detection.CompatibilityMode = compatibilityMode(resolved.Provider, resolved.APIType)
	output.Detection.Confidence = resolved.Confidence
	output.Detection.ModelListSource = resolved.ModelListSource
	output.Detection.CandidateProviders = []schema.Provider{resolved.Provider}
	output.Detection.Evidence = append(output.Detection.Evidence, collectEvidence(results)...)
	output.Models = resolved.Models
	if resolved.FailureKind != "" {
		output.Errors = append(output.Errors, schema.ErrorDetail{
			Code:    string(resolved.FailureKind),
			Message: string(resolved.FailureKind),
			Kind:    resolved.FailureKind,
		})
	}

	return output, nil
}

func staticFingerprints(baseURL string) []Fingerprint {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	host := strings.ToLower(parsed.Host)
	path := strings.ToLower(parsed.Path)
	var fingerprints []Fingerprint

	if strings.Contains(host, "anthropic") || strings.Contains(path, "anthropic") {
		fingerprints = append(fingerprints, Fingerprint{
			Provider:   schema.ProviderAnthropic,
			APIType:    "anthropic",
			Confidence: schema.ConfidenceHigh,
			Kind:       FingerprintHostPath,
			Source:     "static.host_path",
			Summary:    "host/path strongly matches anthropic",
		})
	}
	if strings.Contains(host, "google") || strings.Contains(host, "generativelanguage") || strings.Contains(path, "gemini") {
		fingerprints = append(fingerprints, Fingerprint{
			Provider:   schema.ProviderGemini,
			APIType:    "gemini",
			Confidence: schema.ConfidenceHigh,
			Kind:       FingerprintHostPath,
			Source:     "static.host_path",
			Summary:    "host/path strongly matches gemini",
		})
	}
	if strings.Contains(path, "/v1") || strings.Contains(host, "openai") {
		fingerprints = append(fingerprints, Fingerprint{
			Provider:   schema.ProviderOpenAICompatible,
			APIType:    "openai-compatible",
			Confidence: schema.ConfidenceMedium,
			Kind:       FingerprintHostPath,
			Source:     "static.host_path",
			Summary:    "path/host suggests an openai-compatible endpoint",
		})
	}

	return fingerprints
}

func resolveFingerprints(fingerprints []Fingerprint) (ProbeResult, bool) {
	if len(fingerprints) == 0 {
		return ProbeResult{Provider: schema.ProviderUnknown, APIType: "unknown", Confidence: schema.ConfidenceLow}, false
	}

	first := fingerprints[0].Provider
	for _, fp := range fingerprints[1:] {
		if fp.Provider != first && fp.Confidence == schema.ConfidenceHigh {
			return ProbeResult{Provider: schema.ProviderUnknown, APIType: "unknown", Confidence: schema.ConfidenceLow}, true
		}
	}

	best := fingerprints[0]
	for _, fp := range fingerprints[1:] {
		if rankConfidence(fp.Confidence) > rankConfidence(best.Confidence) {
			best = fp
		}
	}

	return ProbeResult{
		Provider:        best.Provider,
		APIType:         best.APIType,
		Confidence:      best.Confidence,
		Evidence:        toEvidence(fingerprints),
		Models:          nil,
		ModelListSource: "",
		FailureKind:     "",
	}, false
}

func resolveProbeResults(results []ProbeResult) (ProbeResult, bool) {
	if len(results) == 0 {
		return ProbeResult{Provider: schema.ProviderUnknown, APIType: "unknown", Confidence: schema.ConfidenceLow}, false
	}

	best := results[0]
	for _, result := range results[1:] {
		if result.Provider != best.Provider && rankConfidence(result.Confidence) == rankConfidence(best.Confidence) && result.Confidence == schema.ConfidenceHigh {
			return ProbeResult{Provider: schema.ProviderUnknown, APIType: "unknown", Confidence: schema.ConfidenceLow}, true
		}
		if rankConfidence(result.Confidence) > rankConfidence(best.Confidence) {
			best = result
		}
	}

	return best, false
}

func adaptersForProviders(adapters []Adapter, providers []schema.Provider) []Adapter {
	if len(providers) == 0 {
		return nil
	}

	providerSet := make(map[schema.Provider]struct{}, len(providers))
	for _, provider := range providers {
		providerSet[provider] = struct{}{}
	}

	filtered := make([]Adapter, 0, len(adapters))
	for _, adapter := range adapters {
		if _, ok := providerSet[adapter.Name()]; ok {
			filtered = append(filtered, adapter)
		}
	}

	return filtered
}

func candidateProviders(fingerprints []Fingerprint) []schema.Provider {
	seen := make(map[schema.Provider]struct{}, len(fingerprints))
	providers := make([]schema.Provider, 0, len(fingerprints))
	for _, fp := range fingerprints {
		if _, ok := seen[fp.Provider]; ok {
			continue
		}
		seen[fp.Provider] = struct{}{}
		providers = append(providers, fp.Provider)
	}
	return providers
}

func candidateProvidersFromResults(results []ProbeResult) []schema.Provider {
	seen := make(map[schema.Provider]struct{}, len(results))
	providers := make([]schema.Provider, 0, len(results))
	for _, result := range results {
		if result.Provider == schema.ProviderUnknown {
			continue
		}
		if _, ok := seen[result.Provider]; ok {
			continue
		}
		seen[result.Provider] = struct{}{}
		providers = append(providers, result.Provider)
	}
	return providers
}

func toEvidence(fingerprints []Fingerprint) []schema.Evidence {
	evidence := make([]schema.Evidence, 0, len(fingerprints))
	for _, fp := range fingerprints {
		evidence = append(evidence, schema.Evidence{Kind: string(fp.Kind), Source: fp.Source, Summary: fp.Summary})
	}
	return evidence
}

func collectEvidence(results []ProbeResult) []schema.Evidence {
	evidence := make([]schema.Evidence, 0)
	for _, result := range results {
		evidence = append(evidence, result.Evidence...)
	}
	return evidence
}

func compatibilityMode(provider schema.Provider, apiType string) string {
	if provider == schema.ProviderOpenAICompatible && apiType == "openai-compatible" {
		return "compatible"
	}
	if provider == schema.ProviderUnknown {
		return ""
	}
	return "native"
}

func rankConfidence(confidence schema.Confidence) int {
	switch confidence {
	case schema.ConfidenceHigh:
		return 3
	case schema.ConfidenceMedium:
		return 2
	case schema.ConfidenceLow:
		return 1
	default:
		return 0
	}
}
