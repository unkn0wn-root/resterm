package bodyref

import "testing"

func TestParseBodyFileReference(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{in: "< ./payload.xml", want: "./payload.xml", ok: true},
		{in: "<payload.xml", want: "payload.xml", ok: true},
		{in: "<?xml version=\"1.0\"?>", ok: false},
		{in: "<soap:Envelope xmlns:soap=\"http://schemas.xmlsoap.org/soap/envelope/\">", ok: false},
		{in: "<get_the_infos_ssm/>", ok: false},
		{in: "</soap:Body>", ok: false},
		{in: "<", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := Parse(tt.in, Line, AllowNoSpace)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("Parse(%q)=(%q,%v), want (%q,%v)", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseExplicitBodyFileReferenceAllowsGreaterThan(t *testing.T) {
	got, ok := Parse("< ./payload>v1.xml", Line, AllowNoSpace)
	if !ok || got != "./payload>v1.xml" {
		t.Fatalf("Parse()=(%q,%v), want explicit body file", got, ok)
	}
}

func TestParseExplicitOnlyRejectsNoSpaceReference(t *testing.T) {
	if got, ok := Parse("<payload.xml", Line, ExplicitOnly); ok || got != "" {
		t.Fatalf("expected no-space reference to be rejected, got (%q,%v)", got, ok)
	}

	got, ok := Parse("< ./payload.xml", Line, ExplicitOnly)
	if !ok || got != "./payload.xml" {
		t.Fatalf("Parse()=(%q,%v), want explicit body file", got, ok)
	}
}

func TestParseEmbeddedBodyFileReference(t *testing.T) {
	got, ok := Parse("@body < ./payload.json", Inline, AllowNoSpace)
	if !ok || got != "./payload.json" {
		t.Fatalf("Parse()=(%q,%v), want body file", got, ok)
	}

	if got, ok := Parse("@body < ./payload>v1.json", Inline, AllowNoSpace); !ok ||
		got != "./payload>v1.json" {
		t.Fatalf("Parse()=(%q,%v), want explicit body file", got, ok)
	}

	if got, ok := Parse("@body <soap:Envelope>", Inline, AllowNoSpace); ok || got != "" {
		t.Fatalf("expected xml markup to remain body text, got (%q,%v)", got, ok)
	}
}
