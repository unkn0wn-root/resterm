package collection

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPackUnpackBundleRoundTrip(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "requests.http", "GET https://example.com\n")
	writeFile(t, ws, "resterm.env.example.json", "{\"dev\":{\"token\":\"SAFE\"}}\n")

	bund := filepath.Join(t.TempDir(), "bundle")
	exp, err := ExportBundle(t.Context(), ExportOptions{Workspace: ws, OutDir: bund})
	if err != nil {
		t.Fatalf("export bundle: %v", err)
	}

	arc := filepath.Join(t.TempDir(), "bundle.zip")
	pk, err := PackBundle(t.Context(), PackOptions{BundleDir: bund, OutFile: arc})
	if err != nil {
		t.Fatalf("pack bundle: %v", err)
	}
	if pk.FileCount != exp.FileCount {
		t.Fatalf("packed count=%d want %d", pk.FileCount, exp.FileCount)
	}

	out := filepath.Join(t.TempDir(), "bundle-unpacked")
	un, err := UnpackBundle(t.Context(), UnpackOptions{InFile: arc, OutDir: out})
	if err != nil {
		t.Fatalf("unpack bundle: %v", err)
	}
	if un.FileCount != exp.FileCount {
		t.Fatalf("unpacked count=%d want %d", un.FileCount, exp.FileCount)
	}

	if got := fileMode(t, out).Perm(); runtime.GOOS != "windows" && got != 0o700 {
		t.Fatalf("unpacked bundle mode = %v, want 0700", got)
	}
	got, want := fileMode(t, filepath.Join(out, ManifestFile)), fileMode(t, filepath.Join(bund, ManifestFile))
	if got != want {
		t.Fatalf("unpacked manifest mode = %v, want %v like the exported one", got, want)
	}

	if _, err := ImportBundle(ImportOptions{BundleDir: out, Workspace: t.TempDir()}); err != nil {
		t.Fatalf("import unpacked bundle: %v", err)
	}

	if _, err := UnpackBundle(t.Context(), UnpackOptions{InFile: arc, OutDir: out, Force: true}); err != nil {
		t.Fatalf("force unpack over a bundle: %v", err)
	}
	if got := fileMode(t, out).Perm(); runtime.GOOS != "windows" && got != 0o700 {
		t.Fatalf("force unpacked bundle mode = %v, want 0700", got)
	}
	notes := t.TempDir()
	writeFile(t, notes, "todo.txt", "keep")
	_, err = UnpackBundle(t.Context(), UnpackOptions{InFile: arc, OutDir: notes, Force: true})
	if err == nil || !strings.Contains(err.Error(), "is not a resterm bundle") {
		t.Fatalf("force unpack over notes = %v, want an error with %q", err, "is not a resterm bundle")
	}
	if got := readTree(t, os.DirFS(notes)); !maps.Equal(got, map[string]string{"todo.txt": "keep"}) {
		t.Fatalf("notes changed to %v", got)
	}
}

func TestPackBundleForce(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "requests.http", "GET https://example.com\n")

	bund := filepath.Join(t.TempDir(), "bundle")
	if _, err := ExportBundle(t.Context(), ExportOptions{Workspace: ws, OutDir: bund}); err != nil {
		t.Fatalf("export bundle: %v", err)
	}

	arc := filepath.Join(t.TempDir(), "bundle.zip")
	if err := os.WriteFile(arc, []byte("old"), 0o600); err != nil {
		t.Fatalf("write old archive: %v", err)
	}

	if _, err := PackBundle(t.Context(), PackOptions{BundleDir: bund, OutFile: arc}); err == nil {
		t.Fatalf("expected conflict without force")
	}
	f, err := os.Open(arc)
	if err != nil {
		t.Fatal(err)
	}
	old, err := f.Stat()
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PackBundle(t.Context(), PackOptions{BundleDir: bund, OutFile: arc, Force: true}); err != nil {
		t.Fatalf("force pack bundle: %v", err)
	}
	got, err := os.Stat(arc)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(got, old) {
		t.Fatal("force pack rewrote the old archive in place, want a new file moved over it")
	}
	if runtime.GOOS != "windows" && got.Mode().Perm() != 0o600 {
		t.Fatalf("archive mode = %v, want 0600", got.Mode().Perm())
	}
	zr, err := zip.OpenReader(arc)
	if err != nil {
		t.Fatalf("open forced archive: %v", err)
	}
	if err := zr.Close(); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(filepath.Dir(arc)); err != nil || len(entries) != 1 {
		t.Fatalf("ReadDir = %v, %v, want only the archive", entries, err)
	}

	t.Run("read-only parent", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("read-only folders do not stop root or Windows")
		}
		data, err := os.ReadFile(arc)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Dir(arc), 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(filepath.Dir(arc), 0o755) })

		_, err = PackBundle(t.Context(), PackOptions{BundleDir: bund, OutFile: arc, Force: true})
		if !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), "create temp archive") {
			t.Fatalf("force pack in a read-only folder = %v, want a permission error at create temp archive", err)
		}
		if got, err := os.ReadFile(arc); err != nil || !bytes.Equal(got, data) {
			t.Fatalf("archive changed after a failed pack: %v", err)
		}
	})

	dir := t.TempDir()
	writeFile(t, dir, "docs/notes.txt", "keep")
	if err := os.Symlink(arc, filepath.Join(dir, "link")); err != nil {
		t.Skipf("cannot create a link: %v", err)
	}
	for _, name := range []string{"docs", "link"} {
		o := PackOptions{BundleDir: bund, OutFile: filepath.Join(dir, name)}
		if _, err := PackBundle(t.Context(), o); err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("pack over %s = %v, want an error with %q", name, err, "already exists")
		}
		o.Force = true
		if _, err := PackBundle(t.Context(), o); err == nil || !strings.Contains(err.Error(), "is not a file") {
			t.Fatalf("force pack over %s = %v, want an error with %q", name, err, "is not a file")
		}
	}
	if got := readTree(t, os.DirFS(dir)); !maps.Equal(got, map[string]string{"docs/notes.txt": "keep"}) {
		t.Fatalf("files changed to %v", got)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 2 {
		t.Fatalf("ReadDir(%s) = %v, %v, want only docs and link", dir, entries, err)
	}
	if got, err := os.Readlink(filepath.Join(dir, "link")); err != nil || got != arc {
		t.Fatalf("Readlink(link) = %q, %v, want %q", got, err, arc)
	}
}

type cancelAfter struct {
	context.Context
	cancel context.CancelFunc
	n      int
}

func (c *cancelAfter) Err() error {
	if c.n == 0 {
		c.cancel()
	}
	err := c.Context.Err()
	c.n--
	if c.n == 0 {
		c.cancel()
	}
	return err
}

func TestCancelKeepsOldOrNew(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "requests.http", "POST https://example.com\n\n< ./payloads/body.json\n")
	writeFile(t, ws, "payloads/body.json", `{"a":1}`)
	exp := filepath.Join(t.TempDir(), "out")
	arc := filepath.Join(t.TempDir(), "out.zip")
	unp := filepath.Join(t.TempDir(), "out")
	if _, err := ExportBundle(t.Context(), ExportOptions{Workspace: ws, OutDir: exp}); err != nil {
		t.Fatal(err)
	}
	if _, err := PackBundle(t.Context(), PackOptions{BundleDir: exp, OutFile: arc}); err != nil {
		t.Fatal(err)
	}
	if _, err := UnpackBundle(t.Context(), UnpackOptions{InFile: arc, OutDir: unp}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, ws, "payloads/body.json", `{"a":2}`)
	bund := filepath.Join(t.TempDir(), "bundle")
	if _, err := ExportBundle(t.Context(), ExportOptions{Workspace: ws, OutDir: bund}); err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(t.TempDir(), "in.zip")
	if _, err := PackBundle(t.Context(), PackOptions{BundleDir: bund, OutFile: in}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		out    string
		checks int
		run    func(context.Context) error
	}{
		{name: "export", out: exp, checks: 3, run: func(ctx context.Context) error {
			_, err := ExportBundle(ctx, ExportOptions{Workspace: ws, OutDir: exp, Force: true})
			return err
		}},
		{name: "pack", out: arc, checks: 3, run: func(ctx context.Context) error {
			_, err := PackBundle(ctx, PackOptions{BundleDir: bund, OutFile: arc, Force: true})
			return err
		}},
		{name: "unpack", out: unp, checks: 4, run: func(ctx context.Context) error {
			_, err := UnpackBundle(ctx, UnpackOptions{InFile: in, OutDir: unp, Force: true})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Dir(tt.out)
			before := readTree(t, os.DirFS(dir))
			for n := range 100 {
				ctx, cancel := context.WithCancel(t.Context())
				err := tt.run(&cancelAfter{Context: ctx, cancel: cancel, n: n})
				cancel()
				if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
					t.Fatalf("cancel after %d checks: ReadDir = %v, %v, want only the output", n, entries, err)
				}
				got := readTree(t, os.DirFS(dir))
				if err == nil {
					if n <= tt.checks {
						t.Fatalf("cancel after %d checks was ignored, want it to stop staging", n)
					}
					if maps.Equal(got, before) {
						t.Fatal("output unchanged after a run that got past every check")
					}
					return
				}
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel after %d checks = %v, want %v", n, err, context.Canceled)
				}
				if !maps.Equal(got, before) {
					t.Fatalf("cancel after %d checks changed the files to %v, want %v", n, got, before)
				}
			}
			t.Fatal("no run got past every check")
		})
	}
}

func TestPackBundleRejectsChecksumMismatch(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "requests.http", "GET https://example.com\n")

	bund := filepath.Join(t.TempDir(), "bundle")
	if _, err := ExportBundle(t.Context(), ExportOptions{Workspace: ws, OutDir: bund}); err != nil {
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

	_, err = PackBundle(
		t.Context(),
		PackOptions{BundleDir: bund, OutFile: filepath.Join(t.TempDir(), "bundle.zip")},
	)
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
		{
			name: ManifestFile,
			data: []byte(
				`{"schema":"resterm.collection.bundle","version":1,"files":[{"path":"requests.http","role":"request","size":1,"digest":{"alg":"sha256","value":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}]}`,
			),
		},
		{name: "../evil.txt", data: []byte("x")},
	})

	_, err := UnpackBundle(t.Context(), UnpackOptions{InFile: arc, OutDir: filepath.Join(t.TempDir(), "out")})
	if err == nil {
		t.Fatalf("expected traversal rejection")
	}
	if !strings.Contains(err.Error(), "invalid archive entry") {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = UnpackBundle(ctx, UnpackOptions{InFile: arc, OutDir: filepath.Join(t.TempDir(), "out")})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled unpack = %v, want %v before any entry is read", err, context.Canceled)
	}
}

func TestUnpackBundleRejectsSymlinkEntry(t *testing.T) {
	arc := filepath.Join(t.TempDir(), "bad-symlink.zip")
	makeZip(t, arc, []zipEnt{
		{
			name: ManifestFile,
			data: []byte(
				`{"schema":"resterm.collection.bundle","version":1,"files":[{"path":"requests.http","role":"request","size":1,"digest":{"alg":"sha256","value":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}]}`,
			),
		},
		{name: "requests.http", data: []byte("target"), mode: os.ModeSymlink | 0o777},
	})

	_, err := UnpackBundle(t.Context(), UnpackOptions{InFile: arc, OutDir: filepath.Join(t.TempDir(), "out")})
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
