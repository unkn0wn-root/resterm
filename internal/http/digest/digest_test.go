package digest

import (
	"strings"
	"testing"
)

const (
	rfc2617Challenge = `Digest realm="testrealm@host.com", qop="auth,auth-int", ` +
		`nonce="dcd98b7102dd2f0e8b11d0f600bfb0c093", opaque="5ccc069c403ebaf9f0171e9517f40e41"`
	rfc2617Cnonce = "0a4f113b"
)

var rfc2617User = Credentials{Username: "Mufasa", Password: "Circle Of Life"}

func mustPick(t *testing.T, values ...string) Challenge {
	t.Helper()
	c, ok := Pick(values)
	if !ok {
		t.Fatalf("Pick(%q) found no challenge", values)
	}
	return c
}

func param(t *testing.T, header, name string) string {
	t.Helper()
	ch := parseChallenges(header)
	if len(ch) != 1 {
		t.Fatalf("%q is not one challenge", header)
	}
	v, ok := ch[0].params[name]
	if !ok {
		t.Fatalf("%q has no %s", header, name)
	}
	return v
}

// SHA-256 example from RFC 7616 section 3.9.1.
func TestAuthorizationRFC7616SHA256(t *testing.T) {
	sha := `Digest realm="http-auth@example.org", qop="auth, auth-int", algorithm=SHA-256, ` +
		`nonce="7ypf/xlj9XXwfDPEoM4URrv/xwf94BcCAzFZH4GiTo0v", ` +
		`opaque="FQhe/qaU925kfnzjCev0ciny7QMkPqMAFRtzCUYo5tdS"`
	md5 := strings.Replace(sha, "SHA-256", "MD5", 1)
	cr := Credentials{Username: "Mufasa", Password: "Circle of Life"}
	cnonce := "f2/wE4q74E6zIJEtWaHKaf5wv/H5QzzpXusqGemxURZJ"

	got := mustPick(t, sha, md5).authorization(cr, "GET", "/dir/index.html", nil, cnonce)
	want := "753927fa0e85d155564e2e272a28d1802ca10daf4496794697cf8db5856cb6c1"
	if r := param(t, got, "response"); r != want {
		t.Fatalf("response = %s, want %s\n%s", r, want, got)
	}
}

func TestAuthorizationHeader(t *testing.T) {
	got := mustPick(t, rfc2617Challenge).authorization(rfc2617User, "GET", "/dir/index.html", nil, rfc2617Cnonce)
	want := `Digest username="Mufasa", realm="testrealm@host.com", ` +
		`nonce="dcd98b7102dd2f0e8b11d0f600bfb0c093", uri="/dir/index.html", ` +
		`response="6629fae49393a05397450978507c4ef1", opaque="5ccc069c403ebaf9f0171e9517f40e41", ` +
		`qop=auth, nc=00000001, cnonce="0a4f113b"`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

// Expected hashes were computed with Python's hashlib.
func TestAuthorizationVariants(t *testing.T) {
	const rest = `realm="testrealm@host.com", nonce="dcd98b7102dd2f0e8b11d0f600bfb0c093"`
	tests := []struct {
		name      string
		challenge string
		method    string
		uri       string
		body      string
		want      string
		absent    []string
	}{
		{
			name:      "rfc 2069 form without qop",
			challenge: "Digest " + rest,
			want:      "670fd8c2df070c60b045671b8b24ff02",
			absent:    []string{"qop", "nc", "cnonce", "algorithm", "opaque"},
		},
		{
			name:      "md5-sess",
			challenge: "Digest algorithm=MD5-sess, qop=auth, " + rest,
			want:      "8e3825c57e897f5a0dec6c2d4e5059d0",
		},
		{
			name:      "auth-int hashes the body",
			challenge: `Digest qop="auth-int", ` + rest,
			method:    "POST",
			uri:       "/upload",
			body:      "hello",
			want:      "8935168e979f86fcb58c43e31a59c91b",
		},
		{
			name:      "sha-512-256",
			challenge: "Digest algorithm=SHA-512-256, qop=auth, " + rest,
			want:      "f23c08ec7334a881f8286e68450ddbd9f0cd91c41481f0e1433604da8113c6dc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method, uri := tt.method, tt.uri
			if method == "" {
				method, uri = "GET", "/dir/index.html"
			}
			got := mustPick(t, tt.challenge).authorization(rfc2617User, method, uri, []byte(tt.body), rfc2617Cnonce)
			if r := param(t, got, "response"); r != tt.want {
				t.Fatalf("response = %s, want %s\n%s", r, tt.want, got)
			}
			for _, name := range tt.absent {
				if _, ok := parseChallenges(got)[0].params[name]; ok {
					t.Fatalf("%s should be absent\n%s", name, got)
				}
			}
		})
	}
}

func TestAuthorizationEchoesAlgorithmAsWritten(t *testing.T) {
	got := mustPick(t, `Digest algorithm="sha-256", nonce="n"`).Authorization(rfc2617User, "GET", "/", nil)
	if a := param(t, got, "algorithm"); a != "sha-256" {
		t.Fatalf("algorithm = %q, want the server's sha-256\n%s", a, got)
	}
}

func TestAuthorizationQuotesValues(t *testing.T) {
	cr := Credentials{Username: `a"b\c`, Password: "p"}
	got := mustPick(t, `Digest realm="r", nonce="n"`).Authorization(cr, "GET", "/", nil)
	if u := param(t, got, "username"); u != cr.Username {
		t.Fatalf("username reads back as %q, want %q\n%s", u, cr.Username, got)
	}
}

func TestAuthorizationUsesNewCnonce(t *testing.T) {
	c := mustPick(t, `Digest qop=auth, nonce="n"`)
	a := param(t, c.Authorization(rfc2617User, "GET", "/", nil), "cnonce")
	b := param(t, c.Authorization(rfc2617User, "GET", "/", nil), "cnonce")
	if a == "" || a == b {
		t.Fatalf("cnonce %q then %q, want two different values", a, b)
	}
}

func TestPick(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		ok     bool
		qop    string
		nonce  string
	}{
		{name: "no challenge", values: nil},
		{name: "basic only", values: []string{`Basic realm="r"`}},
		{name: "missing nonce", values: []string{`Digest realm="r"`}},
		{name: "empty algorithm is not md5", values: []string{`Digest algorithm="", nonce="n"`}},
		{name: "unknown algorithm", values: []string{`Digest algorithm=SHA-1, nonce="n"`}},
		{name: "sess needs a qop", values: []string{`Digest algorithm=MD5-sess, nonce="n"`}},
		{name: "unknown qop only", values: []string{`Digest qop="auth-conf", nonce="n"`}},
		{name: "missing algorithm is md5", values: []string{`Digest nonce="n"`}, ok: true, nonce: "n"},
		{
			name:   "auth preferred",
			values: []string{`Digest qop="auth-int, auth", nonce="n"`},
			ok:     true,
			qop:    "auth",
			nonce:  "n",
		},
		{
			name:   "auth-int when alone",
			values: []string{`Digest qop=auth-int, nonce="n"`},
			ok:     true,
			qop:    "auth-int",
			nonce:  "n",
		},
		{
			name:   "case insensitive",
			values: []string{`DIGEST NONCE="n", ALGORITHM=Sha-256-Sess, QOP=AUTH`},
			ok:     true,
			qop:    "auth",
			nonce:  "n",
		},
		{
			name:   "skips what it cannot answer",
			values: []string{`Digest algorithm=SHA-1, nonce="a"`, `Basic realm="r", Digest nonce="b"`},
			ok:     true,
			nonce:  "b",
		},
		{
			name:   "first supported challenge in one header",
			values: []string{`Digest algorithm=MD5, nonce="a", Digest algorithm=SHA-256, nonce="b"`},
			ok:     true,
			nonce:  "a",
		},
		{
			name:   "first supported challenge in separate headers",
			values: []string{`Digest algorithm=MD5, nonce="a"`, `Digest algorithm=SHA-256, nonce="b"`},
			ok:     true,
			nonce:  "a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, ok := Pick(tt.values)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if c.qop != tt.qop || c.params["nonce"] != tt.nonce {
				t.Fatalf("qop %q nonce %q, want %q %q", c.qop, c.params["nonce"], tt.qop, tt.nonce)
			}
		})
	}
}
