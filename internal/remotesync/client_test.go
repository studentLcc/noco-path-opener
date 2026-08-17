package remotesync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientFetchUsesRemoteSyncParamsTemplateAndFindsFirstChangeFormData(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "remote_sync_params.json")
	template := `{
  "params": {
    "condition": {"processCode": "xxxx"},
    "pagination": {"pagenum": 1, "pagesize": 20, "sort": "null"}
  }
}`
	if err := os.WriteFile(templatePath, []byte(template), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var postBody map[string]any
	var postToken string
	var detailToken string
	var detailPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			postToken = r.Header.Get("snc-token")
			if err := json.NewDecoder(r.Body).Decode(&postBody); err != nil {
				t.Fatalf("decode POST body: %v", err)
			}
			_, _ = io.WriteString(w, `{
			  "msgCode": 200,
			  "data": {"records": [{"name": "项目A", "id": "remote-1", "designName": "设计A", "creator": "张三"}]}
			}`)
		case http.MethodGet:
			detailToken = r.Header.Get("snc-token")
			detailPath = r.URL.Path
			_, _ = io.WriteString(w, `{
			  "msgCode": 200,
			  "data": [
			    {"other": "skip"},
			    {"other": "skip-again"},
			    {"changedFormData": {
			      "input": {"value": "wrong-value"},
			      "file_upload_1": {"value": [
			        {"id": "img001", "name": "图片1.png"},
			        {"id": "img002", "name": "图片2.jpg"}
			      ]},
			      "file_upload_extra": {"value": [
			        {"id": "doc001", "name": "报告.pdf"}
			      ]}
			    }}
			  ]
			}`)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		HTTPClient:         server.Client(),
		ParamsTemplatePath: templatePath,
	})
	result, err := client.Fetch(context.Background(), FetchRequest{
		PostURL:     server.URL + "/list",
		GetURL:      server.URL + "/detail/{id}",
		ProcessCode: "process-from-webhook",
		InputField:  "input",
		Token:       "secret-token",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if postToken != "secret-token" || detailToken != "secret-token" {
		t.Fatalf("tokens = POST %q, GET %q, want secret-token", postToken, detailToken)
	}
	if detailPath != "/detail/remote-1" {
		t.Fatalf("detail path = %q, want /detail/remote-1", detailPath)
	}
	params, ok := postBody["params"].(map[string]any)
	if !ok {
		t.Fatalf("params = %T, want object", postBody["params"])
	}
	condition, ok := params["condition"].(map[string]any)
	if !ok || condition["processCode"] != "process-from-webhook" {
		t.Fatalf("condition = %#v, want processCode from webhook", params["condition"])
	}

	if result.Name != "项目A" || result.ID != "remote-1" || result.DesignName != "设计A" || result.Creator != "张三" {
		t.Fatalf("result identity = %+v, want initial record fields", result)
	}
	if string(result.InputValue) != `"wrong-value"` {
		t.Fatalf("InputValue = %s, want first changedFormData input value", result.InputValue)
	}
	if len(result.Files) != 3 || result.Files[0].ID != "img001" || result.Files[0].Name != "图片1.png" || result.Files[2].ID != "doc001" {
		t.Fatalf("Files = %#v, want all files from first changedFormData", result.Files)
	}
}

func TestClientLoadsExternalHeadersForFetchAndDownloads(t *testing.T) {
	dir := t.TempDir()
	paramsPath := filepath.Join(dir, "remote_sync_params.json")
	headersPath := filepath.Join(dir, "remote_sync_headers.json")
	if err := os.WriteFile(paramsPath, []byte(`{"params":{"condition":{"processCode":"x"}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile(params) error = %v", err)
	}
	if err := os.WriteFile(headersPath, []byte(`{"snc-token":"{token}","X-App-Code":"desktop"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(headers) error = %v", err)
	}

	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("snc-token"); got != "secret-token" {
			t.Fatalf("snc-token = %q, want secret-token", got)
		}
		if got := r.Header.Get("X-App-Code"); got != "desktop" {
			t.Fatalf("X-App-Code = %q, want desktop", got)
		}
		methods = append(methods, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/list":
			_, _ = io.WriteString(w, `{"data":{"records":[{"name":"A","id":"1","designName":"D"}]}}`)
		case "/detail/1":
			_, _ = io.WriteString(w, `{"data":[{"changedFormData":{"input":{"value":"v"},"file_upload_1":{"value":[{"id":"file-1","name":"a.txt"}]}}}]}`)
		case "/file/file-1/a.txt":
			_, _ = io.WriteString(w, "file-content")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		HTTPClient:          server.Client(),
		ParamsTemplatePath:  paramsPath,
		HeadersTemplatePath: headersPath,
	})
	result, err := client.Fetch(context.Background(), FetchRequest{
		PostURL:     server.URL + "/list",
		GetURL:      server.URL + "/detail/{id}",
		ProcessCode: "process",
		InputField:  "input",
		Token:       "secret-token",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if err := client.Download(context.Background(), DownloadRequest{
		URL:       server.URL + "/file/{file_id}/{file_name}",
		File:      result.Files[0],
		Token:     "secret-token",
		Directory: dir,
	}); err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if got, want := strings.Join(methods, ", "), "POST /list, GET /detail/1, GET /file/file-1/a.txt"; got != want {
		t.Fatalf("requests = %q, want %q", got, want)
	}
}

func TestClientRejectsInvalidExternalHeadersBeforeRequest(t *testing.T) {
	headersPath := filepath.Join(t.TempDir(), "remote_sync_headers.json")
	if err := os.WriteFile(headersPath, []byte(`{"snc-token":""}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	client := NewClient(Config{HeadersTemplatePath: headersPath})

	_, err := client.Fetch(context.Background(), FetchRequest{
		PostURL:     "https://remote.test/list",
		GetURL:      "https://remote.test/detail/{id}",
		ProcessCode: "process",
		InputField:  "input",
		Token:       "secret-token",
	})
	if err == nil || !strings.Contains(err.Error(), "header") {
		t.Fatalf("Fetch() error = %v, want invalid external header error", err)
	}
}

func TestClientFetchRejectsDetailWithoutChangeFormData(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "remote_sync_params.json")
	if err := os.WriteFile(templatePath, []byte(`{"params":{"condition":{"processCode":"x"}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{"data":{"records":[{"name":"A","id":"1","designName":"D"}]}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"other":"value"},{"still":"missing"}]}`)
	}))
	defer server.Close()

	client := NewClient(Config{HTTPClient: server.Client(), ParamsTemplatePath: templatePath})
	_, err := client.Fetch(context.Background(), FetchRequest{
		PostURL:     server.URL + "/list",
		GetURL:      server.URL + "/detail/{id}",
		ProcessCode: "process",
		InputField:  "input",
		Token:       "token",
	})
	if err == nil || !strings.Contains(err.Error(), "data does not contain changedFormData") {
		t.Fatalf("Fetch() error = %v, want missing changedFormData error", err)
	}
}

func TestClientFetchSkipsMissingInputFieldAndKeepsOtherData(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "remote_sync_params.json")
	if err := os.WriteFile(templatePath, []byte(`{"params":{"condition":{"processCode":"x"}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{"data":{"records":[{"name":"A","id":"1","designName":"D"}]}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"changedFormData":{"file_upload_1":{"value":[{"id":"file-1","name":"a.txt"}]}}}]}`)
	}))
	defer server.Close()

	client := NewClient(Config{HTTPClient: server.Client(), ParamsTemplatePath: templatePath})
	result, err := client.Fetch(context.Background(), FetchRequest{
		PostURL:     server.URL + "/list",
		GetURL:      server.URL + "/detail/{id}",
		ProcessCode: "process",
		InputField:  "missing-input",
		Token:       "token",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v, want missing input field to be skipped", err)
	}
	if len(result.InputValue) != 0 {
		t.Fatalf("InputValue = %s, want empty", result.InputValue)
	}
	if len(result.Files) != 1 || result.Files[0].Name != "a.txt" {
		t.Fatalf("Files = %#v, want one attachment", result.Files)
	}

	fields, err := BuildMappedFields(result, map[string]string{
		"name":         "名称",
		"input_value":  "输入值",
		"file_uploads": "附件",
	})
	if err != nil {
		t.Fatalf("BuildMappedFields() error = %v", err)
	}
	if _, ok := fields["输入值"]; ok {
		t.Fatalf("mapped fields = %#v, want missing input_value to be skipped", fields)
	}
	if _, ok := fields["名称"]; !ok {
		t.Fatalf("mapped fields = %#v, want name field", fields)
	}
	if _, ok := fields["附件"]; !ok {
		t.Fatalf("mapped fields = %#v, want file_uploads field", fields)
	}
}

func TestExtractFilesAcceptsStringEncodedFileObjects(t *testing.T) {
	change := map[string]json.RawMessage{
		"file_upload_1": json.RawMessage(`{
			"value": [
				"{\"id\":\"file-1\",\"name\":\"a.txt\"}"
			]
		}`),
	}

	files, err := extractFiles(change)
	if err != nil {
		t.Fatalf("extractFiles() error = %v", err)
	}
	if len(files) != 1 || files[0].ID != "file-1" || files[0].Name != "a.txt" {
		t.Fatalf("files = %#v, want one decoded file object", files)
	}
}

func TestClientFetchAcceptsNumericRecordAndFileIDs(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "remote_sync_params.json")
	if err := os.WriteFile(templatePath, []byte(`{"params":{"condition":{"processCode":"x"}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	var detailPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{"data":{"records":[{"name":"A","id":123,"designName":"D"}]}}`)
			return
		}
		detailPath = r.URL.Path
		_, _ = io.WriteString(w, `{"data":[{"changedFormData":{"input":{"value":"v"},"file_upload_1":{"value":[{"id":456,"name":"a.txt"}]}}}]}`)
	}))
	defer server.Close()

	client := NewClient(Config{HTTPClient: server.Client(), ParamsTemplatePath: templatePath})
	result, err := client.Fetch(context.Background(), FetchRequest{
		PostURL:     server.URL + "/list",
		GetURL:      server.URL + "/detail/{id}",
		ProcessCode: "process",
		InputField:  "input",
		Token:       "token",
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if result.ID != "123" || detailPath != "/detail/123" {
		t.Fatalf("identity = %q, detail path = %q, want numeric ID converted to URL text", result.ID, detailPath)
	}
	if len(result.Files) != 1 || result.Files[0].ID != "456" {
		t.Fatalf("files = %#v, want numeric file ID converted to text", result.Files)
	}
}

func TestResolveURLReplacesPathAndQueryPlaceholders(t *testing.T) {
	got, err := ResolveURL(
		"https://example.test/download/{file_id}/{file_name}?source={id}",
		map[string]string{
			"file_id":   "a/1",
			"file_name": "图片 1.png",
			"id":        "remote-1",
		},
	)
	if err != nil {
		t.Fatalf("ResolveURL() error = %v", err)
	}
	want := "https://example.test/download/a%2F1/%E5%9B%BE%E7%89%87%201.png?source=remote-1"
	if got != want {
		t.Fatalf("ResolveURL() = %q, want %q", got, want)
	}
}

func TestResolveURLRejectsNonHTTPURL(t *testing.T) {
	_, err := ResolveURL("ftp://example.test/file/{id}", map[string]string{"id": "1"})
	if err == nil || !strings.Contains(err.Error(), "http or https") {
		t.Fatalf("ResolveURL() error = %v, want scheme rejection", err)
	}
}

func TestBuildMappedFieldsEncodesFileListAsJSONArray(t *testing.T) {
	result := Result{
		Name:       "项目A",
		ID:         "remote-1",
		DesignName: "设计A",
		Creator:    "张三",
		InputValue: json.RawMessage(`{"enabled":true}`),
		Files: []File{
			{ID: "img001", Name: "图片1.png"},
			{ID: "img002", Name: "图片2.jpg"},
		},
	}

	fields, err := BuildMappedFields(result, map[string]string{
		"name":         "本地名称",
		"id":           "远程ID",
		"designName":   "设计名称",
		"creator":      "创建人",
		"input_value":  "输入值",
		"file_uploads": "附件列表",
	})
	if err != nil {
		t.Fatalf("BuildMappedFields() error = %v", err)
	}

	if string(fields["本地名称"]) != `"项目A"` || string(fields["远程ID"]) != `"remote-1"` || string(fields["设计名称"]) != `"设计A"` || string(fields["创建人"]) != `"张三"` {
		t.Fatalf("identity fields = %#v", fields)
	}
	if string(fields["输入值"]) != `{"enabled":true}` {
		t.Fatalf("input field = %s, want raw input JSON", fields["输入值"])
	}
	if !strings.Contains(string(fields["附件列表"]), `"id":"img001"`) || !strings.Contains(string(fields["附件列表"]), `"name":"图片2.jpg"`) {
		t.Fatalf("file field = %s, want JSON file list", fields["附件列表"])
	}
}

func TestClientDownloadUsesTokenAndSavesConfiguredFilename(t *testing.T) {
	var gotToken string
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("snc-token")
		gotPath = r.URL.EscapedPath()
		_, _ = io.WriteString(w, "file-content")
	}))
	defer server.Close()

	directory := t.TempDir()
	client := NewClient(Config{HTTPClient: server.Client()})
	err := client.Download(context.Background(), DownloadRequest{
		URL:       server.URL + "/download/{file_id}/{file_name}",
		File:      File{ID: "file/1", Name: "图片 1.txt"},
		Token:     "secret-token",
		Directory: directory,
	})
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if gotToken != "secret-token" {
		t.Fatalf("snc-token = %q, want secret-token", gotToken)
	}
	wantPath := "/download/file%2F1/%E5%9B%BE%E7%89%87%201.txt"
	if gotPath != wantPath {
		t.Fatalf("path = %q, want %q", gotPath, wantPath)
	}
	got, err := os.ReadFile(filepath.Join(directory, "图片 1.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "file-content" {
		t.Fatalf("downloaded content = %q, want file-content", got)
	}
}

func TestClientDownloadSupportsCommonDocumentAndArchiveExtensions(t *testing.T) {
	content := map[string]string{
		"report.docx": "word-bytes",
		"book.xlsx":   "excel-bytes",
		"note.txt":    "text-bytes",
		"archive.zip": "zip-bytes",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)
		value, ok := content[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.WriteString(w, value)
	}))
	defer server.Close()

	directory := t.TempDir()
	client := NewClient(Config{HTTPClient: server.Client()})
	for name, want := range content {
		t.Run(name, func(t *testing.T) {
			err := client.Download(context.Background(), DownloadRequest{
				URL:       server.URL + "/files/{file_name}",
				File:      File{ID: "file-" + name, Name: name},
				Token:     "secret-token",
				Directory: directory,
			})
			if err != nil {
				t.Fatalf("Download() error = %v", err)
			}
			got, err := os.ReadFile(filepath.Join(directory, name))
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			if string(got) != want {
				t.Fatalf("downloaded content = %q, want %q", got, want)
			}
		})
	}
}

func TestClientDownloadRejectsUnsafeOrExistingFilename(t *testing.T) {
	directory := t.TempDir()
	existing := filepath.Join(directory, "existing.txt")
	if err := os.WriteFile(existing, []byte("original"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	client := NewClient(Config{HTTPClient: http.DefaultClient})

	tests := []struct {
		name string
		file File
	}{
		{name: "path traversal", file: File{ID: "1", Name: `..\outside.txt`}},
		{name: "windows invalid character", file: File{ID: "1", Name: `bad?.txt`}},
		{name: "windows reserved name", file: File{ID: "1", Name: `CON.txt`}},
		{name: "windows reserved name with extra extension", file: File{ID: "1", Name: `CON.extra.txt`}},
		{name: "trailing dot", file: File{ID: "1", Name: `bad.`}},
		{name: "trailing space", file: File{ID: "1", Name: `bad.txt `}},
		{name: "existing file", file: File{ID: "1", Name: "existing.txt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.Download(context.Background(), DownloadRequest{
				URL:       "https://example.test/download/{file_id}",
				File:      tt.file,
				Token:     "secret-token",
				Directory: directory,
			})
			if err == nil {
				t.Fatal("Download() error = nil, want filename validation error")
			}
		})
	}

	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("ReadFile(existing) error = %v", err)
	}
	if string(got) != "original" {
		t.Fatalf("existing file = %q, want unchanged original", got)
	}
}

func TestClientDownloadOverwritesExistingFileWhenRequested(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "remote-content")
	}))
	defer server.Close()

	directory := t.TempDir()
	destination := filepath.Join(directory, "existing.txt")
	if err := os.WriteFile(destination, []byte("local-content"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	client := NewClient(Config{HTTPClient: server.Client()})

	err := client.Download(context.Background(), DownloadRequest{
		URL:       server.URL + "/download/{file_id}/{file_name}",
		File:      File{ID: "1", Name: "existing.txt"},
		Token:     "secret-token",
		Directory: directory,
		Overwrite: true,
	})
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "remote-content" {
		t.Fatalf("downloaded content = %q, want remote-content", got)
	}
}

func TestClientRejectsRedirectWithoutForwardingToken(t *testing.T) {
	var targetCalls atomic.Int32
	var targetToken string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls.Add(1)
		targetToken = r.Header.Get("snc-token")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer source.Close()

	client := NewClient(Config{HTTPClient: source.Client()})
	_, err := client.doJSON(context.Background(), http.MethodGet, source.URL, "secret-token", nil)
	if err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("doJSON() error = %v, want redirect rejection", err)
	}
	if targetCalls.Load() != 0 || targetToken != "" {
		t.Fatalf("target calls = %d, token = %q, want no redirected request", targetCalls.Load(), targetToken)
	}
}

func TestClientDownloadDoesNotOverwriteFileCreatedDuringRequest(t *testing.T) {
	requestStarted := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-release
		_, _ = io.WriteString(w, "remote-content")
	}))
	defer server.Close()

	directory := t.TempDir()
	client := NewClient(Config{HTTPClient: server.Client()})
	errs := make(chan error, 1)
	go func() {
		errs <- client.Download(context.Background(), DownloadRequest{
			URL:       server.URL + "/download/{file_id}/{file_name}",
			File:      File{ID: "1", Name: "race.txt"},
			Token:     "secret-token",
			Directory: directory,
		})
	}()

	<-requestStarted
	destination := filepath.Join(directory, "race.txt")
	if err := os.WriteFile(destination, []byte("local-content"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	close(release)

	err := <-errs
	if err == nil {
		t.Fatal("Download() error = nil, want no-overwrite failure")
	}
	got, readErr := os.ReadFile(destination)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if string(got) != "local-content" {
		t.Fatalf("destination = %q, want local-content preserved", got)
	}
}

func TestClientLimitsJSONAndFileResponseSizes(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, strings.Repeat("x", 9))
		}))
		defer server.Close()

		client := NewClient(Config{HTTPClient: server.Client(), MaxJSONBytes: 8})
		_, err := client.doJSON(context.Background(), http.MethodGet, server.URL, "token", nil)
		if err == nil || !strings.Contains(err.Error(), "exceeds 8 bytes") {
			t.Fatalf("doJSON() error = %v, want JSON size limit", err)
		}
	})

	t.Run("file", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, strings.Repeat("x", 9))
		}))
		defer server.Close()

		directory := t.TempDir()
		client := NewClient(Config{HTTPClient: server.Client(), MaxFileBytes: 8})
		err := client.Download(context.Background(), DownloadRequest{
			URL:       server.URL + "/download/{file_id}/{file_name}",
			File:      File{ID: "1", Name: "large.txt"},
			Token:     "token",
			Directory: directory,
		})
		if err == nil || !strings.Contains(err.Error(), "exceeds 8 bytes") {
			t.Fatalf("Download() error = %v, want file size limit", err)
		}
		if _, statErr := os.Stat(filepath.Join(directory, "large.txt")); !os.IsNotExist(statErr) {
			t.Fatalf("download destination exists after limit failure: %v", statErr)
		}
	})
}

func TestClientUsesSeparateJSONAndDownloadTimeouts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = io.WriteString(w, "content")
	}))
	defer server.Close()

	client := NewClient(Config{
		HTTPClient:      server.Client(),
		RequestTimeout:  10 * time.Millisecond,
		DownloadTimeout: 500 * time.Millisecond,
	})
	if _, err := client.doJSON(context.Background(), http.MethodGet, server.URL, "token", nil); err == nil {
		t.Fatal("doJSON() error = nil, want request timeout")
	}

	directory := t.TempDir()
	err := client.Download(context.Background(), DownloadRequest{
		URL:       server.URL + "/download/{file_id}/{file_name}",
		File:      File{ID: "1", Name: "file.txt"},
		Token:     "token",
		Directory: directory,
	})
	if err != nil {
		t.Fatalf("Download() error = %v, want independent longer download timeout", err)
	}
}

func TestClientFetchHonorsWebhookRequestTimeout(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "remote_sync_params.json")
	if err := os.WriteFile(templatePath, []byte(`{"params":{"condition":{"processCode":"x"}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(w, `{"data":{"records":[{"name":"A","id":"1","designName":"D"}]}}`)
	}))
	defer server.Close()

	client := NewClient(Config{
		HTTPClient:         server.Client(),
		ParamsTemplatePath: templatePath,
		RequestTimeout:     time.Second,
	})
	start := time.Now()
	_, err := client.Fetch(context.Background(), FetchRequest{
		PostURL:        server.URL + "/list",
		GetURL:         server.URL + "/detail/{id}",
		ProcessCode:    "process",
		InputField:     "input",
		Token:          "token",
		RequestTimeout: 20 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("Fetch() error = nil, want webhook request timeout")
	}
	if elapsed := time.Since(start); elapsed >= 90*time.Millisecond {
		t.Fatalf("Fetch() elapsed = %v, want timeout before server response", elapsed)
	}
}

func TestClientLogsRemoteRequestsWithoutToken(t *testing.T) {
	logger := &recordingLogger{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "remote failed", http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewClient(Config{
		HTTPClient: server.Client(),
		Logger:     logger,
	})
	_, _ = client.doJSON(context.Background(), http.MethodGet, server.URL+"/detail", "secret-token", nil)

	logged := logger.String()
	for _, token := range []string{
		"remote request start",
		`method=GET`,
		`status=502`,
		"remote request failed",
	} {
		if !strings.Contains(logged, token) {
			t.Fatalf("logs = %q, want containing %q", logged, token)
		}
	}
	if strings.Contains(logged, "secret-token") {
		t.Fatalf("logs contain snc-token: %q", logged)
	}
}

func TestClientDebugLoggingIncludesJSONRequestAndResponseBodies(t *testing.T) {
	logger := &recordingLogger{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"message":"debug-visible"}}`)
	}))
	defer server.Close()

	client := NewClient(Config{
		HTTPClient:        server.Client(),
		Logger:            logger,
		LogResponseBodies: true,
	})
	if _, err := client.doJSON(context.Background(), http.MethodPost, server.URL+"/detail", "secret-token", []byte(`{"processCode":"debug-visible"}`)); err != nil {
		t.Fatalf("doJSON() error = %v", err)
	}

	logged := logger.String()
	for _, want := range []string{
		"remote request headers",
		"Content-Type=application/json",
		"Content-Length=",
		"remote request body",
		`{"processCode":"debug-visible"}`,
		"remote response body",
		`"debug-visible"`,
		"POST",
	} {
		if !strings.Contains(logged, want) {
			t.Fatalf("logs = %q, want containing %q", logged, want)
		}
	}
	if strings.Contains(logged, "secret-token") {
		t.Fatalf("logs contain snc-token: %q", logged)
	}
}

type recordingLogger struct {
	lines []string
}

func (l *recordingLogger) Printf(format string, values ...any) {
	l.lines = append(l.lines, fmt.Sprintf(format, values...))
}

func (l *recordingLogger) String() string {
	return strings.Join(l.lines, "\n")
}
