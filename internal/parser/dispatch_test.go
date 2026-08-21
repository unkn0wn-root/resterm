package parser

import "testing"

func TestIgnoredWorkflowDirectivesWarnInRequest(t *testing.T) {
	tests := []struct {
		name string
		line string
		tag  string
	}{
		{name: "step", line: "@step Next using=Request", tag: "@step"},
		{name: "if", line: "@if true run=Next", tag: "@if"},
		{name: "elif", line: "@elif true run=Next", tag: "@elif"},
		{name: "else", line: "@else run=Next", tag: "@else"},
		{name: "switch", line: "@switch 1", tag: "@switch"},
		{name: "case", line: "@case 1 run=Next", tag: "@case"},
		{name: "default", line: "@default run=Next", tag: "@default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Parse("ignored.http", []byte("GET https://example.com\n# "+tt.line+"\n"))
			if len(doc.Errors) != 0 {
				t.Fatalf("errors = %v, want none", doc.Errors)
			}
			if len(doc.Warnings) != 1 {
				t.Fatalf("warnings = %v, want one", doc.Warnings)
			}
			want := tt.tag + " is not valid in the current context and was ignored"
			if doc.Warnings[0].Message != want {
				t.Fatalf("warning = %q, want %q", doc.Warnings[0].Message, want)
			}
			if len(doc.Requests) != 1 {
				t.Fatalf("requests = %d, want one", len(doc.Requests))
			}
		})
	}
}

func TestIgnoredDirectivesWarnInSourceOrder(t *testing.T) {
	doc := Parse("ignored.http", []byte(`# @if true run=Next
# @nmae Typo
GET https://example.com
`))
	if len(doc.Errors) != 0 {
		t.Fatalf("errors = %v, want none", doc.Errors)
	}
	want := []struct {
		line    int
		message string
	}{
		{line: 1, message: "@if is not valid in the current context and was ignored"},
		{line: 2, message: "@nmae is not a known Resterm directive and was ignored"},
	}
	if len(doc.Warnings) != len(want) {
		t.Fatalf("warnings = %v, want %d", doc.Warnings, len(want))
	}
	for i, expected := range want {
		if got := doc.Warnings[i]; got.Line != expected.line || got.Message != expected.message {
			t.Errorf("warning %d = %+v, want line %d message %q", i, got, expected.line, expected.message)
		}
	}
	if len(doc.Requests) != 1 {
		t.Fatalf("requests = %d, want one", len(doc.Requests))
	}
}

func TestExtraCommentMarkerKeepsDirectiveTextInactive(t *testing.T) {
	doc := Parse("comment.http", []byte(`## @if true run=Next
# ordinary mention of @nmae
GET https://example.com
`))
	if len(doc.Errors) != 0 || len(doc.Warnings) != 0 {
		t.Fatalf("errors = %v warnings = %v, want none", doc.Errors, doc.Warnings)
	}
	if len(doc.Requests) != 1 {
		t.Fatalf("requests = %d, want one", len(doc.Requests))
	}
}

func TestExplicitDirectiveErrorsDoNotGainWarnings(t *testing.T) {
	tests := map[string]string{
		"match outside mock": `# @match query={"mode":"live"}
GET https://example.com
`,
		"unknown mock preamble directive": `# @mock method=GET path=/x
# @nmae Typo
HTTP/1.1 200 OK
`,
	}

	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			doc := Parse("invalid.http", []byte(src))
			if len(doc.Errors) == 0 {
				t.Fatal("expected a parse error")
			}
			if len(doc.Warnings) != 0 {
				t.Fatalf("warnings = %v, want none beside the existing error", doc.Warnings)
			}
		})
	}
}
