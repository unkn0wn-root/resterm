package jsonpath

import (
	"slices"
	"strconv"
	"testing"
)

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

func TestWalk(t *testing.T) {
	key := func(p, k string) (string, bool) {
		return p + " " + strconv.Quote(k), true
	}
	index := func(p string, i int) (string, bool) {
		return p + " " + strconv.Itoa(i), true
	}
	tests := []struct {
		path string
		want string
	}{
		{path: "", want: "$"},
		{path: "  ", want: "$"},
		{path: " $ ", want: "$"},
		{path: "$.", want: "$"},
		{path: "a", want: `$ "a"`},
		{path: "$.a.b", want: `$ "a" "b"`},
		{path: "$a", want: `$ "a"`},
		{path: "a..b", want: `$ "a" "b"`},
		{path: ".a.", want: `$ "a"`},
		{path: "a b.c", want: `$ "a b" "c"`},
		{path: "$[0]", want: "$ 0"},
		{path: "a[ 1 ].b", want: `$ "a" 1 "b"`},
		{path: "a.[0][2]", want: `$ "a" 0 2`},
		{path: "a[-1]", want: `$ "a" -1`},
		{path: "a[x].b", want: `$ "a" "b"`},
		{path: "a[].b", want: `$ "a" "b"`},
		{path: `["0"]`, want: `$ "0"`},
		{path: `$["display.name"]`, want: `$ "display.name"`},
		{path: `['a b'].c`, want: `$ "a b" "c"`},
		{path: `["say \"hi\""]`, want: `$ "say \"hi\""`},
		{path: `['it\'s']`, want: `$ "it's"`},
		{path: `["a\\b\n\r\t\q"]`, want: `$ "a\\b\n\r\tq"`},
		{path: `["a" ]`, want: `$ "a"`},
		{path: `[ "a"]`, want: "$"},
		{path: "a[0", want: `$ "a"`},
		{path: "a[", want: `$ "a"`},
		{path: `a["b`, want: `$ "a"`},
		{path: `a["b\`, want: `$ "a"`},
		{path: `a["b"c].d`, want: `$ "a"`},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, ok := Walk("$", tt.path, key, index)
			if !ok || got != tt.want {
				t.Fatalf("Walk(%q) = %q, %t, want %q", tt.path, got, ok, tt.want)
			}
		})
	}

	var seen []string
	miss := func(p, k string) (string, bool) {
		seen = append(seen, k)
		return p + "." + k, k != "miss"
	}
	got, ok := Walk("$", "a.miss.b[0]", miss, index)
	if ok || got != "" || !slices.Equal(seen, []string{"a", "miss"}) {
		t.Fatalf("Walk = %q, %t after reading %q, want a miss after a and miss", got, ok, seen)
	}
}

func TestGetMisses(t *testing.T) {
	doc := map[string]any{"a": []any{nil, map[string]any{"b": 1.0}}, "n": nil}
	tests := []struct {
		path string
		want any
		ok   bool
	}{
		{path: "a[1].b", want: 1.0, ok: true},
		{path: "n", ok: true},
		{path: "a[0]", ok: true},
		{path: "a[2]", ok: false},
		{path: "a[-1]", ok: false},
		{path: "a.b", ok: false},
		{path: "[0]", ok: false},
		{path: "n.b", ok: false},
		{path: "missing", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got, ok := Get(doc, tt.path); ok != tt.ok || got != tt.want {
				t.Fatalf("Get(%q) = %#v, %t, want %#v, %t", tt.path, got, ok, tt.want, tt.ok)
			}
		})
	}
}
