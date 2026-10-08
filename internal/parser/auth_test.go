package parser

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func authFields(src string) []directive.Field {
	return slices.Collect(directive.ScanFields(src))
}

func TestParseNamedCommandAuth(t *testing.T) {
	src := `# @auth global command name=gh cmd="gh auth token"
# @auth file command name=GCloud cmd='gcloud auth print-access-token' ttl=50m
# @auth file command argv=["mycli","token"]

### Uses a profile
# @auth use=gh header=X-Token timeout=5s scheme=
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

// Before 1.10 a bare word after command was ignored and the line was the
// default. Reading it as a name would send inheriting requests without auth.
func TestParseBareWordDefinitionStaysLoud(t *testing.T) {
	src := "# @auth global command gh argv=[\"gh\",\"auth\",\"token\"]\n\n### r\nGET https://example.com\n"
	doc := Parse("/ws/api.http", []byte(src))
	want := `@auth expects key=value options but got "gh". Write name=gh to name a definition`
	if len(doc.Errors) != 1 || !strings.Contains(doc.Errors[0].Message, want) {
		t.Fatalf("errors = %+v, want one containing %q", doc.Errors, want)
	}
	if len(doc.Auth) != 1 || doc.Auth[0].Name != "" || doc.Auth[0].Spec.Rejected == "" {
		t.Fatalf("profiles = %+v, want an unnamed rejected default", doc.Auth)
	}
}

func TestParseAuthNameReadsLikeOtherOptions(t *testing.T) {
	for _, line := range []string{
		`# @auth file command name=" gh" cmd=x`,
		`# @auth file command NAME=gh cmd=x`,
	} {
		doc := Parse("/ws/api.http", []byte(line+"\n"))
		if len(doc.Errors) != 0 || len(doc.Auth) != 1 || doc.Auth[0].Name != "gh" {
			t.Fatalf("%s: errors = %v, profiles = %+v, want gh", line, doc.Errors, doc.Auth)
		}
	}
}

func TestParseAuthQuotedOptionIsAValue(t *testing.T) {
	doc := Parse("/ws/api.http", []byte(`# @auth file command "name=gh" cmd=x`+"\n"))
	want := `@auth expects key=value options but got "name=gh". Quote a value that has spaces`
	if len(doc.Errors) != 1 || doc.Errors[0].Message != want {
		t.Fatalf("errors = %v, want %q", doc.Errors, want)
	}
	if len(doc.Auth) != 1 || doc.Auth[0].Name != "" || doc.Auth[0].Spec.Rejected == "" {
		t.Fatalf("profiles = %+v, want an unnamed rejected profile", doc.Auth)
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
			want: "@auth use= accepts only header, scheme, timeout. Set cache_key, ttl on the definition",
		},
		{
			name: "use with a bare word",
			src:  "### r\n# @auth use=gh extra\nGET https://example.com\n",
			want: `@auth expects key=value options but got "extra"`,
		},
		{
			name: "cmd with spaces and no quotes",
			src:  "### r\n# @auth command cmd=printf token\nGET https://example.com\n",
			want: `@auth expects key=value options but got "token". Quote a value that has spaces`,
		},
		{
			name: "cmd followed by a field with no key",
			src:  "### r\n# @auth command cmd=whoami =ignored\nGET https://example.com\n",
			want: `@auth option "=ignored" has spaces around =. Write it as key=value`,
		},
		{
			name: "oauth2 field with no key",
			src:  "### r\n# @auth oauth2 token_url=https://id.example.com =ignored\nGET https://example.com\n",
			want: `@auth option "=ignored" has spaces around =. Write it as key=value`,
		},
		{
			name: "cmd written with spaces around =",
			src:  "### r\n# @auth command cmd = \"gh auth token\"\nGET https://example.com\n",
			want: `@auth option "cmd" has spaces around =. Write it as key=value`,
		},
		{
			name: "cmd written with a space after =",
			src:  "### r\n# @auth command cmd= gh\nGET https://example.com\n",
			want: `@auth option "cmd" has spaces around =. Write it as key=value`,
		},
		{
			name: "oauth2 option written with spaces around =",
			src:  "### r\n# @auth oauth2 token_url = https://id.example.com client_id=a\nGET https://example.com\n",
			want: `@auth option "token_url" has spaces around =. Write it as key=value`,
		},
		{
			name: "use written with spaces around =",
			src:  "### r\n# @auth use = gh\nGET https://example.com\n",
			want: `@auth option "use" has spaces around =. Write it as key=value`,
		},
		{
			name: "use written with a space before =",
			src:  "### r\n# @auth use =gh\nGET https://example.com\n",
			want: `@auth option "use" has spaces around =. Write it as key=value`,
		},
		{
			name: "named definition written with spaces around =",
			src:  "# @auth global command name=gh cmd = \"gh auth token\"\n",
			want: `@auth option "cmd" has spaces around =. Write it as key=value`,
		},
		{
			name: "cmd with an unquoted key=value argument",
			src:  "### r\n# @auth command cmd=mycli --role=admin\nGET https://example.com\n",
			want: "@auth command does not accept --role. Quote a cmd value that has spaces",
		},
		{
			name: "command with an unknown option",
			src:  "### r\n# @auth command argv=[\"mycli\"] toekn_path=token\nGET https://example.com\n",
			want: "@auth command does not accept toekn_path",
		},
		{
			name: "named definition with an unquoted key=value argument",
			src:  "# @auth global command name=gh cmd=gh --hostname=ghe.example.com\n",
			want: "@auth command does not accept --hostname",
		},
		{
			name: "oauth2 value with spaces and no quotes",
			src:  "### r\n# @auth oauth2 token_url=https://id.example.com scope=read write\nGET https://example.com\n",
			want: `@auth expects key=value options but got "write"`,
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
			src:  "# @auth file command name=gh cache_key=gh\n",
			want: "@auth command gh requires cmd or argv",
		},
		{
			name: "name on a request",
			src:  "### r\n# @auth command name=gh cmd=\"gh auth token\"\nGET https://example.com\n",
			want: "@auth request scope does not support name=",
		},
		{
			name: "bare word on a request",
			src:  "### r\n# @auth command gh cmd=\"gh auth token\"\nGET https://example.com\n",
			want: `@auth expects key=value options but got "gh". Quote a value that has spaces`,
		},
		{
			name: "bare word on a file definition",
			src:  "# @auth file command gh cmd=\"gh auth token\"\n",
			want: `@auth expects key=value options but got "gh". Write name=gh to name a definition`,
		},
		{
			name: "bare word that cannot be a name",
			src:  "# @auth file command g!h cmd=\"gh auth token\"\n",
			want: `@auth expects key=value options but got "g!h". Quote a value that has spaces`,
		},
		{
			name: "name written with a space after =",
			src:  "# @auth file command name= gh cmd=\"gh auth token\"\n",
			want: `@auth option "name" has spaces around =. Write it as key=value`,
		},
		{
			name: "empty name",
			src:  "# @auth file command name= cmd=\"gh auth token\"\n",
			want: "@auth name= requires a profile name",
		},
		{
			name: "repeated name",
			src:  "# @auth file command name=gh cmd=\"gh auth token\" name=other\n",
			want: `@auth option "name" is repeated`,
		},
		{
			name: "invalid name",
			src:  "# @auth file command name=g!h cmd=\"gh auth token\"\n",
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
	spec, err := parseAuthSpec(authFields(`command cmd="aws ecr get-authorization-token --query 'a[0].b'"`))
	if err != nil || spec == nil {
		t.Fatalf("parseAuthSpec() = %+v, %v", spec, err)
	}
	if got := spec.Params["cmd"]; got != "aws ecr get-authorization-token --query 'a[0].b'" {
		t.Fatalf("cmd = %q", got)
	}
}

// This also fails if name becomes a command option, since name= names the definition.
func TestParseCommandAuthAcceptsEveryOption(t *testing.T) {
	var src strings.Builder
	src.WriteString("command")
	for _, key := range restfile.AuthCommandParams {
		src.WriteString(" " + key + "=v")
	}
	spec, err := parseAuthSpec(authFields(src.String()))
	if err != nil || spec == nil {
		t.Fatalf("parseAuthSpec() = %+v, %v", spec, err)
	}
	if len(spec.Params) != len(restfile.AuthCommandParams) {
		t.Fatalf("params = %v, want every command option", spec.Params)
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
# @auth file command name=gh cache_key=gh
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
	if own == nil || own.Line != 6 || !strings.Contains(own.Rejected, "Set ttl on the definition") {
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
	doc := Parse("/ws/defs.http", []byte("# @auth global command name=gh cmd=\"unterminated\n"))
	if len(doc.Errors) != 1 || len(doc.Auth) != 1 {
		t.Fatalf("errors = %v, profiles = %+v", doc.Errors, doc.Auth)
	}
	if p := doc.Auth[0]; p.Name != "gh" || p.Spec.Rejected == "" {
		t.Fatalf("profile = %+v, want gh kept as rejected", p)
	}
}

func TestParseAuthPositionalValueMayStartWithEquals(t *testing.T) {
	src := "### b\n# @auth basic admin =secret\nGET https://example.com/b\n\n" +
		"### t\n# @auth bearer =token\nGET https://example.com/t\n\n" +
		"### h\n# @auth use gh\nGET https://example.com/h\n"
	doc := Parse("/ws/api.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("errors = %v", doc.Errors)
	}
	want := []map[string]string{
		{"username": "admin", "password": "=secret"},
		{"token": "=token"},
		{"header": "use", "value": "gh"},
	}
	for i, w := range want {
		if got := doc.Requests[i].Metadata.Auth.Params; !maps.Equal(got, w) {
			t.Fatalf("request %d params = %v, want %v", i, got, w)
		}
	}
}

func TestParseSpacedAuthKeepsItsPlace(t *testing.T) {
	for line, name := range map[string]string{
		`# @auth global command name=gh cmd = "gh auth token"`: "gh",
		"# @auth global command name=gh =x":                    "gh",
		"# @auth global command gh =x":                         "",
		"# @auth global command name =gh":                      "",
		`# @auth file command cmd = "gh auth token"`:           "",
		"# @auth global command format =x":                     "",
	} {
		doc := Parse("/ws/defs.http", []byte(line+"\n"))
		if len(doc.Auth) != 1 || doc.Auth[0].Name != name || doc.Auth[0].Spec.Rejected == "" {
			t.Fatalf("%s: profiles = %+v, want %q kept as rejected", line, doc.Auth, name)
		}
	}
	doc := Parse("/ws/api.http", []byte("### r\n# @auth use = gh\nGET https://example.com\n"))
	if a := doc.Requests[0].Metadata.Auth; a == nil || a.Rejected == "" || a.Kind() == restfile.AuthHeader {
		t.Fatalf("auth = %+v, want the line rejected, not read as a header named use", a)
	}
}

func TestParseRejectsDigestAsCustomHeader(t *testing.T) {
	src := "### h\n# @auth Digest sha-256=abc\nGET https://example.com/h\n"
	doc := Parse("/ws/api.http", []byte(src))
	if len(doc.Errors) != 1 || !strings.Contains(doc.Errors[0].Message, "requires a valid auth spec") {
		t.Fatalf("errors = %v, want the Digest header line rejected", doc.Errors)
	}
	if h := doc.Requests[0].Metadata.Auth; h == nil || h.Rejected == "" {
		t.Fatalf("Digest header line = %+v, want rejected", h)
	}
}

// A template can still expand to a valid placement, so only a literal is checked here.
func TestParseAPIKeyPlacement(t *testing.T) {
	for line, want := range map[string]string{
		"# @auth apikey QUERY api_key {{key}}":     "",
		"# @auth apikey {{place}} X-Key {{key}}":   "",
		"# @auth apikey cookie session_id {{key}}": `@auth apikey placement "cookie" is not supported. Use header or query`,
		"# @auth global apikey body X-Key {{key}}": `@auth apikey placement "body" is not supported. Use header or query`,
	} {
		doc := Parse("/ws/api.http", []byte(line+"\nGET https://example.com\n"))
		if want == "" {
			if len(doc.Errors) != 0 {
				t.Fatalf("%s: errors = %v", line, doc.Errors)
			}
			continue
		}
		if len(doc.Errors) != 1 || doc.Errors[0].Message != want {
			t.Fatalf("%s: errors = %v, want %q", line, doc.Errors, want)
		}
	}
}
