package grpcx

import (
	"os"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/filelookup"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/protobuf/proto"
)

type mapFS map[string][]byte

func (m mapFS) ReadFile(name string) ([]byte, error) {
	if data, ok := m[name]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

type errFS struct {
	err   error
	tried []string
}

func (f *errFS) ReadFile(name string) ([]byte, error) {
	f.tried = append(f.tried, name)
	return nil, f.err
}

func TestReaderPrefersBaseDirThenFallbacks(t *testing.T) {
	fs := mapFS{
		"/fallback/msg.json": []byte(`{"from":"fallback"}`),
		"/base/msg.json":     []byte(`{"from":"base"}`),
	}
	rd := newReader(fs, Options{
		BaseDir:          "/base",
		FallbackBaseDirs: []string{"/fallback"},
	})

	got, err := rd.read("msg.json", "grpc message file")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != `{"from":"base"}` {
		t.Fatalf("read = %s, want the base dir copy", got)
	}
}

func TestReaderFallsBackWhenBaseDirMisses(t *testing.T) {
	fs := mapFS{"/fallback/msg.json": []byte(`{"from":"fallback"}`)}
	rd := newReader(fs, Options{
		BaseDir:          "/base",
		FallbackBaseDirs: []string{"/fallback"},
	})

	got, err := rd.read("msg.json", "grpc message file")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != `{"from":"fallback"}` {
		t.Fatalf("read = %s, want the fallback copy", got)
	}
}

func TestReaderHonoursNoFallback(t *testing.T) {
	fs := mapFS{"/fallback/msg.json": []byte(`{"from":"fallback"}`)}
	rd := newReader(fs, Options{
		BaseDir:          "/base",
		FallbackBaseDirs: []string{"/fallback"},
		NoFallback:       true,
	})

	_, err := rd.read("msg.json", "grpc message file")
	if err == nil {
		t.Fatal("expected the fallback dir to be skipped")
	}
	if got := diag.ClassOf(err); got != diag.ClassFilesystem {
		t.Fatalf("class = %q, want %q", got, diag.ClassFilesystem)
	}
	if !strings.Contains(err.Error(), "grpc message file") {
		t.Fatalf("expected the label in %v", err)
	}
}

func TestReaderStopsOnPermissionError(t *testing.T) {
	fs := &errFS{err: os.ErrPermission}
	rd := newReader(fs, Options{
		BaseDir:          "/base",
		FallbackBaseDirs: []string{"/fallback"},
	})

	_, err := rd.read("msg.json", "grpc message file")
	if err == nil {
		t.Fatal("expected the permission error to surface")
	}
	if len(fs.tried) != 1 || fs.tried[0] != "/base/msg.json" {
		t.Fatalf("tried = %v, want only the base dir candidate", fs.tried)
	}
}

func TestReaderTriesEveryCandidateOnMiss(t *testing.T) {
	fs := &errFS{err: os.ErrNotExist}
	rd := newReader(fs, Options{
		BaseDir:          "/base",
		FallbackBaseDirs: []string{"/fallback"},
	})

	if _, err := rd.read("msg.json", "grpc message file"); err == nil {
		t.Fatal("expected a not found error")
	}
	want := []string{"/base/msg.json", "/fallback/msg.json", "msg.json"}
	if len(fs.tried) != len(want) {
		t.Fatalf("tried = %v, want %v", fs.tried, want)
	}
	for i, path := range want {
		if fs.tried[i] != path {
			t.Fatalf("tried = %v, want %v", fs.tried, want)
		}
	}
}

func TestReaderRejectsEmptyPath(t *testing.T) {
	_, err := newReader(filelookup.OSFileSystem{}, Options{}).read("", "grpc descriptor")
	if err == nil || !strings.Contains(err.Error(), "path is empty") {
		t.Fatalf("err = %v, want an empty path error", err)
	}
}

func TestResolveMessageReadsFromFileSystem(t *testing.T) {
	fs := mapFS{"/base/msg.json": []byte(`{"id":"abc"}`)}
	rd := newReader(fs, Options{BaseDir: "/base"})

	got, err := resolveMessage(&restfile.GRPCRequest{MessageFile: "msg.json"}, rd)
	if err != nil {
		t.Fatalf("resolve message: %v", err)
	}
	if got != `{"id":"abc"}` {
		t.Fatalf("message = %q", got)
	}
}

func TestReadDescriptorSetUsesFileSystem(t *testing.T) {
	data, err := proto.Marshal(testSvcDescriptorSet())
	if err != nil {
		t.Fatalf("marshal descriptor set: %v", err)
	}
	fs := mapFS{"/base/svc.protoset": data}
	rd := newReader(fs, Options{BaseDir: "/base"})

	set, err := readDescriptorSet("svc.protoset", rd)
	if err != nil {
		t.Fatalf("read descriptor set: %v", err)
	}
	if len(set.File) != 1 || set.File[0].GetName() != "svc.proto" {
		t.Fatalf("descriptor set = %v", set)
	}
}

func TestDescriptorFileErrorMentionsIncludeImports(t *testing.T) {
	set := testSvcDescriptorSet()
	set.File[0].Dependency = []string{"missing.proto"}

	_, err := filesFromDescriptorFile(set)
	if err == nil {
		t.Fatal("expected an incomplete descriptor set error")
	}
	if !strings.Contains(err.Error(), "--include_imports") {
		t.Fatalf("expected the protoc hint in %v", err)
	}
}
