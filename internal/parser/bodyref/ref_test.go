package bodyref

import "testing"

func TestParseBodyFileReference(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{in: "< ./payload.xml", want: "./payload.xml", ok: true},
		{in: "  < ./payload.xml", want: "./payload.xml", ok: true},
		{in: "<\t./payload.xml", want: "./payload.xml", ok: true},
		{in: "<\u00a0./payload.xml", want: "./payload.xml", ok: true},
		{in: "< ./payload>v1.xml", want: "./payload>v1.xml", ok: true},
		{in: "<?xml version=\"1.0\"?>", ok: false},
		{in: "<soap:Envelope xmlns:soap=\"http://schemas.xmlsoap.org/soap/envelope/\">", ok: false},
		{in: "<get_the_infos_ssm/>", ok: false},
		{in: "</soap:Body>", ok: false},
		{in: "<Invoice>", ok: false},
		{in: "<payload.xml", ok: false},
		{in: "<./payload.xml", ok: false},
		{in: "<", ok: false},
		{in: "< ", ok: false},
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
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{in: "@body < ./payload.json", want: "./payload.json", ok: true},
		{in: "@body\t<\t./payload.json", want: "./payload.json", ok: true},
		{in: "@body < ./payload>v1.json", want: "./payload>v1.json", ok: true},
		{in: "@body <soap:Envelope>", ok: false},
		{in: "@body <./payload.json", ok: false},
		{in: "@body< ./payload.json", ok: false},
		{in: "@body text < ./payload.json", ok: false},
		{in: "@{body} < ./payload.json", ok: false},
		{in: "@1body < ./payload.json", ok: false},
		{in: "@grpc-metadata < ./metadata.json", want: "./metadata.json", ok: true},
		{in: "@body <", ok: false},
		{in: "body < ./payload.json", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := Parse(tt.in, Options{Location: Inline})
			if ok != tt.ok || got != tt.want {
				t.Fatalf("Parse(%q)=(%q,%v), want (%q,%v)", tt.in, got, ok, tt.want, tt.ok)
			}
		})
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

func TestParseBodyFileChecksLineAndInlineForms(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		forceInline bool
		want        string
		ok          bool
	}{
		{name: "line", in: "< ./payload.json", want: "./payload.json", ok: true},
		{name: "inline", in: "@query < ./query.graphql", want: "./query.graphql", ok: true},
		{name: "template head", in: "@{payload} < ./payload.json"},
		{name: "compact", in: "<./payload.json"},
		{name: "force inline", in: "< ./payload.json", forceInline: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseBodyFile(tt.in, tt.forceInline)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("ParseBodyFile(%q)=(%q,%v), want (%q,%v)", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}
