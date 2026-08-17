package remotesync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var placeholderPattern = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)

const (
	defaultMaxJSONBytes    int64 = 8 << 20
	defaultMaxFileBytes    int64 = 1 << 30
	defaultRequestTimeout        = 10 * time.Second
	defaultDownloadTimeout       = 10 * time.Minute
)

type Logger interface {
	Printf(format string, values ...any)
}

type Config struct {
	HTTPClient          *http.Client
	ParamsTemplatePath  string
	HeadersTemplatePath string
	MaxJSONBytes        int64
	MaxFileBytes        int64
	RequestTimeout      time.Duration
	DownloadTimeout     time.Duration
	Logger              Logger
	LogResponseBodies   bool
}

type Spec struct {
	PostURL               string            `json:"post_url"`
	GetURL                string            `json:"get_url"`
	DownloadURL           string            `json:"download_url"`
	ProcessCode           string            `json:"processCode"`
	InputField            string            `json:"input_field"`
	RequestTimeoutSeconds int               `json:"request_timeout_seconds"`
	FieldMapping          map[string]string `json:"field_mapping"`
}

type FetchRequest struct {
	PostURL        string
	GetURL         string
	ProcessCode    string
	InputField     string
	Token          string
	RequestTimeout time.Duration
}

type DownloadRequest struct {
	URL       string
	File      File
	Token     string
	Directory string
	Overwrite bool
}

type File struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Result struct {
	Name       string
	ID         string
	DesignName string
	Creator    string
	InputValue json.RawMessage
	Files      []File
}

type Client struct {
	requestClient       *http.Client
	downloadClient      *http.Client
	paramsTemplatePath  string
	headersTemplatePath string
	maxJSONBytes        int64
	maxFileBytes        int64
	requestTimeout      time.Duration
	logger              Logger
	logResponseBodies   bool
}

func NewClient(cfg Config) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	requestClient := *httpClient
	requestClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return errors.New("remote sync redirects are not allowed")
	}
	downloadClient := requestClient
	requestTimeout := cfg.RequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = defaultRequestTimeout
	}
	downloadTimeout := cfg.DownloadTimeout
	if downloadTimeout <= 0 {
		downloadTimeout = defaultDownloadTimeout
	}
	requestClient.Timeout = 0
	downloadClient.Timeout = downloadTimeout
	maxJSONBytes := cfg.MaxJSONBytes
	if maxJSONBytes <= 0 {
		maxJSONBytes = defaultMaxJSONBytes
	}
	maxFileBytes := cfg.MaxFileBytes
	if maxFileBytes <= 0 {
		maxFileBytes = defaultMaxFileBytes
	}
	return &Client{
		requestClient:       &requestClient,
		downloadClient:      &downloadClient,
		paramsTemplatePath:  strings.TrimSpace(cfg.ParamsTemplatePath),
		headersTemplatePath: strings.TrimSpace(cfg.HeadersTemplatePath),
		maxJSONBytes:        maxJSONBytes,
		maxFileBytes:        maxFileBytes,
		requestTimeout:      requestTimeout,
		logger:              cfg.Logger,
		logResponseBodies:   cfg.LogResponseBodies,
	}
}

func (c *Client) Fetch(ctx context.Context, req FetchRequest) (Result, error) {
	if c == nil || c.requestClient == nil {
		return Result{}, errors.New("remote sync HTTP client is not configured")
	}
	postURL := strings.TrimSpace(req.PostURL)
	getURL := strings.TrimSpace(req.GetURL)
	processCode := strings.TrimSpace(req.ProcessCode)
	inputField := strings.TrimSpace(req.InputField)
	token := strings.TrimSpace(req.Token)
	switch {
	case postURL == "":
		return Result{}, errors.New("remote sync post_url is required")
	case getURL == "":
		return Result{}, errors.New("remote sync get_url is required")
	case processCode == "":
		return Result{}, errors.New("remote sync processCode is required")
	case inputField == "":
		return Result{}, errors.New("remote sync input_field is required")
	case token == "":
		return Result{}, errors.New("snc-token is required")
	}

	headers, err := c.headers(token)
	if err != nil {
		return Result{}, err
	}
	body, err := c.paramsBody(processCode)
	if err != nil {
		return Result{}, err
	}
	identityBody, err := c.doJSONWithHeaders(ctx, http.MethodPost, postURL, body, req.RequestTimeout, headers)
	if err != nil {
		return Result{}, fmt.Errorf("remote sync POST: %w", err)
	}
	name, id, designName, creator, err := decodeIdentity(identityBody)
	if err != nil {
		return Result{}, fmt.Errorf("decode remote identity: %w", err)
	}

	detailURL, err := ResolveURL(getURL, map[string]string{"id": id})
	if err != nil {
		return Result{}, fmt.Errorf("build remote detail URL: %w", err)
	}
	detailBody, err := c.doJSONWithHeaders(ctx, http.MethodGet, detailURL, nil, req.RequestTimeout, headers)
	if err != nil {
		return Result{}, fmt.Errorf("remote sync GET: %w", err)
	}
	inputValue, files, err := decodeDetail(detailBody, inputField)
	if err != nil {
		return Result{}, fmt.Errorf("decode remote detail: %w", err)
	}

	return Result{
		Name:       name,
		ID:         id,
		DesignName: designName,
		Creator:    creator,
		InputValue: inputValue,
		Files:      files,
	}, nil
}

func (c *Client) Download(ctx context.Context, req DownloadRequest) error {
	if c == nil || c.downloadClient == nil {
		return errors.New("remote sync HTTP client is not configured")
	}
	if strings.TrimSpace(req.Token) == "" {
		return errors.New("snc-token is required")
	}
	directory := strings.TrimSpace(req.Directory)
	if directory == "" {
		return errors.New("download directory is required")
	}
	filename, err := ValidateFilename(req.File.Name)
	if err != nil {
		return err
	}
	destination := filepath.Join(directory, filename)
	if info, err := os.Lstat(destination); err == nil {
		if !req.Overwrite {
			return fmt.Errorf("download destination already exists: %s", destination)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("download destination is not a regular file: %s", destination)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check download destination: %w", err)
	}
	endpoint, err := ResolveURL(req.URL, map[string]string{
		"id":        req.File.ID,
		"file_id":   req.File.ID,
		"name":      req.File.Name,
		"file_name": req.File.Name,
	})
	if err != nil {
		return fmt.Errorf("build file download URL: %w", err)
	}

	headers, err := c.headers(req.Token)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create file download request: %w", err)
	}
	applyHeaders(httpReq, headers)
	resp, err := c.downloadClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send file download request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError("file download", resp)
	}

	temp, err := os.CreateTemp(directory, "."+filename+".*.download")
	if err != nil {
		return fmt.Errorf("create download temp file: %w", err)
	}
	tempName := temp.Name()
	defer func() {
		_ = os.Remove(tempName)
	}()
	written, err := io.Copy(temp, io.LimitReader(resp.Body, c.maxFileBytes+1))
	if err != nil {
		_ = temp.Close()
		return fmt.Errorf("write downloaded file: %w", err)
	}
	if written > c.maxFileBytes {
		_ = temp.Close()
		return fmt.Errorf("downloaded file exceeds %d bytes", c.maxFileBytes)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close downloaded file: %w", err)
	}
	if req.Overwrite {
		if err := replaceDownloadedFile(tempName, destination); err != nil {
			return err
		}
	} else if err := os.Link(tempName, destination); err != nil {
		return fmt.Errorf("commit downloaded file without overwrite: %w", err)
	}
	_ = os.Remove(tempName)
	return nil
}

func replaceDownloadedFile(tempName, destination string) error {
	if info, err := os.Lstat(destination); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("download destination is not a regular file: %s", destination)
		}
		if err := os.Remove(destination); err != nil {
			return fmt.Errorf("remove existing download destination: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check download destination: %w", err)
	}
	if err := os.Rename(tempName, destination); err != nil {
		return fmt.Errorf("commit downloaded file with overwrite: %w", err)
	}
	return nil
}

func BuildMappedFields(result Result, mapping map[string]string) (map[string]json.RawMessage, error) {
	if len(mapping) == 0 {
		return nil, errors.New("remote sync field_mapping is required")
	}
	fields := make(map[string]json.RawMessage, len(mapping))
	for source, target := range mapping {
		target = strings.TrimSpace(target)
		if target == "" {
			return nil, fmt.Errorf("remote sync field_mapping[%q] target is empty", source)
		}

		var value json.RawMessage
		switch strings.TrimSpace(source) {
		case "name":
			value = marshalString(result.Name)
		case "id":
			value = marshalString(result.ID)
		case "designName":
			value = marshalString(result.DesignName)
		case "creator":
			value = marshalString(result.Creator)
		case "input_value":
			if len(result.InputValue) == 0 {
				continue
			}
			value = cloneRaw(result.InputValue)
		case "file_uploads":
			var err error
			value, err = json.Marshal(result.Files)
			if err != nil {
				return nil, fmt.Errorf("encode file_uploads: %w", err)
			}
		default:
			return nil, fmt.Errorf("remote sync field_mapping source %q is unsupported", source)
		}
		if len(value) == 0 {
			return nil, fmt.Errorf("remote sync value %q is empty", source)
		}
		fields[target] = value
	}
	return fields, nil
}

func ResolveURL(template string, values map[string]string) (string, error) {
	template = strings.TrimSpace(template)
	if template == "" {
		return "", errors.New("URL is required")
	}

	question := strings.IndexByte(template, '?')
	pathPart := template
	queryPart := ""
	if question >= 0 {
		pathPart = template[:question]
		queryPart = template[question+1:]
	}
	fragment := ""
	if hash := strings.IndexByte(queryPart, '#'); hash >= 0 {
		fragment = queryPart[hash:]
		queryPart = queryPart[:hash]
	}

	var err error
	pathPart, err = replacePlaceholders(pathPart, values, url.PathEscape)
	if err != nil {
		return "", err
	}
	queryPart, err = replacePlaceholders(queryPart, values, url.QueryEscape)
	if err != nil {
		return "", err
	}
	resolved := pathPart
	if question >= 0 {
		resolved += "?" + queryPart
	}
	resolved += fragment
	parsed, err := url.Parse(resolved)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("invalid URL %q", resolved)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("URL must use http or https: %q", resolved)
	}
	return resolved, nil
}

func (c *Client) paramsBody(processCode string) ([]byte, error) {
	if c.paramsTemplatePath == "" {
		return nil, errors.New("remote_sync_params.json path is not configured")
	}
	data, err := os.ReadFile(c.paramsTemplatePath)
	if err != nil {
		return nil, fmt.Errorf("read remote_sync_params.json: %w", err)
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, fmt.Errorf("decode remote_sync_params.json: %w", err)
	}
	params, ok := body["params"]
	if !ok {
		return nil, errors.New("remote_sync_params.json params is required")
	}
	var paramsObject map[string]json.RawMessage
	if err := json.Unmarshal(params, &paramsObject); err != nil {
		return nil, fmt.Errorf("remote_sync_params.json params must be an object: %w", err)
	}
	condition, ok := paramsObject["condition"]
	if !ok {
		return nil, errors.New("remote_sync_params.json params.condition is required")
	}
	var conditionObject map[string]json.RawMessage
	if err := json.Unmarshal(condition, &conditionObject); err != nil {
		return nil, fmt.Errorf("remote_sync_params.json params.condition must be an object: %w", err)
	}
	conditionObject["processCode"], err = json.Marshal(processCode)
	if err != nil {
		return nil, fmt.Errorf("encode processCode: %w", err)
	}
	paramsObject["condition"], err = json.Marshal(conditionObject)
	if err != nil {
		return nil, fmt.Errorf("encode params.condition: %w", err)
	}
	body["params"], err = json.Marshal(paramsObject)
	if err != nil {
		return nil, fmt.Errorf("encode params: %w", err)
	}
	return json.Marshal(body)
}

func (c *Client) doJSON(ctx context.Context, method, endpoint, token string, body []byte) ([]byte, error) {
	return c.doJSONWithHeaders(ctx, method, endpoint, body, 0, defaultHeaders(token))
}

func (c *Client) doJSONWithHeaders(ctx context.Context, method, endpoint string, body []byte, timeout time.Duration, headers http.Header) ([]byte, error) {
	if timeout <= 0 {
		timeout = c.requestTimeout
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	started := time.Now()
	c.logf("remote request start method=%s url=%q", method, endpoint)
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		c.logf("remote request failed method=%s url=%q elapsed=%s error=%v", method, endpoint, time.Since(started), err)
		return nil, fmt.Errorf("create request: %w", err)
	}
	applyHeaders(req, headers)
	if body != nil {
		if req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}
	}
	if c.logResponseBodies {
		c.logf("remote request headers method=%s url=%q headers=%s", method, endpoint, formatDebugHeaders(req.Header, req.ContentLength))
		if body != nil {
			c.logf("remote request body method=%s url=%q body=%s", method, endpoint, string(body))
		}
	}
	resp, err := c.requestClient.Do(req)
	if err != nil {
		c.logf("remote request failed method=%s url=%q elapsed=%s error=%v", method, endpoint, time.Since(started), err)
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logf("remote request failed method=%s url=%q status=%d elapsed=%s", method, endpoint, resp.StatusCode, time.Since(started))
		return nil, responseError(method, resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, c.maxJSONBytes+1))
	if err != nil {
		c.logf("remote request failed method=%s url=%q status=%d elapsed=%s error=%v", method, endpoint, resp.StatusCode, time.Since(started), err)
		return nil, fmt.Errorf("read response: %w", err)
	}
	if int64(len(data)) > c.maxJSONBytes {
		c.logf("remote request failed method=%s url=%q status=%d elapsed=%s error=response too large", method, endpoint, resp.StatusCode, time.Since(started))
		return nil, fmt.Errorf("JSON response exceeds %d bytes", c.maxJSONBytes)
	}
	c.logf("remote request complete method=%s url=%q status=%d bytes=%d elapsed=%s", method, endpoint, resp.StatusCode, len(data), time.Since(started))
	if c.logResponseBodies {
		c.logf("remote response body method=%s url=%q body=%s", method, endpoint, string(data))
	}
	return data, nil
}

func (c *Client) headers(token string) (http.Header, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("snc-token is required")
	}
	if c.headersTemplatePath == "" {
		return defaultHeaders(token), nil
	}
	data, err := os.ReadFile(c.headersTemplatePath)
	if err != nil {
		return nil, fmt.Errorf("read remote_sync_headers.json: %w", err)
	}
	var template map[string]string
	if err := json.Unmarshal(data, &template); err != nil {
		return nil, fmt.Errorf("decode remote_sync_headers.json: %w", err)
	}
	if len(template) == 0 {
		return nil, errors.New("remote_sync_headers.json must contain at least one header")
	}
	headers := make(http.Header, len(template))
	for name, value := range template {
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(strings.ReplaceAll(value, "{token}", token))
		if name == "" || value == "" {
			return nil, errors.New("remote_sync_headers.json contains an empty header name or value")
		}
		headers.Set(name, value)
	}
	return headers, nil
}

func defaultHeaders(token string) http.Header {
	headers := make(http.Header, 1)
	headers.Set("snc-token", strings.TrimSpace(token))
	return headers
}

func applyHeaders(req *http.Request, headers http.Header) {
	for name, values := range headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
}

func formatDebugHeaders(headers http.Header, contentLength int64) string {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names)+1)
	for _, name := range names {
		value := strings.Join(headers.Values(name), ", ")
		if isSensitiveHeader(name) {
			value = "<redacted>"
		}
		parts = append(parts, name+"="+value)
	}
	if contentLength >= 0 {
		parts = append(parts, fmt.Sprintf("Content-Length=%d", contentLength))
	}
	return strings.Join(parts, ", ")
}

func isSensitiveHeader(name string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(name, "-", ""))
	return strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "authorization") ||
		strings.Contains(normalized, "cookie") ||
		strings.Contains(normalized, "apikey") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password")
}

func (c *Client) logf(format string, values ...any) {
	if c != nil && c.logger != nil {
		c.logger.Printf(format, values...)
	}
}

func decodeIdentity(body []byte) (string, string, string, string, error) {
	var response struct {
		Data struct {
			Records []struct {
				Name       string          `json:"name"`
				ID         json.RawMessage `json:"id"`
				DesignName string          `json:"designName"`
				Creator    string          `json:"creator"`
			} `json:"records"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", "", "", "", err
	}
	if len(response.Data.Records) == 0 {
		return "", "", "", "", errors.New("data.records is empty")
	}
	record := response.Data.Records[0]
	id, ok := scalarText(record.ID)
	if !ok {
		return "", "", "", "", errors.New("data.records[0].id is empty")
	}
	return record.Name, id, record.DesignName, record.Creator, nil
}

func decodeDetail(body []byte, inputField string) (json.RawMessage, []File, error) {
	var response struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, nil, err
	}
	for _, item := range response.Data {
		changeRaw, ok := lookupObject(item, "changedFormData")
		if !ok {
			continue
		}
		var inputValue json.RawMessage
		if inputRaw, ok := lookupObject(changeRaw, inputField); ok {
			if valueRaw, ok := lookupRaw(inputRaw, "value"); ok {
				inputValue = cloneRaw(valueRaw)
			}
		}
		files, err := extractFiles(changeRaw)
		if err != nil {
			return nil, nil, err
		}
		return inputValue, files, nil
	}
	return nil, nil, errors.New("data does not contain changedFormData")
}

func extractFiles(change map[string]json.RawMessage) ([]File, error) {
	keys := make([]string, 0)
	for key := range change {
		if strings.HasPrefix(strings.ToLower(key), "file_upload") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	files := make([]File, 0)
	for _, key := range keys {
		uploadObject, ok := decodeObject(change[key])
		if !ok {
			return nil, fmt.Errorf("%s must be an object", key)
		}
		valueRaw, ok := lookupRaw(uploadObject, "value")
		if !ok {
			return nil, fmt.Errorf("%s.value is missing", key)
		}
		items, err := decodeFileUploadItems(valueRaw)
		if err != nil {
			return nil, fmt.Errorf("%s.value must be an array: %w", key, err)
		}
		for _, item := range items {
			id, ok := lookupScalarRaw(item, "id")
			if !ok {
				return nil, fmt.Errorf("%s.value item id is missing", key)
			}
			name, ok := lookupStrictStringRaw(item, "name")
			if !ok {
				return nil, fmt.Errorf("%s.value item name is missing", key)
			}
			files = append(files, File{ID: id, Name: name})
		}
	}
	return files, nil
}

func decodeFileUploadItems(raw json.RawMessage) ([]map[string]json.RawMessage, error) {
	var itemValues []json.RawMessage
	if err := json.Unmarshal(raw, &itemValues); err != nil {
		var encoded string
		if stringErr := json.Unmarshal(raw, &encoded); stringErr != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(encoded), &itemValues); err != nil {
			return nil, err
		}
	}

	items := make([]map[string]json.RawMessage, 0, len(itemValues))
	for _, itemValue := range itemValues {
		item, ok := decodeObject(itemValue)
		if !ok {
			var encoded string
			if err := json.Unmarshal(itemValue, &encoded); err == nil {
				item, ok = decodeObject(json.RawMessage(encoded))
			}
		}
		if !ok {
			return nil, errors.New("file upload item must be an object or JSON object string")
		}
		items = append(items, item)
	}
	return items, nil
}

func replacePlaceholders(template string, values map[string]string, escape func(string) string) (string, error) {
	var missing string
	resolved := placeholderPattern.ReplaceAllStringFunc(template, func(placeholder string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(placeholder, "{"), "}")
		value, ok := values[name]
		if !ok {
			missing = name
			return placeholder
		}
		return escape(value)
	})
	if missing != "" {
		return "", fmt.Errorf("URL placeholder {%s} is not configured", missing)
	}
	return resolved, nil
}

func lookupObject(values map[string]json.RawMessage, key string) (map[string]json.RawMessage, bool) {
	raw, ok := lookupRaw(values, key)
	if !ok {
		return nil, false
	}
	return decodeObject(raw)
}

func lookupRaw(values map[string]json.RawMessage, key string) (json.RawMessage, bool) {
	if raw, ok := values[key]; ok {
		return raw, true
	}
	for candidate, raw := range values {
		if strings.EqualFold(candidate, key) {
			return raw, true
		}
	}
	return nil, false
}

func lookupScalarRaw(values map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := lookupRaw(values, key)
	if !ok {
		return "", false
	}
	return scalarText(raw)
}

func lookupStrictStringRaw(values map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := lookupRaw(values, key)
	if !ok {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	value = strings.TrimSpace(value)
	return value, value != ""
}

func scalarText(raw json.RawMessage) (string, bool) {
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		value = strings.TrimSpace(value)
		return value, value != ""
	}

	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err == nil {
		value := strings.TrimSpace(number.String())
		return value, value != ""
	}
	return "", false
}

func decodeObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return nil, false
	}
	return value, true
}

func marshalString(value string) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}

func ValidateFilename(name string) (string, error) {
	if strings.TrimSpace(name) != name {
		return "", fmt.Errorf("invalid remote file name %q", name)
	}
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `<>:"/\|?*`) {
		return "", fmt.Errorf("invalid remote file name %q", name)
	}
	for _, char := range name {
		if char < 32 {
			return "", fmt.Errorf("invalid remote file name %q", name)
		}
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return "", fmt.Errorf("invalid remote file name %q", name)
	}
	if filepath.Base(name) != name {
		return "", fmt.Errorf("invalid remote file name %q", name)
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	switch base {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return "", fmt.Errorf("invalid remote file name %q", name)
	}
	return name, nil
}

func responseError(operation string, resp *http.Response) error {
	summary, readErr := io.ReadAll(io.LimitReader(resp.Body, 512))
	if readErr != nil {
		return fmt.Errorf("%s failed with status %d: read response body: %w", operation, resp.StatusCode, readErr)
	}
	return fmt.Errorf("%s failed with status %d: %s", operation, resp.StatusCode, strings.TrimSpace(string(summary)))
}
