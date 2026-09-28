package registry

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestIndexPatchNamedUsesWorkspaceGlobal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	defs := filepath.Join(dir, "defs.http")
	use := filepath.Join(dir, "use.http")

	if err := os.WriteFile(
		defs,
		[]byte(`# @patch global githubCliAuth {headers: {"Authorization": "Bearer abc"}}`),
		0o644,
	); err != nil {
		t.Fatalf("write defs: %v", err)
	}
	if err := os.WriteFile(use, []byte("GET https://example.com\n"), 0o644); err != nil {
		t.Fatalf("write use: %v", err)
	}

	ix := New()
	ix.Load(dir, false)

	doc := parser.Parse(use, []byte("GET https://example.com\n"))
	pf, ok := ix.PatchNamed(doc, "githubCliAuth")
	if !ok {
		t.Fatalf("expected workspace patch profile")
	}
	if got := pf.Expression; got != `{headers: {"Authorization": "Bearer abc"}}` {
		t.Fatalf("unexpected patch expression %q", got)
	}
}

func TestIndexDefaultAuthUsesCurrentFileFirst(t *testing.T) {
	t.Parallel()

	ix := New()
	ix.Sync(&restfile.Document{
		Path: "/tmp/defs.http",
		Auth: []restfile.AuthProfile{{
			Scope: directive.ScopeGlobal,
			Spec: restfile.AuthSpec{
				Type:   "bearer",
				Params: map[string]string{"token": "global"},
			},
		}},
	})

	doc := &restfile.Document{
		Path: "/tmp/use.http",
		Auth: []restfile.AuthProfile{{
			Scope: directive.ScopeFile,
			Spec: restfile.AuthSpec{
				Type:   "bearer",
				Params: map[string]string{"token": "file"},
			},
		}},
	}

	pf, ok := ix.DefaultAuth(doc)
	if !ok {
		t.Fatalf("expected inherited auth")
	}
	if got := pf.Spec.Params["token"]; got != "file" {
		t.Fatalf("unexpected auth token %q", got)
	}
}

func TestIndexSSHUsesCurrentDocOverStoredFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "ssh.http")
	if err := os.WriteFile(
		path,
		[]byte(`# @ssh global jump host=old.example.com`),
		0o644,
	); err != nil {
		t.Fatalf("write ssh: %v", err)
	}

	ix := New()
	ix.Load(dir, false)

	doc := &restfile.Document{
		Path: path,
		SSH: []restfile.SSHProfile{{
			Scope: directive.ScopeGlobal,
			Name:  "jump",
			Host:  "new.example.com",
		}},
	}

	_, gs := ix.SSH(doc)
	if len(gs) != 1 {
		t.Fatalf("expected one ssh global, got %d", len(gs))
	}
	if got := gs[0].Host; got != "new.example.com" {
		t.Fatalf("unexpected ssh host %q", got)
	}
}

func TestIndexSSHUsesWorkspaceGlobal(t *testing.T) {
	t.Parallel()

	ix := New()
	ix.Sync(&restfile.Document{
		Path: "/tmp/defs.http",
		SSH: []restfile.SSHProfile{{
			Scope: directive.ScopeGlobal,
			Name:  "jump",
			Host:  "jump.example.com",
		}},
	})

	fs, gs := ix.SSH(&restfile.Document{Path: "/tmp/use.http"})
	if len(fs) != 0 {
		t.Fatalf("expected no file ssh profiles, got %d", len(fs))
	}
	if len(gs) != 1 {
		t.Fatalf("expected one workspace ssh profile, got %d", len(gs))
	}
	if got := gs[0].Host; got != "jump.example.com" {
		t.Fatalf("unexpected ssh host %q", got)
	}
}

func TestIndexK8sUsesWorkspaceGlobal(t *testing.T) {
	t.Parallel()

	ix := New()
	ix.Sync(&restfile.Document{
		Path: "/tmp/defs.http",
		K8s: []restfile.K8sProfile{{
			Scope:   directive.ScopeGlobal,
			Name:    "cluster",
			Target:  "service:api",
			PortStr: "http",
		}},
	})

	fs, gs := ix.K8s(&restfile.Document{Path: "/tmp/use.http"})
	if len(fs) != 0 {
		t.Fatalf("expected no file k8s profiles, got %d", len(fs))
	}
	if len(gs) != 1 {
		t.Fatalf("expected one workspace k8s profile, got %d", len(gs))
	}
	if got := gs[0].Name; got != "cluster" {
		t.Fatalf("unexpected k8s profile %q", got)
	}
}

func TestIndexPatchNamedIsDeterministicAcrossFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := filepath.Join(dir, "a.http")
	b := filepath.Join(dir, "b.http")
	c := filepath.Join(dir, "c.http")

	if err := os.WriteFile(
		a,
		[]byte(`# @patch global same {headers: {"X-From": "a"}}`),
		0o644,
	); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(
		b,
		[]byte(`# @patch global same {headers: {"X-From": "b"}}`),
		0o644,
	); err != nil {
		t.Fatalf("write b: %v", err)
	}
	if err := os.WriteFile(c, []byte("GET https://example.com\n"), 0o644); err != nil {
		t.Fatalf("write c: %v", err)
	}

	ix := New()
	ix.Load(dir, false)

	doc := parser.Parse(c, []byte("GET https://example.com\n"))
	pf, ok := ix.PatchNamed(doc, "same")
	if !ok {
		t.Fatalf("expected named patch")
	}
	if got := pf.Expression; got != `{headers: {"X-From": "a"}}` {
		t.Fatalf("unexpected patch expression %q", got)
	}
}

// A rejected line counts as a default where a default would have applied, and
// nowhere else.
func TestIndexDefaultAuthCountsRejectedDefinitions(t *testing.T) {
	t.Parallel()

	ix := New()
	ix.Sync(parser.Parse("/tmp/fail/defs.http", []byte("# @auth global command gh name=other cmd=x\n")))

	plain := parser.Parse("/tmp/fail/plain.http", []byte("GET https://example.com\n"))
	if pf, ok := ix.DefaultAuth(plain); !ok || pf.Name != "other" || pf.Spec.Rejected == "" {
		t.Fatalf("default = %+v, want the rejected global", pf)
	}

	own := parser.Parse("/tmp/fail/own.http", []byte("# @auth file bearer own\n"))
	if pf, ok := ix.DefaultAuth(own); !ok || pf.Spec.Params["token"] != "own" {
		t.Fatalf("default = %+v, want the file default over a global", pf)
	}

	later := parser.Parse(
		"/tmp/fail/later.http",
		[]byte("# @auth file command gh name=x cmd=y\n# @auth file bearer later\n"),
	)
	if pf, ok := ix.DefaultAuth(later); !ok || pf.Spec.Params["token"] != "later" {
		t.Fatalf("default = %+v, want the later line in the same file", pf)
	}
}

// Names come in lookup order, so each one is the profile use= resolves.
func TestIndexNamesFollowLookupOrder(t *testing.T) {
	t.Parallel()

	defs := parser.Parse("/tmp/names/defs.http", []byte(`# @auth global command name=gh cmd=a
# @auth file command name=private cmd=b
# @auth global command name=Shared cmd=c
# @patch global jsonApi {headers: {"X-From": "defs"}}
# @patch file hidden {headers: {"X-From": "defs"}}
# @ssh global bastion host=jump.example.com user=ops
# @k8s global cluster namespace=default service=api port=http
`))
	saved := parser.Parse("/tmp/names/use.http", []byte("# @auth global command name=stale cmd=x\n"))
	use := parser.Parse("/tmp/names/use.http", []byte(`# @auth file command name=shared cmd=d
# @auth file command cmd=default
`))
	ix := New()
	ix.Sync(defs)
	ix.Sync(saved)

	if got, want := ix.AuthNames(use), []string{"shared", "gh"}; !slices.Equal(got, want) {
		t.Fatalf("auth names = %v, want %v", got, want)
	}
	for _, n := range ix.AuthNames(use) {
		if _, ok := ix.AuthNamed(use, n); !ok {
			t.Fatalf("listed auth name %q does not resolve", n)
		}
	}
	if got, want := ix.PatchNames(use), []string{"jsonApi"}; !slices.Equal(got, want) {
		t.Fatalf("patch names = %v, want %v", got, want)
	}
	if got, want := ix.SSHNames(use), []string{"bastion"}; !slices.Equal(got, want) {
		t.Fatalf("ssh names = %v, want %v", got, want)
	}
	if got, want := ix.K8sNames(use), []string{"cluster"}; !slices.Equal(got, want) {
		t.Fatalf("k8s names = %v, want %v", got, want)
	}

	var none *Index
	if got, want := none.AuthNames(use), []string{"shared"}; !slices.Equal(got, want) {
		t.Fatalf("names without an index = %v, want %v", got, want)
	}
}

func TestIndexAuthNamed(t *testing.T) {
	t.Parallel()

	defs := parser.Parse("/tmp/defs.http", []byte(`# @auth global command name=gh cmd="gh auth token --hostname global"
# @auth file command name=private cmd="private-token"
# @auth global command name=other cmd="other-token"
`))
	use := parser.Parse("/tmp/use.http", []byte(`# @auth file command name=GH cmd="gh auth token --hostname file"
# @auth file command cmd="default-token"
`))
	ix := New()
	ix.Sync(defs)

	for _, idx := range []*Index{ix, nil} {
		pf, ok := idx.AuthNamed(use, "gh")
		if !ok || pf.Spec.Params["cmd"] != "gh auth token --hostname file" {
			t.Fatalf("expected the current file's profile to win, got %+v", pf)
		}
		if pf.Spec.SourcePath != "/tmp/use.http" {
			t.Fatalf("expected the definition's path, got %q", pf.Spec.SourcePath)
		}
		if _, ok := idx.AuthNamed(use, ""); ok {
			t.Fatal("an empty name must not match the unnamed default")
		}
	}

	if pf, ok := ix.AuthNamed(use, "OTHER"); !ok || pf.Spec.SourcePath != "/tmp/defs.http" {
		t.Fatalf("expected a case-insensitive global from another file, got %+v", pf)
	}
	if _, ok := ix.AuthNamed(use, "private"); ok {
		t.Fatal("a file-scoped profile must stay private to its file")
	}
	if pf, ok := ix.DefaultAuth(use); !ok || pf.Spec.Params["cmd"] != "default-token" {
		t.Fatalf("expected the unnamed profile as the default, got %+v", pf)
	}
}
