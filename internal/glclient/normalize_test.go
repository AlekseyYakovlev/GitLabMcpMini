package glclient

import (
	"errors"
	"testing"
)

func TestNormalizeProject(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{"trim spaces and slashes", " group/sub/proj/ ", "group/sub/proj", nil},
		{"numeric id", "123", "123", nil},
		{"pre-encoded path", "group%2Fproj", "group/proj", nil},
		{"https url rejected", "https://gitlab.com/g/p", "", ErrProjectURL},
		{"http url rejected", "http://gitlab.com/g/p", "", ErrProjectURL},
		{"empty", "", "", ErrEmptyProject},
		{"only slash", "/", "", ErrEmptyProject},
		{"only spaces", "   ", "", ErrEmptyProject},
		{"broken escape kept", "100%zz", "100%zz", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeProject(tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeRepoPath(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{"backslashes", `src\main.go`, "src/main.go", nil},
		{"leading dot slash", "./a/b", "a/b", nil},
		{"leading slash", "/a", "a", nil},
		{"dot dot rejected", "a/../b", "", ErrDotDot},
		{"leading dot dot rejected", "../etc", "", ErrDotDot},
		{"backslash dot dot rejected", `a\..\b`, "", ErrDotDot},
		{"root", "", "", nil},
		{"dotfile kept", ".gitignore", ".gitignore", nil},
		{"dot in name kept", "a..b/c", "a..b/c", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeRepoPath(tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
