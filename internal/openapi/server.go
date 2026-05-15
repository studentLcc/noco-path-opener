package openapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"noco-path-opener/internal/pathauth"
)

type Opener interface {
	Open(path string, isDir bool) error
}

type Server struct {
	opener       Opener
	allowedRoots []string
}

type openRequest struct {
	Path string `json:"path"`
}

type successResponse struct {
	Success bool   `json:"success"`
	Path    string `json:"path"`
	Type    string `json:"type"`
}

type errorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

func NewServer(opener Opener, allowedRoots []string) http.Handler {
	return &Server{opener: opener, allowedRoots: allowedRoots}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/open" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.handleOpen(w, r)
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
