package request

import (
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/authcmd"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestRuntimeWritesAreSentAsData(t *testing.T) {
	const secret = "secret-value-9f3a"
	include := filepath.Join(t.TempDir(), "local.txt")
	if err := os.WriteFile(include, []byte("local-file-content"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name  string
		block string
		want  string
	}{
		{
			name:  "js header",
			block: "# @script pre-request\n> request.setHeader(\"X-Copied\", \"{{api.token}}\");\nGET http://example.test\n",
			want:  "X-Copied: {{api.token}}",
		},
		{
			name:  "js body",
			block: "# @script pre-request\n> request.setBody(\"{{api.token}}\");\nPOST http://example.test\n",
			want:  "body: {{api.token}}",
		},
		{
			name:  "js url",
			block: "# @script pre-request\n> request.setURL(\"http://example.test/{{api.token}}\");\nGET http://example.test\n",
			want:  "path: /{{api.token}}",
		},
		{
			name:  "js query",
			block: "# @script pre-request\n> request.setQueryParam(\"q\", \"{{api.token}}\");\nGET http://example.test\n",
			want:  "q: {{api.token}}",
		},
		{
			name:  "rts header",
			block: "# @rts pre-request\n> request.setHeader(\"X-Copied\", \"{\" + \"{api.token}}\")\nGET http://example.test\n",
			want:  "X-Copied: {{api.token}}",
		},
		{
			name:  "apply header",
			block: "# @apply {headers: {\"X-Copied\": \"{\" + \"{api.token}}\"}}\nGET http://example.test\n",
			want:  "X-Copied: {{api.token}}",
		},
		{
			name:  "apply query",
			block: "# @apply {query: {q: \"{\" + \"{api.token}}\"}}\nGET http://example.test\n",
			want:  "q: {{api.token}}",
		},
		{
			name:  "apply body",
			block: "# @apply {body: \"{\" + \"{api.token}}\"}\nPOST http://example.test\n",
			want:  "body: {{api.token}}",
		},
		{
			name:  "apply auth",
			block: "# @apply {auth: {type: \"bearer\", token: \"{\" + \"{api.token}}\"}}\nGET http://example.test\n",
			want:  "Authorization: Bearer {{api.token}}",
		},
		{
			name:  "apply vars",
			block: "# @apply {vars: {\"copied\": \"{\" + \"{api.token}}\"}}\nGET http://example.test\nX-Copied: {{copied}}\n",
			want:  "X-Copied: {{api.token}}",
		},
		{
			name:  "js body include",
			block: "# @script pre-request\n> request.setBody(\"@ " + include + "\");\nPOST http://example.test\n",
			want:  "body: @ " + include,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, req := parseDoc(t, "# @file-secret api.token "+secret+"\n\n### one\n# @name one\n"+tc.block)
			sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
			got := wireText(t, sent)
			if strings.Contains(got, secret) || strings.Contains(got, "local-file-content") {
				t.Fatalf("run-time text was expanded:\n%s", got)
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("sent request does not contain %q:\n%s", tc.want, got)
			}
		})
	}
}

// A response can return a value with an "@ path" line. Placing it in an
// authored body must not read that file.
func TestRuntimeValuesCannotAddBodyIncludes(t *testing.T) {
	include := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(include, []byte("local-file-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	note := "hi\n@ " + include + "\nbye"

	for _, tc := range []struct {
		name string
		src  string
	}{
		{
			name: "capture",
			src: "### a\n# @name a\n# @capture global note {{response.json.note}}\nGET http://example.test/a\n\n" +
				"### b\n# @name b\nPOST http://example.test/b\n\n{\"note\": \"{{note}}\"}\n",
		},
		{
			name: "script var",
			src: "### b\n# @name b\n# @script pre-request\n> vars.set(\"note\", " + strconv.Quote(note) + ");\n" +
				"POST http://example.test/b\n\n{\"note\": \"{{note}}\"}\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := parser.Parse("written.http", []byte(tc.src))
			eng, st := newStubEngine(t)
			env := envWith(t, "dev", nil)
			if len(doc.Requests) == 2 {
				st.body = `{"note":` + strconv.Quote(note) + `}`
				sendWith(t, eng, st, doc, doc.Requests[0], env, ExecOptions{})
			}
			sent := sendWith(t, eng, st, doc, doc.Requests[len(doc.Requests)-1], env, ExecOptions{})
			got := wireText(t, sent)
			if strings.Contains(got, "local-file-content") {
				t.Fatalf("a run-time value read a local file:\n%s", got)
			}
			if !strings.Contains(got, "@ "+include) {
				t.Fatalf("sent body does not keep the value:\n%s", got)
			}
		})
	}
}

// A written multipart body still gets CRLF framing. Framing reads no files.
func TestWrittenMultipartBodyIsFramed(t *testing.T) {
	include := filepath.Join(t.TempDir(), "local.txt")
	if err := os.WriteFile(include, []byte("local-file-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, req := parseDoc(t, `### one
# @name one
# @script pre-request
> request.setBody('--b\nContent-Disposition: form-data; name="f"\n\n@ `+include+`\n--b--');
POST http://example.test
Content-Type: multipart/form-data; boundary=b
`)

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	body, err := io.ReadAll(sent.wire.Body)
	if err != nil {
		t.Fatal(err)
	}
	want := "--b\r\nContent-Disposition: form-data; name=\"f\"\r\n\r\n@ " + include + "\r\n--b--\r\n"
	if string(body) != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

func TestRuntimeWritesStillRenderHelpers(t *testing.T) {
	doc, req := parseDoc(t, `### one
# @name one
# @request id {{$uuid}}
# @script pre-request
> request.setHeader("X-Id", vars.get("id"));
GET http://example.test
`)

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	if got := sent.wire.Header.Get("X-Id"); len(got) != 36 || strings.Contains(got, "{{") {
		t.Fatalf("X-Id = %q, want a rendered UUID", got)
	}
}

func TestCommandAuthOutputIsSentAsData(t *testing.T) {
	doc, req := parseDoc(t, `# @file-secret api.token secret-value-9f3a

### one
# @name one
# @auth command cmd="token"
GET http://example.test
`)
	eng, st := newStubEngine(t)
	eng.rt.AuthCmd().SetExecFunc(func(context.Context, authcmd.Config) ([]byte, error) {
		return []byte("{{api.token}}"), nil
	})

	sent := sendWith(t, eng, st, doc, req, envWith(t, "dev", nil), ExecOptions{})
	if got := sent.wire.Header.Get("Authorization"); got != "Bearer {{api.token}}" {
		t.Fatalf("Authorization = %q, want the command output unchanged", got)
	}
}

// gRPC auth is injected before headers are expanded, so it must be marked as data.
func TestGRPCWrittenAuthIsSentAsData(t *testing.T) {
	res := vars.NewResolver(vars.NewMapProvider("file", map[string]string{"api.token": "secret-value-9f3a"}))
	req := &restfile.Request{
		Method: "GRPC",
		URL:    "localhost:50051",
		GRPC:   &restfile.GRPCRequest{Target: "localhost:50051", FullMethod: "/pkg.Service/Get"},
		Metadata: restfile.RequestMetadata{Auth: &restfile.AuthSpec{
			Type:    restfile.AuthBearer,
			Params:  map[string]string{"token": "{{api.token}}"},
			Written: true,
		}},
	}

	if err := applyGRPCAuth(req, res); err != nil {
		t.Fatalf("applyGRPCAuth: %v", err)
	}
	if err := prepareGRPCRequest(req, res, grpcx.Options{}); err != nil {
		t.Fatalf("prepareGRPCRequest: %v", err)
	}
	if got := req.Headers.Get("Authorization"); got != "Bearer {{api.token}}" {
		t.Fatalf("Authorization = %q, want the written token unchanged", got)
	}
}

func TestGRPCWrittenBodyReplacesOnlyWhenSet(t *testing.T) {
	res := vars.NewResolver(vars.NewMapProvider("file", map[string]string{"x": "1"}))
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "empty keeps the authored message", body: "", want: `{"a": "1"}`},
		{name: "set is data", body: `{"a": "{{x}}"}`, want: `{"a": "{{x}}"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &restfile.Request{
				Method: "GRPC",
				URL:    "localhost:50051",
				Body:   restfile.BodySource{Text: tc.body},
				GRPC: &restfile.GRPCRequest{
					Target:     "localhost:50051",
					FullMethod: "/pkg.S/M",
					Message:    `{"a": "{{x}}"}`,
				},
				Written: restfile.Written{Body: true},
			}
			if err := prepareGRPCRequest(req, res, grpcx.Options{}); err != nil {
				t.Fatalf("prepareGRPCRequest: %v", err)
			}
			if req.GRPC.Message != tc.want {
				t.Fatalf("message = %q, want %q", req.GRPC.Message, tc.want)
			}
		})
	}
}

func wireText(t *testing.T, sent sentRequest) string {
	t.Helper()
	var b strings.Builder
	path, err := url.PathUnescape(sent.wire.URL.EscapedPath())
	if err != nil {
		t.Fatal(err)
	}
	b.WriteString("path: " + path + "\n")
	for key, vals := range sent.wire.URL.Query() {
		b.WriteString(key + ": " + strings.Join(vals, ",") + "\n")
	}
	for key, vals := range sent.wire.Header {
		b.WriteString(key + ": " + strings.Join(vals, ",") + "\n")
	}
	if sent.wire.Body != nil {
		data, err := io.ReadAll(sent.wire.Body)
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString("body: " + string(data) + "\n")
	}
	return b.String()
}

func TestPreRequestScriptTemplatesWarn(t *testing.T) {
	doc, req := parseDoc(t, `### one
# @apply {headers: {"X-Apply": "{{token}}"}}
# @rts pre-request
> request.setHeader("X-Rts", "{{token}}")
# @script pre-request
> request.setHeader("X-Js", "{{= 1 + 1 }}");
> request.setQueryParam("id", "{{$uuid}}");
GET http://example.test
`)
	eng, _ := newStubEngine(t)
	var called []string
	res, err := eng.ExecuteWith(doc, req, envWith(t, "dev", nil), ExecOptions{
		OnWarning: func(w Warning) { called = append(called, string(w)) },
	})
	if err != nil || res.Err != nil {
		t.Fatalf("ExecuteWith() = %v, %v", err, res.Err)
	}
	want := []string{
		`env_ref.http:4: Script sends {{token}} in header X-Rts as written. Use vars.get("token").`,
		"env_ref.http:6: Script sends {{= 1 + 1 }} in header X-Js as written. Write the expression without {{= }}.",
	}
	if !slices.Equal(res.Warnings, want) || !slices.Equal(called, want) {
		t.Fatalf("warnings = %q, callback = %q, want %q", res.Warnings, called, want)
	}
	for _, w := range want {
		if !slices.Contains(res.Explain.Warnings, w) {
			t.Fatalf("explain warnings = %q, want %q", res.Explain.Warnings, w)
		}
	}
}
