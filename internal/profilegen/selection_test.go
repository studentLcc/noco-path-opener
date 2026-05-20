package profilegen

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseSingleSelection(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		count     int
		wantIndex int
		wantErr   string
	}{
		{name: "valid", input: "2", count: 3, wantIndex: 1},
		{name: "valid with spaces", input: " 3 ", count: 3, wantIndex: 2},
		{name: "empty", input: " ", count: 3, wantErr: "selection is required"},
		{name: "not a number", input: "abc", count: 3, wantErr: "not a number"},
		{name: "zero", input: "0", count: 3, wantErr: "out of range 1-3"},
		{name: "too high", input: "4", count: 3, wantErr: "out of range 1-3"},
		{name: "multiple", input: "1,2", count: 3, wantErr: "select exactly one number"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSingleSelection(tt.input, tt.count)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("parseSingleSelection() error = nil, want error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseSingleSelection() error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSingleSelection() error = %v", err)
			}
			if got != tt.wantIndex {
				t.Fatalf("parseSingleSelection() = %d, want %d", got, tt.wantIndex)
			}
		})
	}
}

func TestParseMultiSelection(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		count   int
		want    []int
		wantErr string
	}{
		{name: "valid", input: "1,3,5", count: 5, want: []int{0, 2, 4}},
		{name: "valid with spaces", input: " 2, 4 ", count: 5, want: []int{1, 3}},
		{name: "single value", input: "2", count: 5, want: []int{1}},
		{name: "empty", input: "", count: 5, wantErr: "selection is required"},
		{name: "empty item", input: "1,,2", count: 5, wantErr: "empty item"},
		{name: "not a number", input: "1,nope", count: 5, wantErr: "not a number"},
		{name: "out of range", input: "1,6", count: 5, wantErr: "out of range 1-5"},
		{name: "duplicate", input: "1,3,1", count: 5, wantErr: "selection 1 is duplicated"},
		{name: "no fields", input: "1", count: 0, wantErr: "no fields available"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMultiSelection(tt.input, tt.count)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("parseMultiSelection() error = nil, want error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseMultiSelection() error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMultiSelection() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseMultiSelection() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
