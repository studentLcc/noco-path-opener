package profilegen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const metadataBodySummaryLimit = 240

type Field struct {
	ID    string
	Name  string
	Title string
}

func (f Field) DisplayName() string {
	return firstNonEmpty(f.Title, f.Name)
}

type FieldLister interface {
	ListFields(ctx context.Context, projectID, tableID string) ([]Field, error)
}

type MetadataConfig struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

type MetadataClient struct {
	cfg        MetadataConfig
	httpClient *http.Client
}

func NewMetadataClient(cfg MetadataConfig) *MetadataClient {
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.Token = strings.TrimSpace(cfg.Token)

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}

	return &MetadataClient{
		cfg:        cfg,
		httpClient: httpClient,
	}
}

func (c *MetadataClient) ListFields(ctx context.Context, projectID, tableID string) ([]Field, error) {
	if err := c.requireConfig(); err != nil {
		return nil, err
	}

	baseURL, err := url.Parse(c.cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("list fields: parse NocoDB URL: %w", err)
	}
	baseURL.RawQuery = ""
	baseURL.Fragment = ""
	endpoint, err := url.JoinPath(baseURL.String(), "api", "v2", "meta", "tables", tableID)
	if err != nil {
		return nil, fmt.Errorf("list fields: build metadata path: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("list fields: build request: %w", err)
	}
	req.Header.Set("xc-token", c.cfg.Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list fields: request metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, metadataResponseError("list fields", resp)
	}

	fields, err := decodeFields(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("list fields: %w", err)
	}
	return fields, nil
}

func (c *MetadataClient) requireConfig() error {
	if strings.TrimSpace(c.cfg.BaseURL) == "" || strings.TrimSpace(c.cfg.Token) == "" {
		return errors.New("NocoDB URL and token are required")
	}
	return nil
}

func decodeFields(r io.Reader) ([]Field, error) {
	var raw json.RawMessage
	dec := json.NewDecoder(r)
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode metadata response: %w", err)
	}
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("decode metadata response: contains trailing data")
		}
		return nil, fmt.Errorf("decode metadata response: %w", err)
	}

	items, ok := fieldItems(raw)
	if !ok {
		return nil, errors.New("metadata response does not contain a field list")
	}
	if len(items) == 0 {
		return nil, errors.New("metadata response returned no usable fields")
	}

	fields := make([]Field, 0, len(items))
	for i, item := range items {
		field, err := decodeField(item)
		if err != nil {
			return nil, fmt.Errorf("decode metadata field %d: %w", i, err)
		}
		fields = append(fields, field)
	}
	if len(fields) == 0 {
		return nil, errors.New("metadata response returned no usable fields")
	}
	return fields, nil
}

func fieldItems(raw json.RawMessage) ([]json.RawMessage, bool) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err == nil {
		return items, true
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false
	}

	for _, key := range []string{"columns", "fields", "list", "data"} {
		child, ok := obj[key]
		if !ok {
			continue
		}
		if items, ok := fieldItems(child); ok {
			return items, true
		}
	}

	return nil, false
}

func decodeField(raw json.RawMessage) (Field, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return Field{}, fmt.Errorf("decode field object: %w", err)
	}

	field := Field{
		ID:    rawString(obj["id"]),
		Name:  firstNonEmpty(rawString(obj["column_name"]), rawString(obj["name"])),
		Title: rawString(obj["title"]),
	}
	if field.DisplayName() == "" {
		return Field{}, errors.New("no usable field name")
	}
	return field, nil
}

func rawString(raw json.RawMessage) string {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}

	var n json.Number
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&n); err == nil {
		return strings.TrimSpace(n.String())
	}

	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func metadataResponseError(operation string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, metadataBodySummaryLimit+1))
	summary := strings.TrimSpace(string(body))
	if len(summary) > metadataBodySummaryLimit {
		summary = summary[:metadataBodySummaryLimit] + "..."
	}
	if summary == "" {
		return fmt.Errorf("%s: NocoDB metadata request failed with status %d", operation, resp.StatusCode)
	}
	return fmt.Errorf("%s: NocoDB metadata request failed with status %d: %s", operation, resp.StatusCode, summary)
}
