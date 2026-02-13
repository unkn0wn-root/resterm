package collection

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

func (m Manifest) Normalize() (Manifest, error) {
	out := m

	out.Schema = strings.TrimSpace(out.Schema)
	if out.Schema == "" {
		out.Schema = SchemaName
	}
	if out.Schema != SchemaName {
		return Manifest{}, fmt.Errorf("unsupported schema %q", out.Schema)
	}

	if out.Version == 0 {
		out.Version = SchemaVersion
	}
	if out.Version != SchemaVersion {
		return Manifest{}, fmt.Errorf("unsupported schema version %d", out.Version)
	}

	out.Name = strings.TrimSpace(out.Name)

	fs := slices.Clone(out.Files)
	seen := make(map[string]struct{}, len(fs))
	for i := range fs {
		f, err := normFile(fs[i])
		if err != nil {
			return Manifest{}, fmt.Errorf("file %d: %w", i, err)
		}
		if _, ok := seen[f.Path]; ok {
			return Manifest{}, fmt.Errorf("duplicate file path %q", f.Path)
		}
		seen[f.Path] = struct{}{}
		fs[i] = f
	}
	slices.SortFunc(fs, func(a, b File) int {
		return strings.Compare(a.Path, b.Path)
	})
	out.Files = fs
	return out, nil
}

func EncodeManifest(m Manifest) ([]byte, error) {
	nm, err := m.Normalize()
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(nm, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	return ensureTrailingNewline(data), nil
}

func DecodeManifest(data []byte) (Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}

	// Reject trailing JSON values.
	var tail any
	if err := dec.Decode(&tail); !errors.Is(err, io.EOF) {
		return Manifest{}, errors.New("decode manifest: trailing data")
	}

	return m.Normalize()
}

func normFile(f File) (File, error) {
	p, err := NormRelPath(f.Path)
	if err != nil {
		return File{}, err
	}
	f.Path = p

	if !f.Role.valid() {
		return File{}, fmt.Errorf("invalid role %q", f.Role)
	}
	if f.Size < 0 {
		return File{}, fmt.Errorf("negative size for %q", f.Path)
	}

	d := normDigest(f.Digest)
	if d.Alg == "" {
		d.Alg = AlgSHA256
	}
	if d.Alg != AlgSHA256 {
		return File{}, fmt.Errorf("unsupported digest algorithm %q for %q", d.Alg, f.Path)
	}
	if !ValidSHA256(d.Value) {
		return File{}, fmt.Errorf("invalid sha256 digest for %q", f.Path)
	}
	f.Digest = d

	return f, nil
}

func ensureTrailingNewline(data []byte) []byte {
	if len(data) == 0 || data[len(data)-1] != '\n' {
		return append(data, '\n')
	}
	return data
}
