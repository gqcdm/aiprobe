package diagnostics

import (
	"sort"
	"time"

	"github.com/gqcdm/aiprobe/internal/detect"
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
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	var total int64
	for _, item := range latencies {
		total += item
	}
	if status == "ok" {
		failure = ""
	}
	return schema.DiagnosticsResult{
		Status:       status,
		Reachable:    reachable,
		AuthAccepted: authAccepted,
		LatencyMS: schema.LatencyMS{
			Min: latencies[0],
			P50: latencies[len(latencies)/2],
			Max: latencies[len(latencies)-1],
			Avg: total / int64(len(latencies)),
		},
		SampleCount: input.Samples,
		FailureKind: failure,
	}
}
