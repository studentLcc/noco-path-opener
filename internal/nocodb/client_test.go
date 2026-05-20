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

func TestReadRecordSendsGetRequestAndPreservesRawFields(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotToken string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotToken = r.Header.Get("xc-token")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Id":123,"变更单号":"BG-001","状态":"待处理","Count":42,"Active":true}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "secret-token",
	})

	record, err := client.ReadRecord(context.Background(), ReadRecordRequest{
		BaseID:   "base-1",
		TableID:  "table-1",
		RecordID: json.RawMessage(`12345`),
	})
	if err != nil {
		t.Fatalf("ReadRecord() error = %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodGet)
	}
	if gotPath != "/api/v3/data/base-1/table-1/records/12345" {
		t.Fatalf("path = %q, want /api/v3/data/base-1/table-1/records/12345", gotPath)
	}
	if gotToken != "secret-token" {
		t.Fatalf("xc-token = %q, want secret-token", gotToken)
	}
	if string(record.Fields["Id"]) != "123" {
		t.Fatalf("fields[Id] = %s, want raw number JSON", record.Fields["Id"])
	}
	if string(record.Fields["变更单号"]) != `"BG-001"` {
		t.Fatalf("fields[变更单号] = %s, want raw string JSON", record.Fields["变更单号"])
	}
	if string(record.Fields["状态"]) != `"待处理"` {
		t.Fatalf("fields[状态] = %s, want raw string JSON", record.Fields["状态"])
	}
	if string(record.Fields["Count"]) != "42" {
		t.Fatalf("fields[Count] = %s, want raw number JSON", record.Fields["Count"])
	}
	if string(record.Fields["Active"]) != "true" {
		t.Fatalf("fields[Active] = %s, want raw bool JSON", record.Fields["Active"])
	}
}

func TestReadRecordUsesStringRecordIDInPath(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Id":1}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "secret-token",
	})

	_, err := client.ReadRecord(context.Background(), ReadRecordRequest{
		BaseID:   "base",
		TableID:  "table",
		RecordID: json.RawMessage(`"rec 001"`),
	})
	if err != nil {
		t.Fatalf("ReadRecord() error = %v", err)
	}

	if gotPath != "/api/v3/data/base/table/records/rec%20001" {
		t.Fatalf("path = %q, want escaped string record ID path", gotPath)
	}
}

func TestReadRecordDecodesOfficialV3EnvelopeFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"fields":{"变更单号":"BG-001","状态":"待处理","Count":42,"Active":true}}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "secret-token",
	})

	record, err := client.ReadRecord(context.Background(), ReadRecordRequest{
		BaseID:   "base",
		TableID:  "table",
		RecordID: json.RawMessage(`1`),
	})
	if err != nil {
		t.Fatalf("ReadRecord() error = %v", err)
	}

	if string(record.Fields["变更单号"]) != `"BG-001"` {
		t.Fatalf("fields[变更单号] = %s, want nested raw string JSON", record.Fields["变更单号"])
	}
	if string(record.Fields["状态"]) != `"待处理"` {
		t.Fatalf("fields[状态] = %s, want nested raw string JSON", record.Fields["状态"])
	}
	if string(record.Fields["Count"]) != "42" {
		t.Fatalf("fields[Count] = %s, want nested raw number JSON", record.Fields["Count"])
	}
	if string(record.Fields["Active"]) != "true" {
		t.Fatalf("fields[Active] = %s, want nested raw bool JSON", record.Fields["Active"])
	}
	if _, ok := record.Fields["fields"]; ok {
		t.Fatalf("fields contains envelope key %q, want only nested user fields", "fields")
	}
}

func TestReadRecordPreservesBareRecordWithFieldsColumn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Id":123,"fields":{"nested":"value"},"状态":"待处理"}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "secret-token",
	})

	record, err := client.ReadRecord(context.Background(), ReadRecordRequest{
		BaseID:   "base",
		TableID:  "table",
		RecordID: json.RawMessage(`123`),
	})
	if err != nil {
		t.Fatalf("ReadRecord() error = %v", err)
	}

	if string(record.Fields["Id"]) != "123" {
		t.Fatalf("fields[Id] = %s, want top-level raw number JSON", record.Fields["Id"])
	}
	if string(record.Fields["fields"]) != `{"nested":"value"}` {
		t.Fatalf("fields[fields] = %s, want top-level nested object JSON", record.Fields["fields"])
	}
	if string(record.Fields["状态"]) != `"待处理"` {
		t.Fatalf("fields[状态] = %s, want top-level raw string JSON", record.Fields["状态"])
	}
	if len(record.Fields) != 3 {
		t.Fatalf("len(fields) = %d, want whole top-level record", len(record.Fields))
	}
}

func TestQueryRecordsSendsGetRequestWithEncodedQuotedWhere(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotRawQuery string
	var gotWhere string
	var gotToken string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotRawQuery = r.URL.RawQuery
		gotWhere = r.URL.Query().Get("where")
		gotToken = r.Header.Get("xc-token")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"list":[]}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "secret-token",
	})

	_, err := client.QueryRecords(context.Background(), QueryRecordsRequest{
		BaseID:  "base-1",
		TableID: "table-1",
		Where:   EqualWhere(`File "Name"`, `C:\docs\a.docx`),
	})
	if err != nil {
		t.Fatalf("QueryRecords() error = %v", err)
	}

	wantWhere := `("File \"Name\"",eq,"C:\\docs\\a.docx")`
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodGet)
	}
	if gotPath != "/api/v3/data/base-1/table-1/records" {
		t.Fatalf("path = %q, want /api/v3/data/base-1/table-1/records", gotPath)
	}
	if gotToken != "secret-token" {
		t.Fatalf("xc-token = %q, want secret-token", gotToken)
	}
	if gotWhere != wantWhere {
		t.Fatalf("where = %q, want %q", gotWhere, wantWhere)
	}
	if !strings.Contains(gotRawQuery, "where=") || strings.Contains(gotRawQuery, `"`) || strings.Contains(gotRawQuery, `\`) {
		t.Fatalf("raw query = %q, want encoded where", gotRawQuery)
	}
}

func TestQueryRecordsOmitsBlankWhere(t *testing.T) {
	var gotRawQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"list":[]}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "secret-token",
	})

	_, err := client.QueryRecords(context.Background(), QueryRecordsRequest{
		BaseID:  "base",
		TableID: "table",
		Where:   " \t\n ",
	})
	if err != nil {
		t.Fatalf("QueryRecords() error = %v", err)
	}

	if gotRawQuery != "" {
		t.Fatalf("raw query = %q, want no query for blank where", gotRawQuery)
	}
}

func TestQueryRecordsDecodesOfficialV3RecordsEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"records":[{"id":1,"fields":{"变更单号":"BG-001","状态":"待处理","Count":7}}],"next":null}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "secret-token",
	})

	records, err := client.QueryRecords(context.Background(), QueryRecordsRequest{
		BaseID:  "base",
		TableID: "table",
		Where:   EqualWhere("状态", "待处理"),
	})
	if err != nil {
		t.Fatalf("QueryRecords() error = %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if string(records[0].Fields["变更单号"]) != `"BG-001"` {
		t.Fatalf("fields[变更单号] = %s, want nested raw string JSON", records[0].Fields["变更单号"])
	}
	if string(records[0].Fields["状态"]) != `"待处理"` {
		t.Fatalf("fields[状态] = %s, want nested raw string JSON", records[0].Fields["状态"])
	}
	if string(records[0].Fields["Count"]) != "7" {
		t.Fatalf("fields[Count] = %s, want nested raw number JSON", records[0].Fields["Count"])
	}
	if _, ok := records[0].Fields["fields"]; ok {
		t.Fatalf("fields contains envelope key %q, want only nested user fields", "fields")
	}
}

func TestQueryRecordsDecodesWrappedAndBareListResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "wrapped list",
			body: `{"list":[{"Id":1,"变更单号":"BG-001","状态":"待处理","Count":7}]}`,
		},
		{
			name: "bare list",
			body: `[{"Id":1,"变更单号":"BG-001","状态":"待处理","Count":7}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := NewClient(Config{
				BaseURL: server.URL,
				Token:   "secret-token",
			})

			records, err := client.QueryRecords(context.Background(), QueryRecordsRequest{
				BaseID:  "base",
				TableID: "table",
				Where:   EqualWhere("Path", `C:\a.txt`),
			})
			if err != nil {
				t.Fatalf("QueryRecords() error = %v", err)
			}

			if len(records) != 1 {
				t.Fatalf("len(records) = %d, want 1", len(records))
			}
			if string(records[0].Fields["Id"]) != "1" {
				t.Fatalf("fields[Id] = %s, want raw number JSON", records[0].Fields["Id"])
			}
			if string(records[0].Fields["变更单号"]) != `"BG-001"` {
				t.Fatalf("fields[变更单号] = %s, want raw string JSON", records[0].Fields["变更单号"])
			}
			if string(records[0].Fields["状态"]) != `"待处理"` {
				t.Fatalf("fields[状态] = %s, want raw string JSON", records[0].Fields["状态"])
			}
			if string(records[0].Fields["Count"]) != "7" {
				t.Fatalf("fields[Count] = %s, want raw number JSON", records[0].Fields["Count"])
			}
		})
	}
}

func TestUpdateFieldsSendsPatchRawJSONFieldValues(t *testing.T) {
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

	err := client.UpdateFields(context.Background(), UpdateFieldsRequest{
		BaseID:   "base-1",
		TableID:  "table-1",
		RecordID: json.RawMessage(`"rec-001"`),
		Fields: map[string]json.RawMessage{
			"Path":   json.RawMessage(`"C:\\docs\\a.docx"`),
			"Count":  json.RawMessage(`42`),
			"Active": json.RawMessage(`true`),
		},
	})
	if err != nil {
		t.Fatalf("UpdateFields() error = %v", err)
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
		ID     json.RawMessage            `json:"id"`
		Fields map[string]json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal(gotBody, &body); err != nil {
		t.Fatalf("request body is not JSON: %v; body=%s", err, gotBody)
	}
	if string(body.ID) != `"rec-001"` {
		t.Fatalf("id = %s, want %q", body.ID, `"rec-001"`)
	}
	if string(body.Fields["Path"]) != `"C:\\docs\\a.docx"` {
		t.Fatalf("fields[Path] = %s, want raw string JSON", body.Fields["Path"])
	}
	if string(body.Fields["Count"]) != "42" {
		t.Fatalf("fields[Count] = %s, want raw number JSON", body.Fields["Count"])
	}
	if string(body.Fields["Active"]) != "true" {
		t.Fatalf("fields[Active] = %s, want raw bool JSON", body.Fields["Active"])
	}
}

func TestSyncMethodsRequireConfig(t *testing.T) {
	tests := []struct {
		name string
		call func(*Client) error
	}{
		{
			name: "read",
			call: func(client *Client) error {
				_, err := client.ReadRecord(context.Background(), ReadRecordRequest{
					BaseID:   "base",
					TableID:  "table",
					RecordID: json.RawMessage(`1`),
				})
				return err
			},
		},
		{
			name: "query",
			call: func(client *Client) error {
				_, err := client.QueryRecords(context.Background(), QueryRecordsRequest{
					BaseID:  "base",
					TableID: "table",
					Where:   EqualWhere("Path", "/tmp/a.txt"),
				})
				return err
			},
		},
		{
			name: "update fields",
			call: func(client *Client) error {
				return client.UpdateFields(context.Background(), UpdateFieldsRequest{
					BaseID:   "base",
					TableID:  "table",
					RecordID: json.RawMessage(`1`),
					Fields: map[string]json.RawMessage{
						"Path": json.RawMessage(`"/tmp/a.txt"`),
					},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(Config{})

			err := tt.call(client)

			if err == nil {
				t.Fatal("error = nil, want config error")
			}
			if !strings.Contains(err.Error(), "nocodb_url and nocodb_token must be configured") {
				t.Fatalf("error = %q, want config message", err.Error())
			}
		})
	}
}

func TestSyncMethodsReturnNon2xxErrorWithStatusAndBody(t *testing.T) {
	longBody := strings.Repeat("x", 600)

	tests := []struct {
		name      string
		call      func(*Client) error
		operation string
	}{
		{
			name: "read",
			call: func(client *Client) error {
				_, err := client.ReadRecord(context.Background(), ReadRecordRequest{
					BaseID:   "base",
					TableID:  "table",
					RecordID: json.RawMessage(`1`),
				})
				return err
			},
			operation: "read",
		},
		{
			name: "query",
			call: func(client *Client) error {
				_, err := client.QueryRecords(context.Background(), QueryRecordsRequest{
					BaseID:  "base",
					TableID: "table",
					Where:   EqualWhere("Path", "/tmp/a.txt"),
				})
				return err
			},
			operation: "query",
		},
		{
			name: "update fields",
			call: func(client *Client) error {
				return client.UpdateFields(context.Background(), UpdateFieldsRequest{
					BaseID:   "base",
					TableID:  "table",
					RecordID: json.RawMessage(`1`),
					Fields: map[string]json.RawMessage{
						"Path": json.RawMessage(`"/tmp/a.txt"`),
					},
				})
			},
			operation: "update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, longBody, http.StatusBadGateway)
			}))
			defer server.Close()

			client := NewClient(Config{
				BaseURL: server.URL,
				Token:   "secret-token",
			})

			err := tt.call(client)

			if err == nil {
				t.Fatal("error = nil, want non-2xx error")
			}
			if !strings.Contains(err.Error(), "status 502") {
				t.Fatalf("error = %q, want status 502", err.Error())
			}
			if !strings.Contains(err.Error(), "nocodb "+tt.operation+" failed") {
				t.Fatalf("error = %q, want operation %q", err.Error(), tt.operation)
			}
			if !strings.Contains(err.Error(), strings.Repeat("x", 100)) {
				t.Fatalf("error = %q, want response body summary", err.Error())
			}
			if strings.Contains(err.Error(), strings.Repeat("x", 513)) {
				t.Fatalf("error = %q, want short response body summary", err.Error())
			}
		})
	}
}

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
