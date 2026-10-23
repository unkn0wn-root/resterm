package grpcx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
	"github.com/unkn0wn-root/resterm/internal/diag"
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
		for i, root := range roots {
			rel, err := filepath.Rel(abs(root), abs(p))
			if err != nil || !filepath.IsLocal(rel) {
				continue
			}
			for _, r := range roots[:i] {
				other := filepath.Join(r, rel)
				if _, err := s.rd.read(other, "grpc proto file"); err == nil {
					return nil, diag.New(
						diag.ClassProtocol,
						fmt.Sprintf("proto file %s is shadowed by %s", p, other),
						grpcComponent,
					)
				}
			}
			if _, err := s.rd.read(filepath.Join(root, rel), "grpc proto file"); err != nil {
				return nil, err
			}
			names = append(names, filepath.ToSlash(rel))
			continue next
		}
		name := filepath.Clean(p)
		if _, err := s.rd.read(p, "grpc proto file"); err == nil || !filepath.IsLocal(name) {
			return nil, diag.New(
				diag.ClassProtocol,
				fmt.Sprintf("proto file %s does not reside in any import path", p),
				grpcComponent,
			)
		}
		names = append(names, filepath.ToSlash(name))
	}

	c := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: roots,
			Accessor: func(path string) (io.ReadCloser, error) {
				data, err := s.rd.read(path, "grpc proto file")
				if err != nil {
					return nil, err
				}
				return io.NopCloser(bytes.NewReader(data)), nil
			},
		}),
	}
	fds, err := c.Compile(ctx, names...)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "compile proto files", grpcComponent)
	}
	return fds, nil
}
