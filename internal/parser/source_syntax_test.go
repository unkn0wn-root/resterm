package parser

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/directive"
)

func TestSourceSyntaxComments(t *testing.T) {
	lines := []string{
		"\t# ✅ hash comment",
		"  // slash comment",
		"-- dash comment",
		"## @if this stays prose",
		"/**",
		" * block prose",
		" * @name Blocked",
		" **/",
		"### named request",
		"GET https://example.test",
	}

	got := classifySource(strings.Join(lines, "\r\n"))
	assertSourceKinds(t, got, []SourceLineKind{
		SourceLineComment,
		SourceLineComment,
		SourceLineComment,
		SourceLineComment,
		SourceLineComment,
		SourceLineComment,
		SourceLineDirective,
		SourceLineComment,
		SourceLineRequestSeparator,
		SourceLineCode,
	})

	for no, want := range []string{
		"✅ hash comment",
		"slash comment",
		"dash comment",
		"# @if this stays prose",
		"",
		"block prose",
		"@name Blocked",
		"",
	} {
		if text := sourceContent(lines[no], got[no]); text != want {
			t.Fatalf("line %d content = %q, want %q", no+1, text, want)
		}
	}
}

func TestSourceSyntaxMultilineDirectives(t *testing.T) {
	source := strings.Join([]string{
		"# @assert (",
		"# true",
		"# )",
		"# ordinary prose",
		"# @match json={",
		`# "å": 1`,
		`# } headers={"X-Env":"test"}`,
		"# @mock method=GET path=/health",
		"HTTP/1.1 200 OK",
		"",
		"# response body",
	}, "\n")

	got := classifySource(source)
	assertSourceKinds(t, got, []SourceLineKind{
		SourceLineDirective,
		SourceLineDirectiveValue,
		SourceLineDirectiveValue,
		SourceLineComment,
		SourceLineDirective,
		SourceLineDirectiveValue,
		SourceLineDirectiveValue,
		SourceLineDirective,
		SourceLineCode,
		SourceLineCode,
		SourceLineComment,
	})

	for _, no := range []int{0, 1, 2} {
		if got[no].Args != directive.ArgText {
			t.Fatalf("line %d args = %d, want text", no+1, got[no].Args)
		}
	}
	for _, no := range []int{4, 5, 6} {
		if got[no].Args != directive.ArgOptions {
			t.Fatalf("line %d args = %d, want options", no+1, got[no].Args)
		}
	}
	if got[5].OptionValueEnd != got[5].ContentEnd {
		t.Fatalf("line 6 option value end = %d, want %d", got[5].OptionValueEnd, got[5].ContentEnd)
	}
	if want := got[6].ContentStart + 1; got[6].OptionValueEnd != want {
		t.Fatalf("line 7 option value end = %d, want %d", got[6].OptionValueEnd, want)
	}
	if mocks := Parse("complete.http", []byte(source)).Mocks; len(mocks) != 0 {
		t.Fatalf("parser created %d mocks after a request-opening assertion, want none", len(mocks))
	}
}

func TestSourceSyntaxMultilineOptionTruncatedUTF8(t *testing.T) {
	sources := []string{
		"# @match json={\"a\":\"\xe2\x80\n#  \"} query={}",
		"# @match regex=\"first\xf0\x9f\n# \" query={}",
		"# @match json={\"a\":\"\xc3",
	}

	for _, source := range sources {
		got := classifySource(source)
		if got[0].Kind != SourceLineDirective {
			t.Fatalf("classify(%q) first line kind = %s, want directive", source, got[0].Kind)
		}
	}
}

func TestSourceSyntaxMultilineOptionEscapeAtLineEnd(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		valueRunes int
	}{
		{
			name:       "quoted value",
			source:     "# @match regex=\"first\\\n# \" query={\"page\":\"2\"}",
			valueRunes: 1,
		},
		{
			name:       "JSON string",
			source:     "# @match json={\"value\":\"first\\\n# \"} query={\"page\":\"2\"}",
			valueRunes: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifySource(tt.source)
			line := got[1]
			if line.Kind != SourceLineDirectiveValue {
				t.Fatalf("line kind = %s, want directive value", line.Kind)
			}
			if want := line.ContentStart + tt.valueRunes; line.OptionValueEnd != want {
				t.Fatalf("option value end = %d, want %d", line.OptionValueEnd, want)
			}
		})
	}
}

func TestSourceSyntaxMarksADirectiveBeingTyped(t *testing.T) {
	tests := []struct {
		name   string
		source []string
		line   int
		want   SourceLineKind
	}{
		{
			name:   "file scope",
			source: []string{"# @"},
			want:   SourceLineDirective,
		},
		{
			name:   "separator instead of a name",
			source: []string{"# @:"},
			want:   SourceLineComment,
		},
		{
			name:   "separator before a name",
			source: []string{"# @:x"},
			want:   SourceLineComment,
		},
		{
			name:   "block comment",
			source: []string{"/**", " * @", " */"},
			line:   1,
			want:   SourceLineDirective,
		},
		{
			name:   "mock preamble",
			source: []string{"# @mock method=GET path=/health", "# @", "HTTP/1.1 200 OK"},
			line:   1,
			want:   SourceLineDirective,
		},
		{
			name:   "line an argument runs on",
			source: []string{"# @assert sum(", "# @"},
			line:   1,
			want:   SourceLineDirectiveValue,
		},
		{
			name:   "script block",
			source: []string{"# @script test", "> {%", "# @", "> %}"},
			line:   2,
			want:   SourceLineLiteral,
		},
		{
			name:   "mock response body",
			source: []string{"# @mock method=GET path=/health", "HTTP/1.1 200 OK", "", "# @"},
			line:   3,
			want:   SourceLineLiteral,
		},
		{
			name: "multipart part",
			source: []string{
				"POST https://example.test",
				"Content-Type: multipart/form-data; boundary=B",
				"",
				"--B",
				"",
				"# @",
				"--B--",
			},
			line: 5,
			want: SourceLineLiteral,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifySource(strings.Join(tt.source, "\n"))[tt.line]
			if got.Kind != tt.want {
				t.Fatalf("line %d kind = %v, want %v", tt.line+1, got.Kind, tt.want)
			}
		})
	}
}

func TestSourceSyntaxKeepsLiteralContentUnclassified(t *testing.T) {
	tests := []struct {
		name   string
		source string
		line   int
	}{
		{
			name: "multipart body",
			source: strings.Join([]string{
				"POST https://example.test/upload",
				"Content-Type: multipart/form-data; boundary=B",
				"",
				"--B",
				"Content-Disposition: form-data; name=script",
				"",
				"# literal body",
				"--B--",
			}, "\n"),
			line: 6,
		},
		{
			name:   "mock response body",
			source: "# @mock method=GET path=/health\nHTTP/1.1 200 OK\n\n# literal body",
			line:   3,
		},
		{
			name:   "script block",
			source: "# @script test\n> {%\n// JavaScript comment\n> %}",
			line:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifySource(tt.source)[tt.line].Kind; got != SourceLineLiteral {
				t.Fatalf("line kind = %v, want literal", got)
			}
		})
	}
}

func TestSourceSyntaxReuseAndBounds(t *testing.T) {
	var syntax SourceSyntax
	syntax.Classify("// one\n// two\n// three")
	syntax.Classify("GET https://example.test\n")

	if syntax.Len() != 2 {
		t.Fatalf("classified %d lines, want 2", syntax.Len())
	}
	for _, no := range []int{0, 1, 7} {
		if kind := syntax.Line(no).Kind; kind != SourceLineCode {
			t.Fatalf("line %d kind = %v, want code", no+1, kind)
		}
	}

	line := SourceLine{Kind: SourceLineComment, ContentStart: 2, ContentEnd: 40}
	start, end, ok := line.ContentRange(6)
	if !ok || start != 2 || end != 6 {
		t.Fatalf("ContentRange(6) = (%d, %d, %v), want (2, 6, true)", start, end, ok)
	}
}

func TestSourceSyntaxMockAgreesWithParser(t *testing.T) {
	prefixes := []string{
		"",
		"GET https://example.test",
		"@request value = 1",
		"# @assert sum(",
		"# @workflow flow\n# @step login using=Login",
	}
	for _, spec := range directive.Specs() {
		prefixes = append(prefixes, "# "+spec.Name.Tag(), "# "+spec.Name.Tag()+" value")
	}

	for no, prefix := range prefixes {
		source := prefix
		if source != "" {
			source += "\n"
		}
		source += "# @mock method=GET path=/health\nHTTP/1.1 200 OK\n\n# body marker\n"

		body := strings.Count(source, "\n") - 1
		got := classifySource(source)[body].Kind
		mocked := len(Parse("agree.http", []byte(source)).Mocks) == 1
		if (got == SourceLineLiteral) != mocked {
			t.Fatalf("case %d: body kind = %v, parser found mock = %v", no, got, mocked)
		}
	}
}

func TestArgKindComesFromCatalog(t *testing.T) {
	for name, want := range map[directive.Name]directive.ArgKind{
		"nolog":       directive.ArgNone,
		"settings":    directive.ArgOptions,
		"grpc-method": directive.ArgToken,
	} {
		if got := argKind(name); got != want {
			t.Fatalf("argKind(%q) = %d, want %d", name, got, want)
		}
	}
}

func classifySource(source string) []SourceLine {
	var syntax SourceSyntax
	syntax.Classify(source)
	out := make([]SourceLine, syntax.Len())
	for no := range out {
		out[no] = syntax.Line(no)
	}
	return out
}

func assertSourceKinds(t *testing.T, got []SourceLine, want []SourceLineKind) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("classified %d lines, want %d", len(got), len(want))
	}
	for no := range want {
		if got[no].Kind != want[no] {
			t.Fatalf("line %d kind = %v, want %v", no+1, got[no].Kind, want[no])
		}
	}
}

func sourceContent(line string, syntax SourceLine) string {
	runes := []rune(line)
	start, end, ok := syntax.ContentRange(len(runes))
	if !ok {
		return ""
	}
	return string(runes[start:end])
}
