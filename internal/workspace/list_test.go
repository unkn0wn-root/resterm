package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestListIncludesOnlyReferencedAuxiliaryFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "api.http"), `# @use ./helpers.rts
# @script pre-request
> < ./pre.js
# @when json.file("./flags.json").enabled
# @graphql
# @query < ./addNote.graphql
# @variables < ./addNote.variables.json
POST https://example.com/graphql
Content-Type: application/json
`)
	writeFile(t, filepath.Join(root, "helpers.rts"), `
export fn enabled() {
  return json.file("./module-data.json").enabled
}
`)
	writeFile(t, filepath.Join(root, "pre.js"), "request.setHeader('X-Test', '1')\n")
	writeFile(t, filepath.Join(root, "flags.json"), `{"enabled":true}`)
	writeFile(t, filepath.Join(root, "module-data.json"), `{"enabled":true}`)
	writeFile(t, filepath.Join(root, "addNote.graphql"), "mutation AddNote { addNote { id } }\n")
	writeFile(t, filepath.Join(root, "addNote.variables.json"), `{"id":"1"}`)
	writeFile(t, filepath.Join(root, "orphan.json"), `{}`)
	writeFile(t, filepath.Join(root, "orphan.js"), "")

	entries, err := List(root, ListOptions{})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	got := entryKinds(entries)
	want := map[string]filesvc.FileKind{
		"api.http":               filesvc.FileKindRequest,
		"helpers.rts":            filesvc.FileKindScript,
		"pre.js":                 filesvc.FileKindJavaScript,
		"flags.json":             filesvc.FileKindJSON,
		"module-data.json":       filesvc.FileKindJSON,
		"addNote.graphql":        filesvc.FileKindGraphQL,
		"addNote.variables.json": filesvc.FileKindJSON,
	}
	for name, kind := range want {
		if got[name] != kind {
			t.Fatalf("expected %s as %v, got entries %+v", name, kind, entries)
		}
	}
	if _, ok := got["orphan.json"]; ok {
		t.Fatalf("did not expect orphan json in entries: %+v", entries)
	}
	if _, ok := got["orphan.js"]; ok {
		t.Fatalf("did not expect orphan javascript in entries: %+v", entries)
	}
}

func TestListSkipsMissingOutsideAndDynamicRefs(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "outside.json"), `{}`)
	writeFile(t, filepath.Join(root, "api.http"), `# @when json.file(vars.get("path")).enabled
# @assert json.file("./missing.json").enabled
POST https://example.com
Content-Type: application/json

< `+filepath.Join(outside, "outside.json")+`
`)

	entries, err := List(root, ListOptions{})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	got := entryKinds(entries)
	if len(got) != 1 || got["api.http"] != filesvc.FileKindRequest {
		t.Fatalf("expected only request file, got %+v", entries)
	}
}

func TestListIncludesCurrentAuxiliaryFile(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "scratch.json")
	writeFile(t, file, `{}`)

	entries, err := List(root, ListOptions{CurrentFile: file})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	got := entryKinds(entries)
	if got["scratch.json"] != filesvc.FileKindJSON {
		t.Fatalf("expected current auxiliary file, got %+v", entries)
	}
}

func TestListUsesCurrentDocumentForUnsavedRefs(t *testing.T) {
	root := t.TempDir()
	req := filepath.Join(root, "api.http")
	writeFile(t, req, "GET https://example.com\n")
	writeFile(t, filepath.Join(root, "payload.json"), `{}`)

	doc := parseDoc(req, `POST https://example.com
Content-Type: application/json

< ./payload.json
`)
	entries, err := List(root, ListOptions{
		CurrentFile: req,
		CurrentDoc:  doc,
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	got := entryKinds(entries)
	if got["payload.json"] != filesvc.FileKindJSON {
		t.Fatalf("expected current document refs, got %+v", entries)
	}
}

func entryKinds(entries []filesvc.FileEntry) map[string]filesvc.FileKind {
	out := make(map[string]filesvc.FileKind, len(entries))
	for _, e := range entries {
		out[e.Name] = e.Kind
	}
	return out
}

func writeFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func parseDoc(path string, data string) *restfile.Document {
	return parser.Parse(path, []byte(data))
}
