package mock

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Renaming the source file keeps the parsed specs byte-identical, but served
// events report their source path, so the reload must swap in a fresh handler.
func TestReloadDetectsSourceFileRename(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.http"), "# @mock method=GET path=/x\nHTTP/1.1 200 OK\n\nok")

	reloader := NewReloader(root, false)
	if _, err := reloader.Reload("", nil); err != nil {
		t.Fatalf("initial reload: %v", err)
	}

	if err := os.Rename(filepath.Join(root, "a.http"), filepath.Join(root, "b.http")); err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloader.Reload("", nil)
	if err != nil || reloaded == nil {
		t.Fatalf("rename reload = %v, %v; want a fresh handler", reloaded, err)
	}
	assertResponse(t, reloaded, httptest.NewRequest(http.MethodGet, "/x", nil), http.StatusOK, "ok")
}

// Swapping a response to a different fixture file that currently holds identical
// bytes still changes the configuration, so the reload must not be skipped.
func TestReloadDetectsFixtureRefSwapWithIdenticalContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "mocks.http")
	writeFile(t, filepath.Join(root, "a.txt"), "same")
	writeFile(t, filepath.Join(root, "b.txt"), "same")
	writeFile(t, path, "# @mock method=GET path=/x\nHTTP/1.1 200 OK\n\n< ./a.txt")

	reloader := NewReloader(root, false)
	if _, err := reloader.Reload("", nil); err != nil {
		t.Fatalf("initial reload: %v", err)
	}

	writeFile(t, path, "# @mock method=GET path=/x\nHTTP/1.1 200 OK\n\n< ./b.txt")
	reloaded, err := reloader.Reload("", nil)
	if err != nil || reloaded == nil {
		t.Fatalf("swap reload = %v, %v; want a fresh handler", reloaded, err)
	}
	assertResponse(t, reloaded, httptest.NewRequest(http.MethodGet, "/x", nil), http.StatusOK, "same")
}
