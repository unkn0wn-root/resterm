package ui

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/curl"
	"github.com/unkn0wn-root/resterm/internal/http/version"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (m *Model) requestAtCursor(
	doc *restfile.Document,
	content string,
	cursorLine int,
) (*restfile.Request, bool) {
	if req, _ := requestAtLine(doc, cursorLine); req != nil {
		return req, false
	}
	if inline := buildInlineRequest(content, cursorLine); inline != nil {
		return inline, true
	}
	if doc != nil && len(doc.Requests) > 0 {
		last := doc.Requests[len(doc.Requests)-1]
		if last != nil && cursorLine > last.LineRange.End {
			return last, false
		}
	}
	return nil, false
}

func buildInlineRequest(content string, lineNumber int) *restfile.Request {
	if lineNumber < 1 {
		return nil
	}

	lines := strings.Split(content, "\n")
	if req := inlineCurlRequest(lines, lineNumber); req != nil {
		return req
	}

	if lineNumber > len(lines) {
		return nil
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

func inlineRequestFromLine(raw string, lineNumber int) *restfile.Request {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	method := "GET"
	url := ""

	fields := strings.Fields(trimmed)
	fields, ver := version.SplitToken(fields)
	if len(fields) == 1 {
		url = fields[0]
	} else if len(fields) >= 2 {
		candidate := strings.ToUpper(fields[0])
		if isInlineHTTPMethod(candidate) {
			method = candidate
			url = fields[1]
		}
	}

	if url == "" {
		url = strings.Join(fields, " ")
	}

	url = strings.Trim(url, "\"'")
	if !looksLikeHTTPRequestURL(url) {
		return nil
	}

	return &restfile.Request{
		Method: method,
		URL:    url,
		LineRange: restfile.LineRange{
			Start: lineNumber,
			End:   lineNumber,
		},
		OriginalText: raw,
		Settings:     version.SetIfMissing(nil, ver),
	}
}

func isInlineHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func looksLikeHTTPRequestURL(url string) bool {
	if url == "" {
		return false
	}
	lower := strings.ToLower(url)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "ws://") ||
		strings.HasPrefix(lower, "wss://")
}
