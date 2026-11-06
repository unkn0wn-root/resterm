package collection

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
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
# @grpc accounts.UserService/GetUser
# @grpc-descriptor ./descriptors/service.protoset
GRPC localhost:8080

< ./grpc/message.json

### WS
# @websocket
# @ws send-file < ./ws/frame.bin
GET ws://localhost/ws

### Mock
# @mock method=GET path=/fixture default=true
HTTP/1.1 200 OK

< ./payloads/mock.json
`)
	writeFile(t, ws, "rts/helpers.rts", "module helpers\n")
	writeFile(t, ws, "scripts/test.js", "tests.test('ok', () => true)")
	writeFile(t, ws, "payloads/body.json", `{"hello":"world"}`)
	writeFile(t, ws, "queries/query.graphql", "query Q { ping }")
	writeFile(t, ws, "queries/vars.json", `{"a":1}`)
	writeFile(t, ws, "descriptors/service.protoset", "proto-binary")
	writeFile(t, ws, "grpc/message.json", `{"id":"42"}`)
	writeFile(t, ws, "ws/frame.bin", "frame-bytes")
	writeFile(t, ws, "payloads/mock.json", `{"mock":true}`)
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
	res, err := ExportBundle(t.Context(), ExportOptions{
		Workspace: ws,
		OutDir:    out,
		Recursive: true,
	})
	if err != nil {
		t.Fatalf("export bundle: %v", err)
	}

	if res.FileCount != 11 {
		t.Fatalf("file count=%d want 11", res.FileCount)
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
	if len(mf.Files) != 11 {
		t.Fatalf("manifest file entries=%d want 11", len(mf.Files))
	}

	roleByPath := map[string]FileRole{
		"requests.http":                RoleRequest,
		"rts/helpers.rts":              RoleScript,
		"scripts/test.js":              RoleScript,
		"payloads/body.json":           RoleAsset,
		"payloads/mock.json":           RoleAsset,
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
	if _, err := ExportBundle(t.Context(), ExportOptions{Workspace: ws, OutDir: out}); err != nil {
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
	_, err := ExportBundle(t.Context(), ExportOptions{Workspace: ws, OutDir: out})
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
	_, err := ExportBundle(t.Context(), ExportOptions{Workspace: ws, OutDir: out})
	if err == nil {
		t.Fatalf("expected error for missing dependency")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExportBundleIncludesProtoSources(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "requests.http", `
# @grpc acme.Svc/Call
# @grpc-proto ./proto/svc.proto
GRPC localhost:8080
`)
	writeFile(t, ws, "proto/svc.proto", `syntax = "proto3";
package acme;
import "common.proto";
import "google/protobuf/empty.proto";
service Svc {
  rpc Call(google.protobuf.Empty) returns (Common);
}
`)
	writeFile(t, ws, "proto/common.proto", "syntax = \"proto3\";\npackage acme;\nmessage Common {}\n")
	writeFile(t, ws, "proto/unused.proto", "syntax = \"proto3\";\npackage acme;\nmessage Unused {}\n")

	out := filepath.Join(t.TempDir(), "bundle")
	res, err := ExportBundle(t.Context(), ExportOptions{Workspace: ws, OutDir: out, ProtoSources: protoSources})
	if err != nil {
		t.Fatalf("export bundle: %v", err)
	}
	data, err := os.ReadFile(res.ManifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	mf, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("decode manifest: %v", err)
	}

	got := make(map[string]FileRole, len(mf.Files))
	for _, f := range mf.Files {
		got[f.Path] = f.Role
	}
	want := map[string]FileRole{
		"requests.http":        RoleRequest,
		"proto/svc.proto":      RoleAsset,
		"proto/common.proto":   RoleAsset,
		defaultEnvTemplateFile: RoleEnvTemplate,
	}
	if !maps.Equal(got, want) {
		t.Fatalf("bundle files = %v, want %v", got, want)
	}
}

func TestExportBundleRejectsMissingProtoImport(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "requests.http", `
# @grpc acme.Svc/Call
# @grpc-proto ./proto/svc.proto
GRPC localhost:8080
`)
	writeFile(t, ws, "proto/svc.proto", "syntax = \"proto3\";\nimport \"missing.proto\";\n")

	out := filepath.Join(t.TempDir(), "bundle")
	_, err := ExportBundle(t.Context(), ExportOptions{Workspace: ws, OutDir: out, ProtoSources: protoSources})
	if err == nil {
		t.Fatal("expected error for a missing proto import")
	}
	if !strings.Contains(err.Error(), "requests.http") || !strings.Contains(err.Error(), "missing.proto") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExportBundleIgnoresProtoImportsInTheWorkingDir(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "requests.http", `
# @grpc acme.Svc/Call
# @grpc-proto ./proto/svc.proto
GRPC localhost:8080
`)
	writeFile(t, ws, "proto/svc.proto", "syntax = \"proto3\";\nimport \"common.proto\";\n")
	cwd := t.TempDir()
	writeFile(t, cwd, "proto/common.proto", "syntax = \"proto3\";\n")
	t.Chdir(cwd)

	out := filepath.Join(t.TempDir(), "bundle")
	_, err := ExportBundle(t.Context(), ExportOptions{Workspace: ws, OutDir: out, ProtoSources: protoSources})
	if err == nil {
		t.Fatal("expected the import missing from the workspace to fail the export")
	}
	if !strings.Contains(err.Error(), "svc.proto:2:8:") || !strings.Contains(err.Error(), "common.proto") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExportBundleForce(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "requests.http", "POST https://example.com\n\n< ./payloads/body.json\n")
	writeFile(t, ws, "payloads/body.json", `{"a":1}`)
	bundle := filepath.Join(t.TempDir(), "bundle")
	if _, err := ExportBundle(t.Context(), ExportOptions{Workspace: ws, OutDir: bundle}); err != nil {
		t.Fatal(err)
	}
	want := slices.Sorted(maps.Keys(readTree(t, os.DirFS(bundle))))

	tests := []struct {
		name   string
		bundle bool
		files  map[string]string
		links  map[string]string
		force  bool
		err    string
	}{
		{name: "new folder"},
		{name: "new folder with force", force: true},
		{name: "bundle", bundle: true, files: map[string]string{"out/old.txt": "old"}, force: true},
		{name: "bundle without force", bundle: true, err: "already exists"},
		{name: "file without force", files: map[string]string{"out": "notes"}, err: "already exists"},
		{name: "file", files: map[string]string{"out": "notes"}, force: true, err: "is not a resterm bundle"},
		{
			name:  "folder that is not a bundle",
			files: map[string]string{"out/todo.txt": "keep", "out/.git/HEAD": "ref"},
			force: true,
			err:   "is not a resterm bundle",
		},
		{
			name:  "web manifest",
			files: map[string]string{"out/manifest.json": `{"name":"app","start_url":"/"}`, "out/index.html": "<p>"},
			force: true,
			err:   "is not a resterm bundle",
		},
		{
			name:  "manifest link",
			files: map[string]string{"out/todo.txt": "keep"},
			links: map[string]string{"out/manifest.json": filepath.Join(bundle, ManifestFile)},
			force: true,
			err:   "is not a resterm bundle",
		},
		{name: "link without force", links: map[string]string{"out": bundle}, err: "already exists"},
		{name: "link to a bundle", links: map[string]string{"out": bundle}, force: true, err: "is a link"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, "out")
			if tt.bundle {
				if _, err := ExportBundle(t.Context(), ExportOptions{Workspace: ws, OutDir: out}); err != nil {
					t.Fatal(err)
				}
			}
			for name, data := range tt.files {
				writeFile(t, dir, name, data)
			}
			for name, target := range tt.links {
				if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
					t.Skipf("cannot create a link: %v", err)
				}
			}
			before := readTree(t, os.DirFS(dir))

			_, err := ExportBundle(t.Context(), ExportOptions{Workspace: ws, OutDir: out, Force: tt.force})
			if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
				t.Fatalf("ReadDir(%s) = %v, %v, want only out", dir, entries, err)
			}
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("ExportBundle = %v, want an error with %q", err, tt.err)
				}
				if got := readTree(t, os.DirFS(dir)); !maps.Equal(got, before) {
					t.Fatalf("files changed to %v, want %v", got, before)
				}
				for name, target := range tt.links {
					if got, err := os.Readlink(filepath.Join(dir, name)); err != nil || got != target {
						t.Fatalf("Readlink(%s) = %q, %v, want %q", name, got, err, target)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := slices.Sorted(maps.Keys(readTree(t, os.DirFS(out)))); !slices.Equal(got, want) {
				t.Fatalf("bundle files %v, want %v", got, want)
			}
			if got := fileMode(t, out).Perm(); runtime.GOOS != "windows" && got != 0o700 {
				t.Fatalf("bundle mode = %v, want 0700", got)
			}
			for _, name := range []string{"payloads", "payloads/body.json"} {
				got, want := fileMode(t, filepath.Join(out, name)), fileMode(t, filepath.Join(ws, name))
				if got != want {
					t.Fatalf("%s mode = %v, want %v like the workspace", name, got, want)
				}
			}
		})
	}
}

func TestExportBundleReadOnly(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("read-only folders do not stop root or Windows")
	}
	ws := t.TempDir()
	writeFile(t, ws, "requests.http", "POST https://example.com\n\n< ./payloads/body.json\n")
	writeFile(t, ws, "payloads/body.json", `{"a":1}`)

	tests := []struct {
		name string
		lock string
		err  string
		ok   bool
	}{
		{name: "parent", lock: ".", err: "create temp dir"},
		{name: "bundle", lock: "out", err: "move old bundle aside"},
		{name: "folder in the bundle", lock: "out/payloads", ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, "out")
			if _, err := ExportBundle(t.Context(), ExportOptions{Workspace: ws, OutDir: out}); err != nil {
				t.Fatal(err)
			}
			writeFile(t, out, "old.txt", "old")
			if err := os.Chmod(filepath.Join(dir, tt.lock), 0o555); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
					if err == nil && d.IsDir() {
						_ = os.Chmod(p, 0o755)
					}
					return nil
				})
			})
			before := readTree(t, os.DirFS(out))

			_, err := ExportBundle(t.Context(), ExportOptions{Workspace: ws, OutDir: out, Force: true})
			if tt.ok {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(out, "old.txt")); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("Stat(old.txt) = %v, want it gone with the old bundle", err)
				}
				return
			}
			if !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), tt.err) {
				t.Fatalf("ExportBundle = %v, want a permission error at %s", err, tt.err)
			}
			if got := readTree(t, os.DirFS(out)); !maps.Equal(got, before) {
				t.Fatalf("files changed to %v, want %v", got, before)
			}
			if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
				t.Fatalf("ReadDir(%s) = %v, %v, want only out", dir, entries, err)
			}
		})
	}
}

func TestReplaceDirRestoresBundle(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("read-only folders do not stop root or Windows")
	}
	ws := t.TempDir()
	writeFile(t, ws, "requests.http", "GET https://example.com\n")

	readOnly := func(dir string) error { return os.Chmod(dir, 0o555) }
	tests := []struct {
		name   string
		bundle bool
		fill   func(dir string) error
		err    error
	}{
		{name: "fill fails", bundle: true, fill: func(string) error { return io.ErrShortWrite }, err: io.ErrShortWrite},
		{name: "move fails", bundle: true, fill: readOnly, err: fs.ErrPermission},
		{name: "move to a new path fails", fill: readOnly, err: fs.ErrPermission},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, "out")
			if tt.bundle {
				if _, err := ExportBundle(t.Context(), ExportOptions{Workspace: ws, OutDir: out}); err != nil {
					t.Fatal(err)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			files := readTree(t, os.DirFS(dir))

			if err := replaceDir(t.Context(), out, true, tt.fill); !errors.Is(err, tt.err) {
				t.Fatalf("replaceDir = %v, want %v", err, tt.err)
			}
			if got := readTree(t, os.DirFS(dir)); !maps.Equal(got, files) {
				t.Fatalf("files changed to %v, want %v", got, files)
			}
			if got, err := os.ReadDir(dir); err != nil || len(got) != len(entries) {
				t.Fatalf("ReadDir(%s) = %v, %v, want %v", dir, got, err, entries)
			}
		})
	}
}

func protoSources(gr *restfile.GRPCRequest, dir string) ([]string, error) {
	return grpcx.ProtoSources(gr, grpcx.Options{BaseDir: dir, NoFallback: true})
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

func fileMode(t *testing.T, path string) fs.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode()
}

func readTree(t *testing.T, fsys fs.FS) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		data, err := fs.ReadFile(fsys, name)
		files[name] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
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
