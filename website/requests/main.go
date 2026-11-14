package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"os"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/files"
	"github.com/unkn0wn-root/resterm/internal/parser"
)

type entry struct {
	Line   int    `json:"line"`
	Method string `json:"method"`
	Name   string `json:"name"`
}

type file struct {
	SHA256   string  `json:"sha256"`
	Requests []entry `json:"requests"`
}

func main() {
	out := flag.String("out", "website/src/content/requests.json", "output file")
	flag.Parse()

	js, err := build(os.DirFS("."))
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, js, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s (%d bytes)", *out, len(js))
}

// build lists the requests and workflows in each request file under
// _examples, in the order the Files pane shows them.
func build(fsys fs.FS) ([]byte, error) {
	out := map[string]file{}
	err := fs.WalkDir(fsys, "_examples", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// The site's glob skips dotfiles, so editor swap and lock files never get an entry.
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !files.IsRequest(path) {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		doc := parser.Parse(path, data)
		reqs := make([]entry, 0, len(doc.Requests)+len(doc.Workflows))
		for _, c := range cli.BuildRunRequestChoices(doc) {
			reqs = append(reqs, entry{c.Line, c.Method, c.Name})
		}
		for _, wf := range doc.Workflows {
			reqs = append(reqs, entry{wf.LineRange.Start, "WF", wf.Name})
		}
		sum := sha256.Sum256(data)
		out[path] = file{SHA256: hex.EncodeToString(sum[:]), Requests: reqs}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
