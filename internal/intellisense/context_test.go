package intellisense

import "testing"

type testLines []string

func (t testLines) LineCount() int         { return len(t) }
func (t testLines) LineRunes(i int) []rune { return []rune(t[i]) }

func TestAnalyzeClassifiesContexts(t *testing.T) {
	cases := []struct {
		name      string
		lines     []string
		line, col int
		wantKind  Kind
		wantDir   string
		wantArg   string
		wantQuery string
	}{
		{
			name:     "directive name after comment marker",
			lines:    []string{"# @au"},
			line:     0,
			col:      5,
			wantKind: KindDirective, wantQuery: "au",
		},
		{
			name:     "bare directive marker",
			lines:    []string{"# @"},
			line:     0,
			col:      3,
			wantKind: KindDirective, wantQuery: "",
		},
		{
			name:     "slash comment directive",
			lines:    []string{"// @nam"},
			line:     0,
			col:      7,
			wantKind: KindDirective, wantQuery: "nam",
		},
		{
			name:     "directive arg",
			lines:    []string{"# @auth bea"},
			line:     0,
			col:      11,
			wantKind: KindDirectiveArg, wantDir: "auth", wantQuery: "bea",
		},
		{
			name:     "directive arg use value",
			lines:    []string{"# @apply use=js"},
			line:     0,
			col:      15,
			wantKind: KindDirectiveArg, wantDir: "apply", wantArg: "use", wantQuery: "js",
		},
		{
			name:     "multi-arg directive completes the last token",
			lines:    []string{"# @profile count=1 warm"},
			line:     0,
			col:      23,
			wantKind: KindDirectiveArg, wantDir: "profile", wantQuery: "warm",
		},
		{
			name:     "method at line start",
			lines:    []string{"GE"},
			line:     0,
			col:      2,
			wantKind: KindMethod, wantQuery: "ge",
		},
		{
			name:     "method on empty line",
			lines:    []string{""},
			line:     0,
			col:      0,
			wantKind: KindMethod, wantQuery: "",
		},
		{
			name:     "no method completion in url",
			lines:    []string{"GET https://x"},
			line:     0,
			col:      9,
			wantKind: KindNone,
		},
		{
			name:     "header name in header section",
			lines:    []string{"GET https://x", "Cont"},
			line:     1,
			col:      4,
			wantKind: KindHeaderName, wantQuery: "cont",
		},
		{
			name:     "header value after colon",
			lines:    []string{"GET https://x", "Content-Type: app"},
			line:     1,
			col:      17,
			wantKind: KindHeaderValue, wantDir: "content-type", wantQuery: "app",
		},
		{
			name:     "no completion in body",
			lines:    []string{"GET https://x", "Accept: foo", "", "bodyline"},
			line:     3,
			col:      8,
			wantKind: KindNone,
		},
		{
			name:     "variable inside braces",
			lines:    []string{"GET https://{{ho}}/x"},
			line:     0,
			col:      16,
			wantKind: KindVariable, wantQuery: "ho",
		},
		{
			name:     "variable inside unclosed braces",
			lines:    []string{"x {{ho"},
			line:     0,
			col:      6,
			wantKind: KindVariable, wantQuery: "ho",
		},
		{
			name:     "variable on directive line",
			lines:    []string{"# @file host = {{ba"},
			line:     0,
			col:      19,
			wantKind: KindVariable, wantQuery: "ba",
		},
		{
			name:     "outside closed braces is not a variable",
			lines:    []string{"x {{a}} b"},
			line:     0,
			col:      9,
			wantKind: KindNone,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, ok := Analyze(testLines(tc.lines), tc.line, tc.col)
			if tc.wantKind == KindNone {
				if ok && ctx.Kind != KindNone {
					t.Fatalf("expected no completion, got kind %d (%+v)", ctx.Kind, ctx)
				}
				return
			}
			if !ok {
				t.Fatalf("expected completion context, got none")
			}
			if ctx.Kind != tc.wantKind {
				t.Fatalf("kind = %d, want %d", ctx.Kind, tc.wantKind)
			}
			if ctx.Directive != tc.wantDir {
				t.Fatalf("directive = %q, want %q", ctx.Directive, tc.wantDir)
			}
			if ctx.ArgKey != tc.wantArg {
				t.Fatalf("argKey = %q, want %q", ctx.ArgKey, tc.wantArg)
			}
			if ctx.Query != tc.wantQuery {
				t.Fatalf("query = %q, want %q", ctx.Query, tc.wantQuery)
			}
		})
	}
}

func TestAnalyzeStartMarksReplacementToken(t *testing.T) {
	// "# @au": '@' is at rune index 2, so the replaced span starts there.
	ctx, ok := Analyze(testLines{"# @au"}, 0, 5)
	if !ok || ctx.Kind != KindDirective {
		t.Fatalf("expected directive context, got %+v ok=%v", ctx, ok)
	}
	if ctx.Start != 2 {
		t.Fatalf("directive start = %d, want 2", ctx.Start)
	}

	// Variable token starts right after the leading "{{".
	ctx, ok = Analyze(testLines{"x {{ho"}, 0, 6)
	if !ok || ctx.Kind != KindVariable {
		t.Fatalf("expected variable context, got %+v ok=%v", ctx, ok)
	}
	if ctx.Start != 4 {
		t.Fatalf("variable start = %d, want 4", ctx.Start)
	}
}

func TestAnalyzeRejectsCaretBeforeBraces(t *testing.T) {
	if ctx, ok := Analyze(testLines{"a{{b}}"}, 0, 1); ok && ctx.Kind == KindVariable {
		t.Fatalf("did not expect a variable context before the braces")
	}
}
