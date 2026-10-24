package parser

import (
	"fmt"
	"slices"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

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
		if got := doc.Warnings[i]; got.Span.Start.Line != expected.line || got.Message != expected.message {
			t.Errorf("warning %d = %+v, want line %d message %q", i, got, expected.line, expected.message)
		}
	}
	if len(doc.Requests) != 1 {
		t.Fatalf("requests = %d, want one", len(doc.Requests))
	}
}

func TestRequestEndFindingsReportInLineOrder(t *testing.T) {
	doc := Parse("order.http", []byte(`GRPC localhost:9000
# @grpc a.B/C
# @grpc-import-path vendor
# @nmae Typo
# @retry-when true
# @name a
# @name b
`))
	wantWarnings := []string{
		"3:3 @grpc-import-path without @grpc-proto was ignored",
		"4:3 @nmae is not a known Resterm directive and was ignored",
	}
	if got := findings(doc.Warnings); !slices.Equal(got, wantWarnings) {
		t.Errorf("warnings = %q, want %q", got, wantWarnings)
	}
	wantErrors := []string{
		"5:0 @retry-when requires @retry",
		"7:3 @name directive already defined for this request",
	}
	if got := findings(doc.Errors); !slices.Equal(got, wantErrors) {
		t.Errorf("errors = %q, want %q", got, wantErrors)
	}
}

func TestFindingsOnOneLineKeepTheirOrder(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "options and a placeholder",
			src:  "GET https://example.com\n# @sse max-event=1 {{x\n",
			want: []string{
				`2:8 unknown @sse option "max-event"`,
				`2:20 unknown @sse option "{{x"`,
				"2:20 placeholder {{x is not closed with }}",
			},
		},
		{
			name: "a request end finding after a placeholder",
			src:  "GRPC localhost:9000\n# @grpc-import-path vendor {{x\n",
			want: []string{
				"2:28 placeholder {{x is not closed with }}",
				"2:3 @grpc-import-path without @grpc-proto was ignored",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Parse("order.http", []byte(tt.src))
			if got := findings(doc.Warnings); !slices.Equal(got, tt.want) {
				t.Fatalf("warnings = %q, want %q", got, tt.want)
			}
		})
	}
}

func findings(list []restfile.ParseDiagnostic) []string {
	out := make([]string, 0, len(list))
	for _, d := range list {
		out = append(out, fmt.Sprintf("%d:%d %s", d.Span.Start.Line, d.Span.Start.Col, d.Message))
	}
	return out
}

func TestUnknownDirectiveRejectsWithoutEndingWorkflow(t *testing.T) {
	doc := Parse("ignored.http", []byte(`# @workflow deploy
# @step Create using=Create
# @stpe Omitted using=Omitted
# @step Verify using=Verify
`))
	if len(doc.Errors) != 1 {
		t.Fatalf("errors = %v, want one", doc.Errors)
	}
	if got, want := doc.Errors[0].Span.Start.Line, 3; got != want {
		t.Fatalf("error line = %d, want %d", got, want)
	}
	if got, want := doc.Errors[0].Message, "@stpe is not a known Resterm directive in a workflow"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
	if len(doc.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none beside the error", doc.Warnings)
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

func TestUnknownDirectiveRejectsWithoutClosingWorkflowBranch(t *testing.T) {
	doc := Parse("ignored.http", []byte(`# @workflow deploy
# @if true run=Create
# @nmae Typo
# @else run=Verify
`))
	if len(doc.Errors) != 1 {
		t.Fatalf("errors = %v, want one", doc.Errors)
	}
	if got, want := doc.Errors[0].Message, "@nmae is not a known Resterm directive in a workflow"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
	if len(doc.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none beside the error", doc.Warnings)
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

func TestUnknownDirectiveAfterWorkflowBoundaryStillWarns(t *testing.T) {
	tests := map[string]string{
		"separator": `# @workflow deploy
# @step Create using=Create
### Create
# @nmae Typo
GET https://example.com
`,
		"request directive": `# @workflow deploy
# @step Create using=Create
# @name Create
# @nmae Typo
GET https://example.com
`,
	}

	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			doc := Parse("ignored.http", []byte(src))
			if len(doc.Errors) != 0 {
				t.Fatalf("errors = %v, want none", doc.Errors)
			}
			want := "@nmae is not a known Resterm directive and was ignored"
			if len(doc.Warnings) != 1 || doc.Warnings[0].Message != want {
				t.Fatalf("warnings = %v, want %q", doc.Warnings, want)
			}
			if len(doc.Workflows) != 1 || len(doc.Workflows[0].Steps) != 1 {
				t.Fatalf("workflows = %+v, want one workflow with one step", doc.Workflows)
			}
			if len(doc.Requests) != 1 {
				t.Fatalf("requests = %d, want one", len(doc.Requests))
			}
		})
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
	tests := []struct {
		name         string
		src          string
		wantRequests int
		wantWorkflow int
	}{
		{
			name: "request",
			src: `## @if true run=Next
# ordinary mention of @nmae
GET https://example.com
`,
			wantRequests: 1,
		},
		{
			name: "workflow",
			src: `# @workflow deploy
## @stpe is ordinary prose
# @step Create using=Create
`,
			wantWorkflow: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Parse("comment.http", []byte(tt.src))
			if len(doc.Errors) != 0 || len(doc.Warnings) != 0 {
				t.Fatalf("errors = %v warnings = %v, want none", doc.Errors, doc.Warnings)
			}
			if len(doc.Requests) != tt.wantRequests || len(doc.Workflows) != tt.wantWorkflow {
				t.Fatalf(
					"requests = %d workflows = %d, want %d/%d",
					len(doc.Requests),
					len(doc.Workflows),
					tt.wantRequests,
					tt.wantWorkflow,
				)
			}
		})
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
