package transport

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Integration is the Integration v1 transport: a stateless X-API-KEY client. No
// login, no CSRF — the key authorizes every request directly.
type Integration struct {
	baseURL    string
	apiKey     string
	http       *http.Client
	maxRetries int
}

func NewIntegration(baseURL, apiKey string, httpClient *http.Client, maxRetries int) *Integration {
	return &Integration{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		http:       httpClient,
		maxRetries: maxRetries,
	}
}

// Do executes a request against the Integration v1 surface. path is the full
// path after the host (e.g. "/proxy/network/integration/v1/sites"). It retries
// 429s and returns an *APIError for any other non-2xx status.
func (i *Integration) Do(ctx context.Context, method, path string, body any) (*Response, error) {
	for attempt := 0; ; attempt++ {
		req, err := newRequest(ctx, method, i.baseURL+path, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-API-KEY", i.apiKey)

		resp, err := i.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("integration %s %s: %w", method, path, err)
		}
		data, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read response body: %w", err)
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < i.maxRetries {
			if err := sleep(ctx, retryAfter(resp.Header, attempt)); err != nil {
				return nil, err
			}
			continue
		}

		r := &Response{StatusCode: resp.StatusCode, Body: data, Header: resp.Header}
		if !r.IsSuccess() {
			return r, &APIError{
				Surface:    "integration",
				Method:     method,
				Path:       path,
				StatusCode: resp.StatusCode,
				Message:    extractMessage(data),
				Body:       data,
			}
		}
		return r, nil
	}
}
