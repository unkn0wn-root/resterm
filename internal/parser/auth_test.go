package parser

import (
	"maps"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestParseNamedCommandAuth(t *testing.T) {
	src := `# @auth global command gh cmd="gh auth token"
# @auth file command GCloud cmd='gcloud auth print-access-token' ttl=50m
# @auth file command argv=["mycli","token"]

### Uses a profile
# @auth use=gh header=X-Token timeout=5s
GET https://example.com
`
	doc := Parse("/ws/api.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("expected no parse errors, got %v", doc.Errors)
	}

	want := []struct {
		scope directive.Scope
		name  string
		param string
		value string
	}{
		{directive.ScopeGlobal, "gh", "cmd", "gh auth token"},
		{directive.ScopeFile, "GCloud", "cmd", "gcloud auth print-access-token"},
		{directive.ScopeFile, "", "argv", `["mycli","token"]`},
	}
	if len(doc.Auth) != len(want) {
		t.Fatalf("expected %d auth profiles, got %+v", len(want), doc.Auth)
	}
	for i, w := range want {
		got := doc.Auth[i]
		if got.Scope != w.scope || got.Name != w.name || got.Spec.Params[w.param] != w.value {
			t.Fatalf("profile %d = %+v, want %s %q with %s=%q", i, got, w.scope, w.name, w.param, w.value)
		}
		if got.Spec.Kind() != restfile.AuthCommand || got.Spec.Use != "" {
			t.Fatalf("profile %d spec = %+v, want a command definition", i, got.Spec)
		}
	}

	ref := doc.Requests[0].Metadata.Auth
	if ref == nil || ref.Use != "gh" || ref.Kind() != "" {
		t.Fatalf("request auth = %+v, want a use=gh reference", ref)
	}
	wantParams := map[string]string{"header": "X-Token", "timeout": "5s"}
	if !maps.Equal(ref.Params, wantParams) {
		t.Fatalf("reference params = %v, want %v", ref.Params, wantParams)
	}
	if ref.Origin() != "/ws/api.http:6" {
		t.Fatalf("reference origin = %q, want the request line", ref.Origin())
	}
}

func TestParseAuthProfileErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "use at file scope",
			src:  "# @auth file use=gh\n",
			want: "@auth file scope does not support use=",
		},
		{
			name: "use without a name",
			src:  "### r\n# @auth use=\nGET https://example.com\n",
			want: "@auth use= requires a profile name",
		},
		{
			name: "use with a definition option",
			src:  "### r\n# @auth use=gh ttl=5m cache_key=gh\nGET https://example.com\n",
			want: "@auth use= accepts only header, scheme, timeout; set cache_key, ttl on the definition",
		},
		{
			name: "use with a bare word",
			src:  "### r\n# @auth use=gh extra\nGET https://example.com\n",
			want: `@auth expects key=value options, got "extra"`,
		},
		{
			name: "cmd with spaces and no quotes",
			src:  "### r\n# @auth command cmd=printf token\nGET https://example.com\n",
			want: `@auth expects key=value options, got "token"; quote a value that has spaces`,
		},
		{
			name: "oauth2 value with spaces and no quotes",
			src:  "### r\n# @auth oauth2 token_url=https://id.example.com scope=read write\nGET https://example.com\n",
			want: `@auth expects key=value options, got "write"`,
		},
		{
			name: "unclosed double quote",
			src:  "### r\n# @auth command cmd=\"printf token\nGET https://example.com\n",
			want: `@auth is missing a closing "\""`,
		},
		{
			name: "unclosed single quote",
			src:  "### r\n# @auth bearer 'abc\nGET https://example.com\n",
			want: `@auth is missing a closing "'"`,
		},
		{
			name: "named definition without a command",
			src:  "# @auth file command gh cache_key=gh\n",
			want: "@auth command gh requires cmd or argv",
		},
		{
			name: "name on a request",
			src:  "### r\n# @auth command gh cmd=\"gh auth token\"\nGET https://example.com\n",
			want: "@auth request scope does not support a profile name",
		},
		{
			name: "invalid name",
			src:  "# @auth file command g!h cmd=\"gh auth token\"\n",
			want: `@auth profile name "g!h" is invalid`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Parse("/ws/api.http", []byte(tt.src))
			if len(doc.Errors) != 1 || !strings.Contains(doc.Errors[0].Message, tt.want) {
				t.Fatalf("errors = %+v, want one containing %q", doc.Errors, tt.want)
			}
		})
	}
}

func TestParseCommandAuthSpecCmd(t *testing.T) {
	spec, err := parseAuthSpec(directive.Fields(`command cmd="aws ecr get-authorization-token --query 'a[0].b'"`))
	if err != nil || spec == nil {
		t.Fatalf("parseAuthSpec() = %+v, %v", spec, err)
	}
	if got := spec.Params["cmd"]; got != "aws ecr get-authorization-token --query 'a[0].b'" {
		t.Fatalf("cmd = %q", got)
	}
}

func TestParseCacheKeyOnlyCommandStaysValid(t *testing.T) {
	src := "# @auth file command cache_key=gh\n\n### r\n# @auth command cache_key=gh\nGET https://example.com\n"
	doc := Parse("/ws/api.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("expected the cache_key form to stay valid, got %v", doc.Errors)
	}
}

func TestParseRejectedAuthKeepsItsPlace(t *testing.T) {
	src := `# @auth file bearer file-token
# @auth file command gh cache_key=gh
# @auth global oauth2 token_url=https://id.example.com scope=read write

### Own line rejected
# @auth use=gh ttl=5m
GET https://example.com/a
`
	doc := Parse("/ws/api.http", []byte(src))
	if len(doc.Errors) != 3 {
		t.Fatalf("expected three errors, got %v", doc.Errors)
	}

	named := doc.Auth[1]
	if named.Name != "gh" || named.Spec.Line != 2 || !strings.Contains(named.Spec.Rejected, "requires cmd or argv") {
		t.Fatalf("named profile = %+v, want gh kept as rejected", named)
	}
	global := doc.Auth[2]
	if global.Scope != directive.ScopeGlobal || global.Name != "" || global.Spec.Rejected == "" {
		t.Fatalf("global profile = %+v, want an unnamed rejected default", global)
	}
	own := doc.Requests[0].Metadata.Auth
	if own == nil || own.Line != 6 || !strings.Contains(own.Rejected, "set ttl on the definition") {
		t.Fatalf("request auth = %+v, want its rejected line", own)
	}
}

func TestParseAuthKeepsOpenGroupValues(t *testing.T) {
	for key, val := range map[string]string{"password": "[abc", "client_secret": "Pa(ss"} {
		src := "### r\n# @auth oauth2 token_url=https://id.example.com " + key + "=" + val + "\nGET https://example.com\n"
		doc := Parse("/ws/api.http", []byte(src))
		if len(doc.Errors) != 0 {
			t.Fatalf("%s: expected no errors, got %v", key, doc.Errors)
		}
		if got := doc.Requests[0].Metadata.Auth.Params[key]; got != val {
			t.Fatalf("%s = %q, want %q", key, got, val)
		}
	}
}

func TestParseUnclosedNamedDefinitionKeepsItsName(t *testing.T) {
	doc := Parse("/ws/defs.http", []byte("# @auth global command gh cmd=\"unterminated\n"))
	if len(doc.Errors) != 1 || len(doc.Auth) != 1 {
		t.Fatalf("errors = %v, profiles = %+v", doc.Errors, doc.Auth)
	}
	if p := doc.Auth[0]; p.Name != "gh" || p.Spec.Rejected == "" {
		t.Fatalf("profile = %+v, want gh kept as rejected", p)
	}
}
