package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/files"
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
	want := map[string]files.Kind{
		"api.http":               files.KindRequest,
		"helpers.rts":            files.KindScript,
		"pre.js":                 files.KindJavaScript,
		"flags.json":             files.KindJSON,
		"module-data.json":       files.KindJSON,
		"addNote.graphql":        files.KindGraphQL,
		"addNote.variables.json": files.KindJSON,
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

func TestListIncludesReferencedProtoFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "api.http"), `### Proto
# @grpc acme.v1.Greeter/SayHello
# @grpc-proto ./proto/greeter.proto
# @grpc-import-path ./vendor
GRPC localhost:50051

### Descriptor
# @grpc acme.v1.Greeter/SayHello
# @grpc-descriptor ./greeter.protoset
GRPC localhost:50051
`)
	writeFile(t, filepath.Join(root, "proto", "greeter.proto"), `syntax = "proto3";`)
	writeFile(t, filepath.Join(root, "proto", "unused.proto"), `syntax = "proto3";`)
	writeFile(t, filepath.Join(root, "vendor", "google", "api", "annotations.proto"), `syntax = "proto3";`)
	writeFile(t, filepath.Join(root, "greeter.protoset"), "")

	entries, err := List(root, ListOptions{Recursive: true})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	got := entryKinds(entries)
	if got[filepath.Join("proto", "greeter.proto")] != files.KindProto {
		t.Fatalf("expected the referenced proto file, got %+v", entries)
	}
	for _, name := range []string{
		filepath.Join("proto", "unused.proto"),
		filepath.Join("vendor", "google", "api", "annotations.proto"),
		"greeter.protoset",
	} {
		if _, ok := got[name]; ok {
			t.Fatalf("did not expect %s in entries: %+v", name, entries)
		}
	}
}

func TestListIncludesProtoFilesFoundThroughImportPaths(t *testing.T) {
	const named = `# @grpc acme.Svc/Call
# @grpc-proto acme/svc.proto
# @grpc-import-path proto
GRPC localhost:50051
`
	find := func(gr *restfile.GRPCRequest, dir string) ([]string, error) {
		paths := make([]string, len(gr.ProtoFiles))
		for i, p := range gr.ProtoFiles {
			paths[i] = filepath.Join(dir, "proto", p)
		}
		return paths, nil
	}
	fail := func(gr *restfile.GRPCRequest, dir string) ([]string, error) {
		paths, _ := find(gr, dir)
		return paths, errors.New("proto file is shadowed")
	}

	tests := []struct {
		name  string
		src   string
		find  func(*restfile.GRPCRequest, string) ([]string, error)
		rel   bool
		calls int
		want  []string
	}{
		{
			name:  "found files",
			src:   named,
			find:  find,
			calls: 1,
			want:  []string{filepath.Join("proto", "acme", "svc.proto")},
		},
		{
			name: "templated import path",
			src: `# @grpc acme.Svc/Call
# @grpc-proto acme/svc.proto
# @grpc-import-path proto
# @grpc-import-path {{vendor}}
GRPC localhost:50051
`,
			find: find,
		},
		{
			name: "no proto files",
			src: `GET https://example.com

###
# @grpc acme.Svc/Call
# @grpc-descriptor ./svc.protoset
GRPC localhost:50051
`,
			find: find,
		},
		{
			name:  "error",
			src:   named,
			find:  fail,
			calls: 1,
		},
		{
			name: "no resolver",
			src: named + `
###
# @grpc acme.Svc/Call
# @grpc-proto ./local.proto
GRPC localhost:50051
`,
			want: []string{"local.proto"},
		},
		{
			name:  "relative root",
			src:   named,
			find:  find,
			rel:   true,
			calls: 1,
			want:  []string{filepath.Join("proto", "acme", "svc.proto")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			root := filepath.Join(tmp, "ws")
			writeFile(t, filepath.Join(root, "api.http"), tt.src)
			writeFile(t, filepath.Join(root, "local.proto"), `syntax = "proto3";`)
			writeFile(t, filepath.Join(root, "proto", "acme", "svc.proto"), `syntax = "proto3";`)
			writeFile(t, filepath.Join(root, "proto", "acme", "unused.proto"), `syntax = "proto3";`)
			if tt.rel {
				t.Chdir(tmp)
				root = "ws"
			}

			var calls int
			opt := ListOptions{Recursive: true}
			if tt.find != nil {
				opt.ProtoFiles = func(gr *restfile.GRPCRequest, dir string) ([]string, error) {
					calls++
					return tt.find(gr, dir)
				}
			}
			entries, err := List(root, opt)
			if err != nil {
				t.Fatalf("List returned error: %v", err)
			}

			if calls != tt.calls {
				t.Fatalf("resolver calls = %d, want %d", calls, tt.calls)
			}
			var got []string
			for _, e := range entries {
				if e.Kind == files.KindProto {
					got = append(got, e.Name)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("proto entries = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListResolvesRTSModuleJSONRefsFromDocumentBase(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "api.http"), `# @use ./rts/helpers.rts
# @when helpers.enabled()
GET https://example.com
`)
	writeFile(t, filepath.Join(root, "rts", "helpers.rts"), `module helpers
export fn enabled() {
  return json.file("./flags.json").enabled
}
`)
	writeFile(t, filepath.Join(root, "flags.json"), `{"enabled":true}`)

	entries, err := List(root, ListOptions{})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	got := entryKinds(entries)
	if got["flags.json"] != files.KindJSON {
		t.Fatalf("expected root flags.json from document base, got %+v", entries)
	}
	if _, ok := got[filepath.Join("rts", "flags.json")]; ok {
		t.Fatalf("did not expect module-relative flags.json in entries: %+v", entries)
	}
}

func TestListResolvesSharedRTSModuleJSONRefsPerDocumentBase(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "api.http"), `# @use ./common/helpers.rts
# @when helpers.enabled()
GET https://root.example
`)
	writeFile(t, filepath.Join(root, "nested", "api.http"), `# @use ../common/helpers.rts
# @when helpers.enabled()
GET https://nested.example
`)
	writeFile(t, filepath.Join(root, "common", "helpers.rts"), `module helpers
export fn enabled() {
  return json.file("./flags.json").enabled
}
`)
	writeFile(t, filepath.Join(root, "flags.json"), `{"enabled":true}`)
	writeFile(t, filepath.Join(root, "nested", "flags.json"), `{"enabled":true}`)

	entries, err := List(root, ListOptions{Recursive: true})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	got := entryKinds(entries)
	if got["flags.json"] != files.KindJSON {
		t.Fatalf("expected root flags.json from root document base, got %+v", entries)
	}
	if got[filepath.Join("nested", "flags.json")] != files.KindJSON {
		t.Fatalf("expected nested flags.json from nested document base, got %+v", entries)
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
	if len(got) != 1 || got["api.http"] != files.KindRequest {
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
	if got["scratch.json"] != files.KindJSON {
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
	if got["payload.json"] != files.KindJSON {
		t.Fatalf("expected current document refs, got %+v", entries)
	}
}

func entryKinds(entries []files.Entry) map[string]files.Kind {
	out := make(map[string]files.Kind, len(entries))
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
