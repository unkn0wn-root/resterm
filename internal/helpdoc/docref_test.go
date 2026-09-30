package helpdoc

import "testing"

func TestDocRefURL(t *testing.T) {
	tests := []struct {
		name     string
		ref      DocRef
		expected string
	}{
		{name: "index", ref: Manual(), expected: "https://resterm.app/docs/"},
		{name: "page", ref: DocRef{Page: "grpc"}, expected: "https://resterm.app/docs/grpc/"},
		{name: "nested page", ref: DocRef{Page: "cli/run"}, expected: "https://resterm.app/docs/cli/run/"},
		{
			name:     "heading",
			ref:      DocRef{Page: "ui-tour", Heading: "Timeline & tracing"},
			expected: "https://resterm.app/docs/ui-tour/#timeline--tracing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ref.URL(); got != tt.expected {
				t.Fatalf("URL() = %q, want %q", got, tt.expected)
			}
		})
	}
}
