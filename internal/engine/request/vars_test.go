package request

import (
	"context"
	"testing"

	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestResolveSSHUsesDocumentGlobalProfiles(t *testing.T) {
	t.Parallel()

	e := New(engcfg.Config{}, nil)
	doc := &restfile.Document{
		SSH: []restfile.SSHProfile{
			{
				Scope: restfile.SSHScopeGlobal,
				Name:  "jump",
				Host:  "127.0.0.1",
			},
		},
	}
	req := &restfile.Request{
		SSH: &restfile.SSHSpec{Use: "jump"},
	}

	plan, err := e.resolveSSH(doc, req, nil, "dev")
	if err != nil {
		t.Fatalf("resolveSSH() error = %v", err)
	}
	if plan == nil || plan.Config == nil {
		t.Fatalf("resolveSSH() returned nil plan")
	}
	if got := plan.Config.Name; got != "jump" {
		t.Fatalf("resolveSSH() name = %q, want %q", got, "jump")
	}
	if got := plan.Config.Host; got != "127.0.0.1" {
		t.Fatalf("resolveSSH() host = %q, want %q", got, "127.0.0.1")
	}
	if got := plan.Config.Label; got != "dev" {
		t.Fatalf("resolveSSH() label = %q, want %q", got, "dev")
	}
}

func TestResolveK8sUsesDocumentGlobalProfiles(t *testing.T) {
	t.Parallel()

	e := New(engcfg.Config{}, nil)
	doc := &restfile.Document{
		K8s: []restfile.K8sProfile{
			{
				Scope:   restfile.K8sScopeGlobal,
				Name:    "cluster-api",
				Pod:     "api-server",
				PortStr: "8080",
			},
		},
	}
	req := &restfile.Request{
		K8s: &restfile.K8sSpec{Use: "cluster-api"},
	}

	plan, err := e.resolveK8s(doc, req, nil, "dev")
	if err != nil {
		t.Fatalf("resolveK8s() error = %v", err)
	}
	if plan == nil || plan.Config == nil {
		t.Fatalf("resolveK8s() returned nil plan")
	}
	if got := plan.Config.Name; got != "cluster-api" {
		t.Fatalf("resolveK8s() name = %q, want %q", got, "cluster-api")
	}
	if got := plan.Config.TargetName; got != "api-server" {
		t.Fatalf("resolveK8s() target = %q, want %q", got, "api-server")
	}
	if got := plan.Config.Port; got != 8080 {
		t.Fatalf("resolveK8s() port = %d, want %d", got, 8080)
	}
	if got := plan.Config.Label; got != "dev" {
		t.Fatalf("resolveK8s() label = %q, want %q", got, "dev")
	}
}

func TestRunRTSApplyUsesDocumentGlobalPatchProfiles(t *testing.T) {
	t.Parallel()

	e := New(engcfg.Config{}, nil)
	doc := &restfile.Document{
		Patches: []restfile.PatchProfile{
			{
				Scope:      restfile.PatchScopeGlobal,
				Name:       "githubCliAuth",
				Expression: `{headers: {"Authorization": "Bearer abc"}}`,
				Line:       1,
				Col:        1,
			},
		},
	}
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
		Metadata: restfile.RequestMetadata{
			Applies: []restfile.ApplySpec{{
				Uses: []string{"githubCliAuth"},
				Line: 2,
				Col:  1,
			}},
		},
	}

	if err := e.runRTSApply(context.Background(), doc, req, "", "", nil); err != nil {
		t.Fatalf("runRTSApply() error = %v", err)
	}
	if got := req.Headers.Get("Authorization"); got != "Bearer abc" {
		t.Fatalf("runRTSApply() authorization = %q, want %q", got, "Bearer abc")
	}
}
