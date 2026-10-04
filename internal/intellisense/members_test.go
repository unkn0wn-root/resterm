package intellisense

import (
	"slices"
	"testing"
)

func suggestAtEnd(lines ...string) (Context, []Item) {
	last := len(lines) - 1
	ctx, ok := Analyze(testLines(lines), last, len([]rune(lines[last])))
	if !ok {
		return Context{}, nil
	}
	return ctx, New().Suggest(ctx, Scope{})
}

func TestMemberCompletion(t *testing.T) {
	js := []string{"get", "has", "set", "interpolate", "global"}
	rts := []string{"get", "has", "require", "set", "interpolate", "global"}
	readOnly := []string{"get", "has", "require", "interpolate", "global"}

	tests := []struct {
		name  string
		lines []string
		want  []string
	}{
		{"javascript", []string{"> vars."}, js},
		{"javascript test", []string{"# @script test", "> vars."}, js},
		{"rts", []string{"# @rts pre-request", "> vars."}, rts},
		{"rts option", []string{"# @script pre-request lang=rts", "> vars."}, rts},
		{"javascript global", []string{"> x = vars.global."}, []string{"get", "has", "set", "delete"}},
		{
			"rts global",
			[]string{"# @rts pre-request", "> vars.global."},
			[]string{"get", "has", "require", "set", "delete"},
		},
		{"prefix", []string{"> vars.g"}, []string{"get", "global"}},
		{"block line", []string{"# @rts pre-request", "> {%", "  vars.r"}, []string{"require"}},
		{"argument", []string{"> request.setURL(vars.interpolate(vars.h"}, []string{"has"}},
		{"template literal", []string{"> x = `${vars.h"}, []string{"has"}},
		{"expression", []string{"GET https://example.test/{{= vars."}, readOnly},
		{"expression in a script string", []string{`> vars.interpolate("{{= vars.`}, nil},
		{"assert", []string{"# @assert vars."}, readOnly},
		{"assert global", []string{"# @assert vars.global."}, []string{"get", "has", "require"}},
		{"skip-if", []string{"# @skip-if !vars.h"}, []string{"has"}},
		{"apply continuation", []string{"# @apply {", "#   url: vars."}, readOnly},
		{"javascript string", []string{`> x = "vars.`}, nil},
		{"javascript comment", []string{"> x // vars."}, nil},
		{"javascript comment without space", []string{"> vars.//x"}, nil},
		{"rts string", []string{"# @rts pre-request", "> x = 'vars."}, nil},
		{"rts comment", []string{"# @rts pre-request", "> x # vars."}, nil},
		{"call result", []string{"> f().vars."}, nil},
		{"longer path", []string{"> x.vars."}, nil},
		{"unknown object", []string{"> request."}, nil},
		{"no member yet", []string{"> vars"}, nil},
		{"rejected rts", []string{"# @rts test", "> vars."}, nil},
		{"directive without script", []string{"# @name vars."}, nil},
		{"template text", []string{"> x = `vars."}, nil},
		{"template text after a substitution", []string{"> x = `${a} vars."}, nil},
		{"apostrophe in template text", []string{"> x = `it's ${vars."}, js},
		{"nested template", []string{"> x = `${`${vars."}, js},
		{"object in a substitution", []string{"> x = `${ {a: 1}.a + vars."}, js},
		{"closed template", []string{"> x = `${a}` + vars."}, js},
		{"block comment", []string{"> /* vars."}, nil},
		{"closed block comment", []string{"> /* a */ vars."}, js},
		{"regex with slashes", []string{`> x = s.replace(/^https?:\/\//, "") + vars.`}, js},
		{"regex with a quote", []string{"> x = /'/.test(s) && vars."}, js},
		{"regex with a class", []string{`> x = /[/"]/.test(vars.`}, js},
		{"regex after a keyword", []string{`> return /"/.test(vars.`}, js},
		{"division", []string{"> x = a / b; y = vars."}, js},
		{"regex with a star above", []string{`> x = s.replace(/\/*$/, "")`, "> vars."}, js},
		{"block comment above", []string{"> /*", "> vars."}, nil},
		{"closed block comment above", []string{"> /* a", "> */ vars."}, js},
		{"template text above", []string{"> x = `a", "> vars."}, nil},
		{"substitution above", []string{"> x = `a ${", "> vars."}, js},
		{"template text in a block", []string{"> {%", "x = `a", "vars."}, nil},
		{"comment above another script", []string{"> /*", "# note", "> vars."}, js},
		{"multiline expression", []string{"POST https://example.test", "", "{{=", "  vars."}, readOnly},
		{
			"multiline expression in json",
			[]string{"POST https://example.test", "", `{"a": "{{= 1 +`, "  vars."},
			readOnly,
		},
		{
			"multiline expression across a comment",
			[]string{"POST https://example.test", "", "{{=", "# note", "", "  vars."},
			readOnly,
		},
		{"closed multiline expression", []string{"POST https://example.test", "", "{{= a", "}} vars."}, nil},
		{"multiline variable", []string{"POST https://example.test", "", "{{ name", "  vars."}, nil},
		{"body text", []string{"POST https://example.test", "", `{"a": 1,`, "  vars."}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, items := suggestAtEnd(tt.lines...)
			var got []string
			if ctx.Kind == KindMember {
				got = labels(items)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("members = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMemberCompletionKeepsOtherContexts(t *testing.T) {
	if ctx, _ := suggestAtEnd("GET https://example.test/{{vars.wo"); ctx.Kind != KindVariable {
		t.Fatalf("kind = %d, want variable completion for a template name", ctx.Kind)
	}

	ctx, items := suggestAtEnd(`# @if vars.has("x") r`)
	if ctx.Kind != KindDirectiveArg || !slices.Contains(labels(items), "run=") {
		t.Fatalf("kind = %d, items = %q, want @if options", ctx.Kind, labels(items))
	}
}

func TestMemberInsertText(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		col         int
		label       string
		insert      string
		placeholder string
		next        bool
	}{
		{name: "method", line: "> vars.ge", label: "get", insert: "get(name)", placeholder: "name"},
		{
			name:        "two arguments",
			line:        "> vars.se",
			label:       "set",
			insert:      "set(name, value)",
			placeholder: "name, value",
		},
		{name: "object", line: "> vars.glo", label: "global", insert: "global.", next: true},
		{name: "arguments follow", line: `> vars.ge("x")`, col: 9, label: "get", insert: "get"},
		{name: "member follows", line: "> vars.glo.get(1)", col: 10, label: "global", insert: "global"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := tt.col
			if col == 0 {
				col = len([]rune(tt.line))
			}
			ctx, ok := Analyze(testLines{tt.line}, 0, col)
			if !ok || ctx.Kind != KindMember {
				t.Fatalf("Analyze = %+v, %t, want a member context", ctx, ok)
			}
			if ctx.Start != 7 {
				t.Fatalf("start = %d, want the member name after vars.", ctx.Start)
			}
			items := New().Suggest(ctx, Scope{})
			i := slices.IndexFunc(items, func(it Item) bool { return it.Label == tt.label })
			if i < 0 {
				t.Fatalf("items = %q, want %s", labels(items), tt.label)
			}
			it := items[i]
			if it.InsertText() != tt.insert || it.Placeholder != tt.placeholder || it.Continue != tt.next {
				t.Fatalf(
					"%s inserts %q, placeholder %q, continue %t",
					tt.label,
					it.InsertText(),
					it.Placeholder,
					it.Continue,
				)
			}
			if it.AppendsSpace(ctx.Kind) {
				t.Fatalf("%s appends a space", tt.label)
			}
		})
	}
}
