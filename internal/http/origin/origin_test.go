package origin

import (
	"net/url"
	"testing"
)

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", raw, err)
	}
	return u
}

func TestOfNormalizes(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		same bool
	}{
		{name: "identical", a: "https://a.example.com/x", b: "https://a.example.com/y", same: true},
		{name: "default port", a: "https://a.example.com", b: "https://a.example.com:443", same: true},
		{name: "host case", a: "https://A.Example.COM", b: "https://a.example.com", same: true},
		{name: "scheme case", a: "HTTPS://a.example.com", b: "https://a.example.com", same: true},
		{name: "userinfo is not part of it", a: "https://u:p@a.example.com", b: "https://a.example.com", same: true},
		{name: "subdomain", a: "https://example.com", b: "https://a.example.com"},
		{name: "port", a: "https://a.example.com", b: "https://a.example.com:8443"},
		{name: "scheme", a: "https://a.example.com", b: "http://a.example.com"},
		{name: "http default port against https", a: "http://a.example.com:443", b: "https://a.example.com"},
		{
			name: "wss shares the https origin",
			a:    "wss://a.example.com/socket",
			b:    "https://a.example.com",
			same: true,
		},
		{name: "ws shares the http origin", a: "ws://a.example.com/socket", b: "http://a.example.com", same: true},
		{name: "ws is not the https origin", a: "ws://a.example.com", b: "https://a.example.com"},
		{
			name: "ipv6 address case",
			a:    "https://[FE80::1]",
			b:    "https://[fe80::1]",
			same: true,
		},
		// The operating system decides what a zone id names, so a differently
		// cased one is not known to be the same interface.
		{
			name: "ipv6 zone case",
			a:    "https://[fe80::1%25En0]",
			b:    "https://[fe80::1%25en0]",
		},
		{
			name: "matching ipv6 zone",
			a:    "https://[FE80::1%25En0]/x",
			b:    "https://[fe80::1%25En0]/y",
			same: true,
		},
		// A name has no zone, so the percent sign it decodes to is just part of
		// the name and folds case with the rest of it.
		{
			name: "percent in a registered name",
			a:    "https://EXAMPLE%25FOO.com",
			b:    "https://example%25foo.com",
			same: true,
		},
		{
			name: "international name case",
			a:    "https://M%C3%9CNCHEN.de",
			b:    "https://m%C3%BCnchen.de",
			same: true,
		},
		// Only an IPv6 address has a zone, so nothing here is worth preserving.
		{
			name: "percent after an ipv4 address",
			a:    "https://127.0.0.1%25FOO",
			b:    "https://127.0.0.1%25foo",
			same: true,
		},
		{
			name: "zone on an ipv4 mapped address",
			a:    "https://[::ffff:127.0.0.1%25En0]",
			b:    "https://[::ffff:127.0.0.1%25en0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Same(mustParse(t, tt.a), mustParse(t, tt.b)); got != tt.same {
				t.Fatalf("Same(%s, %s) = %t, want %t", tt.a, tt.b, got, tt.same)
			}
		})
	}
}

func TestSameNeedsAnOrigin(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
	}{
		{name: "relative", a: "/path", b: "/path"},
		{name: "no host", a: "https://", b: "https://"},
		{name: "unknown scheme", a: "file:///etc/passwd", b: "file:///etc/passwd"},
		{name: "one side relative", a: "https://a.example.com", b: "/path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if Same(mustParse(t, tt.a), mustParse(t, tt.b)) {
				t.Fatalf("Same(%s, %s) = true, want false", tt.a, tt.b)
			}
		})
	}
	if Same(nil, nil) {
		t.Fatal("Same(nil, nil) = true, want false")
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{raw: "https://cdn.example.com", want: "https://cdn.example.com"},
		{raw: "https://cdn.example.com/", want: "https://cdn.example.com"},
		{raw: "https://cdn.example.com:8443", want: "https://cdn.example.com:8443"},
		{raw: "https://cdn.example.com:443", want: "https://cdn.example.com"},
		{raw: "  HTTP://CDN.example.com  ", want: "http://cdn.example.com"},
		{raw: "wss://stream.example.com", want: "https://stream.example.com"},
		{raw: "ws://stream.example.com:9000", want: "http://stream.example.com:9000"},
		{raw: "https://cdn.example.com/assets", wantErr: true},
		{raw: "https://cdn.example.com?a=1", wantErr: true},
		{raw: "https://cdn.example.com#f", wantErr: true},
		{raw: "https://user:pass@cdn.example.com", wantErr: true},
		{raw: "cdn.example.com", wantErr: true},
		{raw: "file:///tmp", wantErr: true},
		{raw: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := Parse(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse(%q) error = %v, wantErr %t", tt.raw, err, tt.wantErr)
			}
			if err == nil && got.String() != tt.want {
				t.Fatalf("Parse(%q) = %q, want %q", tt.raw, got.String(), tt.want)
			}
		})
	}
}

func TestSet(t *testing.T) {
	cdn, err := Parse("https://cdn.example.com")
	if err != nil {
		t.Fatal(err)
	}
	other, err := Parse("https://other.example.com")
	if err != nil {
		t.Fatal(err)
	}

	if !(Set{}).Empty() || (Set{}).Allows(cdn) {
		t.Fatal("the zero Set allows an origin, want none")
	}
	if !Any().Allows(cdn) || Any().Empty() {
		t.Fatal("Any() does not allow every origin")
	}
	if Any().Allows(Origin{}) {
		t.Fatal("Any() allows a URL that has no origin")
	}

	set := NewSet(cdn, cdn, Origin{})
	if !set.Allows(cdn) || set.Allows(other) {
		t.Fatalf("NewSet holds the wrong origins: %s", set)
	}
	if set.String() != "https://cdn.example.com" {
		t.Fatalf("String() = %q", set.String())
	}
}

func TestParseSet(t *testing.T) {
	set, err := ParseSet("https://a.example.com, https://b.example.com;https://c.example.com")
	if err != nil {
		t.Fatalf("ParseSet: %v", err)
	}
	for _, raw := range []string{"https://a.example.com", "https://b.example.com", "https://c.example.com"} {
		o, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if !set.Allows(o) {
			t.Fatalf("%s is missing from %s", raw, set)
		}
	}
	if _, err := ParseSet("https://a.example.com nonsense"); err == nil {
		t.Fatal("ParseSet accepted a value that is not an origin")
	}
	if set, err := ParseSet("   "); err != nil || !set.Empty() {
		t.Fatalf("ParseSet(blank) = %v, %v, want an empty set", set, err)
	}
}

func TestSecure(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://a.example.com": true,
		"wss://a.example.com":   true,
		"http://a.example.com":  false,
		"ws://a.example.com":    false,
	} {
		o, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := o.Secure(); got != want {
			t.Fatalf("%s Secure() = %t, want %t", raw, got, want)
		}
	}
}

func TestStringRoundTripsThroughParse(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "ipv4", raw: "https://127.0.0.1:8443", want: "https://127.0.0.1:8443"},
		{name: "host", raw: "https://cdn.example.com", want: "https://cdn.example.com"},
		{name: "ipv6 on the default port", raw: "https://[::1]", want: "https://[::1]"},
		{name: "ipv6 with a port", raw: "https://[::1]:8443", want: "https://[::1]:8443"},
		{
			name: "ipv6 written out",
			raw:  "http://[2001:db8::1]:8080",
			want: "http://[2001:db8::1]:8080",
		},
		{
			name: "link local ipv6 with a zone",
			raw:  "https://[fe80::1%25en0]",
			want: "https://[fe80::1%25en0]",
		},
		{
			name: "link local ipv6 with a zone and a port",
			raw:  "https://[fe80::1%25en0]:8443",
			want: "https://[fe80::1%25en0]:8443",
		},
		{
			name: "a zone keeps its case",
			raw:  "https://[FE80::1%25En0]:8443",
			want: "https://[fe80::1%25En0]:8443",
		},
		{
			name: "international name",
			raw:  "https://M%C3%9CNCHEN.de",
			want: "https://m%C3%BCnchen.de",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := Parse(tt.raw)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.raw, err)
			}
			if got := o.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
			back, err := Parse(o.String())
			if err != nil {
				t.Fatalf("Parse(%q): %v", o.String(), err)
			}
			if back != o {
				t.Fatalf("Parse(%q) = %v, want the origin it came from", o.String(), back)
			}
		})
	}
}
