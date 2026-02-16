package collection

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/parser"
)

const (
	ManifestFile = "manifest.json"

	defaultEnvTemplateFile = "resterm.env.example.json"
	defaultEnvSourceFile   = "resterm.env.json"
	altEnvSourceFile       = "rest-client.env.json"

	envPlaceholder = "REPLACE_ME"
)

type ExportOptions struct {
	Workspace string
	OutDir    string
	Name      string
	Recursive bool
	Force     bool
}

type ExportResult struct {
	OutDir       string
	ManifestPath string
	FileCount    int
}

type expFile struct {
	Role FileRole
	Data []byte
}

type bundleFile struct {
	Path string
	Data []byte
}

func ExportBundle(o ExportOptions) (ExportResult, error) {
	wsAbs, wsReal, err := resolveDir(o.Workspace, "workspace")
	if err != nil {
		return ExportResult{}, err
	}

	outAbs, err := cleanAbsPath(o.OutDir, "output")
	if err != nil {
		return ExportResult{}, err
	}

	byPath := make(map[string]expFile)
	if err := collectRequests(byPath, wsAbs, wsReal, o.Recursive); err != nil {
		return ExportResult{}, err
	}

	envData, err := buildEnvTemplate(wsAbs, wsReal)
	if err != nil {
		return ExportResult{}, err
	}
	if err := addFile(byPath, defaultEnvTemplateFile, RoleEnvTemplate, envData); err != nil {
		return ExportResult{}, err
	}

	name := strings.TrimSpace(o.Name)
	if name == "" {
		name = filepath.Base(wsAbs)
	}

	mf, fs := buildManifest(name, byPath)
	if err := writeBundleDir(outAbs, fs, mf, o.Force); err != nil {
		return ExportResult{}, err
	}

	return ExportResult{
		OutDir:       outAbs,
		ManifestPath: filepath.Join(outAbs, ManifestFile),
		FileCount:    len(mf.Files),
	}, nil
}

func collectRequests(byPath map[string]expFile, rootAbs, rootReal string, rec bool) error {
	ents, err := filesvc.ListRequestFiles(rootAbs, rec)
	if err != nil {
		return fmt.Errorf("list request files: %w", err)
	}

	found := false
	for _, ent := range ents {
		entPath := strings.TrimSpace(ent.Path)
		if entPath == "" || !filesvc.IsRequestFile(entPath) {
			continue
		}

		found = true
		abs, rel, data, err := readWorkspaceFile(rootAbs, rootReal, rootAbs, entPath)
		if err != nil {
			return err
		}
		if err := addFileNormalized(byPath, rel, RoleRequest, data); err != nil {
			return err
		}

		doc := parser.Parse(abs, data)
		if len(doc.Errors) > 0 {
			e := doc.Errors[0]
			return fmt.Errorf("parse %s:%d: %s", rel, e.Line, e.Message)
		}

		baseDir := filepath.Dir(abs)
		for _, r := range collectRefs(doc) {
			_, depRel, depData, depErr := readWorkspaceFile(rootAbs, rootReal, baseDir, r.Path)
			if depErr != nil {
				return depErr
			}
			if err := addFileNormalized(byPath, depRel, r.Role, depData); err != nil {
				return err
			}
		}
	}

	if !found {
		return errors.New("no .http/.rest request files found in workspace")
	}
	return nil
}

// addFileNormalized stores a file keyed by a pre-normalized bundle path.
// Callers must validate rel with NormRelPath before calling.
func addFileNormalized(byPath map[string]expFile, rel string, role FileRole, data []byte) error {
	next := expFile{Role: role, Data: data}
	if cur, ok := byPath[rel]; ok {
		if !slices.Equal(cur.Data, next.Data) {
			return fmt.Errorf("path conflict for %s with different content", rel)
		}
		// Same content can appear under multiple roles; keep the most specific role.
		if roleRank(cur.Role) > roleRank(next.Role) {
			next.Role = cur.Role
		}
	}
	byPath[rel] = next
	return nil
}

func addFile(byPath map[string]expFile, rel string, role FileRole, data []byte) error {
	rel, err := NormRelPath(rel)
	if err != nil {
		return fmt.Errorf("invalid bundle path %q: %w", rel, err)
	}
	return addFileNormalized(byPath, rel, role, data)
}

func roleRank(r FileRole) int {
	switch r {
	case RoleRequest:
		return 4
	case RoleEnvTemplate:
		return 3
	case RoleScript:
		return 2
	default:
		return 1
	}
}

func buildManifest(name string, byPath map[string]expFile) (Manifest, []bundleFile) {
	files := make([]File, 0, len(byPath))
	out := make([]bundleFile, 0, len(byPath))
	for p, f := range byPath {
		files = append(files, File{
			Path:   p,
			Role:   f.Role,
			Size:   int64(len(f.Data)),
			Digest: SumSHA256(f.Data),
		})
		out = append(out, bundleFile{Path: p, Data: f.Data})
	}

	// Files are already valid by construction:
	// paths come from NormRelPath, roles are from fixed enums, size and digest
	// are derived directly from file bytes.
	return NewManifest(name, files), out
}

func writeBundleDir(outAbs string, files []bundleFile, m Manifest, force bool) error {
	parent := filepath.Dir(outAbs)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create output parent dir: %w", err)
	}
	parentReal := parent
	if p, err := filepath.EvalSymlinks(parent); err == nil {
		parentReal = p
	}
	outReal := filepath.Join(parentReal, filepath.Base(outAbs))

	if !force {
		if _, err := os.Stat(outReal); err == nil {
			return fmt.Errorf("output path already exists: %s", outAbs)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat output path: %w", err)
		}
	}

	tmp, err := os.MkdirTemp(parentReal, ".resterm-collection-*")
	if err != nil {
		return fmt.Errorf("create temp bundle dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	for _, f := range files {
		dst, err := SafeJoin(tmp, f.Path)
		if err != nil {
			return fmt.Errorf("prepare bundle path %s: %w", f.Path, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("create bundle dir for %s: %w", f.Path, err)
		}
		if err := os.WriteFile(dst, f.Data, 0o644); err != nil {
			return fmt.Errorf("write bundle file %s: %w", f.Path, err)
		}
	}

	manData, err := EncodeManifest(m)
	if err != nil {
		return err
	}
	manPath := filepath.Join(tmp, ManifestFile)
	if err := os.WriteFile(manPath, manData, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	if force {
		if err := os.RemoveAll(outReal); err != nil {
			return fmt.Errorf("remove previous output: %w", err)
		}
	}
	if err := os.Rename(tmp, outReal); err != nil {
		return fmt.Errorf("move bundle into place: %w", err)
	}
	return nil
}
