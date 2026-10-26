package grpcx

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/filelookup"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func TestProtoSourceNamesFiles(t *testing.T) {
	acme := mapFS{
		"/req/proto/acme/v1/svc.proto": []byte(`syntax = "proto3";
package acme.v1;
import "acme/v1/types.proto";
service Greeter {
  rpc Hello(HelloRequest) returns (HelloReply);
}
`),
		"/req/proto/acme/v1/types.proto": []byte(`syntax = "proto3";
package acme.v1;
message HelloRequest {}
message HelloReply {}
`),
	}
	sibling := mapFS{
		"/req/proto/svc.proto": []byte(`syntax = "proto3";
import "common.proto";
message Req { Common c = 1; }
`),
		"/req/proto/common.proto": []byte(`syntax = "proto3";
message Common {}
`),
	}
	nested := mapFS{
		"/req/proto/y.proto": []byte(`syntax = "proto3";
import "a/x.proto";
message Y { a.X x = 1; }
`),
		"/req/proto/a/x.proto": []byte(`syntax = "proto3";
package a;
message X {}
`),
		"/req/proto/x.proto": []byte(`syntax = "proto3";
package top;
message X {}
`),
	}

	tests := []struct {
		name    string
		fs      mapFS
		paths   []string
		imports []string
		want    []string
	}{
		{
			name:  "sibling import without import paths",
			fs:    sibling,
			paths: []string{"proto/svc.proto"},
			want:  []string{"svc.proto", "common.proto"},
		},
		{
			name:  "nested file listed last",
			fs:    nested,
			paths: []string{"proto/y.proto", "proto/a/x.proto"},
			want:  []string{"y.proto", "a/x.proto"},
		},
		{
			name:  "nested file listed first",
			fs:    nested,
			paths: []string{"proto/a/x.proto", "proto/y.proto"},
			want:  []string{"y.proto", "a/x.proto"},
		},
		{
			name:  "same base name in a dir and its child",
			fs:    nested,
			paths: []string{"proto/a/x.proto", "proto/x.proto"},
			want:  []string{"a/x.proto", "x.proto"},
		},
		{
			name:  "nested file listed by absolute path",
			fs:    nested,
			paths: []string{"proto/y.proto", "/req/proto/a/x.proto"},
			want:  []string{"y.proto", "a/x.proto"},
		},
		{
			name:    "relative import path and file anchored at the request dir",
			fs:      nested,
			paths:   []string{"proto/x.proto"},
			imports: []string{"../req/proto"},
			want:    []string{"x.proto"},
		},
		{
			name:    "first import path that holds the file names it",
			fs:      acme,
			paths:   []string{"proto/acme/v1/svc.proto"},
			imports: []string{"./proto", "./proto/acme"},
			want:    []string{"acme/v1/svc.proto", "acme/v1/types.proto"},
		},
		{
			name:    "listed and imported file compiles once",
			fs:      acme,
			paths:   []string{"./proto/acme/v1/svc.proto", "./proto/acme/v1/types.proto"},
			imports: []string{"./proto"},
			want:    []string{"acme/v1/svc.proto", "acme/v1/types.proto"},
		},
		{
			name:    "import name searched in the import paths",
			fs:      acme,
			paths:   []string{"acme/v1/svc.proto"},
			imports: []string{"./proto"},
			want:    []string{"acme/v1/svc.proto", "acme/v1/types.proto"},
		},
		{
			name:    "import name searched past an import path that holds its path",
			fs:      acme,
			paths:   []string{"acme/v1/svc.proto"},
			imports: []string{".", "proto"},
			want:    []string{"acme/v1/svc.proto", "acme/v1/types.proto"},
		},
		{
			name:    "import name searched past an ancestor import path",
			fs:      acme,
			paths:   []string{"acme/v1/svc.proto"},
			imports: []string{"..", "proto"},
			want:    []string{"acme/v1/svc.proto", "acme/v1/types.proto"},
		},
		{
			name:    "import name searched past an import path that prefixes it",
			fs:      acme,
			paths:   []string{"acme/v1/svc.proto"},
			imports: []string{"acme", "proto"},
			want:    []string{"acme/v1/svc.proto", "acme/v1/types.proto"},
		},
		{
			name:    "absolute file under a relative import path",
			fs:      acme,
			paths:   []string{"/req/proto/acme/v1/svc.proto"},
			imports: []string{"proto"},
			want:    []string{"acme/v1/svc.proto", "acme/v1/types.proto"},
		},
		{
			name:    "relative file under an absolute import path",
			fs:      acme,
			paths:   []string{"proto/acme/v1/svc.proto"},
			imports: []string{"/req/proto"},
			want:    []string{"acme/v1/svc.proto", "acme/v1/types.proto"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := protoSource{
				paths:   tt.paths,
				imports: tt.imports,
				rd:      newReader(tt.fs, Options{BaseDir: "/req"}),
			}
			files, err := src.files(context.Background(), methodID{})
			if err != nil {
				t.Fatalf("files: %v", err)
			}
			for _, path := range tt.want {
				if _, err := files.FindFileByPath(path); err != nil {
					t.Fatalf("registry is missing %s: %v", path, err)
				}
			}
		})
	}
}

func TestProtoSourceImportPathOrder(t *testing.T) {
	fs := mapFS{
		"/req/a/svc.proto": []byte(`syntax = "proto3";
import "common.proto";
`),
		"/req/a/common.proto": []byte(`syntax = "proto3";
package pick;
message FromA {}
`),
		"/req/b/common.proto": []byte(`syntax = "proto3";
package pick;
message FromB {}
`),
	}

	tests := []struct {
		paths   []string
		imports []string
		want    protoreflect.FullName
		other   protoreflect.FullName
	}{
		{paths: []string{"a/svc.proto"}, imports: []string{"./a", "./b"}, want: "pick.FromA", other: "pick.FromB"},
		{paths: []string{"a/svc.proto"}, imports: []string{"./b", "./a"}, want: "pick.FromB", other: "pick.FromA"},
		{paths: []string{"common.proto"}, imports: []string{"./b", "./a"}, want: "pick.FromB", other: "pick.FromA"},
		{paths: []string{"common.proto"}, imports: []string{".", "a", "b"}, want: "pick.FromA", other: "pick.FromB"},
		{paths: []string{"common.proto"}, imports: []string{".", "b", "a"}, want: "pick.FromB", other: "pick.FromA"},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.paths, ",")+" in "+strings.Join(tt.imports, ","), func(t *testing.T) {
			src := protoSource{
				paths:   tt.paths,
				imports: tt.imports,
				rd:      newReader(fs, Options{BaseDir: "/req"}),
			}
			files, err := src.files(context.Background(), methodID{})
			if err != nil {
				t.Fatalf("files: %v", err)
			}
			if _, err := files.FindDescriptorByName(tt.want); err != nil {
				t.Fatalf("registry is missing %s: %v", tt.want, err)
			}
			if _, err := files.FindDescriptorByName(tt.other); err == nil {
				t.Fatalf("registry holds %s, want only %s", tt.other, tt.want)
			}
		})
	}
}

func TestProtoSourceUsesStandardImports(t *testing.T) {
	fs := mapFS{"/req/svc.proto": []byte(`syntax = "proto3";
import "google/protobuf/empty.proto";
import "google/protobuf/timestamp.proto";
service Clock {
  rpc Now(google.protobuf.Empty) returns (google.protobuf.Timestamp);
}
`)}
	src := protoSource{paths: []string{"svc.proto"}, rd: newReader(fs, Options{BaseDir: "/req"})}

	files, err := src.files(context.Background(), methodID{})
	if err != nil {
		t.Fatalf("files: %v", err)
	}
	if _, err := files.FindFileByPath("google/protobuf/empty.proto"); err != nil {
		t.Fatalf("registry is missing empty.proto: %v", err)
	}
	got, err := files.FindFileByPath("google/protobuf/timestamp.proto")
	if err != nil {
		t.Fatalf("registry is missing timestamp.proto: %v", err)
	}
	want, err := protoregistry.GlobalFiles.FindFileByPath("google/protobuf/timestamp.proto")
	if err != nil {
		t.Fatalf("global timestamp.proto: %v", err)
	}
	if got != want {
		t.Fatal("timestamp.proto is a copy, want the protoregistry.GlobalFiles descriptor")
	}
}

func TestProtoSourceErrors(t *testing.T) {
	tests := []struct {
		name      string
		fs        filelookup.FileSystem
		paths     []string
		imports   []string
		fallbacks []string
		want      string
		absent    string
		classes   []diag.Class
	}{
		{
			name: "request dir is not an import path",
			fs: mapFS{
				"/req/proto/svc.proto": []byte(`syntax = "proto3";
import "proto/common.proto";
`),
				"/req/proto/common.proto": []byte(`syntax = "proto3";
`),
			},
			paths:   []string{"proto/svc.proto"},
			want:    "svc.proto:2:8:",
			classes: []diag.Class{diag.ClassProtocol, diag.ClassFilesystem},
		},
		{
			name: "missing import",
			fs: mapFS{"/req/proto/svc.proto": []byte(`syntax = "proto3";

import "nope.proto";
`)},
			paths:   []string{"proto/svc.proto"},
			want:    "svc.proto:3:8:",
			classes: []diag.Class{diag.ClassProtocol, diag.ClassFilesystem},
		},
		{
			name:    "missing file",
			fs:      mapFS{},
			paths:   []string{"proto/missing.proto"},
			want:    "proto/missing.proto",
			classes: []diag.Class{diag.ClassFilesystem},
		},
		{
			name:    "missing file is named by its own path",
			fs:      mapFS{"/req/b/other.proto": []byte(`syntax = "proto3";`)},
			paths:   []string{"a/svc.proto", "b/other.proto"},
			want:    "a/svc.proto",
			absent:  "b/svc.proto",
			classes: []diag.Class{diag.ClassFilesystem},
		},
		{
			name: "syntax error",
			fs: mapFS{"/req/svc.proto": []byte(`syntax = "proto3";
message M {
  string a = 1
  int32 b = 2;
}
`)},
			paths:   []string{"svc.proto"},
			want:    "svc.proto:4:3:",
			classes: []diag.Class{diag.ClassProtocol},
		},
		{
			name: "undefined type",
			fs: mapFS{"/req/svc.proto": []byte(`syntax = "proto3";
message M {
  Missing m = 1;
}
`)},
			paths:   []string{"svc.proto"},
			want:    "svc.proto:3:3:",
			classes: []diag.Class{diag.ClassProtocol},
		},
		{
			name:    "relative file outside every import path",
			fs:      mapFS{"/outside.proto": []byte(`syntax = "proto3";`)},
			paths:   []string{"../outside.proto"},
			imports: []string{"./proto"},
			want:    "proto file ../outside.proto does not reside in any import path",
			classes: []diag.Class{diag.ClassProtocol},
		},
		{
			name:    "missing relative file outside every import path",
			fs:      mapFS{},
			paths:   []string{"../missing.proto"},
			imports: []string{".", "proto"},
			want:    "read grpc proto file ../missing.proto",
			classes: []diag.Class{diag.ClassFilesystem},
		},
		{
			name:    "unreadable relative file outside every import path",
			fs:      &errFS{err: os.ErrPermission},
			paths:   []string{"../outside.proto"},
			imports: []string{"./proto"},
			want:    "proto file ../outside.proto does not reside in any import path",
			classes: []diag.Class{diag.ClassProtocol},
		},
		{
			name:      "relative file outside every import path in a fallback folder",
			fs:        mapFS{"/fb/x.proto": []byte(`syntax = "proto3";`)},
			paths:     []string{"../x.proto"},
			imports:   []string{"./proto"},
			fallbacks: []string{"/fb/sub"},
			want:      "proto file ../x.proto does not reside in any import path",
			classes:   []diag.Class{diag.ClassProtocol},
		},
		{
			name:    "import name in no import path",
			fs:      mapFS{"/req/proto/acme/svc.proto": []byte(`syntax = "proto3";`)},
			paths:   []string{"acme/missing.proto"},
			imports: []string{".", "proto"},
			want:    "proto file acme/missing.proto not found in any import path (., proto)",
			classes: []diag.Class{diag.ClassFilesystem},
		},
		{
			name:    "absolute file outside every import path",
			fs:      mapFS{"/elsewhere/svc.proto": []byte(`syntax = "proto3";`)},
			paths:   []string{"/elsewhere/svc.proto"},
			imports: []string{"./proto"},
			want:    "proto file /elsewhere/svc.proto does not reside in any import path",
			classes: []diag.Class{diag.ClassProtocol},
		},
		{
			name:    "missing absolute file outside every import path",
			fs:      mapFS{},
			paths:   []string{"/elsewhere/missing.proto"},
			imports: []string{"./proto"},
			want:    "read grpc proto file /elsewhere/missing.proto",
			classes: []diag.Class{diag.ClassFilesystem},
		},
		{
			name:    "file on disk outside every import path",
			fs:      mapFS{"/req/proto/acme/v1/greeter.proto": []byte(`syntax = "proto3";`)},
			paths:   []string{"./proto/acme/v1/greeter.proto"},
			imports: []string{"./vendor"},
			want:    "proto file ./proto/acme/v1/greeter.proto does not reside in any import path",
			classes: []diag.Class{diag.ClassProtocol},
		},
		{
			name: "listed file shadowed without import paths",
			fs: mapFS{
				"/req/a/svc.proto": []byte(`syntax = "proto3";
package a;
`),
				"/req/b/svc.proto": []byte(`syntax = "proto3";
package b;
`),
			},
			paths:   []string{"a/svc.proto", "b/svc.proto"},
			want:    "proto file b/svc.proto is shadowed by a/svc.proto",
			classes: []diag.Class{diag.ClassProtocol},
		},
		{
			name: "listed file shadowed by an earlier import path",
			fs: mapFS{
				"/req/p1/x.proto": []byte(`syntax = "proto3";
package p1;
`),
				"/req/p2/x.proto": []byte(`syntax = "proto3";
package p2;
`),
			},
			paths:   []string{"p2/x.proto"},
			imports: []string{"p1", "p2"},
			want:    "proto file p2/x.proto is shadowed by p1/x.proto",
			classes: []diag.Class{diag.ClassProtocol},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := protoSource{
				paths:   tt.paths,
				imports: tt.imports,
				rd:      newReader(tt.fs, Options{BaseDir: "/req", FallbackBaseDirs: tt.fallbacks}),
			}
			_, err := src.files(context.Background(), methodID{})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if tt.absent != "" && strings.Contains(err.Error(), tt.absent) {
				t.Fatalf("err = %v, want no mention of %q", err, tt.absent)
			}
			if got := diag.Classes(err); !slices.Equal(got, tt.classes) {
				t.Fatalf("classes = %v, want %v", got, tt.classes)
			}
		})
	}
}

func TestProtoSources(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		gr      restfile.GRPCRequest
		want    []string
		wantErr string
	}{
		{
			name: "vendored standard import",
			files: map[string]string{
				"proto/svc.proto": `syntax = "proto3";
import "google/protobuf/timestamp.proto";
message Event { google.protobuf.Timestamp at = 1; }
`,
				"vendor/google/protobuf/timestamp.proto": `syntax = "proto3";
package google.protobuf;
message Timestamp {
  int64 seconds = 1;
  int32 nanos = 2;
}
`,
				"vendor/unused.proto": `syntax = "proto3";
message Unused {}
`,
			},
			gr: restfile.GRPCRequest{
				ProtoFiles:  []string{"proto/svc.proto"},
				ImportPaths: []string{"proto", "vendor"},
			},
			want: []string{"proto/svc.proto", "vendor/google/protobuf/timestamp.proto"},
		},
		{
			name: "import name searched past the request dir",
			files: map[string]string{
				"proto/acme/service.proto": `syntax = "proto3";
import "acme/types.proto";
`,
				"proto/acme/types.proto": `syntax = "proto3";`,
			},
			gr: restfile.GRPCRequest{
				ProtoFiles:  []string{"acme/service.proto"},
				ImportPaths: []string{".", "proto"},
			},
			want: []string{"proto/acme/service.proto", "proto/acme/types.proto"},
		},
		{
			name: "compile error",
			files: map[string]string{
				"proto/svc.proto": `syntax = "proto3";
import "missing.proto";
`,
			},
			gr:      restfile.GRPCRequest{ProtoFiles: []string{"proto/svc.proto"}},
			wantErr: "svc.proto:2:8:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, data := range tt.files {
				path := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("mkdir for %s: %v", name, err)
				}
				if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}

			got, err := ProtoSources(&tt.gr, Options{BaseDir: dir})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("proto sources = %q, err = %v, want %q", got, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("proto sources: %v", err)
			}
			want := make([]string, len(tt.want))
			for i, p := range tt.want {
				want[i] = filepath.Join(dir, p)
			}
			if !slices.Equal(got, want) {
				t.Fatalf("proto sources = %q, want %q", got, want)
			}
		})
	}
}

func TestProtoFiles(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string]string{
		"proto/acme/svc.proto": `syntax = "proto3";
import "acme/types.proto";
`,
		"proto/acme/types.proto":             `syntax = "proto3";`,
		"proto/acme/broken.proto":            `syntax = "proto3"; message {`,
		"vendor/acme/svc.proto":              `syntax = "proto3";`,
		"vendor/google/protobuf/empty.proto": `syntax = "proto3";`,
		"p1/x.proto":                         `syntax = "proto3";`,
		"p2/x.proto":                         `syntax = "proto3";`,
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	tests := []struct {
		name    string
		gr      restfile.GRPCRequest
		want    []string
		wantErr string
	}{
		{
			name: "path inside an import path leaves its import out",
			gr: restfile.GRPCRequest{
				ProtoFiles:  []string{"proto/acme/svc.proto"},
				ImportPaths: []string{"proto"},
			},
			want: []string{"proto/acme/svc.proto"},
		},
		{
			name: "name found in an import path",
			gr: restfile.GRPCRequest{
				ProtoFiles:  []string{"acme/svc.proto"},
				ImportPaths: []string{"proto"},
			},
			want: []string{"proto/acme/svc.proto"},
		},
		{
			name: "first import path that holds the name",
			gr: restfile.GRPCRequest{
				ProtoFiles:  []string{"acme/svc.proto"},
				ImportPaths: []string{"vendor", "proto"},
			},
			want: []string{"vendor/acme/svc.proto"},
		},
		{
			name: "name found past an import path that holds its path",
			gr: restfile.GRPCRequest{
				ProtoFiles:  []string{"acme/svc.proto"},
				ImportPaths: []string{".", "proto"},
			},
			want: []string{"proto/acme/svc.proto"},
		},
		{
			name: "vendor before proto after the request dir",
			gr: restfile.GRPCRequest{
				ProtoFiles:  []string{"acme/svc.proto"},
				ImportPaths: []string{".", "vendor", "proto"},
			},
			want: []string{"vendor/acme/svc.proto"},
		},
		{
			name: "proto before vendor after the request dir",
			gr: restfile.GRPCRequest{
				ProtoFiles:  []string{"acme/svc.proto"},
				ImportPaths: []string{".", "proto", "vendor"},
			},
			want: []string{"proto/acme/svc.proto"},
		},
		{
			name: "file that does not compile",
			gr: restfile.GRPCRequest{
				ProtoFiles:  []string{"acme/broken.proto"},
				ImportPaths: []string{"proto"},
			},
			want: []string{"proto/acme/broken.proto"},
		},
		{
			name: "standard import",
			gr: restfile.GRPCRequest{
				ProtoFiles:  []string{"google/protobuf/empty.proto"},
				ImportPaths: []string{"proto"},
			},
		},
		{
			name: "vendored standard import",
			gr: restfile.GRPCRequest{
				ProtoFiles:  []string{"google/protobuf/empty.proto"},
				ImportPaths: []string{"vendor"},
			},
			want: []string{"vendor/google/protobuf/empty.proto"},
		},
		{
			name: "missing name",
			gr: restfile.GRPCRequest{
				ProtoFiles:  []string{"acme/missing.proto"},
				ImportPaths: []string{"proto"},
			},
			wantErr: "acme/missing.proto",
		},
		{
			name: "name in no import path",
			gr: restfile.GRPCRequest{
				ProtoFiles:  []string{"acme/missing.proto"},
				ImportPaths: []string{".", "proto"},
			},
			wantErr: "proto file acme/missing.proto not found in any import path (., proto)",
		},
		{
			name: "shadowed path",
			gr: restfile.GRPCRequest{
				ProtoFiles:  []string{"p2/x.proto"},
				ImportPaths: []string{"p1", "p2"},
			},
			wantErr: "proto file p2/x.proto is shadowed by p1/x.proto",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ProtoFiles(&tt.gr, Options{BaseDir: dir})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("proto files = %q, err = %v, want %q", got, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("proto files: %v", err)
			}
			want := make([]string, len(tt.want))
			for i, p := range tt.want {
				want[i] = filepath.Join(dir, p)
			}
			if !slices.Equal(got, want) {
				t.Fatalf("proto files = %q, want %q", got, want)
			}
		})
	}
}

func TestProtoSourceUsesLookupRoots(t *testing.T) {
	fs := mapFS{"/fallback/proto/svc.proto": []byte(`syntax = "proto3";
message M {}
`)}

	opt := Options{BaseDir: "/req", FallbackBaseDirs: []string{"/fallback"}}
	src := protoSource{paths: []string{"proto/svc.proto"}, rd: newReader(fs, opt)}
	if _, err := src.files(context.Background(), methodID{}); err != nil {
		t.Fatalf("files under the fallback root: %v", err)
	}

	opt.NoFallback = true
	src.rd = newReader(fs, opt)
	_, err := src.files(context.Background(), methodID{})
	if err == nil {
		t.Fatal("expected the fallback root to be skipped")
	}
	if got := diag.Classes(err); !slices.Contains(got, diag.ClassFilesystem) {
		t.Fatalf("classes = %v, want %q", got, diag.ClassFilesystem)
	}
}

const testServiceProto = `syntax = "proto3";
package grpc.testing;

message Payload {
  bytes body = 2;
}

message SimpleRequest {
  Payload payload = 3;
}

message SimpleResponse {
  Payload payload = 1;
}

service TestService {
  rpc UnaryCall(SimpleRequest) returns (SimpleResponse);
}
`

func TestExecuteUnaryWithProtoFiles(t *testing.T) {
	addr := startTestServerWith(t, nil)
	fs := mapFS{"/req/proto/test.proto": []byte(testServiceProto)}
	req := &restfile.Request{GRPC: &restfile.GRPCRequest{
		Target:     addr,
		FullMethod: "/grpc.testing.TestService/UnaryCall",
		ProtoFiles: []string{"proto/test.proto"},
		Message:    `{"payload":{"body":"aGk="}}`,
	}}
	opts := Options{BaseDir: "/req", DefaultPlaintext: restfile.OptOf(true), DialTimeout: time.Second}

	resp, err := NewClient(WithFileSystem(fs)).Execute(context.Background(), req, opts, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var out struct {
		Payload struct {
			Body []byte `json:"body"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatalf("decode response body %s: %v", resp.Body, err)
	}
	if string(out.Payload.Body) != "hi" {
		t.Fatalf("response = %s, want the payload echoed back", resp.Body)
	}
}
