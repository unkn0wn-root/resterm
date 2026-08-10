package parser

import (
	"errors"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

func parseOneRequest(t *testing.T, src string) *restfile.Request {
	t.Helper()
	doc := Parse("continue.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %+v", doc.Errors)
	}
	if len(doc.Requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(doc.Requests))
	}
	return doc.Requests[0]
}

func onlyAssert(t *testing.T, src string) restfile.AssertSpec {
	t.Helper()
	req := parseOneRequest(t, src)
	if len(req.Metadata.Asserts) != 1 {
		t.Fatalf("asserts = %+v, want 1", req.Metadata.Asserts)
	}
	return req.Metadata.Asserts[0]
}

func TestParseAssertSpansCommentLines(t *testing.T) {
	as := onlyAssert(t, `# @assert licensing.assertResponse(
#   response,
#   expected,
#   options
# )
GET https://example.com/
`)

	want := "licensing.assertResponse(\n" +
		"    response,\n" +
		"    expected,\n" +
		"    options\n" +
		"  )"
	if as.Expression != want {
		t.Fatalf("expression = %q, want %q", as.Expression, want)
	}
	if as.Line != 1 || as.Col != 11 {
		t.Fatalf("position = %d:%d, want 1:11", as.Line, as.Col)
	}
}

func TestParseAssertContinuationMarkers(t *testing.T) {
	cases := map[string][2]string{
		"hash":    {"# ", "#   "},
		"slashes": {"// ", "//   "},
		"dashes":  {"-- ", "--   "},
		"mixed":   {"# ", "//   "},
	}
	for name, marker := range cases {
		t.Run(name, func(t *testing.T) {
			src := marker[0] + "@assert sum(\n" + marker[1] + "1, 2\n" + marker[0] + ") == 3\n" +
				"GET https://example.com/\n"
			as := onlyAssert(t, src)
			if !strings.HasPrefix(as.Expression, "sum(\n") ||
				!strings.HasSuffix(as.Expression, ") == 3") {
				t.Fatalf("expression = %q", as.Expression)
			}
			if as.Line != 1 {
				t.Fatalf("line = %d, want 1", as.Line)
			}
		})
	}
}

func TestParseAssertContinuationKeepsMessage(t *testing.T) {
	as := onlyAssert(t, `# @assert contains(
#   header("Content-Type"),
#   "json"
# ) => "content type"
GET https://example.com/
`)
	if as.Message != "content type" {
		t.Fatalf("message = %q, want %q", as.Message, "content type")
	}
	if strings.Contains(as.Expression, "=>") {
		t.Fatalf("expression kept the message: %q", as.Expression)
	}
}

func TestParseAssertDelimitersInsideStringsAndComments(t *testing.T) {
	cases := map[string]string{
		"bracket in a string":   `# @assert body.contains("(")`,
		"bracket in a comment":  `# @assert status == 200 # note (`,
		"escaped quote":         `# @assert body.contains("\"(")`,
		"closer in a string":    `# @assert sum(1, 2) == count(")")`,
		"apostrophe in a value": `# @assert name == 'it (s'`,
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			as := onlyAssert(t, line+"\n# not part of the assertion\nGET https://example.com/\n")
			if strings.Contains(as.Expression, "not part") {
				t.Fatalf("expression swallowed the next line: %q", as.Expression)
			}
		})
	}
}

func TestParseAssertContinuationSpansAStringDelimiter(t *testing.T) {
	as := onlyAssert(t, `# @assert contains(
#   "a )( b",
#   "("
# )
GET https://example.com/
`)
	if !strings.HasSuffix(as.Expression, "\n  )") {
		t.Fatalf("expression = %q", as.Expression)
	}
}

func TestParseAssertMismatchedCloserEndsAtTheMatchingOne(t *testing.T) {
	req := parseOneRequest(t, `# @assert sum(
#   1] + 2
# )
# @assert status == 200
GET https://example.com/
`)
	if len(req.Metadata.Asserts) != 2 {
		t.Fatalf("asserts = %+v, want 2", req.Metadata.Asserts)
	}
	as := req.Metadata.Asserts[0]
	if strings.Contains(as.Expression, "status") {
		t.Fatalf("expression ran past its closer: %q", as.Expression)
	}
	if _, err := rts.ParseExpr("continue.http", as.Line, as.Col, as.Expression); err == nil {
		t.Fatal("expected the script parser to reject the stray closer")
	}
}

func TestParseContinuationReportsPositionsOnLaterLines(t *testing.T) {
	as := onlyAssert(t, `# @assert sum(
#   1,
#   2 3
# )
GET https://example.com/
`)
	_, err := rts.ParseExpr("continue.http", as.Line, as.Col, as.Expression)
	var perr *rts.ParseError
	if !errors.As(err, &perr) {
		t.Fatalf("err = %v, want a parse error", err)
	}
	if perr.Pos.Line != 3 || perr.Pos.Col != 7 {
		t.Fatalf("position = %d:%d, want 3:7", perr.Pos.Line, perr.Pos.Col)
	}
}

func TestParseContinuationStopsAtTheNextDirective(t *testing.T) {
	doc := Parse("continue.http", []byte(`# @assert sum(
# @capture request total response.json.total
GET https://example.com/
`))
	if len(doc.Errors) != 1 {
		t.Fatalf("errors = %+v, want 1", doc.Errors)
	}
	if doc.Errors[0].Line != 1 || !strings.Contains(doc.Errors[0].Message, `missing a closing ")"`) {
		t.Fatalf("error = %+v", doc.Errors[0])
	}
	if len(doc.Requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(doc.Requests))
	}
	req := doc.Requests[0]
	if len(req.Metadata.Asserts) != 0 {
		t.Fatalf("asserts = %+v, want none", req.Metadata.Asserts)
	}
	if len(req.Metadata.Captures) != 1 || req.Metadata.Captures[0].Name != "total" {
		t.Fatalf("captures = %+v, want the @capture below it", req.Metadata.Captures)
	}
}

func TestParseContinuationKeepsAnUnknownAtName(t *testing.T) {
	as := onlyAssert(t, `# @assert has(
#   "@notADirective"
# )
GET https://example.com/
`)
	if !strings.Contains(as.Expression, "@notADirective") {
		t.Fatalf("expression = %q", as.Expression)
	}
}

func TestParseContinuationStopsAtLineKinds(t *testing.T) {
	cases := map[string]string{
		"non-comment line": "# @assert sum(\nGET https://example.com/\n",
		"blank line":       "# @assert sum(\n\nGET https://example.com/\n",
		"separator":        "# @assert sum(\n### next\nGET https://example.com/\n",
		"end of file":      "# @assert sum(\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			doc := Parse("continue.http", []byte(src))
			if len(doc.Errors) != 1 {
				t.Fatalf("errors = %+v, want 1", doc.Errors)
			}
			if doc.Errors[0].Line != 1 {
				t.Fatalf("error line = %d, want the line that opened it", doc.Errors[0].Line)
			}
			if !strings.Contains(doc.Errors[0].Message, `@assert is missing a closing ")"`) {
				t.Fatalf("error = %q", doc.Errors[0].Message)
			}
		})
	}
}

func TestParseContinuationNamesTheInnermostDelimiter(t *testing.T) {
	cases := map[string]string{
		")": "# @assert sum(\n",
		"]": "# @assert items[\n",
		"}": "# @assert count(items) == len({\n",
	}
	for closer, src := range cases {
		t.Run(closer, func(t *testing.T) {
			doc := Parse("continue.http", []byte(src))
			want := `@assert is missing a closing "` + closer + `"`
			if len(doc.Errors) != 1 || doc.Errors[0].Message != want {
				t.Fatalf("errors = %+v, want %q", doc.Errors, want)
			}
		})
	}
}

func TestParseContinuationInsideABlockComment(t *testing.T) {
	as := onlyAssert(t, `/*
 * @assert sum(
 *   1, 2
 * ) == 3
 */
GET https://example.com/
`)
	if as.Expression != "sum(\n 1, 2\n ) == 3" {
		t.Fatalf("expression = %q", as.Expression)
	}
	if _, err := rts.ParseExpr("continue.http", as.Line, as.Col, as.Expression); err != nil {
		t.Fatalf("parse expression: %v", err)
	}
}

func TestParseBlockCommentOptionContinuationKeepsZeroPadding(t *testing.T) {
	b := &documentBuilder{}
	if _, ok := b.readDirective(1, 0, `@match regex="first`); ok || b.open == nil {
		t.Fatal("expected the quoted option to remain open")
	}

	d, ok := b.readDirective(2, 0, `second"`)
	if !ok {
		t.Fatal("expected the quoted option to close")
	}
	if want := "regex=\"first\nsecond\""; d.Args != want {
		t.Fatalf("args = %q, want %q", d.Args, want)
	}
}

func TestParseWhenAndCaptureSpanCommentLines(t *testing.T) {
	req := parseOneRequest(t, `# @when contains(
#   ["dev", "stage"],
#   env
# )
# @capture request total sum(
#   response.json.a,
#   response.json.b
# )
GET https://example.com/
`)
	if req.Metadata.When == nil || !strings.Contains(req.Metadata.When.Expression, "\n") {
		t.Fatalf("when = %+v", req.Metadata.When)
	}
	if req.Metadata.When.Line != 1 {
		t.Fatalf("when line = %d, want 1", req.Metadata.When.Line)
	}
	if len(req.Metadata.Captures) != 1 {
		t.Fatalf("captures = %+v, want 1", req.Metadata.Captures)
	}
	cap := req.Metadata.Captures[0]
	if cap.Name != "total" || !strings.HasPrefix(cap.Expression, "sum(\n") {
		t.Fatalf("capture = %+v", cap)
	}
	if cap.Line != 5 || cap.Col != 26 {
		t.Fatalf("capture position = %d:%d, want 5:26", cap.Line, cap.Col)
	}
}

func TestParseForEachSpansCommentLines(t *testing.T) {
	req := parseOneRequest(t, `# @for-each response.json.items.filter(
#   fn(item) { item.active }
# ) as item
GET https://example.com/
`)
	each := req.Metadata.ForEach
	if each == nil || each.Var != "item" {
		t.Fatalf("for-each = %+v", each)
	}
	if !strings.HasPrefix(each.Expression, "response.json.items.filter(\n") {
		t.Fatalf("expression = %q", each.Expression)
	}
}

func TestParseWorkflowIfSpansCommentLines(t *testing.T) {
	doc := Parse("continue.http", []byte(`# @workflow checkout
# @step login using=Login
# @if contains(
#   ["dev", "stage"],
#   env
# ) run=Charge
`))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %+v", doc.Errors)
	}
	if len(doc.Workflows) != 1 {
		t.Fatalf("workflows = %d, want 1", len(doc.Workflows))
	}
	steps := doc.Workflows[0].Steps
	if len(steps) != 2 || steps[1].If == nil {
		t.Fatalf("steps = %+v", steps)
	}
	branch := steps[1].If.Then
	if !strings.Contains(branch.Cond, "\n") || branch.Run != "Charge" {
		t.Fatalf("branch = %+v", branch)
	}
}

func TestParseContinuationIgnoresBracketsOutsideTheExpression(t *testing.T) {
	t.Run("assert message", func(t *testing.T) {
		as := onlyAssert(t, "# @assert true => expected (\nGET https://example.com/\n")
		if as.Expression != "true" || as.Message != "expected (" {
			t.Fatalf("assert = %+v", as)
		}
	})

	t.Run("for-each variable", func(t *testing.T) {
		req := parseOneRequest(t, "# @for-each response.json.items as item\nGET https://example.com/\n")
		if req.Metadata.ForEach == nil || req.Metadata.ForEach.Var != "item" {
			t.Fatalf("for-each = %+v", req.Metadata.ForEach)
		}
	})

	t.Run("workflow run option", func(t *testing.T) {
		doc := Parse("continue.http", []byte(`# @workflow deploy
# @step build using=Build
# @if true run=Deploy(
`))
		if len(doc.Errors) != 0 {
			t.Fatalf("parse errors: %+v", doc.Errors)
		}
		steps := doc.Workflows[0].Steps
		if len(steps) != 2 || steps[1].If == nil {
			t.Fatalf("steps = %+v", steps)
		}
		if br := steps[1].If.Then; br.Cond != "true" || br.Run != "Deploy(" {
			t.Fatalf("branch = %+v", br)
		}
	})

	t.Run("apply profile name", func(t *testing.T) {
		doc := Parse("continue.http", []byte("# @apply use=jsonApi(\nGET https://example.com/\n"))
		if len(doc.Errors) != 1 || !strings.Contains(doc.Errors[0].Message, `use name "jsonApi(" is invalid`) {
			t.Fatalf("errors = %+v", doc.Errors)
		}
	})
}

func TestParseContinuationSeparatorsInCommentsAndStrings(t *testing.T) {
	t.Run("arrow in a comment", func(t *testing.T) {
		as := onlyAssert(t, "# @assert sum(1, 2) == 3 # why => not\nGET https://example.com/\n")
		if as.Expression != "sum(1, 2) == 3 # why => not" || as.Message != "" {
			t.Fatalf("assert = %+v", as)
		}
	})

	t.Run("arrow in a comment across lines", func(t *testing.T) {
		as := onlyAssert(t, "# @assert contains(\n#   list,   # find => value\n#   x\n# )\nGET https://example.com/\n")
		if !strings.HasPrefix(as.Expression, "contains(") || !strings.HasSuffix(as.Expression, ")") {
			t.Fatalf("expression = %q", as.Expression)
		}
		if as.Message != "" {
			t.Fatalf("message = %q, want none", as.Message)
		}
	})

	t.Run("arrow in a call argument", func(t *testing.T) {
		as := onlyAssert(t, `# @assert contains(body, "=>") => "found"`+"\nGET https://example.com/\n")
		if as.Expression != `contains(body, "=>")` || as.Message != "found" {
			t.Fatalf("assert = %+v", as)
		}
	})

	t.Run("comment inside a spanning capture", func(t *testing.T) {
		req := parseOneRequest(
			t,
			"# @capture request total sum(   # the interesting one\n#   response.json.a\n# )\nGET https://example.com/\n",
		)
		cap := req.Metadata.Captures[0]
		if !strings.HasSuffix(cap.Expression, ")") {
			t.Fatalf("expression = %q, want the whole call", cap.Expression)
		}
		if cap.Mode != restfile.CaptureExprModeRTS {
			t.Fatalf("mode = %v, want RTS", cap.Mode)
		}
	})

	t.Run("option in a string", func(t *testing.T) {
		doc := Parse("continue.http", []byte(`# @workflow deploy
# @step build using=Build
# @if contains("x run=not-an-option", value) run=Deploy
`))
		if len(doc.Errors) != 0 {
			t.Fatalf("parse errors: %+v", doc.Errors)
		}
		br := doc.Workflows[0].Steps[1].If.Then
		if br.Cond != `contains("x run=not-an-option", value)` || br.Run != "Deploy" {
			t.Fatalf("branch = %+v", br)
		}
	})

	t.Run("keyword in a string", func(t *testing.T) {
		req := parseOneRequest(t, `# @for-each tagged("a as b") as item`+"\nGET https://example.com/\n")
		each := req.Metadata.ForEach
		if each == nil || each.Expression != `tagged("a as b")` || each.Var != "item" {
			t.Fatalf("for-each = %+v", each)
		}
	})
}

func TestParseCaptureTemplates(t *testing.T) {
	t.Run("literal delimiters stay literal", func(t *testing.T) {
		for _, expr := range []string{
			"prefix[{{response.status}}",
			"prefix({{response.status}}",
			"prefix{{response.status}}}",
			"anchor#{{response.status}}",
		} {
			t.Run(expr, func(t *testing.T) {
				doc := Parse("continue.http", []byte("# @capture request label "+expr+"\nGET https://example.com/\n"))
				if len(doc.Errors) != 0 {
					t.Fatalf("errors = %+v, want none", doc.Errors)
				}
				cap := doc.Requests[0].Metadata.Captures[0]
				if cap.Expression != expr || cap.Mode != restfile.CaptureExprModeTemplate {
					t.Fatalf("capture = %+v, want the whole text as a template", cap)
				}
			})
		}
	})

	t.Run("balanced ends on its line", func(t *testing.T) {
		req := parseOneRequest(
			t,
			"# @capture request label {{response.status}}\n# @tag smoke\nGET https://example.com/\n",
		)
		cap := req.Metadata.Captures[0]
		if cap.Expression != "{{response.status}}" || cap.Mode != restfile.CaptureExprModeTemplate {
			t.Fatalf("capture = %+v", cap)
		}
		if len(req.Metadata.Tags) != 1 {
			t.Fatalf("tags = %+v, want the directive below it", req.Metadata.Tags)
		}
	})

	t.Run("open marker spans lines", func(t *testing.T) {
		req := parseOneRequest(t, "# @capture request label {{=\n#   response.status\n# }}\nGET https://example.com/\n")
		cap := req.Metadata.Captures[0]
		if !strings.HasPrefix(cap.Expression, "{{=") || !strings.HasSuffix(cap.Expression, "}}") {
			t.Fatalf("expression = %q", cap.Expression)
		}
		if cap.Mode != restfile.CaptureExprModeTemplate {
			t.Fatalf("mode = %v, want template", cap.Mode)
		}
	})

	t.Run("a literal delimiter spans with the marker", func(t *testing.T) {
		req := parseOneRequest(
			t,
			"# @capture request label prefix[{{\n#   response.status\n# }}\nGET https://example.com/\n",
		)
		cap := req.Metadata.Captures[0]
		if !strings.HasPrefix(cap.Expression, "prefix[{{") || !strings.HasSuffix(cap.Expression, "}}") {
			t.Fatalf("expression = %q, want the bracket carried along as text", cap.Expression)
		}
		if cap.Mode != restfile.CaptureExprModeTemplate {
			t.Fatalf("mode = %v, want template", cap.Mode)
		}
	})

	t.Run("an unclosed marker names itself", func(t *testing.T) {
		doc := Parse(
			"continue.http",
			[]byte("# @capture request label prefix {{response.status\nGET https://example.com/\n"),
		)
		if len(doc.Errors) != 1 || doc.Errors[0].Message != `@capture is missing a closing "}}"` {
			t.Fatalf("errors = %+v, want the marker named", doc.Errors)
		}
	})

	t.Run("a marker in a string is not a template", func(t *testing.T) {
		req := parseOneRequest(
			t,
			"# @capture request label contains(response.text(), \"{{token}}\")\nGET https://example.com/\n",
		)
		if cap := req.Metadata.Captures[0]; cap.Mode != restfile.CaptureExprModeRTS {
			t.Fatalf("mode = %v, want RTS", cap.Mode)
		}
	})
}

func TestParseCaptureHashIsTextUntilTheMarkersSayOtherwise(t *testing.T) {
	t.Run("hash before a marker is text", func(t *testing.T) {
		req := parseOneRequest(t, "# @capture request frag anchor#{{response.status}}\nGET https://example.com/\n")
		cap := req.Metadata.Captures[0]
		if cap.Expression != "anchor#{{response.status}}" || cap.Mode != restfile.CaptureExprModeTemplate {
			t.Fatalf("capture = %+v, want the whole text as a template", cap)
		}
	})

	t.Run("a call keeping a marker in its comment is warned about", func(t *testing.T) {
		doc := Parse(
			"continue.http",
			[]byte("# @capture request total sum(a) # use {{x}} here\nGET https://example.com/\n"),
		)
		cap := doc.Requests[0].Metadata.Captures[0]
		if cap.Mode != restfile.CaptureExprModeTemplate {
			t.Fatalf("mode = %v, want template", cap.Mode)
		}
		if len(doc.Warnings) != 1 || !strings.Contains(doc.Warnings[0].Message, "mixes template markers") {
			t.Fatalf("warnings = %+v, want the mixed syntax warning", doc.Warnings)
		}
	})

	t.Run("a quoted marker keeps the capture an expression", func(t *testing.T) {
		expr := `response.statusCode # mention "{{token}}"`
		doc := Parse("continue.http", []byte("# @capture request v "+expr+"\nGET https://example.com/\n"))
		if len(doc.Errors) != 0 || len(doc.Warnings) != 0 {
			t.Fatalf("errors = %+v, warnings = %+v, want none", doc.Errors, doc.Warnings)
		}
		cap := doc.Requests[0].Metadata.Captures[0]
		if cap.Expression != expr || cap.Mode != restfile.CaptureExprModeRTS {
			t.Fatalf("capture = %+v, want the whole line as RTS", cap)
		}
	})

	t.Run("a plainer expression is text without a warning", func(t *testing.T) {
		for _, expr := range []string{
			"response.status # see {{x}}",
			"a.b.c # see {{x}}",
			"1 + 2 # see {{x}}",
		} {
			t.Run(expr, func(t *testing.T) {
				doc := Parse("continue.http", []byte("# @capture request v "+expr+"\nGET https://example.com/\n"))
				if len(doc.Errors) != 0 || len(doc.Warnings) != 0 {
					t.Fatalf("errors = %+v, warnings = %+v, want none", doc.Errors, doc.Warnings)
				}
				cap := doc.Requests[0].Metadata.Captures[0]
				if cap.Expression != expr || cap.Mode != restfile.CaptureExprModeTemplate {
					t.Fatalf("capture = %+v, want the whole line as a template", cap)
				}
			})
		}
	})
}

func TestParseContinuationSpansBehindScopeAndName(t *testing.T) {
	doc := Parse("continue.http", []byte(`# @patch file jsonApi {
#   headers: {"Accept": "application/json"}
# }
# @capture request total sum(
#   response.json.a,
#   response.json.b
# )
# @assert sum(
#   1, 2
# ) == 3 => "adds up"
GET https://example.com/
`))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %+v", doc.Errors)
	}
	if len(doc.Patches) != 1 || !strings.Contains(doc.Patches[0].Expression, "\n") {
		t.Fatalf("patches = %+v", doc.Patches)
	}
	req := doc.Requests[0]
	if len(req.Metadata.Captures) != 1 || !strings.Contains(req.Metadata.Captures[0].Expression, "\n") {
		t.Fatalf("captures = %+v", req.Metadata.Captures)
	}
	if len(req.Metadata.Asserts) != 1 {
		t.Fatalf("asserts = %+v", req.Metadata.Asserts)
	}
	if as := req.Metadata.Asserts[0]; as.Message != "adds up" || !strings.Contains(as.Expression, "\n") {
		t.Fatalf("assert = %+v", as)
	}
}

func TestParseDirectiveWithoutContinuationEndsOnItsLine(t *testing.T) {
	req := parseOneRequest(t, `# @name checkout(
# @tag smoke
GET https://example.com/
`)
	if req.Metadata.Name != "checkout(" {
		t.Fatalf("name = %q", req.Metadata.Name)
	}
	if len(req.Metadata.Tags) != 1 || req.Metadata.Tags[0] != "smoke" {
		t.Fatalf("tags = %+v", req.Metadata.Tags)
	}
}

func TestParseSpanningDirectiveKeepsTheRequestRange(t *testing.T) {
	src := `# @assert sum(
#   1, 2
# ) == 3
GET https://example.com/
`
	doc := Parse("continue.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %+v", doc.Errors)
	}
	req := doc.Requests[0]
	if req.LineRange != (restfile.LineRange{Start: 1, End: 4}) {
		t.Fatalf("range = %+v, want 1..4", req.LineRange)
	}
	if req.OriginalText != strings.TrimRight(src, "\n") {
		t.Fatalf("original text = %q", req.OriginalText)
	}
}

func TestParseUnclosedDirectiveDoesNotStretchTheNextRequest(t *testing.T) {
	doc := Parse("continue.http", []byte(`# @assert sum(
GET https://example.com/
`))
	if len(doc.Errors) != 1 {
		t.Fatalf("errors = %+v, want 1", doc.Errors)
	}
	req := doc.Requests[0]
	if req.LineRange != (restfile.LineRange{Start: 2, End: 2}) {
		t.Fatalf("range = %+v, want 2..2", req.LineRange)
	}
}

func TestParseSpanningFileDirectiveLeavesTheNextRequestAlone(t *testing.T) {
	doc := Parse("continue.http", []byte(`# @patch file jsonApi {
#   headers: {"Accept": "application/json"}
# }
GET https://example.com/
`))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %+v", doc.Errors)
	}
	if len(doc.Patches) != 1 || doc.Patches[0].Name != "jsonApi" {
		t.Fatalf("patches = %+v", doc.Patches)
	}
	req := doc.Requests[0]
	if req.LineRange != (restfile.LineRange{Start: 4, End: 4}) {
		t.Fatalf("range = %+v, want 4..4", req.LineRange)
	}
	if req.OriginalText != "GET https://example.com/" {
		t.Fatalf("original text = %q", req.OriginalText)
	}
}

func TestParseSpanningDirectiveInsideAnOpenRequest(t *testing.T) {
	src := `GET https://example.com/
# @assert sum(
#   1, 2
# ) == 3
`
	doc := Parse("continue.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %+v", doc.Errors)
	}
	req := doc.Requests[0]
	if req.LineRange != (restfile.LineRange{Start: 1, End: 4}) {
		t.Fatalf("range = %+v, want 1..4", req.LineRange)
	}
	if req.OriginalText != strings.TrimRight(src, "\n") {
		t.Fatalf("original text = %q", req.OriginalText)
	}
}

func TestParseWorkflowRangeCoversSpanningDirectives(t *testing.T) {
	cases := map[string]string{
		"if": `# @workflow checkout
# @step login using=Login
# @if contains(
#   ["dev", "stage"],
#   env
# ) run=Charge
`,
		"elif": `# @workflow checkout
# @step login using=Login
# @if false run=Skip
# @elif contains(
#   ["dev", "stage"], env
# ) run=Charge
`,
		"case": `# @workflow checkout
# @step login using=Login
# @switch env
# @case contains(
#   ["dev", "stage"], env
# ) run=Charge
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			doc := Parse("continue.http", []byte(src))
			if len(doc.Errors) != 0 {
				t.Fatalf("parse errors: %+v", doc.Errors)
			}
			if len(doc.Workflows) != 1 {
				t.Fatalf("workflows = %d, want 1", len(doc.Workflows))
			}
			if rg := doc.Workflows[0].LineRange; rg != (restfile.LineRange{Start: 1, End: 6}) {
				t.Fatalf("range = %+v, want 1..6", rg)
			}
		})
	}
}

func TestParseWorkflowRangeCoversARejectedSpanningDirective(t *testing.T) {
	doc := Parse("continue.http", []byte(`# @workflow checkout
# @step login using=Login
# @if contains(
#   ["dev", "stage"], env
# )
`))
	if len(doc.Errors) != 1 || doc.Errors[0].Line != 3 {
		t.Fatalf("errors = %+v, want one on line 3", doc.Errors)
	}
	if len(doc.Workflows) != 1 {
		t.Fatalf("workflows = %d, want 1", len(doc.Workflows))
	}
	if rg := doc.Workflows[0].LineRange; rg != (restfile.LineRange{Start: 1, End: 5}) {
		t.Fatalf("range = %+v, want 1..5", rg)
	}
}

func TestParseSpanningRequestDirectiveLeavesTheWorkflowRange(t *testing.T) {
	doc := Parse("continue.http", []byte(`# @workflow checkout
# @step login using=Login
# @assert sum(
#   1, 2
# ) == 3
GET https://example.com/
`))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %+v", doc.Errors)
	}
	if len(doc.Workflows) != 1 || len(doc.Requests) != 1 {
		t.Fatalf("workflows = %d, requests = %d, want 1 each", len(doc.Workflows), len(doc.Requests))
	}
	if rg := doc.Workflows[0].LineRange; rg != (restfile.LineRange{Start: 1, End: 2}) {
		t.Fatalf("workflow range = %+v, want 1..2", rg)
	}
	if rg := doc.Requests[0].LineRange; rg != (restfile.LineRange{Start: 3, End: 6}) {
		t.Fatalf("request range = %+v, want 3..6", rg)
	}
}
