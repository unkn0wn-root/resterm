package collection

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var zipTime = time.Unix(0, 0).UTC()

type PackOptions struct {
	BundleDir string
	OutFile   string
	Force     bool
}

type PackResult struct {
	BundleDir string
	OutFile   string
	FileCount int
}

type UnpackOptions struct {
	InFile string
	OutDir string
	Force  bool
}

type UnpackResult struct {
	InFile    string
	OutDir    string
	FileCount int
}

func PackBundle(o PackOptions) (PackResult, error) {
	inAbs, inReal, err := resolveDir(o.BundleDir, "bundle")
	if err != nil {
		return PackResult{}, err
	}
	outAbs, err := cleanAbsPath(o.OutFile, "output")
	if err != nil {
		return PackResult{}, err
	}

	mf, err := readBundleManifest(inAbs)
	if err != nil {
		return PackResult{}, err
	}
	man, err := EncodeManifest(mf)
	if err != nil {
		return PackResult{}, err
	}

	raw := make([][]byte, len(mf.Files))
	for i, f := range mf.Files {
		b, bErr := readBundlePayload(inAbs, inReal, f)
		if bErr != nil {
			return PackResult{}, bErr
		}
		raw[i] = b
	}

	par := filepath.Dir(outAbs)
	if err := os.MkdirAll(par, 0o755); err != nil {
		return PackResult{}, fmt.Errorf("create output parent dir: %w", err)
	}
	parReal := par
	if p, pErr := filepath.EvalSymlinks(par); pErr == nil {
		parReal = p
	}
	outReal := filepath.Join(parReal, filepath.Base(outAbs))

	fi, err := os.Lstat(outReal)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return PackResult{}, fmt.Errorf("stat output path: %w", err)
	}
	if err == nil {
		switch {
		case !o.Force:
			return PackResult{}, fmt.Errorf("output path already exists: %s", outAbs)
		case !fi.Mode().IsRegular():
			return PackResult{}, fmt.Errorf("%s exists and is not a file. Remove it or choose another path", outAbs)
		}
	}

	tmp, err := os.CreateTemp(parReal, ".resterm-collection-pack-*.zip")
	if err != nil {
		return PackResult{}, fmt.Errorf("create temp archive: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	zw := zip.NewWriter(tmp)
	if err := addZipFile(zw, ManifestFile, man); err != nil {
		_ = zw.Close()
		return PackResult{}, err
	}
	for i, f := range mf.Files {
		if err := addZipFile(zw, f.Path, raw[i]); err != nil {
			_ = zw.Close()
			return PackResult{}, err
		}
	}
	if err := zw.Close(); err != nil {
		return PackResult{}, fmt.Errorf("finalize archive: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return PackResult{}, fmt.Errorf("sync archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return PackResult{}, fmt.Errorf("close archive: %w", err)
	}
	if err := os.Rename(tmpPath, outReal); err != nil {
		return PackResult{}, fmt.Errorf("move archive into place: %w", err)
	}

	return PackResult{
		BundleDir: inAbs,
		OutFile:   outAbs,
		FileCount: len(mf.Files),
	}, nil
}

func UnpackBundle(o UnpackOptions) (UnpackResult, error) {
	inAbs, err := cleanAbsPath(o.InFile, "archive")
	if err != nil {
		return UnpackResult{}, err
	}
	info, err := os.Stat(inAbs)
	if err != nil {
		return UnpackResult{}, fmt.Errorf("stat archive: %w", err)
	}
	if info.IsDir() {
		return UnpackResult{}, fmt.Errorf("archive is a directory: %s", inAbs)
	}

	outAbs, err := cleanAbsPath(o.OutDir, "output")
	if err != nil {
		return UnpackResult{}, err
	}

	var mf Manifest
	err = replaceDir(outAbs, o.Force, func(dir string) error {
		seen, err := unpackZipToDir(inAbs, dir)
		if err != nil {
			return err
		}

		dirReal := dir
		if p, pErr := filepath.EvalSymlinks(dir); pErr == nil {
			dirReal = p
		}

		mf, err = readBundleManifest(dir)
		if err != nil {
			return err
		}

		want := make(map[string]struct{}, len(mf.Files)+1)
		want[ManifestFile] = struct{}{}
		for _, f := range mf.Files {
			want[f.Path] = struct{}{}
			if _, ok := seen[f.Path]; !ok {
				return fmt.Errorf("bundle file not found: %s", f.Path)
			}
			if _, err := readBundlePayload(dir, dirReal, f); err != nil {
				return err
			}
		}
		for p := range seen {
			if _, ok := want[p]; !ok {
				return fmt.Errorf("unexpected archive entry: %s", p)
			}
		}
		return nil
	})
	if err != nil {
		return UnpackResult{}, err
	}

	return UnpackResult{
		InFile:    inAbs,
		OutDir:    outAbs,
		FileCount: len(mf.Files),
	}, nil
}

func addZipFile(zw *zip.Writer, name string, data []byte) error {
	p, err := NormRelPath(name)
	if err != nil {
		return fmt.Errorf("invalid archive entry %q: %w", name, err)
	}

	h := &zip.FileHeader{Name: p, Method: zip.Deflate}
	h.Modified = zipTime
	h.SetMode(0o644)
	w, err := zw.CreateHeader(h)
	if err != nil {
		return fmt.Errorf("create archive entry %s: %w", p, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write archive entry %s: %w", p, err)
	}
	return nil
}

func unpackZipToDir(inAbs, outDir string) (map[string]struct{}, error) {
	zr, err := zip.OpenReader(inAbs)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer func() { _ = zr.Close() }()

	seen := make(map[string]struct{}, len(zr.File))
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimSpace(f.Name)
		if name == "" {
			return nil, errors.New("archive contains empty entry name")
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("archive entry %q is a symlink", name)
		}

		p, err := NormRelPath(name)
		if err != nil {
			return nil, fmt.Errorf("invalid archive entry %q: %w", name, err)
		}
		if _, ok := seen[p]; ok {
			return nil, fmt.Errorf("duplicate archive entry: %s", p)
		}
		seen[p] = struct{}{}

		dst, err := SafeJoin(outDir, p)
		if err != nil {
			return nil, fmt.Errorf("prepare archive path %s: %w", p, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, fmt.Errorf("create archive dir for %s: %w", p, err)
		}

		r, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open archive entry %s: %w", p, err)
		}
		w, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			_ = r.Close()
			return nil, fmt.Errorf("create extracted file %s: %w", p, err)
		}

		if f.UncompressedSize64 >= uint64(math.MaxInt64) {
			_ = w.Close()
			_ = r.Close()
			return nil, fmt.Errorf("archive entry too large: %s", p)
		}
		want := int64(f.UncompressedSize64)
		n, cpErr := io.Copy(w, io.LimitReader(r, want+1))
		clErr := w.Close()
		rErr := r.Close()
		if cpErr != nil {
			return nil, fmt.Errorf("extract archive entry %s: %w", p, cpErr)
		}
		if clErr != nil {
			return nil, fmt.Errorf("close extracted file %s: %w", p, clErr)
		}
		if rErr != nil {
			return nil, fmt.Errorf("close archive entry %s: %w", p, rErr)
		}
		if n > want {
			return nil, fmt.Errorf(
				"archive entry size exceeds declared size for %s: got more than %d",
				p,
				want,
			)
		}
		if n != want {
			return nil, fmt.Errorf(
				"archive entry size mismatch for %s: got %d want %d",
				p,
				n,
				want,
			)
		}
	}

	if _, ok := seen[ManifestFile]; !ok {
		return nil, fmt.Errorf("archive missing %s", ManifestFile)
	}
	return seen, nil
}
