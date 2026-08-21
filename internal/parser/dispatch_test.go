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

func TestUnknownDirectiveDoesNotEndWorkflow(t *testing.T) {
	doc := Parse("ignored.http", []byte(`# @workflow deploy
# @step Create using=Create
# @nmae Typo
# @step Verify using=Verify
`))
	if len(doc.Errors) != 0 {
		t.Fatalf("errors = %v, want none", doc.Errors)
	}
	if len(doc.Warnings) != 1 {
		t.Fatalf("warnings = %v, want one", doc.Warnings)
	}
	if got, want := doc.Warnings[0].Message, "@nmae is not a known Resterm directive and was ignored"; got != want {
		t.Fatalf("warning = %q, want %q", got, want)
	}
	if len(doc.Workflows) != 1 {
		t.Fatalf("workflows = %d, want one", len(doc.Workflows))
	}
	steps := doc.Workflows[0].Steps
	if len(steps) != 2 {
		t.Fatalf("steps = %+v, want Create and Verify", steps)
	}
	if steps[0].Name != "Create" || steps[1].Name != "Verify" {
		t.Fatalf("step names = %q, %q, want Create, Verify", steps[0].Name, steps[1].Name)
	}
}

func TestUnknownDirectiveDoesNotCloseWorkflowBranch(t *testing.T) {
	doc := Parse("ignored.http", []byte(`# @workflow deploy
# @if true run=Create
# @nmae Typo
# @else run=Verify
`))
	if len(doc.Errors) != 0 {
		t.Fatalf("errors = %v, want none", doc.Errors)
	}
	if len(doc.Warnings) != 1 {
		t.Fatalf("warnings = %v, want one", doc.Warnings)
	}
	if len(doc.Workflows) != 1 {
		t.Fatalf("workflows = %d, want one", len(doc.Workflows))
	}
	steps := doc.Workflows[0].Steps
	if len(steps) != 1 || steps[0].If == nil {
		t.Fatalf("steps = %+v, want one if branch", steps)
	}
	if steps[0].If.Else == nil || steps[0].If.Else.Run != "Verify" {
		t.Fatalf("else = %+v, want run=Verify", steps[0].If.Else)
	}
}

func TestKnownIgnoredDirectiveDoesNotEndWorkflow(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		warning string
	}{
		{
			name:    "inactive GraphQL operation",
			line:    "@operation Ignored",
			warning: "@operation is not valid in the current context and was ignored",
		},
		{
			name:    "invalid body option",
			line:    "@body inline maybe",
			warning: "@body is not valid in the current context and was ignored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "# @workflow deploy\n" +
				"# @step Create using=Create\n" +
				"# " + tt.line + "\n" +
				"# @step Verify using=Verify\n"
			doc := Parse("ignored.http", []byte(src))
			if len(doc.Errors) != 0 {
				t.Fatalf("errors = %v, want none", doc.Errors)
			}
			if len(doc.Warnings) != 1 || doc.Warnings[0].Message != tt.warning {
				t.Fatalf("warnings = %v, want %q", doc.Warnings, tt.warning)
			}
			if len(doc.Workflows) != 1 {
				t.Fatalf("workflows = %d, want one", len(doc.Workflows))
			}
			steps := doc.Workflows[0].Steps
			if len(steps) != 2 {
				t.Fatalf("steps = %+v, want Create and Verify", steps)
			}
			if steps[0].Name != "Create" || steps[1].Name != "Verify" {
				t.Fatalf("step names = %q, %q, want Create, Verify", steps[0].Name, steps[1].Name)
			}
		})
	}
}

func TestKnownIgnoredDirectiveDoesNotCloseWorkflowBranch(t *testing.T) {
	doc := Parse("ignored.http", []byte(`# @workflow deploy
# @if true run=Create
# @operation Ignored
# @else run=Verify
`))
	if len(doc.Errors) != 0 {
		t.Fatalf("errors = %v, want none", doc.Errors)
	}
	wantWarning := "@operation is not valid in the current context and was ignored"
	if len(doc.Warnings) != 1 || doc.Warnings[0].Message != wantWarning {
		t.Fatalf("warnings = %v, want %q", doc.Warnings, wantWarning)
	}
	if len(doc.Workflows) != 1 {
		t.Fatalf("workflows = %d, want one", len(doc.Workflows))
	}
	steps := doc.Workflows[0].Steps
	if len(steps) != 1 || steps[0].If == nil {
		t.Fatalf("steps = %+v, want one if branch", steps)
	}
	if steps[0].If.Else == nil || steps[0].If.Else.Run != "Verify" {
		t.Fatalf("else = %+v, want run=Verify", steps[0].If.Else)
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
