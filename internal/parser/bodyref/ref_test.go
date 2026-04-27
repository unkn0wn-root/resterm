package bodyref

import "testing"

func TestParseBodyFileReference(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{in: "< ./payload.xml", want: "./payload.xml", ok: true},
		{in: "< ./payload>v1.xml", want: "./payload>v1.xml", ok: true},
		{in: "<?xml version=\"1.0\"?>", ok: false},
		{in: "<soap:Envelope xmlns:soap=\"http://schemas.xmlsoap.org/soap/envelope/\">", ok: false},
		{in: "<get_the_infos_ssm/>", ok: false},
		{in: "</soap:Body>", ok: false},
		{in: "<Invoice>", ok: false},
		{in: "<payload.xml", ok: false},
		{in: "<./payload.xml", ok: false},
		{in: "<", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := Parse(tt.in, Options{Location: Line})
			if ok != tt.ok || got != tt.want {
				t.Fatalf("Parse(%q)=(%q,%v), want (%q,%v)", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseEmbeddedBodyFileReference(t *testing.T) {
	got, ok := Parse("@body < ./payload.json", Options{
		Location: Inline,
	})
	if !ok || got != "./payload.json" {
		t.Fatalf("Parse()=(%q,%v), want body file", got, ok)
	}

	if got, ok := Parse("@body < ./payload>v1.json", Options{
		Location: Inline,
	}); !ok ||
		got != "./payload>v1.json" {
		t.Fatalf("Parse()=(%q,%v), want explicit body file", got, ok)
	}

	if got, ok := Parse("@body <soap:Envelope>", Options{
		Location: Inline,
	}); ok || got != "" {
		t.Fatalf("expected xml markup to remain body text, got (%q,%v)", got, ok)
	}

	if got, ok := Parse("@body <./payload.json", Options{Location: Inline}); ok || got != "" {
		t.Fatalf("expected compact reference to remain body text, got (%q,%v)", got, ok)
	}
}

func TestParseForceInlineRejectsBodyReferences(t *testing.T) {
	if got, ok := Parse("< ./payload.xml", Options{
		Location:    Line,
		ForceInline: true,
	}); ok || got != "" {
		t.Fatalf("expected forced inline body text, got (%q,%v)", got, ok)
	}
}
