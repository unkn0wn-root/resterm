package restfile

import "testing"

func TestOriginFormats(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"auth path and line", (&AuthSpec{SourcePath: "a.http", Line: 3}).Origin(), "a.http:3"},
		{"auth path only", (&AuthSpec{SourcePath: "a.http"}).Origin(), "a.http"},
		{"auth line only", (&AuthSpec{Line: 3}).Origin(), "line 3"},
		{"auth empty", (&AuthSpec{}).Origin(), ""},
		{"auth nil", (*AuthSpec)(nil).Origin(), ""},
		{
			"request path and line",
			(&Request{SourcePath: "b.http", LineRange: LineRange{Start: 10}}).Origin(),
			"b.http:10",
		},
		{"request empty", (&Request{}).Origin(), ""},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Fatalf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}
