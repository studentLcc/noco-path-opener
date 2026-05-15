package pathauth

import "testing"

func TestAllowedWithEmptyRootsAllowsAnyPath(t *testing.T) {
	allowed, err := IsAllowed(`D:\docs\a.docx`, nil)
	if err != nil {
		t.Fatalf("IsAllowed() error = %v", err)
	}
	if !allowed {
		t.Fatal("IsAllowed() = false, want true")
	}

	allowed, err = IsAllowed(`D:\docs\a.docx`, []string{})
	if err != nil {
		t.Fatalf("IsAllowed() error = %v", err)
	}
	if !allowed {
		t.Fatal("IsAllowed() = false, want true")
	}
}

func TestAllowedRootsMatchExactRootOrChild(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		roots []string
		want  bool
	}{
		{name: "exact", path: `D:\docs`, roots: []string{`D:\docs`}, want: true},
		{name: "child", path: `D:\docs\a.docx`, roots: []string{`D:\docs`}, want: true},
		{name: "sibling prefix", path: `D:\docs-old\a.docx`, roots: []string{`D:\docs`}, want: false},
		{name: "different root", path: `E:\docs\a.docx`, roots: []string{`D:\docs`}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsAllowed(tt.path, tt.roots)
			if err != nil {
				t.Fatalf("IsAllowed() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("IsAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}
