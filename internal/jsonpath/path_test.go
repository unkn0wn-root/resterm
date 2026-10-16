package jsonpath

import (
	"errors"
	"reflect"
	"testing"
)

func TestGet(t *testing.T) {
	doc := map[string]any{
		"a":        map[string]any{"b": []any{map[string]any{"c": 1.0}}},
		"items":    []any{10.0, 20.0, 30.0},
		"a.b":      "dot",
		"a b":      "space",
		`say "hi"`: "quote",
		"it's":     "apostrophe",
		"line\n":   "newline",
		"0":        "zero",
		"":         "empty",
		"$":        "dollar",
		"ключ":     "unicode",
		"n":        nil,
	}
	tests := []struct {
		path   string
		want   any
		at     string
		reason string
		err    string
	}{
		{path: "", want: doc},
		{path: "  ", want: doc},
		{path: "$", want: doc},
		{path: " $ ", want: doc},
		{path: "a.b[0].c", want: 1.0},
		{path: "$.a.b[0].c", want: 1.0},
		{path: " a.b[0].c ", want: 1.0},
		{path: `$["a"]['b'][0]["c"]`, want: 1.0},
		{path: "items[0]", want: 10.0},
		{path: "items[ 1 ]", want: 20.0},
		{path: "items[+1]", want: 20.0},
		{path: "items[01]", want: 20.0},
		{path: "items[-1]", want: 30.0},
		{path: "items[-3]", want: 10.0},
		{path: "items.[0]", want: 10.0},
		{path: "$.[0]", at: "$[0]", reason: "cannot index an object"},
		{path: "$[0]", at: "$[0]", reason: "cannot index an object"},
		{path: "[0]", at: "$[0]", reason: "cannot index an object"},
		{path: `["a.b"]`, want: "dot"},
		{path: `['a b']`, want: "space"},
		{path: `[ "a b" ]`, want: "space"},
		{path: `["say \"hi\""]`, want: "quote"},
		{path: `['it\'s']`, want: "apostrophe"},
		{path: `["it's"]`, want: "apostrophe"},
		{path: `["line\n"]`, want: "newline"},
		{path: `["a\.b"]`, want: "dot"},
		{path: "0", want: "zero"},
		{path: `["0"]`, want: "zero"},
		{path: `[""]`, want: "empty"},
		{path: "$.$", want: "dollar"},
		{path: `["$"]`, want: "dollar"},
		{path: "ключ", want: "unicode"},
		{path: "n", want: nil},

		{path: "missing", at: "$.missing", reason: "key not found"},
		{path: "A", at: "$.A", reason: "key not found"},
		{path: "a.x.y", at: "$.a.x", reason: "key not found"},
		{path: `["a b"].c`, at: `$["a b"].c`, reason: "cannot traverse non-container value"},
		{path: "items[3]", at: "$.items[3]", reason: "index 3 out of range"},
		{path: "items[-4]", at: "$.items[-4]", reason: "index -4 out of range"},
		{path: "items.x", at: "$.items.x", reason: "cannot access object key on array"},
		{path: "a[0]", at: "$.a[0]", reason: "cannot index an object"},
		{path: "n.x", at: "$.n.x", reason: "cannot traverse non-container value"},
		{path: "items[0][0]", at: "$.items[0][0]", reason: "cannot traverse non-container value"},

		{path: "$.", err: "empty path segment"},
		{path: ".a", err: "empty path segment"},
		{path: "a..b", err: "empty path segment"},
		{path: "a.", err: "empty path segment"},
		{path: "items[*]", err: `invalid array index "*"`},
		{path: "items[abc]", err: `invalid array index "abc"`},
		{path: "items[1e0]", err: `invalid array index "1e0"`},
		{path: "items[]", err: "empty array index"},
		{path: "items[ ]", err: "empty array index"},
		{path: "items[0", err: "missing closing bracket"},
		{path: "items[", err: "missing closing bracket"},
		{path: `["a"`, err: "missing closing bracket"},
		{path: `["a"b]`, err: "missing closing bracket"},
		{path: `["a`, err: "missing closing quote"},
		{path: `["a\`, err: "missing closing quote"},
		{path: "$a", err: `expected '.' or '[', got 'a'`},
		{path: "$$", err: `expected '.' or '[', got '$'`},
		{path: "a]b", err: `expected '.' or '[', got ']'`},
		{path: "items[0]x", err: `expected '.' or '[', got 'x'`},
		{path: "a b", err: `unquoted key "a b" has a space. Use ["a b"]`},
		{path: "a. b", err: `unquoted key " b" has a space. Use [" b"]`},
		{path: "a\x00", err: `unquoted key "a\x00" has a control character. Use a quoted key`},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := Get(doc, tt.path)
			var miss *Miss
			switch {
			case tt.err != "":
				if err == nil || errors.As(err, &miss) || err.Error() != tt.err {
					t.Fatalf("Get(%q) error = %v, want syntax error %q", tt.path, err, tt.err)
				}
			case tt.reason != "":
				if !errors.As(err, &miss) || miss.At.Format("$") != tt.at || miss.Reason != tt.reason {
					t.Fatalf("Get(%q) error = %v, want a miss at %s: %s", tt.path, err, tt.at, tt.reason)
				}
			case err != nil || !reflect.DeepEqual(got, tt.want):
				t.Fatalf("Get(%q) = %#v, %v, want %#v", tt.path, got, err, tt.want)
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
			p, err := Parse(path)
			if err != nil || p.Format("$") != path {
				t.Fatalf("Parse(%s) = %s, %v", path, p.Format("$"), err)
			}
			doc := map[string]any{"x": []any{map[string]any{tt.key: "hit", tt.key + "!": "miss"}}}
			if got, err := Get(doc, Key("$.x[0]", tt.key)); err != nil || got != "hit" {
				t.Fatalf("Get(%s) = %#v, %v", Key("$.x[0]", tt.key), got, err)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	p, err := Parse(`items[-1]["a b"].id`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := p.Format("json"), `json.items[-1]["a b"].id`; got != want {
		t.Fatalf("Format = %s, want %s", got, want)
	}
	if got := Path(nil).Format("json"); got != "json" {
		t.Fatalf("root Format = %s, want json", got)
	}
}
