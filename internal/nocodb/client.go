package nocodb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 10 * time.Second

type Config struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

type UpdateRequest struct {
	BaseID    string
	TableID   string
	RecordID  json.RawMessage
	PathField string
	PathValue string
}

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(cfg Config) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}

	return &Client{
		baseURL:    strings.TrimSpace(cfg.BaseURL),
		token:      strings.TrimSpace(cfg.Token),
		httpClient: httpClient,
	}
}

func (c *Client) UpdateRecord(ctx context.Context, req UpdateRequest) error {
	if c.baseURL == "" || c.token == "" {
		return fmt.Errorf("nocodb_url and nocodb_token must be configured")
	}

	endpoint, err := url.JoinPath(c.baseURL, "api", "v3", "data", req.BaseID, req.TableID, "records")
	if err != nil {
		return fmt.Errorf("build URL: %w", err)
	}

	body, err := json.Marshal(struct {
		ID     json.RawMessage   `json:"id"`
		Fields map[string]string `json:"fields"`
	}{
		ID: req.RecordID,
		Fields: map[string]string{
			req.PathField: req.PathValue,
		},
	})
	if err != nil {
		return fmt.Errorf("encode body: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("xc-token", c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		summary, readErr := io.ReadAll(io.LimitReader(resp.Body, 512))
		if readErr != nil {
			return fmt.Errorf("nocodb update failed with status %d: read response body: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("nocodb update failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(summary)))
	}

	return nil
}
