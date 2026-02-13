package collection

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ImportOptions struct {
	BundleDir string
	Workspace string
	Force     bool
	DryRun    bool
}

type ImportAction string

const (
	ImportCreate    ImportAction = "create"
	ImportOverwrite ImportAction = "overwrite"
)

type ImportOp struct {
	Action ImportAction
	Path   string
	Size   int64
}

type ImportResult struct {
	BundleDir   string
	Workspace   string
	FileCount   int
	Created     int
	Overwritten int
	Ops         []ImportOp
}

type impFile struct {
	op  ImportOp
	dst string
	raw []byte
}

func ImportBundle(o ImportOptions) (ImportResult, error) {
	bundleAbs, bundleReal, err := resolveDir(o.BundleDir, "bundle")
	if err != nil {
		return ImportResult{}, err
	}
	wsAbs, wsReal, err := prepImportWorkspace(o.Workspace, o.DryRun)
	if err != nil {
		return ImportResult{}, err
	}

	mf, err := readBundleManifest(bundleAbs)
	if err != nil {
		return ImportResult{}, err
	}
	plan, err := buildImportPlan(mf, bundleAbs, bundleReal, wsAbs, o.Force)
	if err != nil {
		return ImportResult{}, err
	}

	if !o.DryRun {
		if err := applyImportPlan(plan, wsAbs, wsReal); err != nil {
			return ImportResult{}, err
		}
	}

	res := ImportResult{
		BundleDir: bundleAbs,
		Workspace: wsAbs,
		FileCount: len(plan),
		Ops:       make([]ImportOp, 0, len(plan)),
	}
	for _, f := range plan {
		res.Ops = append(res.Ops, f.op)
		switch f.op.Action {
		case ImportCreate:
			res.Created++
		case ImportOverwrite:
			res.Overwritten++
		}
	}
	return res, nil
}

func prepImportWorkspace(path string, dry bool) (string, string, error) {
	abs, err := cleanAbsPath(path, "workspace")
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("stat workspace: %w", err)
		}
		if dry {
			return abs, abs, nil
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return "", "", fmt.Errorf("create workspace dir: %w", err)
		}
	} else if !info.IsDir() {
		return "", "", fmt.Errorf("workspace is not a directory: %s", abs)
	}
	real := abs
	if p, err := filepath.EvalSymlinks(abs); err == nil {
		real = p
	}
	return abs, real, nil
}

func readBundleManifest(bundleAbs string) (Manifest, error) {
	path := filepath.Join(bundleAbs, ManifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	mf, err := DecodeManifest(data)
	if err != nil {
		return Manifest{}, err
	}
	if len(mf.Files) == 0 {
		return Manifest{}, errors.New("bundle manifest has no files")
	}
	if err := validateImportPaths(mf.Files); err != nil {
		return Manifest{}, err
	}
	return mf, nil
}

func validateImportPaths(files []File) error {
	for i := 0; i < len(files)-1; i++ {
		a := files[i].Path
		b := files[i+1].Path
		if strings.HasPrefix(b, a+"/") {
			return fmt.Errorf(
				"invalid manifest: path %q conflicts with nested path %q",
				a,
				b,
			)
		}
	}
	return nil
}

func buildImportPlan(
	mf Manifest,
	bundleAbs, bundleReal, wsAbs string,
	force bool,
) ([]impFile, error) {
	out := make([]impFile, 0, len(mf.Files))
	for _, f := range mf.Files {
		raw, err := readBundlePayload(bundleAbs, bundleReal, f)
		if err != nil {
			return nil, err
		}
		dst, err := SafeJoin(wsAbs, f.Path)
		if err != nil {
			return nil, fmt.Errorf("prepare destination path %s: %w", f.Path, err)
		}

		act, err := importAction(dst, force)
		if err != nil {
			return nil, fmt.Errorf("prepare destination path %s: %w", f.Path, err)
		}
		out = append(out, impFile{
			op: ImportOp{
				Action: act,
				Path:   f.Path,
				Size:   f.Size,
			},
			dst: dst,
			raw: raw,
		})
	}
	return out, nil
}

func readBundlePayload(bundleAbs, bundleReal string, f File) ([]byte, error) {
	src, err := SafeJoin(bundleAbs, f.Path)
	if err != nil {
		return nil, fmt.Errorf("prepare source path %s: %w", f.Path, err)
	}

	info, err := os.Stat(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("bundle file not found: %s", f.Path)
		}
		return nil, fmt.Errorf("stat bundle file %s: %w", f.Path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("bundle path is a directory: %s", f.Path)
	}

	real := src
	if p, err := filepath.EvalSymlinks(src); err == nil {
		real = p
	}
	if !withinBase(bundleReal, real) {
		return nil, fmt.Errorf(
			"bundle file escapes bundle root via symlink: %s (%s)",
			f.Path,
			real,
		)
	}

	raw, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("read bundle file %s: %w", f.Path, err)
	}
	if int64(len(raw)) != f.Size {
		return nil, fmt.Errorf(
			"bundle file size mismatch for %s: got %d want %d",
			f.Path,
			len(raw),
			f.Size,
		)
	}
	if !VerifyDigest(raw, f.Digest) {
		return nil, fmt.Errorf("bundle file checksum mismatch for %s", f.Path)
	}
	return raw, nil
}

func importAction(dst string, force bool) (ImportAction, error) {
	info, err := os.Lstat(dst)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ImportCreate, nil
		}
		return "", err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("destination is a symlink: %s", dst)
	}
	if info.IsDir() {
		return "", fmt.Errorf("destination is a directory: %s", dst)
	}
	if !force {
		return "", fmt.Errorf("destination file already exists: %s", dst)
	}
	return ImportOverwrite, nil
}

func applyImportPlan(plan []impFile, wsAbs, wsReal string) error {
	for _, f := range plan {
		if err := checkNoSymlinkEscape(wsAbs, wsReal, f.dst); err != nil {
			return fmt.Errorf("destination %s: %w", f.op.Path, err)
		}

		dir := filepath.Dir(f.dst)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create destination dir for %s: %w", f.op.Path, err)
		}
		if err := checkNoSymlinkEscape(wsAbs, wsReal, f.dst); err != nil {
			return fmt.Errorf("destination %s: %w", f.op.Path, err)
		}
		if err := writeFileAtomic(f.dst, f.raw, 0o644); err != nil {
			return fmt.Errorf("write destination file %s: %w", f.op.Path, err)
		}
	}
	return nil
}

func checkNoSymlinkEscape(baseAbs, baseReal, dst string) error {
	if !withinBase(baseAbs, dst) {
		return fmt.Errorf("path escapes workspace root (%s)", dst)
	}
	rel, err := filepath.Rel(baseAbs, dst)
	if err != nil {
		return err
	}
	rel = filepath.Clean(rel)
	if rel == "." {
		return nil
	}

	cur := baseAbs
	for _, seg := range strings.Split(rel, string(os.PathSeparator)) {
		if seg == "" || seg == "." {
			continue
		}
		cur = filepath.Join(cur, seg)
		info, err := os.Lstat(cur)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		real, err := filepath.EvalSymlinks(cur)
		if err != nil {
			return err
		}
		if !withinBase(baseReal, real) {
			return fmt.Errorf("path escapes workspace via symlink (%s)", real)
		}
	}
	return nil
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".resterm-import-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
