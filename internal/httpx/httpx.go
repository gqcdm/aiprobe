package httpx

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/gqcdm/aiprobe/internal/schema"
)

const (
	DefaultDetectionTimeout   = 5 * time.Second
	DefaultDiagnosticsTimeout = 15 * time.Second
	DefaultMaxRetries         = 1
)

type ClientConfig struct {
	DetectionTimeout   time.Duration
	DiagnosticsTimeout time.Duration
	MaxRetries         int
}

func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		DetectionTimeout:   DefaultDetectionTimeout,
		DiagnosticsTimeout: DefaultDiagnosticsTimeout,
		MaxRetries:         DefaultMaxRetries,
	}
}

func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func NormalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("base URL is required")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse base URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("base URL must include scheme and host")
	}

	cleanPath := path.Clean(parsed.EscapedPath())
	if cleanPath == "." || cleanPath == "/" {
		parsed.Path = ""
	} else {
		parsed.Path = strings.TrimSuffix(cleanPath, "/")
	}
	parsed.RawPath = parsed.Path
	parsed.Fragment = ""

	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func JoinURL(baseURL string, segments ...string) (string, error) {
	normalized, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return "", err
	}

	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", fmt.Errorf("parse normalized base URL: %w", err)
	}

	parts := make([]string, 0, len(segments)+1)
	if parsed.Path != "" {
		parts = append(parts, strings.Trim(parsed.Path, "/"))
	}
	for _, segment := range segments {
		trimmed := strings.Trim(segment, "/ ")
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}

	if len(parts) == 0 {
		parsed.Path = ""
	} else {
		parsed.Path = "/" + path.Join(parts...)
	}

	return parsed.String(), nil
}

func RedactSecret(value string) string {
	return schema.RedactAPIKey(value)
}

func ClassifyFailure(resp *http.Response, err error) schema.FailureKind {
	if err != nil {
		if isTransientNetworkError(err) {
			return schema.FailureNetworkError
		}
		return schema.FailureNetworkError
	}
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

	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if contentType != "" && !strings.Contains(contentType, "json") {
		return schema.FailureInvalidResponse
	}

	return ""
}

func ShouldRetry(resp *http.Response, err error, attempt, maxRetries int) bool {
	if attempt >= maxRetries {
		return false
	}
	if err != nil {
		return isTransientNetworkError(err)
	}
	if resp == nil {
		return false
	}
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return false
	}
	return false
}

func ReadResponseBody(resp *http.Response) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, nil
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func isTransientNetworkError(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	return false
}
