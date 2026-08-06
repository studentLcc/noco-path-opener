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
	BaseID      string          `json:"base_id"`
	TableID     string          `json:"table_id"`
	RecordID    json.RawMessage `json:"record_id"`
	PathField   string          `json:"path_field"`
	CurrentPath string          `json:"current_path"`
	BaseDir     string          `json:"base_dir"`
	FolderName  string          `json:"folder_name"`
	SyncProfile string          `json:"sync_profile"`
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

	if s.dispatcher == nil {
		writeError(w, http.StatusInternalServerError, "webhook dispatcher not configured")
		return
	}
	if err := s.dispatcher.Dispatch(actions.Request{
		BaseID:      baseID,
		TableID:     tableID,
		RecordID:    recordID,
		PathField:   pathField,
		CurrentPath: req.CurrentPath,
		BaseDir:     strings.TrimSpace(req.BaseDir),
		FolderName:  strings.TrimSpace(req.FolderName),
		SyncProfile: strings.TrimSpace(req.SyncProfile),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "webhook dispatch failed")
		return
	}
	writeJSON(w, http.StatusAccepted, queuedResponse{Success: true, Queued: true})
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
