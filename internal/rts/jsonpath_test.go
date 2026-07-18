package rts

import "testing"

func TestValidJSONPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "user.id", want: true},
		{path: "items[0].id", want: true},
		{path: "items.[0].id", want: true}, // JSONPathGet resolves this form too
		{path: `$["display.name"]`, want: true},
		{path: "user..id", want: false},
		{path: "items[nope]", want: false},
		{path: "items[-1]", want: false},
		{path: "items[0", want: false},
		{path: "$user", want: false},
		{path: "user.", want: false},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			if got := ValidJSONPath(test.path); got != test.want {
				t.Fatalf("ValidJSONPath(%q) = %t, want %t", test.path, got, test.want)
			}
		})
	}
}
