package handlers

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/raul/gator/internal/models"
)

type opnsenseAPIClient struct {
	baseURL   string
	apiKey    string
	apiSecret string
	http      *http.Client
}

func newOPNsenseAPIClient(cfg models.FirewallConfig) *opnsenseAPIClient {
	baseURL := strings.TrimSpace(strings.TrimRight(cfg.Host, "/"))
	if !strings.Contains(baseURL, "://") {
		baseURL = "https://" + baseURL
	}

	return &opnsenseAPIClient{
		baseURL:   baseURL,
		apiKey:    cfg.APIKey,
		apiSecret: cfg.APISecret,
		http: &http.Client{
			Timeout: 8 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.SkipTLS},
			},
		},
	}
}

func (c *opnsenseAPIClient) Get(ctx context.Context, path string) (map[string]any, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

func (c *opnsenseAPIClient) Post(ctx context.Context, path string, payload any) (map[string]any, error) {
	return c.do(ctx, http.MethodPost, path, payload)
}

// GetRaw performs a GET and returns the raw response body (for non-JSON endpoints like config backup).
func (c *opnsenseAPIClient) GetRaw(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth(c.apiKey, c.apiSecret)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("authentication failed with OPNsense API")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	return raw, nil
}

// PostRaw sends a POST with a raw body and returns the raw response bytes.
// Used for endpoints that accept/return non-JSON data (e.g. CSV upload).
func (c *opnsenseAPIClient) PostRaw(ctx context.Context, path string, contentType string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth(c.apiKey, c.apiSecret)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("authentication failed with OPNsense API")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	return raw, nil
}

func (c *opnsenseAPIClient) do(ctx context.Context, method, path string, payload any) (map[string]any, error) {
	var body io.Reader
	if payload != nil {
		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		body = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth(c.apiKey, c.apiSecret)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("authentication failed with OPNsense API")
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	result := map[string]any{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return result, nil
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse response JSON: %w", err)
	}

	return result, nil
}
