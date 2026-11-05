package headless

import (
	"slices"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestEngineSecretsCoversFile(t *testing.T) {
	t.Setenv("RST441_DEV_TOK", "dev-env-s3c")
	t.Setenv("RST441_STAGE_TOK", "stage-env-s3c")
	t.Setenv("RST441_REQ_TOK", "req-env-s3c")
	t.Setenv("RST441_DOT", "dot-plain")
	src := "# @file-secret fs file-s3c\n\n" +
		"### One\n# @name one\nGET https://api.example.com/one?d={{env.RST441_DOT}}\n\n" +
		"### Two\n# @name two\n# @request-secret tok req-s3c\n# @request rt env:RST441_REQ_TOK\nGET https://api.example.com/two\n\n" +
		"### Three\n# @name three\n# @compare dev stage\nGET https://api.example.com/three\n"
	doc := parser.Parse("secrets.http", []byte(src))
	cat, err := vars.NewCatalog(vars.EnvironmentSet{
		"dev":   {"tok": "env:RST441_DEV_TOK"},
		"stage": {"tok": "env:RST441_STAGE_TOK"},
	})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	env, err := cat.Resolve(cat.DefaultSelection())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	e := New(engine.Config{FilePath: "secrets.http", Catalog: cat})
	t.Cleanup(func() { _ = e.Close() })

	got := e.Secrets(doc, env)
	for _, want := range []string{"file-s3c", "req-s3c", "req-env-s3c", "dev-env-s3c", "stage-env-s3c"} {
		if !slices.Contains(got, want) {
			t.Errorf("Secrets() = %q, want %q", got, want)
		}
	}
	if slices.Contains(got, "dot-plain") {
		t.Errorf("Secrets() = %q, want {{env.NAME}} values left out", got)
	}
}
