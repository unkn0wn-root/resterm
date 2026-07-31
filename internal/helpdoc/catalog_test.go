package helpdoc

import (
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/directive"
)

func TestTopics(t *testing.T) {
	topics := Topics()
	if len(topics) == 0 {
		t.Fatal("Topics() returned no topics")
	}
	for _, topic := range topics {
		if strings.TrimSpace(topic.Body) == "" {
			t.Errorf("topic %q has an empty body", topic.ID)
		}
		if strings.HasPrefix(topic.Body, "# "+topic.Title) {
			t.Errorf("topic %q repeats its catalog title in Body", topic.ID)
		}
	}
}

func TestLookup(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected string
	}{
		{name: "id", query: "quick-start", expected: "quick-start"},
		{name: "normalized title", query: "Quick Start", expected: "quick-start"},
		{name: "underscore", query: "quick_start", expected: "quick-start"},
		{name: "alias", query: "oauth", expected: "authentication"},
		{name: "keyword", query: "GET", expected: "requests"},
		{name: "keyword template", query: "template", expected: "variables"},
		{name: "plain variables", query: "variables", expected: "variables"},
		{name: "directive variables", query: "@variables", expected: "graphql"},
		{name: "directive alias", query: "@skip-if", expected: "rts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topic, ok := Lookup(tt.query)
			if !ok {
				t.Fatalf("Lookup(%q) did not find a topic", tt.query)
			}
			if topic.ID != tt.expected {
				t.Fatalf("Lookup(%q).ID = %q, want %q", tt.query, topic.ID, tt.expected)
			}
		})
	}

	if _, ok := Lookup("no-such-topic"); ok {
		t.Fatal("Lookup() found an unknown topic")
	}
	if _, ok := Lookup("@no-such-directive"); ok {
		t.Fatal("Lookup() found an unknown directive")
	}
}

func TestSearch(t *testing.T) {
	got := Search("oauth credential")
	if len(got) != 1 || got[0].ID != "authentication" {
		t.Fatalf("Search() = %+v, want authentication", got)
	}
	if got := Search("words-that-do-not-exist"); len(got) != 0 {
		t.Fatalf("Search() = %+v, want no results", got)
	}
}

func TestSuggestExcludesIncidentalBodyMatches(t *testing.T) {
	tests := []struct {
		query    string
		expected string
	}{
		{query: "web", expected: "streaming"},
		{query: "grpc", expected: "grpc"},
		{query: "auth", expected: "authentication"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := Suggest(tt.query)
			if len(got) != 1 || got[0].ID != tt.expected {
				t.Fatalf("Suggest(%q) = %+v, want %q", tt.query, got, tt.expected)
			}
		})
	}
}

func TestDirectiveCoverage(t *testing.T) {
	for _, spec := range directive.Specs() {
		if _, ok := Directive(spec.Name); !ok {
			t.Errorf("Directive(%q) did not find a topic", spec.Name)
		}
		for _, alias := range spec.Aliases {
			if _, ok := Lookup("@" + alias.String()); !ok {
				t.Errorf("Lookup(@%s) did not find a topic", alias)
			}
		}
	}
}

func TestDocumentHeadingsExist(t *testing.T) {
	root := os.DirFS("../..")
	for _, topic := range Topics() {
		if topic.Doc.Heading == "" {
			continue
		}
		data, err := fs.ReadFile(root, topic.Doc.Path)
		if err != nil {
			t.Errorf("read %s for %s: %v", topic.Doc.Path, topic.ID, err)
			continue
		}
		if !hasHeading(string(data), topic.Doc.Heading) {
			t.Errorf("%s does not contain heading %q for %s", topic.Doc.Path, topic.Doc.Heading, topic.ID)
		}
	}
}

func hasHeading(doc, expected string) bool {
	for line := range strings.SplitSeq(doc, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			continue
		}
		if strings.TrimSpace(strings.TrimLeft(line, "#")) == expected {
			return true
		}
	}
	return false
}
