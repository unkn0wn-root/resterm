package recorder

import (
	"errors"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/mock"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func ValidateMocks(src mock.Sources, doc *restfile.Document) error {
	if len(doc.Mocks) == 0 {
		return nil
	}
	if _, err := mock.Load(src, doc); err != nil {
		return errors.New("recorded mocks conflict with the document or cannot load their fixtures")
	}
	return nil
}

func ParseOverlay(path, text string) (*restfile.Document, error) {
	doc := parser.Parse(path, []byte(text))
	if parser.Check(doc) != nil {
		return nil, errors.New("recording cannot be inserted into a document with parse errors")
	}
	return doc, nil
}

// AppendText preserves the existing final body, including its trailing newlines.
func AppendText(path, existing, block string) (string, error) {
	if block == "" {
		return existing, nil
	}
	if strings.TrimSpace(existing) == "" {
		return block, nil
	}

	before := parser.Parse(path, []byte(existing))
	for _, joint := range joints(existing) {
		text := existing + joint + block
		if holds(before, parser.Parse(path, []byte(text))) {
			return text, nil
		}
	}
	return "", errors.New("appending the recording would change existing content. End the file with ### and retry")
}

func joints(existing string) [2]string {
	if strings.HasSuffix(existing, "\n") {
		return [2]string{separatorLine, "\n" + separatorLine}
	}
	return [2]string{"\n" + separatorLine, "\n\n" + separatorLine}
}

func holds(before, after *restfile.Document) bool {
	if len(after.Requests) < len(before.Requests) || len(after.Mocks) < len(before.Mocks) {
		return false
	}
	for i, want := range before.Requests {
		if sameRequest(after.Requests[i], want) != nil {
			return false
		}
	}
	for i, want := range before.Mocks {
		if sameMock(after.Mocks[i], want) != nil {
			return false
		}
	}
	return true
}
