package collection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestImportBundleHappyPath(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "requests.http", `
# @use ./rts/helpers.rts
GET https://example.com
`)
	writeFile(t, src, "rts/helpers.rts", "module helpers\n")
	writeFile(t, src, "resterm.env.example.json", `{"dev":{"token":"SAFE"}}`+"\n")

	bundle := filepath.Join(t.TempDir(), "bundle")
	if _, err := ExportBundle(ExportOptions{Workspace: src, OutDir: bundle}); err != nil {
		t.Fatalf("export bundle: %v", err)
	}

	dst := filepath.Join(t.TempDir(), "workspace")
	res, err := ImportBundle(ImportOptions{BundleDir: bundle, Workspace: dst})
	if err != nil {
		t.Fatalf("import bundle: %v", err)
	}
	if res.FileCount != 3 {
		t.Fatalf("file count=%d want 3", res.FileCount)
	}
	if res.Created != 3 || res.Overwritten != 0 {
		t.Fatalf("created=%d overwritten=%d", res.Created, res.Overwritten)
	}

	mfData, err := os.ReadFile(filepath.Join(bundle, ManifestFile))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	mf, err := DecodeManifest(mfData)
	if err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	for _, f := range mf.Files {
		srcPath, err := SafeJoin(bundle, f.Path)
		if err != nil {
			t.Fatalf("safe join src %s: %v", f.Path, err)
		}
		dstPath, err := SafeJoin(dst, f.Path)
		if err != nil {
			t.Fatalf("safe join dst %s: %v", f.Path, err)
		}
		srcData, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("read src file %s: %v", f.Path, err)
		}
		dstData, err := os.ReadFile(dstPath)
		if err != nil {
			t.Fatalf("read dst file %s: %v", f.Path, err)
		}
		if string(srcData) != string(dstData) {
			t.Fatalf("file content mismatch for %s", f.Path)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, ManifestFile)); !os.IsNotExist(err) {
		t.Fatalf("manifest file should not be copied into workspace")
	}
}

func TestImportBundleDryRun(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "requests.http", "GET https://example.com\n")

	bundle := filepath.Join(t.TempDir(), "bundle")
	if _, err := ExportBundle(ExportOptions{Workspace: src, OutDir: bundle}); err != nil {
		t.Fatalf("export bundle: %v", err)
	}

	dst := filepath.Join(t.TempDir(), "workspace")
	res, err := ImportBundle(ImportOptions{
		BundleDir: bundle,
		Workspace: dst,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("dry-run import: %v", err)
	}
	if res.FileCount == 0 {
		t.Fatalf("expected planned operations")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("workspace should not be created in dry-run")
	}
}

func TestImportBundleConflictAndForce(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "requests.http", "GET https://example.com\n")

	bundle := filepath.Join(t.TempDir(), "bundle")
	if _, err := ExportBundle(ExportOptions{Workspace: src, OutDir: bundle}); err != nil {
		t.Fatalf("export bundle: %v", err)
	}

	dst := t.TempDir()
	writeFile(t, dst, "requests.http", "OLD")

	_, err := ImportBundle(ImportOptions{BundleDir: bundle, Workspace: dst})
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected conflict error: %v", err)
	}

	res, err := ImportBundle(ImportOptions{
		BundleDir: bundle,
		Workspace: dst,
		Force:     true,
	})
	if err != nil {
		t.Fatalf("force import: %v", err)
	}
	if res.Overwritten == 0 {
		t.Fatalf("expected overwrite operations")
	}

	data, err := os.ReadFile(filepath.Join(dst, "requests.http"))
	if err != nil {
		t.Fatalf("read overwritten file: %v", err)
	}
	if !strings.Contains(string(data), "https://example.com") {
		t.Fatalf("destination file was not overwritten")
	}
}

func TestImportBundleRejectsChecksumMismatch(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "requests.http", "GET https://example.com\n")

	bundle := filepath.Join(t.TempDir(), "bundle")
	if _, err := ExportBundle(ExportOptions{Workspace: src, OutDir: bundle}); err != nil {
		t.Fatalf("export bundle: %v", err)
	}

	path := filepath.Join(bundle, "requests.http")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bundle file: %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("bundle file is unexpectedly empty")
	}
	if raw[0] == 'X' {
		raw[0] = 'Y'
	} else {
		raw[0] = 'X'
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("tamper bundle file: %v", err)
	}

	_, err = ImportBundle(ImportOptions{
		BundleDir: bundle,
		Workspace: t.TempDir(),
	})
	if err == nil {
		t.Fatalf("expected checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImportBundleRejectsTraversalManifest(t *testing.T) {
	bundle := t.TempDir()
	manifest := map[string]any{
		"schema":  SchemaName,
		"version": SchemaVersion,
		"files": []any{
			map[string]any{
				"path": "../evil.txt",
				"role": "asset",
				"size": 1,
				"digest": map[string]any{
					"alg":   "sha256",
					"value": strings.Repeat("a", 64),
				},
			},
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundle, ManifestFile), data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err = ImportBundle(ImportOptions{
		BundleDir: bundle,
		Workspace: t.TempDir(),
	})
	if err == nil {
		t.Fatalf("expected traversal rejection")
	}
	if !strings.Contains(err.Error(), "path escapes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImportBundleRejectsNestedPathConflict(t *testing.T) {
	bundle := t.TempDir()
	manifest := map[string]any{
		"schema":  SchemaName,
		"version": SchemaVersion,
		"files": []any{
			map[string]any{
				"path": "a",
				"role": "asset",
				"size": 1,
				"digest": map[string]any{
					"alg":   "sha256",
					"value": strings.Repeat("a", 64),
				},
			},
			map[string]any{
				"path": "a/b",
				"role": "asset",
				"size": 1,
				"digest": map[string]any{
					"alg":   "sha256",
					"value": strings.Repeat("b", 64),
				},
			},
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundle, ManifestFile), data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err = ImportBundle(ImportOptions{
		BundleDir: bundle,
		Workspace: t.TempDir(),
	})
	if err == nil {
		t.Fatalf("expected nested path conflict rejection")
	}
	if !strings.Contains(err.Error(), "conflicts with nested path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImportBundleRejectsSymlinkEscapeInBundle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}

	bundle := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	link := filepath.Join(bundle, "requests.http")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not available: %v", err)
	}

	sum := SumSHA256([]byte("outside"))
	mf := Manifest{
		Schema:  SchemaName,
		Version: SchemaVersion,
		Files: []File{
			{
				Path:   "requests.http",
				Role:   RoleRequest,
				Size:   int64(len("outside")),
				Digest: sum,
			},
		},
	}
	enc, err := EncodeManifest(mf)
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundle, ManifestFile), enc, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err = ImportBundle(ImportOptions{
		BundleDir: bundle,
		Workspace: t.TempDir(),
	})
	if err == nil {
		t.Fatalf("expected symlink escape rejection")
	}
	if !strings.Contains(err.Error(), "escapes bundle root via symlink") {
		t.Fatalf("unexpected error: %v", err)
	}
}
