package registry

import (
	"os"
	"path/filepath"
	"testing"

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
			Scope: restfile.AuthScopeGlobal,
			Spec: restfile.AuthSpec{
				Type:   "bearer",
				Params: map[string]string{"token": "global"},
			},
		}},
	})

	doc := &restfile.Document{
		Path: "/tmp/use.http",
		Auth: []restfile.AuthProfile{{
			Scope: restfile.AuthScopeFile,
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
			Scope: restfile.SSHScopeGlobal,
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
			Scope: restfile.SSHScopeGlobal,
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
			Scope:   restfile.K8sScopeGlobal,
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
