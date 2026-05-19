package nocodb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdateRecordSendsPatchRequestShapeAndHeaders(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotToken string
	var gotContentType string
	var gotBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotToken = r.Header.Get("xc-token")
		gotContentType = r.Header.Get("Content-Type")

		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "secret-token",
	})

	err := client.UpdateRecord(context.Background(), UpdateRequest{
		BaseID:    "base-1",
		TableID:   "table-1",
		RecordID:  json.RawMessage(`12345`),
		PathField: "LocalPath",
		PathValue: `D:\docs\a.docx`,
	})
	if err != nil {
		t.Fatalf("UpdateRecord() error = %v", err)
	}

	if gotMethod != http.MethodPatch {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodPatch)
	}
	if gotPath != "/api/v3/data/base-1/table-1/records" {
		t.Fatalf("path = %q, want /api/v3/data/base-1/table-1/records", gotPath)
	}
	if gotToken != "secret-token" {
		t.Fatalf("xc-token = %q, want secret-token", gotToken)
	}
	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotContentType)
	}

	var body struct {
		ID     json.RawMessage   `json:"id"`
		Fields map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(gotBody, &body); err != nil {
		t.Fatalf("request body is not JSON: %v; body=%s", err, gotBody)
	}
	if string(body.ID) != "12345" {
		t.Fatalf("id = %s, want 12345", body.ID)
	}
	if body.Fields["LocalPath"] != `D:\docs\a.docx` {
		t.Fatalf("fields[LocalPath] = %q, want path value", body.Fields["LocalPath"])
	}
	if len(body.Fields) != 1 {
		t.Fatalf("fields = %+v, want only LocalPath", body.Fields)
	}
}

func TestUpdateRecordPreservesStringRecordID(t *testing.T) {
	var gotBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "secret-token",
	})

	err := client.UpdateRecord(context.Background(), UpdateRequest{
		BaseID:    "base",
		TableID:   "table",
		RecordID:  json.RawMessage(`"rec-001"`),
		PathField: "Path",
		PathValue: "/tmp/a.txt",
	})
	if err != nil {
		t.Fatalf("UpdateRecord() error = %v", err)
	}

	var body struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(gotBody, &body); err != nil {
		t.Fatalf("request body is not JSON: %v; body=%s", err, gotBody)
	}
	if string(body.ID) != `"rec-001"` {
		t.Fatalf("id = %s, want %q", body.ID, `"rec-001"`)
	}
}

func TestUpdateRecordRequiresConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{name: "missing base url", cfg: Config{Token: "secret-token"}},
		{name: "missing token", cfg: Config{BaseURL: "http://example.test"}},
		{name: "trimmed empty", cfg: Config{BaseURL: "   ", Token: "   "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.cfg)

			err := client.UpdateRecord(context.Background(), UpdateRequest{
				BaseID:    "base",
				TableID:   "table",
				RecordID:  json.RawMessage(`1`),
				PathField: "Path",
				PathValue: "/tmp/a.txt",
			})

			if err == nil {
				t.Fatal("UpdateRecord() error = nil, want config error")
			}
			if !strings.Contains(err.Error(), "nocodb_url and nocodb_token must be configured") {
				t.Fatalf("error = %q, want config message", err.Error())
			}
		})
	}
}

func TestUpdateRecordReturnsNon2xxErrorWithStatusAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request details", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "secret-token",
	})

	err := client.UpdateRecord(context.Background(), UpdateRequest{
		BaseID:    "base",
		TableID:   "table",
		RecordID:  json.RawMessage(`1`),
		PathField: "Path",
		PathValue: "/tmp/a.txt",
	})

	if err == nil {
		t.Fatal("UpdateRecord() error = nil, want non-2xx error")
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("error = %q, want status 400", err.Error())
	}
	if !strings.Contains(err.Error(), "bad request details") {
		t.Fatalf("error = %q, want response body summary", err.Error())
	}
}
