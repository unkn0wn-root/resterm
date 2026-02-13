package collection

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackUnpackBundleRoundTrip(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "requests.http", "GET https://example.com\n")
	writeFile(t, ws, "resterm.env.example.json", "{\"dev\":{\"token\":\"SAFE\"}}\n")

	bund := filepath.Join(t.TempDir(), "bundle")
	exp, err := ExportBundle(ExportOptions{Workspace: ws, OutDir: bund})
	if err != nil {
		t.Fatalf("export bundle: %v", err)
	}

	arc := filepath.Join(t.TempDir(), "bundle.zip")
	pk, err := PackBundle(PackOptions{BundleDir: bund, OutFile: arc})
	if err != nil {
		t.Fatalf("pack bundle: %v", err)
	}
	if pk.FileCount != exp.FileCount {
		t.Fatalf("packed count=%d want %d", pk.FileCount, exp.FileCount)
	}

	out := filepath.Join(t.TempDir(), "bundle-unpacked")
	un, err := UnpackBundle(UnpackOptions{InFile: arc, OutDir: out})
	if err != nil {
		t.Fatalf("unpack bundle: %v", err)
	}
	if un.FileCount != exp.FileCount {
		t.Fatalf("unpacked count=%d want %d", un.FileCount, exp.FileCount)
	}

	if _, err := os.Stat(filepath.Join(out, ManifestFile)); err != nil {
		t.Fatalf("expected manifest in unpacked bundle: %v", err)
	}

	if _, err := ImportBundle(ImportOptions{BundleDir: out, Workspace: t.TempDir()}); err != nil {
		t.Fatalf("import unpacked bundle: %v", err)
	}
}

func TestPackBundleForce(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "requests.http", "GET https://example.com\n")

	bund := filepath.Join(t.TempDir(), "bundle")
	if _, err := ExportBundle(ExportOptions{Workspace: ws, OutDir: bund}); err != nil {
		t.Fatalf("export bundle: %v", err)
	}

	arc := filepath.Join(t.TempDir(), "bundle.zip")
	if err := os.WriteFile(arc, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old archive: %v", err)
	}

	if _, err := PackBundle(PackOptions{BundleDir: bund, OutFile: arc}); err == nil {
		t.Fatalf("expected conflict without force")
	}
	if _, err := PackBundle(PackOptions{BundleDir: bund, OutFile: arc, Force: true}); err != nil {
		t.Fatalf("force pack bundle: %v", err)
	}
}

func TestPackBundleRejectsChecksumMismatch(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "requests.http", "GET https://example.com\n")

	bund := filepath.Join(t.TempDir(), "bundle")
	if _, err := ExportBundle(ExportOptions{Workspace: ws, OutDir: bund}); err != nil {
		t.Fatalf("export bundle: %v", err)
	}

	p := filepath.Join(bund, "requests.http")
	raw, err := os.ReadFile(p)
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
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatalf("tamper bundle file: %v", err)
	}

	_, err = PackBundle(PackOptions{BundleDir: bund, OutFile: filepath.Join(t.TempDir(), "bundle.zip")})
	if err == nil {
		t.Fatalf("expected checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnpackBundleRejectsTraversalEntry(t *testing.T) {
	arc := filepath.Join(t.TempDir(), "bad.zip")
	makeZip(t, arc, []zipEnt{
		{name: ManifestFile, data: []byte(`{"schema":"resterm.collection.bundle","version":1,"files":[{"path":"requests.http","role":"request","size":1,"digest":{"alg":"sha256","value":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}]}`)},
		{name: "../evil.txt", data: []byte("x")},
	})

	_, err := UnpackBundle(UnpackOptions{InFile: arc, OutDir: filepath.Join(t.TempDir(), "out")})
	if err == nil {
		t.Fatalf("expected traversal rejection")
	}
	if !strings.Contains(err.Error(), "invalid archive entry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnpackBundleRejectsSymlinkEntry(t *testing.T) {
	arc := filepath.Join(t.TempDir(), "bad-symlink.zip")
	makeZip(t, arc, []zipEnt{
		{name: ManifestFile, data: []byte(`{"schema":"resterm.collection.bundle","version":1,"files":[{"path":"requests.http","role":"request","size":1,"digest":{"alg":"sha256","value":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}]}`)},
		{name: "requests.http", data: []byte("target"), mode: os.ModeSymlink | 0o777},
	})

	_, err := UnpackBundle(UnpackOptions{InFile: arc, OutDir: filepath.Join(t.TempDir(), "out")})
	if err == nil {
		t.Fatalf("expected symlink rejection")
	}
	if !strings.Contains(err.Error(), "is a symlink") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type zipEnt struct {
	name string
	data []byte
	mode os.FileMode
}

func makeZip(t *testing.T, path string, ents []zipEnt) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := zip.NewWriter(f)
	for _, e := range ents {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		m := e.mode
		if m == 0 {
			m = 0o644
		}
		h.SetMode(m)
		w, err := zw.CreateHeader(h)
		if err != nil {
			_ = zw.Close()
			_ = f.Close()
			t.Fatalf("create zip entry %s: %v", e.name, err)
		}
		if _, err := w.Write(e.data); err != nil {
			_ = zw.Close()
			_ = f.Close()
			t.Fatalf("write zip entry %s: %v", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		_ = f.Close()
		t.Fatalf("close zip writer: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close zip file: %v", err)
	}
}
