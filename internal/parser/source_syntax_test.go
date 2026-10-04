package parser

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
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
			want:   SourceLineScript,
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

func TestSourceSyntaxScriptLines(t *testing.T) {
	tests := []struct {
		name    string
		source  []string
		line    int
		kind    SourceLineKind
		lang    string
		content string
	}{
		{
			name:    "default language",
			source:  []string{`> vars.get("a")`},
			kind:    SourceLineScript,
			lang:    "js",
			content: `vars.get("a")`,
		},
		{
			name:    "rts block",
			source:  []string{"# @rts pre-request", "> vars.set(1)"},
			line:    1,
			kind:    SourceLineScript,
			lang:    "rts",
			content: "vars.set(1)",
		},
		{
			name:   "lang option",
			source: []string{"# @script pre-request lang=rts", "> x"},
			line:   1,
			kind:   SourceLineScript,
			lang:   "rts",
		},
		{
			name:   "bare script keeps the language",
			source: []string{"# @rts pre-request", "> a", "# @script", "> b"},
			line:   3,
			kind:   SourceLineScript,
			lang:   "rts",
		},
		{
			name:   "rejected rts drops the line",
			source: []string{"# @rts test", "> a"},
			line:   1,
			kind:   SourceLineScript,
		},
		{
			name:   "new request starts with javascript",
			source: []string{"# @rts pre-request", "> a", "GET https://a.test", "", "###", "> b"},
			line:   5,
			kind:   SourceLineScript,
			lang:   "js",
		},
		{
			name:   "include",
			source: []string{"> < scripts/pre.js"},
			kind:   SourceLineCode,
		},
		{
			name:   "include without a path",
			source: []string{"> <"},
			kind:   SourceLineScript,
			lang:   "js",
		},
		{
			name:    "block line",
			source:  []string{"# @rts pre-request", "> {%", "  vars.get(1)", "> %}"},
			line:    2,
			kind:    SourceLineScript,
			lang:    "rts",
			content: "  vars.get(1)",
		},
		{
			name:    "block line with a marker",
			source:  []string{"> {%", "> vars.get(1)", "> %}"},
			line:    1,
			kind:    SourceLineScript,
			lang:    "js",
			content: "vars.get(1)",
		},
		{
			name:    "block comment line",
			source:  []string{"> {%", "// JavaScript comment", "> %}"},
			line:    1,
			kind:    SourceLineScript,
			lang:    "js",
			content: "// JavaScript comment",
		},
		{
			name:   "block markers",
			source: []string{"> {%", "x", "> %}"},
			line:   2,
			kind:   SourceLineLiteral,
		},
		{
			name:    "indented marker",
			source:  []string{"  >  é = 1"},
			kind:    SourceLineScript,
			lang:    "js",
			content: " é = 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifySource(strings.Join(tt.source, "\n"))[tt.line]
			if got.Kind != tt.kind || got.ScriptLang != tt.lang {
				t.Fatalf("line %d = %v %q, want %v %q", tt.line+1, got.Kind, got.ScriptLang, tt.kind, tt.lang)
			}
			if tt.content != "" {
				if text := sourceContent(tt.source[tt.line], got); text != tt.content {
					t.Fatalf("line %d content = %q, want %q", tt.line+1, text, tt.content)
				}
			}
		})
	}
}

func TestSourceSyntaxScriptLangAgreesWithParser(t *testing.T) {
	prefixes := []string{
		"",
		"# @script pre-request",
		"# @script test",
		"# @script pre-request lang=rts",
		"# @script pre-request rts",
		"# @script rts",
		"# @script pre-request lang=python",
		"# @script pre-request bogus=1",
		"# @rts pre-request",
		"# @rts",
		"# @rts test",
		"# @rts pre-request lang=js",
		"# @rts pre-request\n# @script",
		"# @rts pre-request\n# @rts bogus",
		"# @rts bogus\n# @script",
		"# @script pre-request lang=rts\n# @script test",
		"GET https://a.test\n\n###\n# @rts pre-request",
	}
	bodies := []string{
		"> a",
		"> a\n> < x.js\n> b",
		"> <",
		"> {%\na\n> b\n> < x.js\n> %}",
		"  >   a",
	}

	for _, prefix := range prefixes {
		for _, body := range bodies {
			source := strings.TrimPrefix(prefix+"\n"+body+"\nGET https://example.test", "\n")
			want := make(map[int]restfile.ScriptLine)
			langs := make(map[int]string)
			for _, req := range Parse("agree.http", []byte(source)).Requests {
				for _, block := range req.Metadata.Scripts {
					for _, l := range block.Lines {
						want[l.Line], langs[l.Line] = l, block.Lang
					}
				}
			}

			lines := strings.Split(source, "\n")
			for no, got := range classifySource(source) {
				l, ok := want[no+1]
				if got.Kind != SourceLineScript || got.ScriptLang == "" {
					if ok {
						t.Errorf("%q line %d = %v, parser runs it as %s", source, no+1, got.Kind, langs[no+1])
					}
					continue
				}
				if !ok || got.ScriptLang != langs[no+1] {
					t.Errorf("%q line %d lang = %q, parser has %q", source, no+1, got.ScriptLang, langs[no+1])
					continue
				}
				if start := utf8.RuneCountInString(lines[no][:l.Col-1]); got.ContentStart != start {
					t.Errorf("%q line %d starts at %d, parser has %d", source, no+1, got.ContentStart, start)
				}
			}
		}
	}
}

func TestSourceSyntaxInExprAgreesWithParser(t *testing.T) {
	heads := []string{
		"POST https://a.test\nContent-Type: application/json\n\n",
		"POST https://a.test\nContent-Type: multipart/form-data; boundary=X\n\n--X\n" +
			"Content-Disposition: form-data; name=\"a\"\n\n",
	}
	bodies := []string{
		"{{=\n  vars.get(\"a\")\n}}",
		"{\"a\": \"{{= 1 +\n  vars.get(\"a\")\n}}\"}",
		"{{=\n# note\n\n> x\n@v = 1\n  vars.get(\"a\")\n}}",
		"{{= a\n}} b\n{{= c\n  d }}",
		"{{ name\n  x\n}}",
		"{\n{= a\nb",
		"{{= a } b\nc",
		"{{ \n= a\nb }}",
		"{{= a\n> {%\nb\n> %}\nc }}",
	}

	for _, head := range heads {
		for _, body := range bodies {
			source := head + body + "\n--X--\n\n###\nGET https://b.test\n\n{{= x\ny }}"
			want := make(map[int]bool)
			for _, req := range Parse("agree.http", []byte(source)).Requests {
				var s vars.PlaceholderScanner
				for i, text := range strings.Split(req.Body.Text, "\n") {
					want[req.Body.Lines[i]] = s.InExpr()
					s.Feed(text + "\n")
				}
			}

			lines := strings.Split(source, "\n")
			for no, got := range classifySource(source) {
				// A blank line has nothing to complete.
				w := want[no+1] && strings.TrimSpace(lines[no]) != ""
				if got.InExpr != w {
					t.Errorf("%q line %d InExpr = %t, parser body has %t", source, no+1, got.InExpr, w)
				}
			}
		}
	}
}

func TestSourceSyntaxScriptArgsAgreesWithParser(t *testing.T) {
	captures := []string{
		"# @capture file id response.json.id",
		"# @capture file url {{response.json.base}}/vars.",
		`# @capture file q "{{a}}" + vars.get("b")`,
		"# @capture file id f(\n#   response.json.id)",
		"# @capture file url f(\n#   {{response.json.base}})",
		"# @capture file url {{\n#   response.json.base}}",
	}
	for _, c := range captures {
		source := c + "\nGET https://example.test"
		caps := Parse("agree.http", []byte(source)).Requests[0].Metadata.Captures
		if len(caps) != 1 {
			t.Fatalf("%q parsed %d captures, want 1", source, len(caps))
		}
		want := caps[0].Mode == restfile.CaptureExprModeRTS
		for no, got := range classifySource(c) {
			if got.ScriptArgs != want {
				t.Errorf("%q line %d ScriptArgs = %t, want %t from the parser", c, no+1, got.ScriptArgs, want)
			}
		}
	}

	for source, want := range map[string][]bool{
		"# @assert status == 200":          {true},
		"# @assert (\n#   true\n# )":       {true, true, true},
		"# @name vars":                     {false},
		"# @capture file id f(\n#   vars.": {true, true},
	} {
		for no, got := range classifySource(source) {
			if got.ScriptArgs != want[no] {
				t.Errorf("%q line %d ScriptArgs = %t, want %t", source, no+1, got.ScriptArgs, want[no])
			}
		}
	}
}

func TestOpenDirectiveScriptArgsMatchesRescan(t *testing.T) {
	sources := []string{
		"# @capture file x f(\n#   a,\n#   {{b}} {{c\n#   d}}",
		"# @capture file x {{a}} {{b\n#   c}}",
		"# @capture file x f(\n#   \"{{a}}\",\n#   b)",
		"# @capture file x {{\n#   a}}",
		"# @capture file x f(\n#   g(\n#   1))",
		"# @capture file x f(\n#   {{a}})",
		"# @assert (\n#   true\n# )",
	}
	for _, src := range sources {
		var r directiveReader
		for i, raw := range strings.Split(src, "\n") {
			c, _ := makeLine(i+1, raw, "").comment()
			res := r.read(i+1, c)
			if res.open == nil {
				t.Fatalf("%q line %d is not in a multiline directive", src, i+1)
			}
			d := res.open.d
			d.Args = res.open.args.String()
			if got, want := res.open.scriptArgs(), d.scriptArgs(); got != want {
				t.Errorf("%q line %d scriptArgs = %t, rescan gives %t", src, i+1, got, want)
			}
		}
		if _, open := r.pending(); open {
			t.Errorf("%q did not complete", src)
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
