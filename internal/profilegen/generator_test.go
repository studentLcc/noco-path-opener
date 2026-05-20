package profilegen

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"noco-path-opener/internal/config"
)

type fieldCall struct {
	baseID  string
	tableID string
}

type fakeFieldLister struct {
	fields []Field
	err    error
	calls  []fieldCall
}

func (f *fakeFieldLister) ListFields(ctx context.Context, baseID string, tableID string) ([]Field, error) {
	f.calls = append(f.calls, fieldCall{baseID: baseID, tableID: tableID})
	if f.err != nil {
		return nil, f.err
	}
	return append([]Field(nil), f.fields...), nil
}

func TestGenerateProfilePromptsFetchesFieldsAndBuildsProfile(t *testing.T) {
	local := &fakeFieldLister{fields: []Field{
		{ID: "c1", Title: "变更编号"},
		{ID: "c2", Title: "状态"},
		{ID: "c3", Title: "负责人"},
	}}
	remote := &fakeFieldLister{fields: []Field{
		{ID: "r1", Title: "变更编号"},
		{ID: "r2", Title: "状态"},
		{ID: "r3", Title: "负责人"},
		{ID: "r4", Title: "远端备注"},
	}}
	input := strings.NewReader(strings.Join([]string{
		"change-log-main",
		"p_local",
		"m_local",
		"p_remote",
		"m_remote",
		"1",
		"1",
		"2,3",
	}, "\n") + "\n")
	var prompts bytes.Buffer

	profile, err := Generator{
		In:           input,
		PromptOut:    &prompts,
		LocalFields:  local,
		RemoteFields: remote,
	}.Generate(context.Background())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if profile.Name != "change-log-main" ||
		profile.LocalBaseID != "p_local" ||
		profile.LocalTableID != "m_local" ||
		profile.LocalLookupField != "变更编号" ||
		profile.RemoteBaseID != "p_remote" ||
		profile.RemoteTableID != "m_remote" ||
		profile.RemoteLookupField != "变更编号" {
		t.Fatalf("Generate() profile = %+v, want selected values", profile)
	}
	if !reflect.DeepEqual(profile.SyncFields, []string{"状态", "负责人"}) {
		t.Fatalf("SyncFields = %#v, want 状态 and 负责人", profile.SyncFields)
	}
	if !reflect.DeepEqual(local.calls, []fieldCall{{baseID: "p_local", tableID: "m_local"}}) {
		t.Fatalf("local calls = %#v, want local base/table call", local.calls)
	}
	if !reflect.DeepEqual(remote.calls, []fieldCall{{baseID: "p_remote", tableID: "m_remote"}}) {
		t.Fatalf("remote calls = %#v, want remote base/table call", remote.calls)
	}
	for _, want := range []string{"Profile name:", "Local fields:", "1. 变更编号", "Remote fields:", "Sync field numbers"} {
		if !strings.Contains(prompts.String(), want) {
			t.Fatalf("prompts = %q, want containing %q", prompts.String(), want)
		}
	}
}

func TestGenerateProfileUsesAllRemoteFieldsWhenSyncSelectionIsBlank(t *testing.T) {
	local := &fakeFieldLister{fields: []Field{
		{ID: "c1", Title: "变更编号"},
		{ID: "c2", Title: "状态"},
		{ID: "c3", Title: "负责人"},
	}}
	remote := &fakeFieldLister{fields: []Field{
		{ID: "r1", Title: "变更编号"},
		{ID: "r2", Title: "状态"},
		{ID: "r3", Title: "负责人"},
	}}
	input := strings.NewReader(strings.Join([]string{
		"change-log-main",
		"p_local",
		"m_local",
		"p_remote",
		"m_remote",
		"1",
		"1",
		"",
	}, "\n") + "\n")

	profile, err := Generator{
		In:           input,
		PromptOut:    ioDiscard{},
		LocalFields:  local,
		RemoteFields: remote,
	}.Generate(context.Background())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !reflect.DeepEqual(profile.SyncFields, []string{"变更编号", "状态", "负责人"}) {
		t.Fatalf("SyncFields = %#v, want all remote fields", profile.SyncFields)
	}
}

func TestGenerateProfileRejectsRemoteSyncFieldsMissingLocally(t *testing.T) {
	local := &fakeFieldLister{fields: []Field{
		{ID: "c1", Title: "变更编号"},
	}}
	remote := &fakeFieldLister{fields: []Field{
		{ID: "r1", Title: "变更编号"},
		{ID: "r2", Title: "状态"},
	}}
	input := strings.NewReader(strings.Join([]string{
		"change-log-main",
		"p_local",
		"m_local",
		"p_remote",
		"m_remote",
		"1",
		"1",
		"2",
	}, "\n") + "\n")

	_, err := Generator{
		In:           input,
		PromptOut:    ioDiscard{},
		LocalFields:  local,
		RemoteFields: remote,
	}.Generate(context.Background())
	if err == nil {
		t.Fatal("Generate() error = nil, want missing local sync field error")
	}
	if !strings.Contains(err.Error(), "selected sync fields do not exist in local table: 状态") {
		t.Fatalf("Generate() error = %q, want missing local field", err.Error())
	}
}

func TestGenerateProfileRejectsEmptyRequiredPrompt(t *testing.T) {
	_, err := Generator{
		In:           strings.NewReader("\n"),
		PromptOut:    ioDiscard{},
		LocalFields:  &fakeFieldLister{},
		RemoteFields: &fakeFieldLister{},
	}.Generate(context.Background())
	if err == nil {
		t.Fatal("Generate() error = nil, want required prompt error")
	}
	if !strings.Contains(err.Error(), "Profile name is required") {
		t.Fatalf("Generate() error = %q, want profile name required", err.Error())
	}
}

func TestGenerateProfileWrapsMetadataErrors(t *testing.T) {
	local := &fakeFieldLister{err: errors.New("local unavailable")}
	input := strings.NewReader(strings.Join([]string{
		"change-log-main",
		"p_local",
		"m_local",
		"p_remote",
		"m_remote",
	}, "\n") + "\n")

	_, err := Generator{
		In:           input,
		PromptOut:    ioDiscard{},
		LocalFields:  local,
		RemoteFields: &fakeFieldLister{},
	}.Generate(context.Background())
	if err == nil {
		t.Fatal("Generate() error = nil, want metadata error")
	}
	if !strings.Contains(err.Error(), "fetch local fields: local unavailable") {
		t.Fatalf("Generate() error = %q, want wrapped metadata error", err.Error())
	}
}

func TestGenerateProfileReturnsFieldListWriterError(t *testing.T) {
	wantErr := errors.New("prompt writer unavailable")
	local := &fakeFieldLister{fields: []Field{
		{ID: "c1", Title: "变更编号"},
	}}
	remote := &fakeFieldLister{fields: []Field{
		{ID: "r1", Title: "变更编号"},
	}}
	input := strings.NewReader(strings.Join([]string{
		"change-log-main",
		"p_local",
		"m_local",
		"p_remote",
		"m_remote",
	}, "\n") + "\n")

	profile, err := Generator{
		In:           input,
		PromptOut:    &failAfterWriter{allowedWrites: 5, err: wantErr},
		LocalFields:  local,
		RemoteFields: remote,
	}.Generate(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Generate() error = %v, want writer error %v", err, wantErr)
	}
	if !reflect.DeepEqual(profile, config.SyncProfile{}) {
		t.Fatalf("Generate() profile = %+v, want zero value after writer error", profile)
	}
	if len(local.calls) != 1 || len(remote.calls) != 1 {
		t.Fatalf("metadata calls = local %#v remote %#v, want both fetched before field-list print", local.calls, remote.calls)
	}
}

func TestGenerateProfileWrapsInvalidSyncSelectionWithPromptLabel(t *testing.T) {
	local := &fakeFieldLister{fields: []Field{
		{ID: "c1", Title: "变更编号"},
		{ID: "c2", Title: "状态"},
	}}
	remote := &fakeFieldLister{fields: []Field{
		{ID: "r1", Title: "变更编号"},
		{ID: "r2", Title: "状态"},
	}}
	input := strings.NewReader(strings.Join([]string{
		"change-log-main",
		"p_local",
		"m_local",
		"p_remote",
		"m_remote",
		"1",
		"1",
		"3",
	}, "\n") + "\n")

	_, err := Generator{
		In:           input,
		PromptOut:    ioDiscard{},
		LocalFields:  local,
		RemoteFields: remote,
	}.Generate(context.Background())
	if err == nil {
		t.Fatal("Generate() error = nil, want invalid sync selection error")
	}
	for _, want := range []string{
		"Sync field numbers from remote table",
		"selection 3 is out of range 1-2",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Generate() error = %q, want containing %q", err.Error(), want)
		}
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}

type failAfterWriter struct {
	allowedWrites int
	err           error
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.allowedWrites <= 0 {
		return 0, w.err
	}
	w.allowedWrites--
	return len(p), nil
}
