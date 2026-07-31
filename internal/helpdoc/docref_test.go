package helpdoc

import "testing"

func TestDocsRef(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected string
	}{
		{name: "release", version: "v1.2.3", expected: "v1.2.3"},
		{name: "prerelease", version: "v1.2.3-rc.1", expected: "v1.2.3-rc.1"},
		{name: "development", version: "dev", expected: "main"},
		{name: "empty", version: "", expected: "main"},
		{name: "git describe", version: "v1.2.3-4-gabc1234", expected: "main"},
		{name: "dirty tag", version: "v1.2.3-dirty", expected: "main"},
		{name: "commit", version: "abc1234", expected: "main"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DocsRef(tt.version); got != tt.expected {
				t.Fatalf("DocsRef(%q) = %q, want %q", tt.version, got, tt.expected)
			}
		})
	}
}

func TestDocRefURL(t *testing.T) {
	ref := DocRef{Path: "docs/resterm.md", Heading: "Timeline & tracing"}
	want := "https://github.com/unkn0wn-root/resterm/blob/v1.2.3/docs/resterm.md#timeline--tracing"
	if got := ref.URL("v1.2.3"); got != want {
		t.Fatalf("URL() = %q, want %q", got, want)
	}

	bare := DocRef{Path: "docs/cli.md"}
	want = "https://github.com/unkn0wn-root/resterm/blob/main/docs/cli.md"
	if got := bare.URL("main"); got != want {
		t.Fatalf("URL() = %q, want %q", got, want)
	}
}
