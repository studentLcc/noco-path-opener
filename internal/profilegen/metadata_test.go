package profilegen

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetadataClientListFieldsDecodesCommonFieldNames(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotToken string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotToken = r.Header.Get("xc-token")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"columns": [
				{"id": "c1", "title": "变更编号", "column_name": "change_no"},
				{"id": 42, "column_name": "status"},
				{"id": "c3", "name": "owner"}
			]
		}`))
	}))
	defer server.Close()

	client := NewMetadataClient(MetadataConfig{
		BaseURL: server.URL,
		Token:   "secret-token",
	})

	fields, err := client.ListFields(context.Background(), "p_local", "m_local")
	if err != nil {
		t.Fatalf("ListFields() error = %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodGet)
	}
	if gotPath != "/api/v2/meta/tables/m_local" {
		t.Fatalf("path = %q, want /api/v2/meta/tables/m_local", gotPath)
	}
	if gotToken != "secret-token" {
		t.Fatalf("xc-token = %q, want secret-token", gotToken)
	}
	if len(fields) != 3 {
		t.Fatalf("len(fields) = %d, want 3", len(fields))
	}
	if fields[0].ID != "c1" || fields[0].Name != "change_no" || fields[0].Title != "变更编号" || fields[0].DisplayName() != "变更编号" {
		t.Fatalf("fields[0] = %+v, want title-backed field", fields[0])
	}
	if fields[1].ID != "42" || fields[1].Name != "status" || fields[1].DisplayName() != "status" {
		t.Fatalf("fields[1] = %+v, want column_name-backed field", fields[1])
	}
	if fields[2].ID != "c3" || fields[2].Name != "owner" || fields[2].DisplayName() != "owner" {
		t.Fatalf("fields[2] = %+v, want name-backed field", fields[2])
	}
}

func TestMetadataClientListFieldsReportsNon2xxWithBodySummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "metadata unavailable because token is invalid", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewMetadataClient(MetadataConfig{
		BaseURL: server.URL,
		Token:   "secret-token",
	})

	_, err := client.ListFields(context.Background(), "p_local", "m_local")
	if err == nil {
		t.Fatal("ListFields() error = nil, want non-2xx error")
	}
	for _, want := range []string{"list fields", "503", "metadata unavailable"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ListFields() error = %q, want containing %q", err.Error(), want)
		}
	}
}

func TestMetadataClientListFieldsEscapesTableIDOnce(t *testing.T) {
	var gotRequestURI string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.RequestURI

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"title":"x"}]`))
	}))
	defer server.Close()

	client := NewMetadataClient(MetadataConfig{
		BaseURL: server.URL,
		Token:   "secret-token",
	})

	_, err := client.ListFields(context.Background(), "p_local", "tbl 1")
	if err != nil {
		t.Fatalf("ListFields() error = %v", err)
	}

	if gotRequestURI != "/api/v2/meta/tables/tbl%201" {
		t.Fatalf("request URI = %q, want /api/v2/meta/tables/tbl%%201", gotRequestURI)
	}
}

func TestMetadataClientListFieldsAppendsAfterBasePathAndStripsQuery(t *testing.T) {
	var gotPath string
	var gotRawQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotRawQuery = r.URL.RawQuery

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"title":"x"}]`))
	}))
	defer server.Close()

	client := NewMetadataClient(MetadataConfig{
		BaseURL: server.URL + "/noco?keep=1#fragment",
		Token:   "secret-token",
	})

	_, err := client.ListFields(context.Background(), "p_local", "m_local")
	if err != nil {
		t.Fatalf("ListFields() error = %v", err)
	}

	if gotPath != "/noco/api/v2/meta/tables/m_local" {
		t.Fatalf("path = %q, want /noco/api/v2/meta/tables/m_local", gotPath)
	}
	if gotRawQuery != "" {
		t.Fatalf("raw query = %q, want empty", gotRawQuery)
	}
}

func TestMetadataClientListFieldsTrimsConfig(t *testing.T) {
	var gotToken string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("xc-token")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"title":"x"}]`))
	}))
	defer server.Close()

	client := NewMetadataClient(MetadataConfig{
		BaseURL: " \t" + server.URL + "\n",
		Token:   " secret-token \n",
	})

	_, err := client.ListFields(context.Background(), "p_local", "m_local")
	if err != nil {
		t.Fatalf("ListFields() error = %v", err)
	}

	if gotToken != "secret-token" {
		t.Fatalf("xc-token = %q, want secret-token", gotToken)
	}
}

func TestDecodeFieldsAcceptsAlternateEnvelopes(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "raw array", body: `[{"title":"变更编号"}]`},
		{name: "fields key", body: `{"fields":[{"title":"变更编号"}]}`},
		{name: "list key", body: `{"list":[{"title":"变更编号"}]}`},
		{name: "nested columns list", body: `{"columns":{"list":[{"title":"变更编号"}]}}`},
		{name: "data key", body: `{"data":[{"title":"变更编号"}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields, err := decodeFields(strings.NewReader(tt.body))
			if err != nil {
				t.Fatalf("decodeFields() error = %v", err)
			}
			if len(fields) != 1 || fields[0].DisplayName() != "变更编号" {
				t.Fatalf("decodeFields() = %+v, want one field named 变更编号", fields)
			}
		})
	}
}

func TestDecodeFieldsRejectsMalformedOrEmptyResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "not json", body: `{`, want: "decode metadata response"},
		{name: "no field list", body: `{"table":"m_local"}`, want: "does not contain a field list"},
		{name: "empty field list", body: `{"columns":[]}`, want: "returned no usable fields"},
		{name: "field has no usable name", body: `{"columns":[{"id":"c1"}]}`, want: "no usable field name"},
		{name: "trailing garbage", body: `[{"title":"x"}] garbage`, want: "decode metadata response"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeFields(strings.NewReader(tt.body))
			if err == nil {
				t.Fatal("decodeFields() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("decodeFields() error = %q, want containing %q", err.Error(), tt.want)
			}
		})
	}
}

func TestMetadataClientRequiresConfiguration(t *testing.T) {
	tests := []struct {
		name string
		cfg  MetadataConfig
	}{
		{name: "missing url", cfg: MetadataConfig{Token: "token"}},
		{name: "missing token", cfg: MetadataConfig{BaseURL: "http://example.test"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMetadataClient(tt.cfg)

			_, err := client.ListFields(context.Background(), "p_local", "m_local")
			if err == nil {
				t.Fatal("ListFields() error = nil, want configuration error")
			}
			if !strings.Contains(err.Error(), "NocoDB URL and token are required") {
				t.Fatalf("ListFields() error = %q, want configuration error", err.Error())
			}
		})
	}
}
