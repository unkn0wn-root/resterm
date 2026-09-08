package parser

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestDiagnosticSourceSpans(t *testing.T) {
	for _, tt := range []struct {
		name, source, text string
		line               int
		severity           diag.Severity
	}{
		{"unknown", "# @nmae example\nGET http://x", "@nmae", 1, diag.SeverityWarning},
		{"unicode indent", "\u3000// @nmae example\r\nGET http://x", "@nmae", 1, diag.SeverityWarning},
		{"unicode name", "# @Kelvin value\nGET http://x", "@Kelvin", 1, diag.SeverityWarning},
		{"dash", "\t-- @sse max-event=5\nGET http://x", "max-event", 1, diag.SeverityWarning},
		{"inline block", "  /* @nmae example */\nGET http://x", "@nmae", 1, diag.SeverityWarning},
		{"block", "/*\n * @sse max-event=5\n */\nGET http://x", "max-event", 2, diag.SeverityWarning},
		{"continued option", "# @mock method=GET path=/x\n# @match query={\n# \"hø\":\"st\"} typo=bob\nHTTP/1.1 200 OK", "typo", 3, diag.SeverityError},
		{"continued block", "# @workflow w\n/* @if sum(\n * 1) run=A using=B */", "run using", 3, diag.SeverityError},
		{"unknown workflow", "# @workflow w\n# @nmae typo", "@nmae", 2, diag.SeverityError},
		{"missing argument", "# @use\nGET http://x", "@use", 1, diag.SeverityError},
		{"unclosed", "# @assert sum(\nGET http://x", "@assert", 1, diag.SeverityError},
		{"mock", "# @mock method=GET path=/x typo=true\nHTTP/1.1 200 OK\n", "typo", 1, diag.SeverityError},
		{"unknown options", "# @ssh host=h userr=a otherr=b\nGET http://x", "userr otherr", 1, diag.SeverityWarning},
		{"alias conflict", "# @sse idle=1s idle-timeout=2s\nGET http://x", "idle idle-timeout", 1, diag.SeverityError},
		{"repeated option", "# @sse idle=1s idle=2s\nGET http://x", "idle idle", 1, diag.SeverityError},
		{"workflow aliases", "# @workflow w\n# @step s run=A using=B", "run using", 2, diag.SeverityError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			doc := Parse("test.http", []byte(tt.source))
			rep := Diagnostics(doc)
			if len(rep.Items) != 1 {
				t.Fatalf("findings = %+v", rep.Items)
			}
			item := rep.Items[0]
			if item.Severity != tt.severity || item.Span.Start.Line != tt.line {
				t.Fatalf("finding = %+v", item)
			}
			spans := []diag.Span{item.Span}
			for _, label := range item.Labels {
				spans = append(spans, label.Span)
			}
			var marked []string
			for _, span := range spans {
				marked = append(marked, sourceSpanText(t, tt.source, span))
				if span.Start.Path != "test.http" || span.End.Path != "test.http" {
					t.Fatalf("missing paths: %+v", span)
				}
			}
			if got := strings.Join(marked, " "); got != tt.text {
				t.Fatalf("marked %q, want %q", got, tt.text)
			}
		})
	}
}

func TestUnfinishedWorkflowDiagnosticLocations(t *testing.T) {
	for _, directive := range []struct {
		name, source, message string
	}{
		{"when", "# @when true", "@when must be followed by @step"},
		{"for-each", "# @for-each [1] as item", "@for-each must be followed by @step"},
		{"switch", "# @switch true", "@switch requires at least one @case or @default"},
		{"continued when", "# @when (\n# true\n# )", "@when must be followed by @step"},
	} {
		for _, ending := range []struct {
			name, source string
		}{
			{"EOF", ""},
			{"trailing lines", "\n\n# trailing comment\n\n"},
			{"separator", "\n\n# trailing comment\n### next\nGET http://x"},
			{"request", "\n\n# trailing comment\nGET http://x"},
			{"workflow", "\n\n# trailing comment\n# @workflow next\n# @step s run=A"},
		} {
			t.Run(directive.name+"/"+ending.name, func(t *testing.T) {
				source := "# @workflow w\n" + directive.source + ending.source
				doc := Parse("workflow.http", []byte(source))
				if len(doc.Errors) != 1 || doc.Errors[0].Message != directive.message {
					t.Fatalf("errors = %+v, want one error %q", doc.Errors, directive.message)
				}
				if got := doc.Errors[0].Span.Start.Line; got != 2 {
					t.Errorf("error line = %d, want 2", got)
				}
				rep := Diagnostics(doc)
				if len(rep.Items) != 1 {
					t.Fatalf("diagnostics = %+v, want one", rep.Items)
				}
				item := rep.Items[0]
				if item.Severity != diag.SeverityError || item.Span.Start.Line != 2 || item.Span.Start.Col != 1 {
					t.Fatalf("diagnostic = %+v, want error at 2:1", item)
				}
				firstLine, _, _ := strings.Cut(directive.source, "\n")
				if got := sourceSpanText(t, source, item.Span); got != firstLine {
					t.Errorf("marked %q, want %q", got, firstLine)
				}
				if err := Check(doc); err == nil {
					t.Fatal("unfinished workflow must still fail Check")
				}
			})
		}
	}
}

func sourceSpanText(t *testing.T, source string, span diag.Span) string {
	t.Helper()
	lines := strings.Split(source, "\n")
	if span.Start.Line < 1 || span.Start.Line > len(lines) || span.End.Line != span.Start.Line {
		t.Fatalf("invalid line span %+v", span)
	}
	line := lines[span.Start.Line-1]
	if span.Start.Col < 1 || span.End.Col < span.Start.Col || span.End.Col > len(line)+1 {
		t.Fatalf("invalid column span %+v on %q", span, line)
	}
	return line[span.Start.Col-1 : span.End.Col-1]
}

// Check, WarningTexts, and the editor all read the same span.
func TestDiagnosticsShareLocationsWithCheck(t *testing.T) {
	source := "# @mock method=GET path=/x\n# @match query={\n# \"one\":\"two\"} typo=x typo=y\nHTTP/1.1 200 OK"
	doc := Parse("test.http", []byte(source))
	rep := Diagnostics(doc)
	if len(doc.Errors) != 2 || len(rep.Items[0].Labels) != 1 {
		t.Fatalf("findings = %+v", rep.Items)
	}
	if doc.Errors[0].Span.Start.Line != 3 || rep.Items[0].Span.Start.Line != 3 {
		t.Fatalf("errors=%+v report=%+v", doc.Errors, rep.Items)
	}
	rep.Items[0].Labels[0].Span.Start.Line = 999
	if doc.Errors[0].Labels[0].Span.Start.Line == 999 {
		t.Fatal("report shares labels with the document")
	}
	if err := Check(doc); err == nil || !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("Check location = %v, want line 3", err)
	}
	warning := Parse("test.http", []byte("# @sse max-event=5\nGET http://x"))
	want := slices.Clone(WarningTexts(warning))
	Diagnostics(warning)
	if !slices.Equal(want, WarningTexts(warning)) || Check(warning) != nil {
		t.Fatal("warnings changed")
	}
}

func TestSourceDirectiveOwners(t *testing.T) {
	source := "# @assert sum(\n# 1) == 1\nGET http://x\n> {%\n# @nmae body\n%}"
	var syntax SourceSyntax
	syntax.Classify(source)
	if syntax.Line(0).Directive != directive.Assert || syntax.Line(1).Directive != directive.Assert {
		t.Fatal("continuation lost its owner")
	}
	if syntax.Line(4).Directive != "" {
		t.Fatal("body has directive owner")
	}
	if got := Diagnostics(Parse("test.http", []byte(source))); len(got.Items) != 0 {
		t.Fatalf("body warnings: %+v", got)
	}
}

func BenchmarkDiagnostics(b *testing.B) {
	source := []byte(strings.Repeat("### r\n# @sse max-event=5\nGET http://x\n", 1000))
	b.ReportAllocs()
	for b.Loop() {
		Diagnostics(Parse("large.http", source))
	}
}

func TestUnclosedPlaceholderWarnings(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         []string // "line:marked text" for each warning
	}{
		{
			name:   "request line",
			source: "GET http://x/{{id}",
			want:   []string{"1:{{id}"},
		},
		{
			name:   "header value",
			source: "GET http://x\nAuthorization: Bearer {{tok}",
			want:   []string{"2:{{tok}"},
		},
		{
			name:   "variable value",
			source: "@a = {{b}\nGET http://x",
			want:   []string{"1:{{b}"},
		},
		{
			name:   "directive argument",
			source: "GET http://x\n# @auth bearer {{tok}",
			want:   []string{"2:{{tok}"},
		},
		{
			name:   "body line, comments dropped from the body",
			source: "POST http://x\n\n{\n# note\n  \"a\": \"{{tok}\"\n}",
			want:   []string{"5:{{tok}"},
		},
		{
			name:   "two on one body line",
			source: "POST http://x\n\n{\"a\":\"{{x}\",\"b\":\"{{y}\"}",
			want:   []string{"3:{{x}", "3:{{y}"},
		},
		{
			name:   "multipart part",
			source: "POST http://x\nContent-Type: multipart/form-data; boundary=b\n\n--b\nx: {{y}\n--b--",
			want:   []string{"5:{{y}"},
		},
		{
			name:   "placeholder spanning body lines",
			source: "@tok = 1\n\nPOST http://x\n\n{\n  \"a\": \"{{\ntok\n}}\"\n}",
		},
		{
			name:   "opening never closed anywhere in the body",
			source: "POST http://x\n\n{\n  \"a\": \"{{\ntok\n\"\n}",
			want:   []string{"4:{{"},
		},
		{
			name:   "braces quoted in a script argument",
			source: "GET http://x\n# @assert contains(\"{{\", \"{\")",
		},
		{
			// Balance the braces so the parser accepts the complete directive.
			name:   "script argument outside its strings",
			source: "GET http://x\n# @assert contains(\"{{\", \"{\") && {{tok} == 1 }",
			want:   []string{"2:{{tok}"},
		},
		{
			name:   "quoted value of a plain argument",
			source: "GET http://x\n# @auth bearer \"{{tok}\"",
			want:   []string{"2:{{tok}"},
		},
		{
			name:   "braces in a script comment",
			source: "GET http://x\n# @assert true # {{bad}",
		},
		{
			name:   "braces quoted in an RTS capture",
			source: "GET http://x\n# @capture request x = json(\"{{\")",
		},
		{
			name:   "quoted braces in a template capture",
			source: "GET http://x\n# @capture request x {{response.status}} \"{{bad}\"",
			want:   []string{"2:{{bad}"},
		},
		{
			name:   "gRPC message",
			source: "# @grpc pkg.Svc/M\nGRPC localhost:50051\n\n{\n  \"a\": \"{{tok}\"\n}",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			doc := Parse("test.http", []byte(tt.source))
			if len(doc.Errors) != 0 {
				t.Fatalf("errors = %+v", doc.Errors)
			}
			var got []string
			for _, item := range doc.Warnings {
				marked := sourceSpanText(t, tt.source, item.Span)
				if want := unclosedMessage(vars.Unclosed{Text: marked}); item.Message != want {
					t.Fatalf("warning %q does not name what it marks, want %q", item.Message, want)
				}
				got = append(got, fmt.Sprintf("%d:%s", item.Span.Start.Line, marked))
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("warnings = %v, want %v", got, tt.want)
			}
		})
	}
}
