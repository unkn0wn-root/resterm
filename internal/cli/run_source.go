package cli

import (
	"fmt"
	"path/filepath"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	str "github.com/unkn0wn-root/resterm/internal/util"
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
	if len(doc.Errors) > 0 {
		err := doc.Errors[0]
		msg := str.Trim(err.Message)
		if err.Line > 0 {
			return nil, fmt.Errorf("parse error at line %d: %s", err.Line, msg)
		}
		return nil, fmt.Errorf("parse error: %s", msg)
	}
	return doc, nil
}
