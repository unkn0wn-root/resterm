package target

import "testing"

func TestParseRef(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		kind Kind
		ref  string
	}{
		{name: "podByName", raw: "api", kind: Pod, ref: "api"},
		{name: "colon", raw: "svc:api", kind: Service, ref: "api"},
		{name: "slash", raw: "deploy/api", kind: Deployment, ref: "api"},
		{name: "trim", raw: " sts / api-0 ", kind: StatefulSet, ref: "api-0"},
		{name: "alias", raw: "svc/web", kind: Service, ref: "web"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, ref, err := ParseRef(tc.raw)
			if err != nil {
				t.Fatalf("ParseRef(%q) err: %v", tc.raw, err)
			}
			if kind != tc.kind || ref != tc.ref {
				t.Fatalf("ParseRef(%q)=(%q,%q), want (%q,%q)", tc.raw, kind, ref, tc.kind, tc.ref)
			}
		})
	}
}

func TestParseRefRejectsInvalid(t *testing.T) {
	cases := []string{
		"",
		"job/api",
		"svc:",
		"deploy/",
	}

	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			if _, _, err := ParseRef(raw); err == nil {
				t.Fatalf("ParseRef(%q) expected error", raw)
			}
		})
	}
}

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
