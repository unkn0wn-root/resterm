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

func TestGet(t *testing.T) {
	doc := map[string]any{
		"items": []any{map[string]any{"display.name": "Ada"}},
	}
	got, ok := Get(doc, `$["items"][0]["display.name"]`)
	if !ok || got != "Ada" {
		t.Fatalf("Get() = %#v, %t", got, ok)
	}
}
