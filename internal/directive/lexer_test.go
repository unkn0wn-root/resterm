package directive

import (
	"reflect"
	"testing"
)

func TestFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "quoted value",
			input: `name="hello resterm" enabled=true`,
			want:  []string{"name=hello resterm", "enabled=true"},
		},
		{
			name:  "balanced json",
			input: `body={"nested":{"value":"hello world"}} next=true`,
			want:  []string{`body={"nested":{"value":"hello world"}}`, "next=true"},
		},
		{
			name:  "bracket inside a json string is not nesting",
			input: `body={"v":"}"} next=true`,
			want:  []string{`body={"v":"}"}`, "next=true"},
		},
		{
			name:  "escaped quote inside a json string",
			input: `body={"v":"a\"}"} next=true`,
			want:  []string{`body={"v":"a\"}"}`, "next=true"},
		},
		{
			name:  "backslash is literal without escapes",
			input: `kubeconfig=C:\Users\me`,
			want:  []string{`kubeconfig=C:\Users\me`},
		},
		{
			name:  "quote only groups at the start of a value",
			input: `a=x"y" b`,
			want:  []string{`a=x"y"`, "b"},
		},
		{
			name:  "single quotes group too",
			input: `a='x y' b`,
			want:  []string{"a=x y", "b"},
		},
		{
			name:  "runs of whitespace collapse",
			input: "  a \t b  ",
			want:  []string{"a", "b"},
		},
		{
			name:  "argument list keeps its spaces",
			input: "latency=random(100ms, 500ms) name=slow",
			want:  []string{"latency=random(100ms, 500ms)", "name=slow"},
		},
		{
			name:  "nested argument list",
			input: "a=f(g(1, 2), 3) b",
			want:  []string{"a=f(g(1, 2), 3)", "b"},
		},
		{
			name:  "parenthesis in a plain value is ordinary text",
			input: "path=/x(y z=1",
			want:  []string{"path=/x(y", "z=1"},
		},
		{
			name:  "parenthesis without a name is ordinary text",
			input: "a=(x y)",
			want:  []string{"a=(x", "y)"},
		},
		{
			name:  "unclosed argument list runs to the end",
			input: "latency=random(100ms name=slow",
			want:  []string{"latency=random(100ms name=slow"},
		},
		{name: "empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Fields(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Fields(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFieldsEscaped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "escaped space joins the token",
			input: `path=hello\ resterm next=true`,
			want:  []string{"path=hello resterm", "next=true"},
		},
		{
			name:  "escaped quote is literal",
			input: `a=\"x\" b`,
			want:  []string{`a="x"`, "b"},
		},
		{
			// The unquoted form still needs doubling, which is why quoting is
			// the documented way to write a path.
			name:  "unquoted backslash escapes the next rune",
			input: `p=C:\Users`,
			want:  []string{"p=C:Users"},
		},
		{
			name:  "doubled backslash survives unquoted",
			input: `p=C:\\Users`,
			want:  []string{`p=C:\Users`},
		},
		{
			name:  "quoted backslashes are kept",
			input: `p="C:\Users\me" next=1`,
			want:  []string{`p=C:\Users\me`, "next=1"},
		},
		{
			name:  "quoted quote is escapable",
			input: `d="say \"hi\"" next=1`,
			want:  []string{`d=say "hi"`, "next=1"},
		},
		{
			name:  "quoted double backslash collapses",
			input: `p="a\\b"`,
			want:  []string{`p=a\b`},
		},
		{
			name:  "trailing backslash stays",
			input: `p=x\`,
			want:  []string{`p=x\`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := fieldsEscaped(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("fieldsEscaped(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFieldOption(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{name: "bare", key: "header", value: "X-Token", want: "header=X-Token"},
		{name: "template", key: "token", value: "{{token}}", want: "token={{token}}"},
		{name: "spaces", key: "scope", value: "read write", want: "scope='read write'"},
		{name: "spaces and a single quote", key: "cmd", value: "mycli --name 'a b'", want: `cmd="mycli --name 'a b'"`},
		{name: "spaced json", key: "argv", value: `["tool","a b"]`, want: `argv='["tool","a b"]'`},
		{name: "json with both quote kinds", key: "argv", value: `["tool","it's b"]`, want: `argv=["tool","it's b"]`},
		{name: "leading quote", key: "scheme", value: `"quoted"`, want: `scheme='"quoted"'`},
		{name: "open call", key: "client_secret", value: "Pa(ss", want: "client_secret='Pa(ss'"},
		{name: "open bracket", key: "password", value: "[abc", want: "password='[abc'"},
		{name: "spaces and both quote kinds", key: "scope", value: `say "hi" it's`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := FieldOption(tt.key, tt.value)
			if got != tt.want || ok != (tt.want != "") {
				t.Fatalf("FieldOption(%q, %q) = %q, %v, want %q", tt.key, tt.value, got, ok, tt.want)
			}
			if !ok {
				return
			}
			if fields := Fields(got); len(fields) != 1 || fields[0] != tt.key+"="+tt.value {
				t.Fatalf("Fields(%q) = %q, want the value back", got, fields)
			}
		})
	}
}
