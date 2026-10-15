package jsonpath

import "testing"

func TestValid(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "user.id", want: true},
		{path: "items[0].id", want: true},
		{path: "items.[0].id", want: true},
		{path: `$["display.name"]`, want: true},
		{path: "user..id", want: false},
		{path: "items[invalid-index]", want: false},
		{path: "items[-1]", want: false},
		{path: "items[0", want: false},
		{path: "$user", want: false},
		{path: "user.", want: false},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			if got := Valid(test.path); got != test.want {
				t.Fatalf("Valid(%q) = %t, want %t", test.path, got, test.want)
			}
		})
	}
}

func TestKeyRoundTripsThroughGet(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{key: "id", want: "$.id"},
		{key: "_id2", want: "$._id2"},
		{key: "ID", want: "$.ID"},
		{key: "0", want: `$["0"]`},
		{key: "2fa", want: `$["2fa"]`},
		{key: "", want: `$[""]`},
		{key: "display.name", want: `$["display.name"]`},
		{key: "a b", want: `$["a b"]`},
		{key: " a ", want: `$[" a "]`},
		{key: "user-id", want: `$["user-id"]`},
		{key: "$ref", want: `$["$ref"]`},
		{key: "a[0]", want: `$["a[0]"]`},
		{key: "]", want: `$["]"]`},
		{key: `say "hi"`, want: `$["say \"hi\""]`},
		{key: "it's", want: `$["it's"]`},
		{key: `back\slash`, want: `$["back\\slash"]`},
		{key: "line\nbreak\r\tend", want: `$["line\nbreak\r\tend"]`},
		{key: "café", want: `$["café"]`},
		{key: "\x00\x1b", want: "$[\"\x00\x1b\"]"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			path := Key("$", tt.key)
			if path != tt.want {
				t.Fatalf("Key(%q) = %s, want %s", tt.key, path, tt.want)
			}
			if !Valid(path) {
				t.Fatalf("Valid(%s) = false", path)
			}
			doc := map[string]any{"x": []any{map[string]any{tt.key: "hit", tt.key + "!": "miss"}}}
			if got, ok := Get(doc, Key("$.x[0]", tt.key)); !ok || got != "hit" {
				t.Fatalf("Get(%s) = %#v, %t", Key("$.x[0]", tt.key), got, ok)
			}
		})
	}
}

func TestGet(t *testing.T) {
	doc := map[string]any{
		"items": []any{map[string]any{"display.name": "Ada"}},
	}
	got, ok := Get(doc, `$["items"][0]["display.name"]`)
	if !ok || got != "Ada" {
		t.Fatalf("Get() = %#v, %t", got, ok)
	}
}
