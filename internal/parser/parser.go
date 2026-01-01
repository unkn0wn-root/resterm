package parser

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"regexp"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const maxScanToken = 1024 * 1024

var (
	variableLineRe = regexp.MustCompile(
		`^@(?:(global(?:-secret)?|file(?:-secret)?|request(?:-secret)?)\s+)?([A-Za-z0-9_.-]+)(?:\s*(?::|=)\s*(.+?)|\s+(\S.*))$`,
	)
	nameValueRe = regexp.MustCompile(`^([A-Za-z0-9_.-]+)(?:\s*(?::|=)\s*(.*?)|\s+(\S.*))?$`)
)

func Parse(path string, data []byte) *restfile.Document {
	scanner := bufio.NewScanner(bytes.NewReader(normalizeNewlines(data)))
	scanner.Buffer(make([]byte, 0, 1024), maxScanToken)

	doc := &restfile.Document{Path: path, Raw: data}
	builder := newDocumentBuilder(doc)

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		builder.processLine(lineNumber, line)
	}

	if err := scanner.Err(); err != nil {
		msg := fmt.Sprintf("parse error: %v", err)
		if errors.Is(err, bufio.ErrTooLong) {
			msg = fmt.Sprintf("parse error: line exceeds %d bytes", maxScanToken)
		}
		builder.addError(lineNumber+1, msg)
	}

	builder.finish()

	return doc
}

func normalizeNewlines(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}
