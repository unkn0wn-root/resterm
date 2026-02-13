package collection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportBundleCollectsDepsAndEnvTemplate(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "requests.http", `
# @use ./rts/helpers.rts

### HTTP Include
# @script test js
> < ./scripts/test.js
POST https://example.com/items
Content-Type: application/json

@ ./payloads/body.json

### GraphQL
# @graphql
# @query < ./queries/query.graphql
# @variables < ./queries/vars.json
POST https://example.com/graphql
Content-Type: application/json

### GRPC
# @grpc foo.Bar/Baz
# @grpc-descriptor ./descriptors/service.protoset
GRPC localhost:8080

< ./grpc/message.json

### WS
# @websocket
# @ws send-file < ./ws/frame.bin
GET ws://localhost/ws
`)
	writeFile(t, ws, "rts/helpers.rts", "module helpers\n")
	writeFile(t, ws, "scripts/test.js", "tests.test('ok', () => true)")
	writeFile(t, ws, "payloads/body.json", `{"hello":"world"}`)
	writeFile(t, ws, "queries/query.graphql", "query Q { ping }")
	writeFile(t, ws, "queries/vars.json", `{"a":1}`)
	writeFile(t, ws, "descriptors/service.protoset", "proto-binary")
	writeFile(t, ws, "grpc/message.json", `{"id":"42"}`)
	writeFile(t, ws, "ws/frame.bin", "frame-bytes")
	writeFile(t, ws, "resterm.env.json", `
{
  "dev": {
    "token": "secret-123",
    "number": 123,
    "enabled": true,
    "nested": {
      "key": "value"
    },
    "items": [1, {"x":"y"}]
  }
}
`)

	out := filepath.Join(t.TempDir(), "bundle")
	res, err := ExportBundle(ExportOptions{
		Workspace: ws,
		OutDir:    out,
		Recursive: true,
	})
	if err != nil {
		t.Fatalf("export bundle: %v", err)
	}

	if res.FileCount != 10 {
		t.Fatalf("file count=%d want 10", res.FileCount)
	}
	if res.ManifestPath != filepath.Join(out, ManifestFile) {
		t.Fatalf("manifest path=%q want %q", res.ManifestPath, filepath.Join(out, ManifestFile))
	}

	manifestData, err := os.ReadFile(res.ManifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	mf, err := DecodeManifest(manifestData)
	if err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(mf.Files) != 10 {
		t.Fatalf("manifest file entries=%d want 10", len(mf.Files))
	}

	roleByPath := map[string]FileRole{
		"requests.http":                RoleRequest,
		"rts/helpers.rts":              RoleScript,
		"scripts/test.js":              RoleScript,
		"payloads/body.json":           RoleAsset,
		"queries/query.graphql":        RoleAsset,
		"queries/vars.json":            RoleAsset,
		"descriptors/service.protoset": RoleAsset,
		"grpc/message.json":            RoleAsset,
		"ws/frame.bin":                 RoleAsset,
		defaultEnvTemplateFile:         RoleEnvTemplate,
	}

	for _, f := range mf.Files {
		wantRole, ok := roleByPath[f.Path]
		if !ok {
			t.Fatalf("unexpected file in manifest: %s", f.Path)
		}
		if f.Role != wantRole {
			t.Fatalf("role for %s=%q want %q", f.Path, f.Role, wantRole)
		}
		dst, err := SafeJoin(out, f.Path)
		if err != nil {
			t.Fatalf("safe join %s: %v", f.Path, err)
		}
		data, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("read bundle file %s: %v", f.Path, err)
		}
		if int64(len(data)) != f.Size {
			t.Fatalf("size for %s=%d want %d", f.Path, f.Size, len(data))
		}
		if !VerifyDigest(data, f.Digest) {
			t.Fatalf("digest mismatch for %s", f.Path)
		}
	}

	envData, err := os.ReadFile(filepath.Join(out, defaultEnvTemplateFile))
	if err != nil {
		t.Fatalf("read env template: %v", err)
	}
	var env any
	if err := json.Unmarshal(envData, &env); err != nil {
		t.Fatalf("decode env template: %v", err)
	}
	assertRedactedLeaves(t, env)
}

func TestExportBundlePrefersExistingEnvExample(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "requests.http", "GET https://example.com\n")
	writeFile(t, ws, defaultEnvSourceFile, `{"dev":{"token":"secret"}}`)
	writeFile(t, ws, defaultEnvTemplateFile, `{"dev":{"token":"SAFE"}}`+"\n")

	out := filepath.Join(t.TempDir(), "bundle")
	if _, err := ExportBundle(ExportOptions{Workspace: ws, OutDir: out}); err != nil {
		t.Fatalf("export bundle: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(out, defaultEnvTemplateFile))
	if err != nil {
		t.Fatalf("read exported env template: %v", err)
	}
	want, err := os.ReadFile(filepath.Join(ws, defaultEnvTemplateFile))
	if err != nil {
		t.Fatalf("read source env example: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("env example should be copied as-is\ngot:  %s\nwant: %s", got, want)
	}
}

func TestExportBundleRejectsOutsideDependency(t *testing.T) {
	ws := t.TempDir()
	parent := filepath.Dir(ws)
	writeFile(t, parent, "outside.json", `{"secret":"x"}`)
	writeFile(t, ws, "requests.http", `
POST https://example.com
Content-Type: application/json

@ ../outside.json
`)

	out := filepath.Join(t.TempDir(), "bundle")
	_, err := ExportBundle(ExportOptions{Workspace: ws, OutDir: out})
	if err == nil {
		t.Fatalf("expected error for outside dependency")
	}
	if !strings.Contains(err.Error(), "outside workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExportBundleRejectsMissingDependency(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "requests.http", `
POST https://example.com
Content-Type: application/json

@ ./missing.json
`)

	out := filepath.Join(t.TempDir(), "bundle")
	_, err := ExportBundle(ExportOptions{Workspace: ws, OutDir: out})
	if err == nil {
		t.Fatalf("expected error for missing dependency")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExportBundleForceOverwrite(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "requests.http", "GET https://example.com\n")

	out := filepath.Join(t.TempDir(), "bundle")
	writeFile(t, filepath.Dir(out), filepath.Base(out), "old")

	if _, err := ExportBundle(ExportOptions{Workspace: ws, OutDir: out}); err == nil {
		t.Fatalf("expected error when output already exists")
	}
	if _, err := ExportBundle(ExportOptions{
		Workspace: ws,
		OutDir:    out,
		Force:     true,
	}); err != nil {
		t.Fatalf("force export should succeed: %v", err)
	}
}

func writeFile(t *testing.T, root, rel, data string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func assertRedactedLeaves(t *testing.T, v any) {
	t.Helper()
	switch cur := v.(type) {
	case map[string]any:
		for _, child := range cur {
			assertRedactedLeaves(t, child)
		}
	case []any:
		for _, child := range cur {
			assertRedactedLeaves(t, child)
		}
	default:
		s, ok := cur.(string)
		if !ok {
			t.Fatalf("expected redacted leaf string, got %T (%v)", cur, cur)
		}
		if s != envPlaceholder {
			t.Fatalf("expected redacted leaf %q, got %q", envPlaceholder, s)
		}
	}
}
