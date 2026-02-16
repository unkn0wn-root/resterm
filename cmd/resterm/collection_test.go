package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleCollectionSubcommandNotMatched(t *testing.T) {
	handled, err := handleCollectionSubcommand([]string{"history"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if handled {
		t.Fatalf("expected not handled")
	}
}

func TestHandleCollectionSubcommandAmbiguousFile(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "collection"), []byte("data"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	handled, err := handleCollectionSubcommand([]string{"collection"})
	if !handled {
		t.Fatalf("expected collection to be handled")
	}
	if err == nil {
		t.Fatalf("expected ambiguity error")
	}
}

func TestRunCollectionRequiresSubcommand(t *testing.T) {
	if err := runCollection(nil); err == nil {
		t.Fatalf("expected error for missing subcommand")
	}
}

func TestRunCollectionUnknownSubcommand(t *testing.T) {
	err := runCollection([]string{"unknown"})
	if err == nil {
		t.Fatalf("expected error for unknown subcommand")
	}
}

func TestRunCollectionHelpFlagShowsUsage(t *testing.T) {
	stdout, stderr, err := captureCollectionIO(t, func() error {
		return runCollection([]string{"-h"})
	})
	if err != nil {
		t.Fatalf("help flag: %v", err)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("expected empty stderr on help flag, got %q", stderr)
	}
	if !strings.Contains(stdout, "Usage: resterm collection <export|import|pack|unpack> [flags]") {
		t.Fatalf("expected collection usage in stdout, got %q", stdout)
	}
}

func TestRunCollectionFlagErrorsHaveCommandPrefix(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{args: []string{"export", "--bad"}, want: "collection export:"},
		{args: []string{"import", "--bad"}, want: "collection import:"},
		{args: []string{"pack", "--bad"}, want: "collection pack:"},
		{args: []string{"unpack", "--bad"}, want: "collection unpack:"},
	}
	for _, tc := range cases {
		err := runCollection(tc.args)
		if err == nil {
			t.Fatalf("expected error for %v", tc.args)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("expected error %q to contain %q", err.Error(), tc.want)
		}
	}
}

func TestRunCollectionLibraryErrorsHaveCommandPrefix(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{
			args: []string{
				"export",
				"--workspace", filepath.Join(t.TempDir(), "missing-workspace"),
				"--out", filepath.Join(t.TempDir(), "bundle"),
			},
			want: "collection export:",
		},
		{
			args: []string{
				"import",
				"--in", filepath.Join(t.TempDir(), "missing-bundle"),
				"--workspace", filepath.Join(t.TempDir(), "workspace"),
			},
			want: "collection import:",
		},
		{
			args: []string{
				"pack",
				"--in", filepath.Join(t.TempDir(), "missing-bundle"),
				"--out", filepath.Join(t.TempDir(), "bundle.zip"),
			},
			want: "collection pack:",
		},
		{
			args: []string{
				"unpack",
				"--in", filepath.Join(t.TempDir(), "missing.zip"),
				"--out", filepath.Join(t.TempDir(), "bundle"),
			},
			want: "collection unpack:",
		},
	}
	for _, tc := range cases {
		err := runCollection(tc.args)
		if err == nil {
			t.Fatalf("expected error for %v", tc.args)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("expected error %q to contain %q", err.Error(), tc.want)
		}
	}
}

func TestRunCollectionRequiresFlags(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{args: []string{"export"}, want: "collection export: --workspace is required"},
		{
			args: []string{"export", "--workspace", "."},
			want: "collection export: --out is required",
		},
		{args: []string{"import"}, want: "collection import: --in is required"},
		{
			args: []string{"import", "--in", "./bundle"},
			want: "collection import: --workspace is required",
		},
		{args: []string{"pack"}, want: "collection pack: --in is required"},
		{
			args: []string{"pack", "--in", "./bundle"},
			want: "collection pack: --out is required",
		},
		{args: []string{"unpack"}, want: "collection unpack: --in is required"},
		{
			args: []string{"unpack", "--in", "./bundle.zip"},
			want: "collection unpack: --out is required",
		},
	}
	for _, tc := range cases {
		err := runCollection(tc.args)
		if err == nil {
			t.Fatalf("expected error for %v", tc.args)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("expected error %q to contain %q", err.Error(), tc.want)
		}
	}
}

func TestRunCollectionSubcommandHelpShowsUsage(t *testing.T) {
	stdout, stderr, err := captureCollectionIO(t, func() error {
		return runCollection([]string{"export", "-h"})
	})
	if err != nil {
		t.Fatalf("export -h: %v", err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("expected empty stdout on help, got %q", stdout)
	}
	if !strings.Contains(stderr, "Usage: resterm collection export [flags]") {
		t.Fatalf("expected usage in stderr, got %q", stderr)
	}
	if !strings.Contains(stderr, "-workspace") {
		t.Fatalf("expected --workspace flag in help output, got %q", stderr)
	}
}

func TestRunCollectionExportImportE2E(t *testing.T) {
	src := t.TempDir()
	writeCollectionFile(t, src, "requests.http", `
# @use ./rts/helpers.rts
GET https://example.com
`)
	writeCollectionFile(t, src, "rts/helpers.rts", "module helpers\n")
	writeCollectionFile(t, src, "resterm.env.example.json", `{"dev":{"token":"SAFE"}}`+"\n")

	bundle := filepath.Join(t.TempDir(), "bundle")
	stdout, stderr, err := captureCollectionIO(t, func() error {
		return runCollection([]string{
			"export",
			"--workspace", src,
			"--out", bundle,
		})
	})
	if err != nil {
		t.Fatalf("collection export: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr on export, got %q", stderr)
	}
	if !strings.Contains(stdout, "Exported") {
		t.Fatalf("unexpected export output: %q", stdout)
	}

	dst := t.TempDir()
	stdout, stderr, err = captureCollectionIO(t, func() error {
		return runCollection([]string{
			"import",
			"--in", bundle,
			"--workspace", dst,
		})
	})
	if err != nil {
		t.Fatalf("collection import: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr on import, got %q", stderr)
	}
	if !strings.Contains(stdout, "Imported") {
		t.Fatalf("unexpected import output: %q", stdout)
	}

	if _, err := os.Stat(filepath.Join(dst, "requests.http")); err != nil {
		t.Fatalf("expected requests.http in destination: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "rts", "helpers.rts")); err != nil {
		t.Fatalf("expected helpers.rts in destination: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "resterm.env.example.json")); err != nil {
		t.Fatalf("expected env template in destination: %v", err)
	}
}

func TestRunCollectionImportDryRun(t *testing.T) {
	src := t.TempDir()
	writeCollectionFile(t, src, "requests.http", "GET https://example.com\n")

	bundle := filepath.Join(t.TempDir(), "bundle")
	if err := runCollection([]string{
		"export",
		"--workspace", src,
		"--out", bundle,
	}); err != nil {
		t.Fatalf("collection export: %v", err)
	}

	dst := filepath.Join(t.TempDir(), "workspace")
	stdout, stderr, err := captureCollectionIO(t, func() error {
		return runCollection([]string{
			"import",
			"--in", bundle,
			"--workspace", dst,
			"--dry-run",
		})
	})
	if err != nil {
		t.Fatalf("collection import dry-run: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr on dry-run import, got %q", stderr)
	}
	if !strings.Contains(stdout, "Dry-run: planned") {
		t.Fatalf("unexpected dry-run output: %q", stdout)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("workspace should not exist after dry-run")
	}
}

func TestRunCollectionPackUnpackE2E(t *testing.T) {
	src := t.TempDir()
	writeCollectionFile(t, src, "requests.http", "GET https://example.com\n")

	bund := filepath.Join(t.TempDir(), "bundle")
	if err := runCollection([]string{
		"export",
		"--workspace", src,
		"--out", bund,
	}); err != nil {
		t.Fatalf("collection export: %v", err)
	}

	arc := filepath.Join(t.TempDir(), "bundle.zip")
	stdout, stderr, err := captureCollectionIO(t, func() error {
		return runCollection([]string{
			"pack",
			"--in", bund,
			"--out", arc,
		})
	})
	if err != nil {
		t.Fatalf("collection pack: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr on pack, got %q", stderr)
	}
	if !strings.Contains(stdout, "Packed") {
		t.Fatalf("unexpected pack output: %q", stdout)
	}

	out := filepath.Join(t.TempDir(), "bundle-unpacked")
	stdout, stderr, err = captureCollectionIO(t, func() error {
		return runCollection([]string{
			"unpack",
			"--in", arc,
			"--out", out,
		})
	})
	if err != nil {
		t.Fatalf("collection unpack: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr on unpack, got %q", stderr)
	}
	if !strings.Contains(stdout, "Unpacked") {
		t.Fatalf("unexpected unpack output: %q", stdout)
	}

	if _, err := os.Stat(filepath.Join(out, "manifest.json")); err != nil {
		t.Fatalf("expected manifest in unpacked bundle: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "requests.http")); err != nil {
		t.Fatalf("expected requests.http in unpacked bundle: %v", err)
	}
}

func writeCollectionFile(t *testing.T, root, rel, data string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func captureCollectionIO(t *testing.T, fn func() error) (string, string, error) {
	t.Helper()

	oldOut := os.Stdout
	oldErr := os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}

	os.Stdout = outW
	os.Stderr = errW
	defer func() {
		os.Stdout = oldOut
		os.Stderr = oldErr
	}()

	runErr := fn()

	_ = outW.Close()
	_ = errW.Close()

	outData, outErr := io.ReadAll(outR)
	if outErr != nil {
		t.Fatalf("read stdout: %v", outErr)
	}
	errData, errErr := io.ReadAll(errR)
	if errErr != nil {
		t.Fatalf("read stderr: %v", errErr)
	}

	_ = outR.Close()
	_ = errR.Close()
	return string(outData), string(errData), runErr
}
