package launch

import (
	"errors"
	"reflect"
	"testing"
)

func TestLauncherOpen(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		target   string
		expected []string
	}{
		{
			name: "macos url", goos: "darwin", target: "https://example.com/docs",
			expected: []string{"open", "https://example.com/docs"},
		},
		{
			name: "windows url", goos: "windows", target: "https://example.com/docs",
			expected: []string{"rundll32", "url.dll,FileProtocolHandler", "https://example.com/docs"},
		},
		{
			name: "linux url", goos: "linux", target: "https://example.com/docs",
			expected: []string{"xdg-open", "https://example.com/docs"},
		},
		{
			name: "macos file", goos: "darwin", target: "/tmp/response.json",
			expected: []string{"open", "/tmp/response.json"},
		},
		{
			name: "windows file", goos: "windows", target: "/tmp/response.json",
			expected: []string{"rundll32", "url.dll,FileProtocolHandler", "/tmp/response.json"},
		},
		{
			name: "linux file", goos: "linux", target: "/tmp/response.json",
			expected: []string{"xdg-open", "/tmp/response.json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			l := Launcher{
				goos: tt.goos,
				start: func(name string, args ...string) error {
					got = append([]string{name}, args...)
					return nil
				},
			}
			if err := l.Open(tt.target); err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("Open() command = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestLauncherReturnsStartError(t *testing.T) {
	want := errors.New("start failed")
	l := Launcher{goos: "linux", start: func(string, ...string) error { return want }}
	if got := l.Open("https://example.com"); !errors.Is(got, want) {
		t.Fatalf("Open() error = %v, want %v", got, want)
	}
}
