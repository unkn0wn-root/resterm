package request

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/authcmd"
	"github.com/unkn0wn-root/resterm/internal/engine"
	rtrun "github.com/unkn0wn-root/resterm/internal/engine/runtime"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type namedAuthRig struct {
	eng   *Engine
	ws    string
	calls atomic.Int32
	seen  authcmd.Config
	sent  *http.Request
}

func newNamedAuthRig(t *testing.T, files map[string]string) *namedAuthRig {
	t.Helper()
	rig := &namedAuthRig{ws: t.TempDir()}
	for name, src := range files {
		path := filepath.Join(rig.ws, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	client := httpx.NewClientWithOptions(
		httpx.WithHTTPFactory(func(httpx.Options) (*http.Client, error) {
			return &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					rig.sent = r
					return &http.Response{
						Status:     "200 OK",
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader("ok")),
						Request:    r,
					}, nil
				}),
			}, nil
		}),
	)
	cfg := engine.Config{Client: client, WorkspaceRoot: rig.ws, Recursive: true}
	rig.eng = New(cfg, rtrun.New(rtrun.Config{}))
	rig.eng.rt.AuthCmd().SetExecFunc(func(_ context.Context, cfg authcmd.Config) ([]byte, error) {
		rig.seen = cfg
		return fmt.Appendf(nil, "token-%d", rig.calls.Add(1)), nil
	})
	return rig
}

func (r *namedAuthRig) doc(t *testing.T, name string) *restfile.Document {
	t.Helper()
	doc := r.parse(t, name)
	if len(doc.Errors) != 0 {
		t.Fatalf("parse %s: %v", name, doc.Errors)
	}
	return doc
}

func (r *namedAuthRig) parse(t *testing.T, name string) *restfile.Document {
	t.Helper()
	path := filepath.Join(r.ws, name)
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return parser.Parse(path, src)
}

func (r *namedAuthRig) run(
	t *testing.T,
	doc *restfile.Document,
	req int,
	env string,
	mode ExecMode,
) engine.RequestResult {
	t.Helper()
	r.sent = nil
	res, err := r.eng.ExecuteWith(doc, doc.Requests[req], testEnv(env), ExecOptions{Mode: mode})
	if err != nil {
		t.Fatalf("ExecuteWith: %v", err)
	}
	return res
}

func (r *namedAuthRig) header(t *testing.T, name string) string {
	t.Helper()
	if r.sent == nil {
		t.Fatal("request was never sent")
	}
	return r.sent.Header.Get(name)
}

func TestNamedCommandAuthResolvesFromWorkspace(t *testing.T) {
	rig := newNamedAuthRig(t, map[string]string{
		"auth/defs.http": `# @auth global command gh cmd="gh auth token"` + "\n",
		"api.http": `### User
# @auth use=gh
GET https://example.test/user

### Repos
# @auth use=gh
GET https://example.test/repos
`,
	})
	doc := rig.doc(t, "api.http")

	// Start with the second request to check that either request can fill the cache.
	if res := rig.run(t, doc, 1, "dev", ExecModeSend); res.Err != nil {
		t.Fatalf("run repos: %v", res.Err)
	}
	if got := rig.header(t, "Authorization"); got != "Bearer token-1" {
		t.Fatalf("Authorization = %q", got)
	}
	if !slices.Equal(rig.seen.Argv, []string{"gh", "auth", "token"}) {
		t.Fatalf("argv = %q", rig.seen.Argv)
	}
	if want := filepath.Join(rig.ws, "auth"); rig.seen.Dir != want {
		t.Fatalf("command dir = %q, want the definition's %q", rig.seen.Dir, want)
	}

	if res := rig.run(t, doc, 0, "dev", ExecModeSend); res.Err != nil {
		t.Fatalf("run user: %v", res.Err)
	}
	if got := rig.header(t, "Authorization"); got != "Bearer token-1" {
		t.Fatalf("expected the cached token, got %q", got)
	}
	if res := rig.run(t, doc, 0, "prod", ExecModeSend); res.Err != nil {
		t.Fatalf("run user in prod: %v", res.Err)
	}
	if got := rig.header(t, "Authorization"); got != "Bearer token-2" {
		t.Fatalf("expected a separate token per environment, got %q", got)
	}
}

func TestNamedCommandAuthPrefersFileScopeAndAppliesOverrides(t *testing.T) {
	rig := newNamedAuthRig(t, map[string]string{
		"defs.http": `# @auth global command gh cmd="gh-global token"` + "\n",
		"api.http": `# @file who Ada Lovelace
# @auth file command gh cmd="gh-file token --user {{ who }}"

### User
# @auth use=GH header=X-Token scheme=Token
GET https://example.test/user
`,
	})
	doc := rig.doc(t, "api.http")

	if res := rig.run(t, doc, 0, "dev", ExecModeSend); res.Err != nil {
		t.Fatalf("run: %v", res.Err)
	}
	if want := []string{"gh-file", "token", "--user", "Ada Lovelace"}; !slices.Equal(rig.seen.Argv, want) {
		t.Fatalf("argv = %q, want %q", rig.seen.Argv, want)
	}
	if got := rig.header(t, "X-Token"); got != "Token token-1" {
		t.Fatalf("X-Token = %q", got)
	}
	if got := rig.header(t, "Authorization"); got != "" {
		t.Fatalf("expected no Authorization header, got %q", got)
	}
}

func TestNamedCommandAuthMissingNameFails(t *testing.T) {
	rig := newNamedAuthRig(t, map[string]string{
		"api.http": `### User
# @auth use=nope
GET https://example.test/user
`,
	})
	doc := rig.doc(t, "api.http")

	for _, mode := range []ExecMode{ExecModeSend, ExecModePreview} {
		res := rig.run(t, doc, 0, "dev", mode)
		want := fmt.Sprintf(`@auth use="nope" not found (%s:2)`, filepath.Join(rig.ws, "api.http"))
		if res.Err == nil || !strings.Contains(res.Err.Error(), want) {
			t.Fatalf("mode %v: error = %v, want %q", mode, res.Err, want)
		}
	}
	if rig.sent != nil || rig.calls.Load() != 0 {
		t.Fatalf("a missing profile must not send or run anything")
	}
}

func TestNamedCommandAuthPreviewUsesCacheOnly(t *testing.T) {
	rig := newNamedAuthRig(t, map[string]string{
		"api.http": `# @auth file command gh cmd="gh auth token"

### User
# @auth use=gh
GET https://example.test/user
`,
	})
	doc := rig.doc(t, "api.http")

	res := rig.run(t, doc, 0, "dev", ExecModePreview)
	if !authStageHas(res, xplain.StageSkipped, xplain.SummaryCommandAuthExecutionSkipped) {
		t.Fatalf("expected a skipped command auth stage, got %+v", res.Explain)
	}
	if rig.calls.Load() != 0 {
		t.Fatal("preview must not run the command")
	}

	rig.run(t, doc, 0, "dev", ExecModeSend)
	res = rig.run(t, doc, 0, "dev", ExecModePreview)
	if !authStageHas(res, xplain.StageOK, xplain.SummaryAuthPrepared) {
		t.Fatalf("expected preview to use the cached token, got %+v", res.Explain)
	}
	if rig.calls.Load() != 1 {
		t.Fatalf("expected one run, got %d", rig.calls.Load())
	}
}

func TestApplyPatchReplacesNamedAuth(t *testing.T) {
	rig := newNamedAuthRig(t, map[string]string{
		"api.http": `### User
# @auth use=missing
# @apply {auth: {type: "bearer", token: "patched"}}
GET https://example.test/user
`,
	})
	doc := rig.doc(t, "api.http")

	if res := rig.run(t, doc, 0, "dev", ExecModeSend); res.Err != nil {
		t.Fatalf("run: %v", res.Err)
	}
	if got := rig.header(t, "Authorization"); got != "Bearer patched" {
		t.Fatalf("Authorization = %q", got)
	}
}

func TestRejectedAuthLineIsNotSent(t *testing.T) {
	rig := newNamedAuthRig(t, map[string]string{
		"defs.http": `# @auth global command gh cache_key=gh
# @auth global bearer global-token
`,
		"api.http": `# @auth file bearer file-token

### Own line rejected
# @auth use=gh ttl=5m
GET https://example.test/own

### Named definition rejected
# @auth use=gh
GET https://example.test/named
`,
		"broken.http": `# @auth file oauth2 token_url=https://id.example.test scope=read write

### Inherits the rejected file line
GET https://example.test/inherits

### Own auth wins
# @auth bearer own-token
GET https://example.test/own-auth

### Opts out
# @auth none
GET https://example.test/none
`,
	})
	api := rig.parse(t, "api.http")
	broken := rig.parse(t, "broken.http")
	defs := filepath.Join(rig.ws, "defs.http")

	refused := []struct {
		name string
		doc  *restfile.Document
		req  int
		want string
	}{
		{"own line", api, 0, "Set ttl on the definition (" + filepath.Join(rig.ws, "api.http") + ":4)"},
		{"named definition", api, 1, "@auth command gh requires cmd or argv (" + defs + ":1)"},
		{"inherited file line", broken, 0, `got "write"`},
	}
	for _, tt := range refused {
		for _, mode := range []ExecMode{ExecModeSend, ExecModePreview} {
			res := rig.run(t, tt.doc, tt.req, "dev", mode)
			if res.Err == nil || !strings.Contains(res.Err.Error(), tt.want) {
				t.Fatalf("%s (mode %v): error = %v, want %q", tt.name, mode, res.Err, tt.want)
			}
			if rig.sent != nil {
				t.Fatalf("%s (mode %v): request was sent", tt.name, mode)
			}
		}
	}

	if res := rig.run(t, broken, 1, "dev", ExecModeSend); res.Err != nil {
		t.Fatalf("own auth: %v", res.Err)
	}
	if got := rig.header(t, "Authorization"); got != "Bearer own-token" {
		t.Fatalf("own auth header = %q", got)
	}
	if res := rig.run(t, broken, 2, "dev", ExecModeSend); res.Err != nil {
		t.Fatalf("@auth none: %v", res.Err)
	}
	if got := rig.header(t, "Authorization"); got != "" {
		t.Fatalf("@auth none sent %q", got)
	}
}

func TestRejectedGlobalAuthBlocksInheritance(t *testing.T) {
	rig := newNamedAuthRig(t, map[string]string{
		"defs.http":  "# @auth global bearer 'unclosed\n",
		"plain.http": "### Inherits\nGET https://example.test/plain\n",
	})
	doc := rig.doc(t, "plain.http")
	res := rig.run(t, doc, 0, "dev", ExecModeSend)
	if res.Err == nil || !strings.Contains(res.Err.Error(), `@auth is missing a closing "'"`) {
		t.Fatalf("error = %v, want the rejected global line", res.Err)
	}
	if rig.sent != nil {
		t.Fatal("request was sent")
	}
}

func TestRejectedNamedDefinitionBlocksOnlyItsUsers(t *testing.T) {
	for def, want := range map[string]string{
		"# @auth global command gh cmd=\"unterminated\n": `@auth is missing a closing "\""`,
		"# @auth global command gh =x\n":                 `@auth option "=x" has spaces around =`,
	} {
		rig := newNamedAuthRig(t, map[string]string{
			"defs.http": def,
			"api.http":  "### Unrelated\nGET https://example.test/plain\n\n### Uses gh\n# @auth use=gh\nGET https://example.test/gh\n",
		})
		doc := rig.doc(t, "api.http")
		if res := rig.run(t, doc, 0, "dev", ExecModeSend); res.Err != nil {
			t.Fatalf("%q: unrelated request: %v", def, res.Err)
		}
		if got := rig.header(t, "Authorization"); got != "" {
			t.Fatalf("%q: unrelated request sent %q", def, got)
		}
		res := rig.run(t, doc, 1, "dev", ExecModeSend)
		if res.Err == nil || !strings.Contains(res.Err.Error(), want) {
			t.Fatalf("%q: use=gh error = %v, want the rejected definition", def, res.Err)
		}
	}
}

func TestMisplacedGlobalAuthBlocksInheritance(t *testing.T) {
	rig := newNamedAuthRig(t, map[string]string{
		"defs.http":  "### Setup\n# @name setup\n# @auth global bearer defs-token\nGET https://example.test/setup\n",
		"plain.http": "### Inherits\nGET https://example.test/plain\n",
		"own.http":   "# @auth file bearer own-token\n\n### Own\nGET https://example.test/own\n",
	})
	if parser.Check(rig.parse(t, "defs.http")) == nil {
		t.Fatal("expected the misplaced line to be a parse error")
	}
	res := rig.run(t, rig.doc(t, "plain.http"), 0, "dev", ExecModeSend)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "@auth global scope must be declared outside a request") {
		t.Fatalf("error = %v, want the misplaced global line", res.Err)
	}
	if rig.sent != nil {
		t.Fatal("request was sent")
	}
	if res := rig.run(t, rig.doc(t, "own.http"), 0, "dev", ExecModeSend); res.Err != nil {
		t.Fatalf("own auth: %v", res.Err)
	}
	if got := rig.header(t, "Authorization"); got != "Bearer own-token" {
		t.Fatalf("own auth header = %q", got)
	}
}

func TestCommandAuthWithUnknownOptionNeverRuns(t *testing.T) {
	rig := newNamedAuthRig(t, map[string]string{
		"defs.http": "# @auth global command gh cmd=gh --hostname=ghe.example.test\n",
		"api.http":  "### Own line\n# @auth command cmd=mycli --role=admin\nGET https://example.test/own\n\n### Named\n# @auth use=gh\nGET https://example.test/named\n",
	})
	doc := rig.parse(t, "api.http")
	for i, want := range []string{"does not accept --role", "does not accept --hostname"} {
		res := rig.run(t, doc, i, "dev", ExecModeSend)
		if res.Err == nil || !strings.Contains(res.Err.Error(), want) {
			t.Fatalf("request %d: error = %v, want %q", i, res.Err, want)
		}
		if rig.sent != nil {
			t.Fatalf("request %d was sent", i)
		}
	}
	if n := rig.calls.Load(); n != 0 {
		t.Fatalf("command ran %d times", n)
	}
}

func TestApplyCommandAuthWithUnknownOptionNeverRuns(t *testing.T) {
	rig := newNamedAuthRig(t, map[string]string{
		"api.http": `# @patch file role {auth: {type: "command", cmd: "mycli", role: "admin"}}

### Apply
# @apply {auth: {type: "command", cmd: "mycli", role: "admin"}}
GET https://example.test/apply

### Patch profile
# @apply use=role
GET https://example.test/patch
`,
	})
	doc := rig.doc(t, "api.http")
	for i := range doc.Requests {
		res := rig.run(t, doc, i, "dev", ExecModeSend)
		if res.Err == nil || !strings.Contains(res.Err.Error(), "does not accept role") {
			t.Fatalf("request %d: error = %v, want the unknown option", i, res.Err)
		}
		if rig.sent != nil {
			t.Fatalf("request %d was sent", i)
		}
	}
	if n := rig.calls.Load(); n != 0 {
		t.Fatalf("command ran %d times", n)
	}
}

func authStageHas(res engine.RequestResult, status xplain.StageStatus, summary string) bool {
	if res.Explain == nil {
		return false
	}
	for _, st := range res.Explain.Stages {
		if st.Name == xplain.StageAuth && st.Status == status && st.Summary == summary {
			return true
		}
	}
	return false
}
