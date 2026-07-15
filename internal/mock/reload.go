package mock

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type Reloader struct {
	root      string
	recursive bool

	digest   string
	fp       string
	fixtures []string
	failing  bool
}

func NewReloader(root string, recursive bool) *Reloader {
	return &Reloader{root: root, recursive: recursive}
}

func (r *Reloader) Reload(overlayPath string, overlay []byte) (*Handler, error) {
	fp := r.fingerprint(overlayPath, overlay)
	if !r.failing && fp != "" && fp == r.fp {
		return nil, nil
	}

	var doc *restfile.Document
	if overlay != nil {
		doc = parser.Parse(overlayPath, overlay)
	}
	h, err := Load(r.root, r.recursive, doc)
	if err != nil {
		r.failing = true
		return nil, err
	}

	r.fp = fp
	r.fixtures = h.fixtures
	if !r.failing && h.Digest() == r.digest {
		return nil, nil
	}
	r.failing = false
	r.digest = h.Digest()
	return h, nil
}

func (r *Reloader) fingerprint(overlayPath string, overlay []byte) string {
	root := strings.TrimSpace(r.root)
	if root == "" {
		root = "."
	}
	info, err := os.Stat(root)
	if err != nil {
		return ""
	}

	h := sha256.New()
	if info.IsDir() {
		entries, err := filesvc.ListRequestFiles(root, r.recursive)
		if err != nil {
			return ""
		}
		for _, e := range entries {
			if e.Kind == filesvc.FileKindRequest {
				writeStat(h, e.Path)
			}
		}
	} else {
		writeStat(h, root)
	}
	for _, f := range r.fixtures {
		writeStat(h, f)
	}
	fmt.Fprintf(h, "%s\x00%d\x00", overlayPath, len(overlay))
	h.Write(overlay)
	return hex.EncodeToString(h.Sum(nil))
}

func writeStat(hash io.Writer, path string) {
	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(hash, "%s\x00missing\x00", path)
		return
	}
	fmt.Fprintf(hash, "%s\x00%d\x00%d\x00", path, info.Size(), info.ModTime().UnixNano())
}
