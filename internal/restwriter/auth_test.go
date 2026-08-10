package restwriter

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// The custom header form used to be dropped here, which saved the request
// without its auth.
func TestRenderRoundTripsEveryAuthForm(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   restfile.AuthKind
		params map[string]string
	}{
		{
			name:   "basic",
			source: "# @auth basic user pass",
			want:   restfile.AuthBasic,
			params: map[string]string{"username": "user", "password": "pass"},
		},
		{
			name:   "bearer",
			source: "# @auth bearer tok-123",
			want:   restfile.AuthBearer,
			params: map[string]string{"token": "tok-123"},
		},
		{
			name:   "apikey",
			source: "# @auth apikey header X-API-Key key-123",
			want:   restfile.AuthAPIKey,
			params: map[string]string{"placement": "header", "name": "X-API-Key", "value": "key-123"},
		},
		{
			name:   "apikey in query",
			source: "# @auth apikey query api_key key-123",
			want:   restfile.AuthAPIKey,
			params: map[string]string{"placement": "query", "name": "api_key", "value": "key-123"},
		},
		{
			name:   "custom header",
			source: "# @auth X-Release-Auth header-token",
			want:   restfile.AuthHeader,
			params: map[string]string{"header": "X-Release-Auth", "value": "header-token"},
		},
		{
			name:   "custom header keeps its case",
			source: "# @auth X-MiXeD-CaSe secret",
			want:   restfile.AuthHeader,
			params: map[string]string{"header": "X-MiXeD-CaSe", "value": "secret"},
		},
		{
			name:   "command",
			source: `# @auth command argv=["gh","auth","token"] cache_key=gh`,
			want:   restfile.AuthCommand,
			params: map[string]string{"argv": `["gh","auth","token"]`, "cache_key": "gh"},
		},
		{
			name:   "oauth2",
			source: "# @auth oauth2 token_url=https://id.example.com/token client_id=demo",
			want:   restfile.AuthOAuth2,
			params: map[string]string{"token_url": "https://id.example.com/token", "client_id": "demo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "### r\n# @name r\n" + tt.source + "\nGET https://example.com/\n"
			doc := parser.Parse("r.http", []byte(src))
			if len(doc.Errors) != 0 {
				t.Fatalf("source did not parse: %v", doc.Errors)
			}
			checkAuth(t, doc, tt.want, tt.params, src)

			out := mustRender(t, doc)
			if !strings.Contains(out, "@auth") {
				t.Fatalf("render dropped the @auth directive:\n%s", out)
			}
			back := parser.Parse("r.http", []byte(out))
			if len(back.Errors) != 0 {
				t.Fatalf("rendered document did not parse: %v\n%s", back.Errors, out)
			}
			checkAuth(t, back, tt.want, tt.params, out)

			if again := mustRender(t, back); again != out {
				t.Fatalf("render is not idempotent:\nfirst:\n%s\nsecond:\n%s", out, again)
			}
		})
	}
}

func checkAuth(
	t *testing.T,
	doc *restfile.Document,
	want restfile.AuthKind,
	params map[string]string,
	src string,
) {
	t.Helper()
	auth := doc.Requests[0].Metadata.Auth
	if auth == nil {
		t.Fatalf("auth is missing:\n%s", src)
	}
	if got := auth.Kind(); got != want {
		t.Fatalf("kind = %q, want %q:\n%s", got, want, src)
	}
	for key, val := range params {
		if got := auth.Params[key]; got != val {
			t.Fatalf("param %q = %q, want %q:\n%s", key, got, val, src)
		}
	}
}

// An @apply expression can name any type, so unwritable forms are reachable.
func TestRenderRejectsAuthItCannotWrite(t *testing.T) {
	tests := []struct {
		name string
		auth restfile.AuthSpec
	}{
		{
			name: "unsupported type",
			auth: restfile.AuthSpec{Type: "digest", Params: map[string]string{"user": "u"}},
		},
		{
			name: "custom header with no name",
			auth: restfile.AuthSpec{Type: restfile.AuthHeader, Params: map[string]string{"value": "v"}},
		},
		{
			name: "custom header with no value",
			auth: restfile.AuthSpec{Type: restfile.AuthHeader, Params: map[string]string{"header": "X-A"}},
		},
		{
			name: "oauth2 with no parameters",
			auth: restfile.AuthSpec{Type: restfile.AuthOAuth2, Params: map[string]string{"nope": ""}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := &restfile.Document{Requests: []*restfile.Request{{
				Method:   "GET",
				URL:      "https://example.com/",
				Metadata: restfile.RequestMetadata{Name: "r", Auth: &tt.auth},
			}}}
			out, err := Render(doc, Options{})
			if err == nil {
				t.Fatalf("Render succeeded and dropped the auth:\n%s", out)
			}
			if !strings.Contains(err.Error(), "@auth") {
				t.Fatalf("error = %v, want it to name @auth", err)
			}
		})
	}
}

// Writing one of these would read back as a different form and send the value
// a different way.
func TestRenderRejectsReservedCustomHeaderNames(t *testing.T) {
	reserved := []string{
		"basic", "bearer", "apikey", "api-key", "oauth2", "command",
		"none", "request", "file", "global",
		"Bearer", "BASIC", "None", "File", "Api-Key",
	}
	// Several words give the reserved form enough tokens to parse, which is when
	// the swap goes unnoticed instead of failing.
	values := []string{"secret", "user pass", "a b c", "a b c d"}

	for _, name := range reserved {
		for _, value := range values {
			t.Run(name+"/"+value, func(t *testing.T) {
				doc := headerAuthDoc(name, value)
				out, err := Render(doc, Options{})
				if err == nil {
					t.Fatalf("Render accepted reserved header name %q:\n%s", name, out)
				}
				if !strings.Contains(err.Error(), name) {
					t.Fatalf("error = %v, want it to name %q", err, name)
				}
			})
		}
	}
}

// "header" is the only kind with no keyword of its own, so it stays usable.
func TestRenderKeepsHeaderNamesThatAreNotReserved(t *testing.T) {
	names := []string{"X-Release-Auth", "Authorization", "X-API-Token", "x-custom", "header", "digest", "token"}
	values := []string{"secret", "user pass", "a b c"}

	for _, name := range names {
		for _, value := range values {
			t.Run(name+"/"+value, func(t *testing.T) {
				out := mustRender(t, headerAuthDoc(name, value))
				back := parser.Parse("r.http", []byte(out))
				if len(back.Errors) != 0 {
					t.Fatalf("rendered document did not parse: %v\n%s", back.Errors, out)
				}
				checkAuth(t, back, restfile.AuthHeader, map[string]string{
					"header": name,
					"value":  value,
				}, out)
			})
		}
	}
}

func headerAuthDoc(name, value string) *restfile.Document {
	return &restfile.Document{Requests: []*restfile.Request{{
		Method: "GET",
		URL:    "https://example.com/",
		Metadata: restfile.RequestMetadata{Name: "r", Auth: &restfile.AuthSpec{
			Type:   restfile.AuthHeader,
			Params: map[string]string{"header": name, "value": value},
		}},
	}}}
}
