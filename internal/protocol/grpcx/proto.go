package grpcx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

type protoSource struct {
	paths   []string
	imports []string
	rd      reader
}

func (s protoSource) files(ctx context.Context, _ methodID) (*protoregistry.Files, error) {
	fds, err := s.compile(ctx)
	if err != nil {
		return nil, err
	}

	// protodesc.NewFiles would copy the GlobalFiles well-known types.
	files := new(protoregistry.Files)
	var add func(protoreflect.FileDescriptor) error
	add = func(fd protoreflect.FileDescriptor) error {
		if _, err := files.FindFileByPath(fd.Path()); err == nil {
			return nil
		}
		imps := fd.Imports()
		for i := range imps.Len() {
			if err := add(imps.Get(i).FileDescriptor); err != nil {
				return err
			}
		}
		return files.RegisterFile(fd)
	}
	for _, fd := range fds {
		if err := add(fd); err != nil {
			return nil, diag.WrapAs(diag.ClassProtocol, err, "build descriptors from proto files", grpcComponent)
		}
	}
	return files, nil
}

func (s protoSource) compile(ctx context.Context) (linker.Files, error) {
	res, names, err := s.resolve()
	if err != nil {
		return nil, err
	}
	c := protocompile.Compiler{Resolver: res}
	fds, err := c.Compile(ctx, names...)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "compile proto files", grpcComponent)
	}
	return fds, nil
}

func (s protoSource) resolve() (protocompile.Resolver, []string, error) {
	abs := func(path string) string {
		if filepath.IsAbs(path) {
			return path
		}
		return filepath.Join(s.rd.lookup.BaseDir, path)
	}

	roots := s.imports
	if len(roots) == 0 {
		roots = make([]string, len(s.paths))
		for i, p := range s.paths {
			roots[i] = filepath.Dir(p)
		}
		slices.SortFunc(roots, func(a, b string) int { return strings.Compare(abs(a), abs(b)) })
		roots = slices.Compact(roots)
	}

	// Use import names, so a file that is listed and also imported compiles once.
	names := make([]string, 0, len(s.paths))
next:
	for _, p := range s.paths {
		name := filepath.Clean(p)
		if len(s.imports) > 0 && filepath.IsLocal(name) {
			if _, err := s.rd.read(name, "grpc proto file"); errors.Is(err, fs.ErrNotExist) {
				names = append(names, filepath.ToSlash(name))
				continue
			}
		}
		for i, root := range roots {
			rel, err := filepath.Rel(abs(root), abs(p))
			if err != nil || !filepath.IsLocal(rel) {
				continue
			}
			for _, r := range roots[:i] {
				other := filepath.Join(r, rel)
				if _, err := s.rd.read(other, "grpc proto file"); err == nil {
					return nil, nil, diag.New(
						diag.ClassProtocol,
						fmt.Sprintf("proto file %s is shadowed by %s", p, other),
						grpcComponent,
					)
				}
			}
			if _, err := s.rd.read(filepath.Join(root, rel), "grpc proto file"); err != nil {
				return nil, nil, err
			}
			names = append(names, filepath.ToSlash(rel))
			continue next
		}
		return nil, nil, diag.New(
			diag.ClassProtocol,
			fmt.Sprintf("proto file %s does not reside in any import path", p),
			grpcComponent,
		)
	}

	res := protocompile.WithStandardImports(&protocompile.SourceResolver{
		ImportPaths: roots,
		Accessor: func(path string) (io.ReadCloser, error) {
			data, err := s.rd.read(path, "grpc proto file")
			if err != nil {
				return nil, err
			}
			return io.NopCloser(bytes.NewReader(data)), nil
		},
	})
	for i, name := range names {
		if _, err := res.FindFileByPath(name); err != nil {
			return nil, nil, diag.WrapAs(
				diag.ClassFilesystem,
				err,
				fmt.Sprintf("proto file %s not found in any import path (%s)", s.paths[i], strings.Join(roots, ", ")),
				grpcComponent,
			)
		}
	}
	return res, names, nil
}

func ProtoSources(gr *restfile.GRPCRequest, opt Options) ([]string, error) {
	rec := &recordFS{read: make(map[string]struct{})}
	src := protoSource{paths: gr.ProtoFiles, imports: gr.ImportPaths, rd: newReader(rec, opt)}
	if _, err := src.compile(context.Background()); err != nil {
		return nil, err
	}
	return rec.paths(), nil
}

// ProtoFiles is like ProtoSources but compiles nothing, so imports are left out.
func ProtoFiles(gr *restfile.GRPCRequest, opt Options) ([]string, error) {
	rec := &recordFS{read: make(map[string]struct{})}
	src := protoSource{paths: gr.ProtoFiles, imports: gr.ImportPaths, rd: newReader(rec, opt)}
	if _, _, err := src.resolve(); err != nil {
		return nil, err
	}
	return rec.paths(), nil
}

type recordFS struct {
	mu   sync.Mutex
	read map[string]struct{}
}

func (r *recordFS) ReadFile(name string) ([]byte, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.read[name] = struct{}{}
	r.mu.Unlock()
	return data, nil
}

func (r *recordFS) paths() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Sorted(maps.Keys(r.read))
}
