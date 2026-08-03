package grpcclient

import (
	"fmt"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/filelookup"
)

type reader struct {
	fs     filelookup.FileSystem
	lookup filelookup.Lookup
}

func newReader(fs filelookup.FileSystem, opt Options) reader {
	return reader{
		fs:     fs,
		lookup: filelookup.For(opt.BaseDir, opt.FallbackBaseDirs, opt.NoFallback),
	}
}

// ReadMessageFile uses the same lookup rules as Execute.
func ReadMessageFile(path string, opt Options) ([]byte, error) {
	return newReader(filelookup.OSFileSystem{}, opt).read(path, "grpc message file")
}

func (r reader) read(path, label string) ([]byte, error) {
	if path == "" {
		return nil, diag.New(
			diag.ClassFilesystem,
			fmt.Sprintf("%s path is empty", label),
			grpcComponent,
		)
	}

	data, tried, err := r.lookup.Read(r.fs, path)
	if err == nil {
		return data, nil
	}

	op := fmt.Sprintf("read %s %s", label, path)
	if tried != path {
		op += fmt.Sprintf(" (last tried %s)", tried)
	}
	return nil, diag.WrapAs(diag.ClassFilesystem, err, op, grpcComponent)
}
