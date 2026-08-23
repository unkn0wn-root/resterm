package ui

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/curl"
	"github.com/unkn0wn-root/resterm/internal/http/version"
	httpbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/http"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (m *Model) requestAtCursor(
	doc *restfile.Document,
	content string,
	cursorLine int,
) (*restfile.Request, bool, error) {
	if req, _ := requestAtLine(doc, cursorLine); req != nil {
		return req, false, nil
	}
	// A line the parser refused is still the line the user is on, so report why
	// instead of falling back to the last request and running something else.
	inline, err := buildInlineRequest(content, cursorLine)
	if err != nil {
		return nil, false, err
	}
	if inline != nil {
		return inline, true, nil
	}
	if doc != nil && len(doc.Requests) > 0 {
		last := doc.Requests[len(doc.Requests)-1]
		if last != nil && cursorLine > last.LineRange.End {
			return last, false, nil
		}
	}
	return nil, false, nil
}

func buildInlineRequest(content string, lineNumber int) (*restfile.Request, error) {
	if lineNumber < 1 {
		return nil, nil
	}

	lines := strings.Split(content, "\n")
	if req := inlineCurlRequest(lines, lineNumber); req != nil {
		return req, nil
	}

	if lineNumber > len(lines) {
		return nil, nil
	}
	return inlineRequestFromLine(lines[lineNumber-1], lineNumber)
}

func inlineCurlRequest(lines []string, lineNumber int) *restfile.Request {
	idx := lineNumber - 1
	if idx < 0 || idx >= len(lines) {
		return nil
	}

	start, end, command := curl.ExtractCommand(lines, idx)
	if command == "" {
		return nil
	}

	parsed, err := curl.ParseCommand(command)
	if err != nil {
		return nil
	}
	parsed.LineRange = restfile.LineRange{Start: start + 1, End: end + 1}
	parsed.OriginalText = strings.Join(lines[start:end+1], "\n")
	return parsed
}

// Use the parser grammar so unsupported versions cannot look like non-requests.
func inlineRequestFromLine(raw string, lineNumber int) (*restfile.Request, error) {
	ml, ok, err := httpbuilder.ParseRequestLine(raw)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	return &restfile.Request{
		Method: ml.Method,
		URL:    ml.URL,
		LineRange: restfile.LineRange{
			Start: lineNumber,
			End:   lineNumber,
		},
		OriginalText: raw,
		Settings:     version.SetIfMissing(nil, ml.Version),
	}, nil
}
