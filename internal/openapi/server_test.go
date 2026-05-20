package openapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"noco-path-opener/internal/actions"
)

type fakeOpener struct {
	paths []string
	err   error
}

func (f *fakeOpener) Open(path string, isDir bool) error {
	f.paths = append(f.paths, path)
	return f.err
}

type fakeWebhookDispatcher struct {
	requests []actions.Request
	err      error
}

func (f *fakeWebhookDispatcher) Dispatch(req actions.Request) error {
	if f.err != nil {
		return f.err
	}
	f.requests = append(f.requests, req)
	return nil
}

type fakeActionRunner struct {
	run func(context.Context, actions.Request, actions.Controller) error
}

func (f fakeActionRunner) Run(ctx context.Context, req actions.Request, controller actions.Controller) error {
	return f.run(ctx, req, controller)
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

func TestWebhookRejectsInvalidRequestsWithJSON(t *testing.T) {
	tests := []struct {
		name   string
		method string
		body   string
		status int
		error  string
	}{
		{name: "unsupported method", method: http.MethodGet, body: `{}`, status: http.StatusMethodNotAllowed, error: "method not allowed"},
		{name: "invalid json", method: http.MethodPost, body: `{`, status: http.StatusBadRequest, error: "invalid JSON body"},
		{name: "missing base id", method: http.MethodPost, body: `{"table_id":"tbl","record_id":1,"path_field":"Path"}`, status: http.StatusBadRequest, error: "base_id is required"},
		{name: "empty base id", method: http.MethodPost, body: `{"base_id":"   ","table_id":"tbl","record_id":1,"path_field":"Path"}`, status: http.StatusBadRequest, error: "base_id is required"},
		{name: "missing table id", method: http.MethodPost, body: `{"base_id":"base","record_id":1,"path_field":"Path"}`, status: http.StatusBadRequest, error: "table_id is required"},
		{name: "empty table id", method: http.MethodPost, body: `{"base_id":"base","table_id":"   ","record_id":1,"path_field":"Path"}`, status: http.StatusBadRequest, error: "table_id is required"},
		{name: "missing record id", method: http.MethodPost, body: `{"base_id":"base","table_id":"tbl","path_field":"Path"}`, status: http.StatusBadRequest, error: "record_id is required"},
		{name: "null record id", method: http.MethodPost, body: `{"base_id":"base","table_id":"tbl","record_id":null,"path_field":"Path"}`, status: http.StatusBadRequest, error: "record_id is required"},
		{name: "empty string record id", method: http.MethodPost, body: `{"base_id":"base","table_id":"tbl","record_id":"","path_field":"Path"}`, status: http.StatusBadRequest, error: "record_id is required"},
		{name: "object record id", method: http.MethodPost, body: `{"base_id":"base","table_id":"tbl","record_id":{"id":1},"path_field":"Path"}`, status: http.StatusBadRequest, error: "record_id must be a string or number"},
		{name: "missing path field", method: http.MethodPost, body: `{"base_id":"base","table_id":"tbl","record_id":1}`, status: http.StatusBadRequest, error: "path_field is required"},
		{name: "empty path field", method: http.MethodPost, body: `{"base_id":"base","table_id":"tbl","record_id":1,"path_field":"   "}`, status: http.StatusBadRequest, error: "path_field is required"},
		{name: "trailing garbage", method: http.MethodPost, body: `{"base_id":"base","table_id":"tbl","record_id":1,"path_field":"Path"}garbage`, status: http.StatusBadRequest, error: "invalid JSON body"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dispatcher := &fakeWebhookDispatcher{}
			handler := NewServerWithWebhook(&fakeOpener{}, nil, dispatcher)
			req := httptest.NewRequest(tt.method, "/webhook", strings.NewReader(tt.body))
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
			if len(dispatcher.requests) != 0 {
				t.Fatalf("dispatcher called with %v, want no calls", dispatcher.requests)
			}
		})
	}
}

func TestWebhookReturnsErrorWhenDispatcherMissing(t *testing.T) {
	handler := NewServerWithWebhook(&fakeOpener{}, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{"base_id":"base","table_id":"tbl","record_id":123,"path_field":"Path"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body.String())
	}

	var body errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body.Success {
		t.Fatal("success = true, want false")
	}
	if body.Error != "webhook dispatcher not configured" {
		t.Fatalf("error = %q, want %q", body.Error, "webhook dispatcher not configured")
	}
}

func TestWebhookReturnsAcceptedAndQueuesDispatcher(t *testing.T) {
	dispatcher := &fakeWebhookDispatcher{}
	handler := NewServerWithWebhook(&fakeOpener{}, nil, dispatcher)
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{"base_id":"base","table_id":"tbl","record_id":123,"path_field":"Path","current_path":"/tmp/a.txt"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	var body queuedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if !body.Success || !body.Queued {
		t.Fatalf("body = %+v, want success and queued true", body)
	}
	if len(dispatcher.requests) != 1 {
		t.Fatalf("dispatcher calls = %d, want 1", len(dispatcher.requests))
	}

	got := dispatcher.requests[0]
	if got.BaseID != "base" || got.TableID != "tbl" || got.PathField != "Path" || got.CurrentPath != "/tmp/a.txt" {
		t.Fatalf("request = %+v, want base/table/path fields preserved", got)
	}
	if string(got.RecordID) != "123" {
		t.Fatalf("record_id = %s, want 123", got.RecordID)
	}
	if strings.TrimSpace(rec.Body.String()) != `{"success":true,"queued":true}` {
		t.Fatalf("body = %q, want queued success JSON", rec.Body.String())
	}
}

func TestWebhookSyncProfileIsOptional(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing sync profile",
			body: `{"base_id":"base","table_id":"tbl","record_id":123,"path_field":"Path"}`,
			want: "",
		},
		{
			name: "blank sync profile",
			body: `{"base_id":"base","table_id":"tbl","record_id":123,"path_field":"Path","sync_profile":"   "}`,
			want: "",
		},
		{
			name: "trims sync profile",
			body: `{"base_id":"base","table_id":"tbl","record_id":123,"path_field":"Path","sync_profile":"  project-sync  "}`,
			want: "project-sync",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dispatcher := &fakeWebhookDispatcher{}
			handler := NewServerWithWebhook(&fakeOpener{}, nil, dispatcher)
			req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusAccepted {
				t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
			}
			if len(dispatcher.requests) != 1 {
				t.Fatalf("dispatcher calls = %d, want 1", len(dispatcher.requests))
			}
			if got := dispatcher.requests[0].SyncProfile; got != tt.want {
				t.Fatalf("SyncProfile = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWebhookDuplicateRowReturnsAcceptedWithoutQueuingAnotherRun(t *testing.T) {
	entries := make(chan actions.Request, 2)
	release := make(chan struct{})
	defer close(release)
	flow := &actions.Flow{
		Runner: fakeActionRunner{run: func(ctx context.Context, req actions.Request, controller actions.Controller) error {
			entries <- req
			<-release
			return nil
		}},
	}
	handler := NewServerWithWebhook(&fakeOpener{}, nil, actions.NewAsyncDispatcher(flow, nil))

	first := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{"base_id":"base","table_id":"tbl","record_id":123,"path_field":"Path"}`))
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, first)

	if firstRec.Code != http.StatusAccepted {
		t.Fatalf("first status = %d, want 202; body=%s", firstRec.Code, firstRec.Body.String())
	}
	waitForWebhookRunnerEntry(t, entries)

	second := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{"base_id":"base","table_id":"tbl","record_id":123,"path_field":"Path"}`))
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, second)

	if secondRec.Code != http.StatusAccepted {
		t.Fatalf("second status = %d, want 202; body=%s", secondRec.Code, secondRec.Body.String())
	}
	var body queuedResponse
	if err := json.Unmarshal(secondRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if !body.Success || !body.Queued {
		t.Fatalf("body = %+v, want success and queued true", body)
	}

	select {
	case got := <-entries:
		t.Fatalf("duplicate row entered runner: %+v", got)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestWebhookPreservesStringRecordID(t *testing.T) {
	dispatcher := &fakeWebhookDispatcher{}
	handler := NewServerWithWebhook(&fakeOpener{}, nil, dispatcher)
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{"base_id":"base","table_id":"tbl","record_id":"rec-001","path_field":"Path"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	if len(dispatcher.requests) != 1 {
		t.Fatalf("dispatcher calls = %d, want 1", len(dispatcher.requests))
	}
	if string(dispatcher.requests[0].RecordID) != `"rec-001"` {
		t.Fatalf("record_id = %s, want %q", dispatcher.requests[0].RecordID, `"rec-001"`)
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

func waitForWebhookRunnerEntry(t *testing.T, entries <-chan actions.Request) actions.Request {
	t.Helper()

	select {
	case got := <-entries:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook runner entry")
		return actions.Request{}
	}
}
