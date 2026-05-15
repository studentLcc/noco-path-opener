package openapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeOpener struct {
	paths []string
	err   error
}

func (f *fakeOpener) Open(path string, isDir bool) error {
	f.paths = append(f.paths, path)
	return f.err
}

func TestOpenRejectsInvalidRequestsWithJSON(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
		body   string
		status int
		error  string
	}{
		{name: "unknown route", method: http.MethodPost, target: "/missing", body: `{}`, status: http.StatusNotFound, error: "not found"},
		{name: "unsupported method", method: http.MethodGet, target: "/open", body: `{}`, status: http.StatusMethodNotAllowed, error: "method not allowed"},
		{name: "invalid json", method: http.MethodPost, target: "/open", body: `{`, status: http.StatusBadRequest, error: "invalid JSON body"},
		{name: "missing path", method: http.MethodPost, target: "/open", body: `{}`, status: http.StatusBadRequest, error: "path is required"},
		{name: "empty path", method: http.MethodPost, target: "/open", body: `{"path":"   "}`, status: http.StatusBadRequest, error: "path is required"},
		{name: "nonexistent path", method: http.MethodPost, target: "/open", body: `{"path":"/path/that/does/not/exist"}`, status: http.StatusNotFound, error: "path does not exist"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opener := &fakeOpener{}
			handler := NewServer(opener, nil)
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.status, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", got)
			}

			var body errorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not JSON: %v", err)
			}
			if body.Success {
				t.Fatal("success = true, want false")
			}
			if body.Error != tt.error {
				t.Fatalf("error = %q, want %q", body.Error, tt.error)
			}
			if len(opener.paths) != 0 {
				t.Fatalf("opener called with %v, want no calls", opener.paths)
			}
		})
	}
}

func TestOpenReturnsFileAndDirectorySuccess(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tests := []struct {
		name     string
		path     string
		wantType string
	}{
		{name: "file", path: file, wantType: "file"},
		{name: "directory", path: dir, wantType: "directory"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opener := &fakeOpener{}
			handler := NewServer(opener, nil)
			req := httptest.NewRequest(http.MethodPost, "/open", strings.NewReader(`{"path":"`+escapeJSON(tt.path)+`"}`))
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
			}

			var body successResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not JSON: %v", err)
			}
			if !body.Success {
				t.Fatal("success = false, want true")
			}
			if body.Path != tt.path {
				t.Fatalf("path = %q, want %q", body.Path, tt.path)
			}
			if body.Type != tt.wantType {
				t.Fatalf("type = %q, want %q", body.Type, tt.wantType)
			}
			if len(opener.paths) != 1 || opener.paths[0] != tt.path {
				t.Fatalf("opener paths = %v, want [%q]", opener.paths, tt.path)
			}
		})
	}
}

func TestOpenReturnsInternalServerErrorWhenOpenFails(t *testing.T) {
	dir := t.TempDir()
	opener := &fakeOpener{err: errors.New("open failed")}
	handler := NewServer(opener, nil)
	req := httptest.NewRequest(http.MethodPost, "/open", strings.NewReader(`{"path":"`+escapeJSON(dir)+`"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body.String())
	}
	var body errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body.Error != "path exists but cannot be opened" {
		t.Fatalf("error = %q, want %q", body.Error, "path exists but cannot be opened")
	}
}

func TestOpenRejectsDisallowedPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	opener := &fakeOpener{}
	handler := NewServer(opener, []string{filepath.Join(dir, "allowed")})
	req := httptest.NewRequest(http.MethodPost, "/open", strings.NewReader(`{"path":"`+escapeJSON(file)+`"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	var body errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body.Error != "path not allowed" {
		t.Fatalf("error = %q, want %q", body.Error, "path not allowed")
	}
	if len(opener.paths) != 0 {
		t.Fatalf("opener called with %v, want no calls", opener.paths)
	}
}

func escapeJSON(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return strings.Trim(string(data), `"`)
}
