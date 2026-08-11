package request

import (
	"context"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/directive"
	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestResolveHTTPOptionsFallbackEnvEnable(t *testing.T) {
	e := New(engcfg.Config{
		FilePath:      "/tmp/request.http",
		WorkspaceRoot: "/workspace",
	}, nil)
	opts := httpx.Options{FallbackBaseDirs: []string{"/extra"}}

	t.Setenv("RESTERM_ENABLE_FALLBACK", "true")
	resolved := e.resolveHTTPOptions(nil, opts)
	if len(resolved.FallbackBaseDirs) == 0 {
		t.Fatalf("expected fallbacks enabled, got %v", resolved.FallbackBaseDirs)
	}
	if resolved.NoFallback {
		t.Fatalf("expected NoFallback to be false when enabled")
	}
}

func TestResolveSSHUsesDocumentGlobalProfiles(t *testing.T) {
	t.Parallel()

	e := New(engcfg.Config{}, nil)
	doc := &restfile.Document{
		SSH: []restfile.SSHProfile{
			{
				Scope: directive.ScopeGlobal,
				Name:  "jump",
				Host:  "127.0.0.1",
			},
		},
	}
	req := &restfile.Request{
		SSH: &restfile.SSHSpec{Use: "jump"},
	}

	plan, err := e.resolveSSH(doc, req, nil, testEnv("dev"))
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
				Scope:   directive.ScopeGlobal,
				Name:    "cluster-api",
				Pod:     "api-server",
				PortStr: "8080",
			},
		},
	}
	req := &restfile.Request{
		K8s: &restfile.K8sSpec{Use: "cluster-api"},
	}

	plan, err := e.resolveK8s(doc, req, nil, testEnv("dev"))
	if err != nil {
		t.Fatalf("resolveK8s() error = %v", err)
	}
	if plan == nil || plan.Config == nil {
		t.Fatalf("resolveK8s() returned nil plan")
	}
	if got := plan.Config.Name; got != "cluster-api" {
		t.Fatalf("resolveK8s() name = %q, want %q", got, "cluster-api")
	}
	if got := plan.Config.Target.Name; got != "api-server" {
		t.Fatalf("resolveK8s() target = %q, want %q", got, "api-server")
	}
	if got := plan.Config.Port.Number; got != 8080 {
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
				Scope:      directive.ScopeGlobal,
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

	if err := e.runRTSApply(
		context.Background(),
		doc,
		req,
		testEnv(""),
		"",
		nil,
		rts.Locals{},
	); err != nil {
		t.Fatalf("runRTSApply() error = %v", err)
	}
	if got := req.Headers.Get("Authorization"); got != "Bearer abc" {
		t.Fatalf("runRTSApply() authorization = %q, want %q", got, "Bearer abc")
	}
}

func TestProvidersExpandDeclaredVariableTemplates(t *testing.T) {
	t.Parallel()

	e := New(engcfg.Config{}, nil)
	doc := &restfile.Document{
		Variables: []restfile.Variable{{Name: "file.greeting", Value: "hello {{name}}"}},
	}
	req := &restfile.Request{
		Variables: []restfile.Variable{{Name: "trace.id", Value: "{{$uuid}}"}},
	}
	globs := map[string]vars.GlobalMutation{
		"captured": {Name: "captured", Value: "{{file.greeting}}"},
	}
	plan := e.buildVariablePlan(varSources{
		doc:     doc,
		req:     req,
		env:     testEnv("dev"),
		globals: globs,
		sec:     keepSecrets,
		run:     runVars{scripts: map[string]string{"name": "resterm"}},
	})
	res := vars.NewResolver(plan.providers()...)

	greeting, err := res.ExpandTemplates("{{file.greeting}}")
	if err != nil {
		t.Fatalf("expand err: %v", err)
	}
	if greeting != "hello resterm" {
		t.Fatalf("expected nested file variable expansion, got %q", greeting)
	}

	first, err := res.ExpandTemplates("{{trace.id}}")
	if err != nil {
		t.Fatalf("expand err: %v", err)
	}
	second, err := res.ExpandTemplates("{{trace.id}}")
	if err != nil {
		t.Fatalf("expand err: %v", err)
	}
	if first == "" || first != second {
		t.Fatalf("expected stable request variable value, got %q vs %q", first, second)
	}
	if strings.Contains(first, "{{") {
		t.Fatalf("expected dynamic to expand, got %q", first)
	}

	captured, err := res.ExpandTemplates("{{captured}}")
	if err != nil {
		t.Fatalf("expand err: %v", err)
	}
	if captured != "{{file.greeting}}" {
		t.Fatalf("expected runtime global to stay verbatim, got %q", captured)
	}
}
