package httpx

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "root URL", input: " https://example.com/ ", want: "https://example.com"},
		{name: "proxy path", input: "https://example.com/proxy/openai/", want: "https://example.com/proxy/openai"},
		{name: "double slashes", input: "https://example.com/proxy//openai///", want: "https://example.com/proxy/openai"},
		{name: "missing scheme", input: "example.com", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeBaseURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none with value %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeBaseURL returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}

	joined, err := JoinURL("https://example.com/proxy/openai/", "/v1/", "/models/")
	if err != nil {
		t.Fatalf("JoinURL returned error: %v", err)
	}
	if joined != "https://example.com/proxy/openai/v1/models" {
		t.Fatalf("expected joined URL to preserve proxy path, got %q", joined)
	}

	versioned, err := JoinVersionedURL("https://example.com/v1", "v1", "models")
	if err != nil {
		t.Fatalf("JoinVersionedURL returned error: %v", err)
	}
	if versioned != "https://example.com/v1/models" {
		t.Fatalf("expected versioned URL without duplicate v1, got %q", versioned)
	}

	proxiedVersioned, err := JoinVersionedURL("https://example.com/proxy/openai/v1/", "v1", "chat", "completions")
	if err != nil {
		t.Fatalf("JoinVersionedURL returned error: %v", err)
	}
	if proxiedVersioned != "https://example.com/proxy/openai/v1/chat/completions" {
		t.Fatalf("expected versioned URL to preserve proxy path, got %q", proxiedVersioned)
	}
}

func TestClassifyFailures(t *testing.T) {
	tests := []struct {
		name string
		resp *http.Response
		err  error
		want string
	}{
		{name: "timeout", err: timeoutError{}, want: string("network_error")},
		{name: "eof", err: io.EOF, want: string("network_error")},
		{name: "rate limit", resp: &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Content-Type": []string{"application/json"}}}, want: string("rate_limited")},
		{name: "auth", resp: &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{"Content-Type": []string{"application/json"}}}, want: string("auth_failed")},
		{name: "non json", resp: &http.Response{StatusCode: http.StatusBadGateway, Header: http.Header{"Content-Type": []string{"text/html"}}}, want: string("invalid_response")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyFailure(tt.resp, tt.err)
			if string(got) != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}

	if ShouldRetry(&http.Response{StatusCode: http.StatusUnauthorized}, nil, 0, 1) {
		t.Fatal("expected no retry for 4xx responses")
	}
	if !ShouldRetry(nil, timeoutError{}, 0, 1) {
		t.Fatal("expected retry for transient timeout error")
	}
	if ShouldRetry(nil, timeoutError{}, 1, 1) {
		t.Fatal("expected retry budget to stop after max retries")
	}

	config := DefaultClientConfig()
	if config.DetectionTimeout <= 0 || config.DiagnosticsTimeout <= 0 {
		t.Fatalf("expected positive timeouts, got detection=%v diagnostics=%v", config.DetectionTimeout, config.DiagnosticsTimeout)
	}
	if config.MaxRetries != 1 {
		t.Fatalf("expected max retries to default to 1, got %d", config.MaxRetries)
	}

	client := NewClient(2 * time.Second)
	if client.Timeout != 2*time.Second {
		t.Fatalf("expected client timeout 2s, got %v", client.Timeout)
	}

	if redacted := RedactSecret("test-secret"); !strings.Contains(redacted, "****") {
		t.Fatalf("expected redacted secret marker, got %q", redacted)
	}

}
