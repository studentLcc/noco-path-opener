package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"noco-path-opener/internal/actions"
	"noco-path-opener/internal/pathauth"
	"noco-path-opener/internal/remotesync"
)

type Opener interface {
	Open(path string, isDir bool) error
}

type Server struct {
	opener       Opener
	allowedRoots []string
	dispatcher   actions.Dispatcher
}

type openRequest struct {
	Path string `json:"path"`
}

type webhookRequest struct {
	BaseID      string           `json:"base_id"`
	TableID     string           `json:"table_id"`
	RecordID    json.RawMessage  `json:"record_id"`
	PathField   string           `json:"path_field"`
	CurrentPath string           `json:"current_path"`
	BaseDir     string           `json:"base_dir"`
	FolderName  string           `json:"folder_name"`
	RemoteSync  *remotesync.Spec `json:"remote_sync"`
}

type successResponse struct {
	Success bool   `json:"success"`
	Path    string `json:"path"`
	Type    string `json:"type"`
}

type queuedResponse struct {
	Success bool `json:"success"`
	Queued  bool `json:"queued"`
}

type errorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

func NewServer(opener Opener, allowedRoots []string) http.Handler {
	return NewServerWithWebhook(opener, allowedRoots, nil)
}

func NewServerWithWebhook(opener Opener, allowedRoots []string, dispatcher actions.Dispatcher) http.Handler {
	return &Server{opener: opener, allowedRoots: allowedRoots, dispatcher: dispatcher}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/open":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleOpen(w, r)
	case "/webhook":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleWebhook(w, r)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	var req openRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logResult("", "invalid JSON body")
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	path := strings.TrimSpace(req.Path)
	if path == "" {
		logResult(path, "path is required")
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	allowed, err := pathauth.IsAllowed(path, s.allowedRoots)
	if err != nil {
		logResult(path, fmt.Sprintf("authorization error: %v", err))
		writeError(w, http.StatusForbidden, "path not allowed")
		return
	}
	if !allowed {
		logResult(path, "path not allowed")
		writeError(w, http.StatusForbidden, "path not allowed")
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			logResult(path, "path does not exist")
			writeError(w, http.StatusNotFound, "path does not exist")
			return
		}
		logResult(path, fmt.Sprintf("stat failed: %v", err))
		writeError(w, http.StatusInternalServerError, "path exists but cannot be opened")
		return
	}

	isDir := info.IsDir()
	if err := s.opener.Open(path, isDir); err != nil {
		logResult(path, fmt.Sprintf("open failed: %v", err))
		writeError(w, http.StatusInternalServerError, "path exists but cannot be opened")
		return
	}

	pathType := "file"
	if isDir {
		pathType = "directory"
	}
	logResult(path, "opened "+pathType)
	writeJSON(w, http.StatusOK, successResponse{Success: true, Path: path, Type: pathType})
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	var req webhookRequest
	if err := decodeWebhookJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	baseID := strings.TrimSpace(req.BaseID)
	if baseID == "" {
		writeError(w, http.StatusBadRequest, "base_id is required")
		return
	}
	tableID := strings.TrimSpace(req.TableID)
	if tableID == "" {
		writeError(w, http.StatusBadRequest, "table_id is required")
		return
	}
	recordID, err := normalizeRecordID(req.RecordID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pathField := strings.TrimSpace(req.PathField)
	if pathField == "" {
		writeError(w, http.StatusBadRequest, "path_field is required")
		return
	}
	remoteSync, err := normalizeRemoteSync(req.RemoteSync)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if s.dispatcher == nil {
		writeError(w, http.StatusInternalServerError, "webhook dispatcher not configured")
		return
	}
	if err := s.dispatcher.Dispatch(actions.Request{
		BaseID:            baseID,
		TableID:           tableID,
		RecordID:          recordID,
		PathField:         pathField,
		CurrentPath:       req.CurrentPath,
		BaseDir:           strings.TrimSpace(req.BaseDir),
		FolderName:        strings.TrimSpace(req.FolderName),
		DynamicRemoteSync: remoteSync,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "webhook dispatch failed")
		return
	}
	log.Printf("webhook queued base_id=%q table_id=%q record_id=%q dynamic_sync=%t", baseID, tableID, string(recordID), remoteSync != nil)
	writeJSON(w, http.StatusAccepted, queuedResponse{Success: true, Queued: true})
}

func normalizeRemoteSync(spec *remotesync.Spec) (*remotesync.Spec, error) {
	if spec == nil {
		return nil, nil
	}
	normalized := &remotesync.Spec{
		PostURL:               strings.TrimSpace(spec.PostURL),
		GetURL:                strings.TrimSpace(spec.GetURL),
		DownloadURL:           strings.TrimSpace(spec.DownloadURL),
		ProcessCode:           strings.TrimSpace(spec.ProcessCode),
		InputField:            strings.TrimSpace(spec.InputField),
		RequestTimeoutSeconds: spec.RequestTimeoutSeconds,
		FieldMapping:          make(map[string]string, len(spec.FieldMapping)),
	}
	switch {
	case normalized.PostURL == "":
		return nil, fmt.Errorf("remote_sync.post_url is required")
	case normalized.GetURL == "":
		return nil, fmt.Errorf("remote_sync.get_url is required")
	case normalized.DownloadURL == "":
		return nil, fmt.Errorf("remote_sync.download_url is required")
	case !strings.Contains(normalized.GetURL, "{id}"):
		return nil, fmt.Errorf("remote_sync.get_url must contain {id}")
	case !strings.Contains(normalized.DownloadURL, "{file_id}") && !strings.Contains(normalized.DownloadURL, "{id}"):
		return nil, fmt.Errorf("remote_sync.download_url must contain {file_id} or {id}")
	case !strings.Contains(normalized.DownloadURL, "{file_name}") && !strings.Contains(normalized.DownloadURL, "{name}"):
		return nil, fmt.Errorf("remote_sync.download_url must contain {file_name} or {name}")
	case normalized.ProcessCode == "":
		return nil, fmt.Errorf("remote_sync.processCode is required")
	case normalized.InputField == "":
		return nil, fmt.Errorf("remote_sync.input_field is required")
	case normalized.RequestTimeoutSeconds < 0 || normalized.RequestTimeoutSeconds > 120:
		return nil, fmt.Errorf("remote_sync.request_timeout_seconds must be between 1 and 120")
	case len(spec.FieldMapping) == 0:
		return nil, fmt.Errorf("remote_sync.field_mapping is required")
	}
	if _, err := remotesync.ResolveURL(normalized.PostURL, map[string]string{}); err != nil {
		return nil, fmt.Errorf("remote_sync.post_url is invalid: %w", err)
	}
	if _, err := remotesync.ResolveURL(normalized.GetURL, map[string]string{"id": "record-id"}); err != nil {
		return nil, fmt.Errorf("remote_sync.get_url is invalid: %w", err)
	}
	if _, err := remotesync.ResolveURL(normalized.DownloadURL, map[string]string{
		"id":        "file-id",
		"file_id":   "file-id",
		"name":      "file-name",
		"file_name": "file-name",
	}); err != nil {
		return nil, fmt.Errorf("remote_sync.download_url is invalid: %w", err)
	}

	supported := map[string]bool{
		"name":         true,
		"id":           true,
		"designName":   true,
		"creator":      true,
		"input_value":  true,
		"file_uploads": true,
	}
	targets := make(map[string]string, len(spec.FieldMapping))
	sources := make(map[string]struct{}, len(spec.FieldMapping))
	for source, target := range spec.FieldMapping {
		source = strings.TrimSpace(source)
		target = strings.TrimSpace(target)
		if _, exists := sources[source]; exists {
			return nil, fmt.Errorf("remote_sync.field_mapping source %q is duplicated", source)
		}
		sources[source] = struct{}{}
		if !supported[source] {
			return nil, fmt.Errorf("remote_sync.field_mapping source %q is unsupported", source)
		}
		if target == "" {
			return nil, fmt.Errorf("remote_sync.field_mapping[%q] target is required", source)
		}
		if previous, exists := targets[target]; exists {
			return nil, fmt.Errorf("remote_sync.field_mapping target %q is used by both %q and %q", target, previous, source)
		}
		targets[target] = source
		normalized.FieldMapping[source] = target
	}
	return normalized, nil
}

func decodeWebhookJSON(body io.Reader, v any) error {
	decoder := json.NewDecoder(body)
	decoder.UseNumber()
	if err := decoder.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing data")
		}
		return err
	}
	return nil
}

func normalizeRecordID(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, fmt.Errorf("record_id is required")
	}
	if trimmed[0] == '"' {
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return nil, fmt.Errorf("record_id must be a string or number")
		}
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("record_id is required")
		}
		return append(json.RawMessage(nil), trimmed...), nil
	}
	if trimmed[0] >= '0' && trimmed[0] <= '9' || trimmed[0] == '-' {
		var value json.Number
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return nil, fmt.Errorf("record_id must be a string or number")
		}
		return append(json.RawMessage(nil), trimmed...), nil
	}
	return nil, fmt.Errorf("record_id must be a string or number")
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Success: false, Error: message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func logResult(path string, result string) {
	log.Printf("open path=%q result=%q", path, result)
}
