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
