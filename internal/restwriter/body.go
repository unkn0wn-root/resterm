package restwriter

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/parser/bodyref"
	"github.com/unkn0wn-root/resterm/internal/util"
)

const maxBodyLineLen = 1 << 20

// A body that is only a file reference would be read back as one, so it has to
// be rejected rather than written as an inline body that changes meaning.
func NormalizeMockBody(body string) (string, error) {
	body, err := NormalizeInlineBody(body)
	if err != nil || body == "" {
		return body, err
	}
	lines := strings.Split(body, "\n")
	_, isFile := bodyref.Parse(lines[0], bodyref.Options{Location: bodyref.Line})
	if isFile && util.AllBlank(lines[1:]) {
		return "", errors.New("mock body looks like a file reference")
	}
	return body, nil
}

func NormalizeInlineBody(body string) (string, error) {
	if !utf8.ValidString(body) {
		return "", errors.New("body is not valid UTF-8")
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	for line := range strings.SplitSeq(body, "\n") {
		switch {
		case len(line) >= maxBodyLineLen:
			return "", errors.New("body contains a line longer than the parser limit")
		case strings.HasPrefix(strings.TrimSpace(line), "###"):
			return "", errors.New("body contains a request separator")
		case strings.ContainsFunc(line, isBodyControl):
			return "", errors.New("body contains control characters")
		}
	}
	return body, nil
}

// A tab is the one control character a body may carry through the parser.
func isBodyControl(r rune) bool {
	return unicode.IsControl(r) && r != '\t'
}
