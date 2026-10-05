package mock

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/watcher"
)

type Reloader struct {
	src Sources

	digest   string
	fp       string
	fixtures []string
	failing  bool
}

func NewReloader(src Sources) *Reloader {
	return &Reloader{src: src}
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
	h, err := Load(r.src, doc)
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
	res, err := r.src.resolve()
	if err != nil {
		return ""
	}

	h := sha256.New()
	settled := true
	for _, f := range res.files {
		settled = writeStat(h, f) && settled
	}
	for _, f := range r.fixtures {
		settled = writeStat(h, f) && settled
	}
	if !settled {
		// An empty fingerprint makes Reload compare content instead.
		return ""
	}
	_, _ = fmt.Fprintf(h, "%s\x00%d\x00", overlayPath, len(overlay))
	_, _ = h.Write(overlay)
	return hex.EncodeToString(h.Sum(nil))
}

func writeStat(w io.Writer, path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		_, _ = fmt.Fprintf(w, "%s\x00missing\x00", path)
		return true
	}
	_, _ = fmt.Fprintf(w, "%s\x00%d\x00%d\x00", path, info.Size(), info.ModTime().UnixNano())
	return watcher.Settled(info.ModTime())
}
