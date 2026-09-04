package parser

import (
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
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

func TestDiagnosticsPreserveLegacyLocationsAndOwnData(t *testing.T) {
	source := "# @mock method=GET path=/x\n# @match query={\n# \"one\":\"two\"} typo=x typo=y\nHTTP/1.1 200 OK"
	doc := Parse("test.http", []byte(source))
	rep := Diagnostics(doc)
	if len(doc.Errors) != 2 || len(rep.Items[0].Labels) != 1 {
		t.Fatalf("findings = %+v", rep.Items)
	}
	if doc.Errors[0].Line != 2 || rep.Items[0].Span.Start.Line != 3 {
		t.Fatalf("legacy=%+v report=%+v", doc.Errors, rep.Items)
	}
	rep.Source[0] = '!'
	rep.Items[0].Labels[0].Span.Start.Line = 999
	if string(doc.Raw) != source || doc.Errors[0].Labels[0].Span.Start.Line == 999 {
		t.Fatal("report mutated document")
	}
	if err := Check(doc); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("legacy Check location changed: %v", err)
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
