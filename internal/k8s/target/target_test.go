package target

import "testing"

func TestIsValidPortName(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		ok   bool
	}{
		{name: "simple", raw: "http", ok: true},
		{name: "identifier", raw: "api-port_1", ok: true},
		{name: "template", raw: "{{port_name}}", ok: true},
		{name: "templateWithPrefix", raw: "svc-{{port_name}}", ok: true},
		{name: "partialTemplateOpen", raw: "{{port_name", ok: false},
		{name: "partialTemplateClose", raw: "port_name}}", ok: false},
		{name: "badChars", raw: "!!!", ok: false},
		{name: "empty", raw: " ", ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidPortName(tc.raw); got != tc.ok {
				t.Fatalf("IsValidPortName(%q)=%v want %v", tc.raw, got, tc.ok)
			}
		})
	}
}
