package restwriter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/parser/bodyref"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/util"
)

func renderMock(b *strings.Builder, mock *restfile.Mock) error {
	if mock == nil {
		return errors.New("writer: mock is nil")
	}
	if err := mock.CheckShape(); err != nil {
		return fmt.Errorf("writer: %w", err)
	}
	title := strings.Join(strings.Fields(mock.Title), " ")
	if title == "" {
		title = fmt.Sprintf("Mock %s %s", strings.ToUpper(mock.Method), mock.Path)
	}
	b.WriteString("### ")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(directive.Mock.Comment())
	b.WriteString(" method=")
	b.WriteString(strings.ToUpper(strings.TrimSpace(mock.Method)))
	b.WriteString(" path=")
	b.WriteString(strings.TrimSpace(mock.Path))
	if mock.Sequence != "" {
		b.WriteString(" sequence=")
		b.WriteString(mock.Sequence)
		if key := mock.SequenceKey.String(); key != "" {
			b.WriteString(" sequence-key=")
			b.WriteString(key)
		}
	} else if mock.Name != "" {
		b.WriteString(" name=")
		b.WriteString(mock.Name)
	}
	if mock.Default {
		b.WriteString(" default=true")
	}
	if mock.Latency > 0 {
		b.WriteString(" latency=")
		b.WriteString(mock.Latency.String())
	}
	if mock.DisableInterpolation {
		b.WriteString(" interpolate=false")
	}
	b.WriteString("\n")
	if mock.Expectation != nil {
		fmt.Fprintf(
			b,
			"%s calls=%d\n",
			directive.Expect.Comment(),
			mock.Expectation.Calls,
		)
	}
	if err := renderMockMatch(b, mock.Match); err != nil {
		return err
	}

	for i, resp := range mock.Responses {
		if i > 0 {
			b.WriteString(restfile.MockSequenceDelimiter + "\n")
		}
		if err := renderMockResponse(b, resp, mock.Sequence != ""); err != nil {
			return err
		}
	}
	return nil
}

func renderMockResponse(b *strings.Builder, resp restfile.MockResponse, sequence bool) error {
	file := strings.TrimSpace(resp.Body.FilePath)
	body := resp.Body.Text
	if file == "" && body != "" {
		if !restfile.ResponseAllowsBody(resp.Status) {
			return fmt.Errorf("status %d cannot have a response body", resp.Status)
		}
		var err error
		body, err = NormalizeMockBody(body)
		if err != nil {
			return err
		}
		if sequence {
			for line := range strings.SplitSeq(body, "\n") {
				if restfile.IsMockSequenceDelimiter(line) {
					return errors.New("mock sequence body contains a response delimiter")
				}
			}
		}
	}

	status := resp.Status
	fmt.Fprintf(b, "HTTP/1.1 %d", status)
	if text := http.StatusText(status); text != "" {
		b.WriteString(" ")
		b.WriteString(text)
	}
	b.WriteString("\n")
	renderHeaders(b, resp.Headers)
	b.WriteString("\n")
	if file != "" {
		b.WriteString("< ")
		b.WriteString(file)
		b.WriteString("\n")
		return nil
	}
	if body != "" {
		b.WriteString(body)
		b.WriteString("\n")
	}
	return nil
}

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

func MockNameSlug(raw string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.' {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-._")
}

func UniqueMockName(base string, used map[string]struct{}) string {
	base = strings.Trim(strings.TrimSpace(base), "-")
	if base == "" {
		base = "scenario"
	}
	if _, exists := used[base]; !exists {
		used[base] = struct{}{}
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if _, exists := used[candidate]; exists {
			continue
		}
		used[candidate] = struct{}{}
		return candidate
	}
}

func NormalizeInlineBody(body string) (string, error) {
	if !utf8.ValidString(body) {
		return "", errors.New("body is not valid UTF-8")
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.SplitSeq(body, "\n")
	for line := range lines {
		if len(line) >= 1<<20 {
			return "", errors.New("body contains a line longer than the parser limit")
		}
		if strings.HasPrefix(strings.TrimSpace(line), "###") {
			return "", errors.New("body contains a request separator")
		}
		for _, r := range line {
			if unicode.IsControl(r) && r != '\t' {
				return "", errors.New("body contains control characters")
			}
		}
	}
	return body, nil
}

func renderMockMatch(b *strings.Builder, match restfile.MockMatch) error {
	var fields []string
	if len(match.Query) > 0 {
		data, err := json.Marshal(match.Query)
		if err != nil {
			return fmt.Errorf("writer: marshal mock query matchers: %w", err)
		}
		fields = append(fields, "query="+string(data))
	}
	if len(match.Headers) > 0 {
		data, err := json.Marshal(match.Headers)
		if err != nil {
			return fmt.Errorf("writer: marshal mock header matchers: %w", err)
		}
		fields = append(fields, "headers="+string(data))
	}
	if len(match.JSON) > 0 {
		fields = append(fields, "json="+formatMockJSON(match.JSON))
	}
	if len(match.JSONRules) > 0 {
		fields = append(fields, "json-rules="+formatMockJSON(match.JSONRules))
	}
	if len(fields) == 0 {
		return nil
	}
	b.WriteString(directive.Match.Comment())
	b.WriteString(" ")
	b.WriteString(strings.Join(fields, " "))
	b.WriteString("\n")
	return nil
}

func formatMockJSON(raw []byte) string {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err == nil {
		raw = compact.Bytes()
	}
	return strconv.Quote(string(raw))
}
