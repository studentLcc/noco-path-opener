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

type ReadRecordRequest struct {
	BaseID   string
	TableID  string
	RecordID json.RawMessage
}

type QueryRecordsRequest struct {
	BaseID  string
	TableID string
	Where   string
}

type UpdateFieldsRequest struct {
	BaseID   string
	TableID  string
	RecordID json.RawMessage
	Fields   map[string]json.RawMessage
}

type Record struct {
	Fields map[string]json.RawMessage
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
	if err := c.requireConfig(); err != nil {
		return err
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
		return responseError("update", resp)
	}

	return nil
}

func (c *Client) ReadRecord(ctx context.Context, req ReadRecordRequest) (Record, error) {
	if err := c.requireConfig(); err != nil {
		return Record{}, err
	}

	recordID, err := recordIDPathPart(req.RecordID)
	if err != nil {
		return Record{}, err
	}

	endpoint, err := url.JoinPath(c.baseURL, "api", "v3", "data", req.BaseID, req.TableID, "records", recordID)
	if err != nil {
		return Record{}, fmt.Errorf("build URL: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Record{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("xc-token", c.token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return Record{}, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Record{}, responseError("read", resp)
	}

	fields, err := decodeRecordFields(resp.Body)
	if err != nil {
		return Record{}, fmt.Errorf("decode response: %w", err)
	}

	return Record{Fields: fields}, nil
}

func (c *Client) QueryRecords(ctx context.Context, req QueryRecordsRequest) ([]Record, error) {
	if err := c.requireConfig(); err != nil {
		return nil, err
	}

	endpoint, err := url.JoinPath(c.baseURL, "api", "v3", "data", req.BaseID, req.TableID, "records")
	if err != nil {
		return nil, fmt.Errorf("build URL: %w", err)
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	if strings.TrimSpace(req.Where) != "" {
		q := u.Query()
		q.Set("where", req.Where)
		u.RawQuery = q.Encode()
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("xc-token", c.token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, responseError("query", resp)
	}

	records, err := decodeRecords(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return records, nil
}

func (c *Client) UpdateFields(ctx context.Context, req UpdateFieldsRequest) error {
	if err := c.requireConfig(); err != nil {
		return err
	}

	endpoint, err := url.JoinPath(c.baseURL, "api", "v3", "data", req.BaseID, req.TableID, "records")
	if err != nil {
		return fmt.Errorf("build URL: %w", err)
	}

	body, err := json.Marshal(struct {
		ID     json.RawMessage            `json:"id"`
		Fields map[string]json.RawMessage `json:"fields"`
	}{
		ID:     req.RecordID,
		Fields: req.Fields,
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
		return responseError("update", resp)
	}

	return nil
}

func EqualWhere(field string, value string) string {
	return fmt.Sprintf("(%s,eq,%s)", quoteWhereValue(field), quoteWhereValue(value))
}

func quoteWhereValue(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(encoded)
}

func recordIDPathPart(raw json.RawMessage) (string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "", fmt.Errorf("record id cannot be empty")
	}

	var stringID string
	if err := json.Unmarshal([]byte(trimmed), &stringID); err == nil {
		return stringID, nil
	}

	var numberID json.Number
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&numberID); err != nil {
		return "", fmt.Errorf("decode record id: %w", err)
	}

	return numberID.String(), nil
}

func decodeRecordFields(r io.Reader) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.NewDecoder(r).Decode(&fields); err != nil {
		return nil, err
	}
	if nested, ok := nestedFields(fields); ok {
		fields = nested
	}
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	return fields, nil
}

func decodeRecords(r io.Reader) ([]Record, error) {
	var raw json.RawMessage
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, err
	}

	var official struct {
		Records []recordEnvelope `json:"records"`
	}
	if err := json.Unmarshal(raw, &official); err == nil && official.Records != nil {
		return wrapRecordEnvelopes(official.Records), nil
	}

	var wrapped struct {
		List []map[string]json.RawMessage `json:"list"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.List != nil {
		return wrapRecords(wrapped.List), nil
	}

	var records []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, err
	}
	return wrapRecords(records), nil
}

type recordEnvelope struct {
	Fields map[string]json.RawMessage `json:"fields"`
}

func nestedFields(fields map[string]json.RawMessage) (map[string]json.RawMessage, bool) {
	if _, ok := fields["id"]; !ok {
		return nil, false
	}

	raw, ok := fields["fields"]
	if !ok {
		return nil, false
	}

	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err != nil {
		return nil, false
	}
	if nested == nil {
		return nil, false
	}
	return nested, true
}

func wrapRecordEnvelopes(envelopes []recordEnvelope) []Record {
	records := make([]Record, len(envelopes))
	for i := range envelopes {
		fields := envelopes[i].Fields
		if fields == nil {
			fields = map[string]json.RawMessage{}
		}
		records[i] = Record{Fields: fields}
	}
	return records
}

func wrapRecords(fields []map[string]json.RawMessage) []Record {
	records := make([]Record, len(fields))
	for i := range fields {
		if fields[i] == nil {
			fields[i] = map[string]json.RawMessage{}
		}
		records[i] = Record{Fields: fields[i]}
	}
	return records
}

func (c *Client) requireConfig() error {
	if c.baseURL == "" || c.token == "" {
		return fmt.Errorf("nocodb_url and nocodb_token must be configured")
	}
	return nil
}

func responseError(operation string, resp *http.Response) error {
	summary, readErr := io.ReadAll(io.LimitReader(resp.Body, 512))
	if readErr != nil {
		return fmt.Errorf("nocodb %s failed with status %d: read response body: %w", operation, resp.StatusCode, readErr)
	}
	return fmt.Errorf("nocodb %s failed with status %d: %s", operation, resp.StatusCode, strings.TrimSpace(string(summary)))
}
