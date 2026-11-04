package cli

import (
	"path/filepath"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/redact"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const stdinRunName = "stdin.http"

type RunSource struct {
	Path  string
	Data  []byte
	Stdin bool
}

// StdinRunPath gives stdin-backed runs a stable request-file path so relative
// resolution still has a workspace anchor.
func StdinRunPath(workspace string) string {
	root := resolveWorkspace("", workspace)
	return filepath.Join(root, stdinRunName)
}

func ParseRunDoc(src RunSource) (*restfile.Document, error) {
	doc := parser.Parse(src.Path, src.Data)
	if err := parser.Check(doc); err != nil {
		return nil, diag.Redact(err, redact.New(doc.SecretValues()...).Text)
	}
	return doc, nil
}
