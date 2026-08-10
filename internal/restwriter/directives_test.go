package restwriter

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// Every case uses a declaration the parser could actually have built.
func TestDirectiveArgs(t *testing.T) {
	line := func(name directive.Name, text string) string {
		if text == "" {
			return ""
		}
		return name.Tag() + " " + text
	}

	tests := map[string]struct {
		got  string
		want string
	}{
		"patch": {
			got: line(patchArg(restfile.PatchProfile{
				Scope: directive.ScopeFile, Name: "auth", Expression: `{ headers: {} }`,
			})),
			want: `@patch file auth { headers: {} }`,
		},
		"apply profiles": {
			got:  line(applyArg(restfile.ApplySpec{Uses: []string{"auth", "trace"}})),
			want: `@apply use=auth, use=trace`,
		},
		"apply expression": {
			got:  line(applyArg(restfile.ApplySpec{Expression: `{ query: {} }`})),
			want: `@apply { query: {} }`,
		},
		"when": {
			got:  line(conditionArg(restfile.ConditionSpec{Expression: `env == "dev"`})),
			want: `@when env == "dev"`,
		},
		"skip-if": {
			got:  line(conditionArg(restfile.ConditionSpec{Expression: "1 == 1", Negate: true})),
			want: `@skip-if 1 == 1`,
		},
		"for-each": {
			got:  line(forEachArg(restfile.ForEachSpec{Expression: "[1, 2]", Var: "n"})),
			want: `@for-each [1, 2] as n`,
		},
		"workflow step for-each": {
			got:  line(stepForEachArg(restfile.WorkflowForEach{Expr: "[1]", Var: "n"})),
			want: `@for-each [1] as n`,
		},
		"capture": {
			got: line(captureArg(restfile.CaptureSpec{
				Scope: directive.ScopeRequest, Name: "token", Expression: "response.json.token",
			})),
			want: `@capture request token response.json.token`,
		},
		"secret capture": {
			got: line(captureArg(restfile.CaptureSpec{
				Scope: directive.ScopeGlobal, Secret: true, Name: "t", Expression: "response.text()",
			})),
			want: `@capture global-secret t response.text()`,
		},
		"assert": {
			got:  line(assertArg(restfile.AssertSpec{Expression: "status == 200"})),
			want: `@assert status == 200`,
		},
		"assert with a message": {
			got:  line(assertArg(restfile.AssertSpec{Expression: "status == 200", Message: "healthy"})),
			want: `@assert status == 200 => healthy`,
		},
		"assert with an already quoted message": {
			got:  line(assertArg(restfile.AssertSpec{Expression: "status == 200", Message: `"quoted"`})),
			want: `@assert status == 200 => ""quoted""`,
		},
		"assert with a padded message": {
			got:  line(assertArg(restfile.AssertSpec{Expression: "status == 200", Message: " padded "})),
			want: `@assert status == 200 => " padded "`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("= %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestCommentLinesRestoresContinuationMarkers(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"single line":     {in: "sum(1, 2)", want: "sum(1, 2)"},
		"indent kept":     {in: "sum(\n   1,\n   2\n)", want: "sum(\n#  1,\n#  2\n#)"},
		"no indent":       {in: "sum(\n1\n)", want: "sum(\n#1\n#)"},
		"trailing suffix": {in: "[\n  1\n] as n", want: "[\n# 1\n#] as n"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := commentLines(tt.in); got != tt.want {
				t.Fatalf("commentLines(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestWriteOneSkipsAbsentDeclarations(t *testing.T) {
	var b strings.Builder
	w := directiveWriter{b: &b}
	writeOne(w, (*restfile.ConditionSpec)(nil), conditionArg)
	writeOne(w, (*restfile.ForEachSpec)(nil), forEachArg)
	writeEach(w, []restfile.CaptureSpec(nil), captureArg)

	if b.Len() != 0 {
		t.Fatalf("rendered %q, want nothing", b.String())
	}
}

func TestRenderRejectsPatchOutsideFileAndGlobalScope(t *testing.T) {
	scopes := map[string]directive.Scope{
		"request": directive.ScopeRequest,
		"unknown": directive.Scope(42),
	}
	for name, scope := range scopes {
		t.Run(name, func(t *testing.T) {
			doc := &restfile.Document{Patches: []restfile.PatchProfile{
				{Scope: scope, Name: "auth", Expression: "{}"},
			}}
			out, err := Render(doc, Options{})
			if err == nil {
				t.Fatalf("rendered %q, want an error", out)
			}
			if !strings.Contains(err.Error(), "file or global") {
				t.Fatalf("error = %v, want it to name the valid scopes", err)
			}
		})
	}
}

func TestRenderWritesPatchesInFileAndGlobalScope(t *testing.T) {
	doc := &restfile.Document{Patches: []restfile.PatchProfile{
		{Scope: directive.ScopeFile, Name: "auth", Expression: "{}"},
		{Scope: directive.ScopeGlobal, Name: "trace", Expression: "{}"},
	}}
	out, err := Render(doc, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := "# @patch file auth {}\n# @patch global trace {}\n"
	if !strings.Contains(out, want) {
		t.Fatalf("rendered %q, want it to contain %q", out, want)
	}
}
