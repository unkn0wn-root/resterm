package restwriter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// The responses of a sequence share one block, so the writer carries whether it
// is writing one to keep their bodies clear of the delimiter between them.
type mockWriter struct {
	directiveWriter
	sequence bool
}

func renderMock(w directiveWriter, mock *restfile.Mock) error {
	if mock == nil {
		return errors.New("writer: mock is nil")
	}
	mw := mockWriter{directiveWriter: w, sequence: mock.Sequence != ""}
	if err := mw.write(mock); err != nil {
		return fmt.Errorf("writer: %w", err)
	}
	return nil
}

func (w mockWriter) write(m *restfile.Mock) error {
	if err := m.CheckShape(); err != nil {
		return err
	}
	w.writeTitle(m)
	w.writeDeclaration(m)
	w.writeExpectation(m.Expectation)
	if err := w.writeMatch(m.Match); err != nil {
		return err
	}
	return w.writeResponses(m.Responses)
}

func (w mockWriter) writeTitle(m *restfile.Mock) {
	title := strings.Join(strings.Fields(m.Title), " ")
	if title == "" {
		title = fmt.Sprintf("Mock %s %s", strings.ToUpper(m.Method), m.Path)
	}
	w.title(title)
}

// A sequence carries its own name, so a mock is either one named response or a
// sequence of them.
func (w mockWriter) writeDeclaration(m *restfile.Mock) {
	w.head(directive.Mock, "")
	w.option("method", strings.ToUpper(strings.TrimSpace(m.Method)))
	w.option("path", strings.TrimSpace(m.Path))
	switch {
	case m.Sequence != "":
		w.option("sequence", m.Sequence)
		if key := m.SequenceKey.String(); key != "" {
			w.option("sequence-key", key)
		}
	case m.Name != "":
		w.option("name", m.Name)
	}
	if m.Default {
		w.option("default", "true")
	}
	if m.Latency > 0 {
		w.option("latency", m.Latency.String())
	}
	if m.DisableInterpolation {
		w.option("interpolate", "false")
	}
	w.end()
}

func (w mockWriter) writeExpectation(e *restfile.MockExpectation) {
	if e == nil {
		return
	}
	w.head(directive.Expect, "")
	w.option("calls", strconv.FormatUint(e.Calls, 10))
	w.end()
}

func (w mockWriter) writeMatch(m restfile.MockMatch) error {
	if !m.HasConditions() {
		return nil
	}
	fields, err := matchFields(m)
	if err != nil {
		return err
	}
	w.line(directive.Match, strings.Join(fields, " "))
	return nil
}

// A json match may be any JSON value, including a bare string, so it is quoted
// where the bracketed matcher objects are not.
func matchFields(m restfile.MockMatch) ([]string, error) {
	fields, err := appendMatchers(nil, "query", m.Query)
	if err != nil {
		return nil, err
	}
	fields, err = appendMatchers(fields, "headers", m.Headers)
	if err != nil {
		return nil, err
	}
	if len(m.JSON) > 0 {
		fields = append(fields, "json="+quoteMockJSON(m.JSON))
	}
	if len(m.JSONRules) > 0 {
		fields = append(fields, "json-rules="+quoteMockJSON(m.JSONRules))
	}
	return fields, nil
}

// Matchers are always an object, which the option lexer groups on its own.
func appendMatchers[M ~map[string]V, V any](fields []string, key string, matchers M) ([]string, error) {
	if len(matchers) == 0 {
		return fields, nil
	}
	data, err := json.Marshal(matchers)
	if err != nil {
		return nil, fmt.Errorf("marshal mock %s matchers: %w", key, err)
	}
	return append(fields, key+"="+string(data)), nil
}

func quoteMockJSON(raw []byte) string {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err == nil {
		raw = compact.Bytes()
	}
	return strconv.Quote(string(raw))
}

func (w mockWriter) writeResponses(responses []restfile.MockResponse) error {
	for i, resp := range responses {
		if i > 0 {
			w.b.WriteString(restfile.MockSequenceDelimiter + "\n")
		}
		if err := w.writeResponse(resp); err != nil {
			return err
		}
	}
	return nil
}

func (w mockWriter) writeResponse(resp restfile.MockResponse) error {
	file, body, err := w.responseBody(resp)
	if err != nil {
		return err
	}

	w.writeStatusLine(resp.Status)
	renderHeaders(w.b, resp.Headers)
	w.b.WriteString("\n")
	switch {
	case file != "":
		fmt.Fprintf(w.b, "< %s\n", file)
	case body != "":
		// A body can be large, so it is written straight through.
		w.b.WriteString(body)
		w.b.WriteString("\n")
	}
	return nil
}

func (w mockWriter) writeStatusLine(status int) {
	fmt.Fprintf(w.b, "HTTP/1.1 %d", status)
	if text := http.StatusText(status); text != "" {
		fmt.Fprintf(w.b, " %s", text)
	}
	w.b.WriteString("\n")
}

// A response body is written either as a file reference or inline, and only an
// inline one has to be checked against what the parser will accept when it
// reads the block back. At most one of the two is returned.
func (w mockWriter) responseBody(resp restfile.MockResponse) (file, body string, err error) {
	if file = strings.TrimSpace(resp.Body.FilePath); file != "" {
		return file, "", nil
	}
	if resp.Body.Text == "" {
		return "", "", nil
	}
	if !restfile.ResponseAllowsBody(resp.Status) {
		return "", "", fmt.Errorf("status %d cannot have a response body", resp.Status)
	}
	body, err = NormalizeMockBody(resp.Body.Text)
	if err != nil {
		return "", "", err
	}
	if w.sequence {
		for line := range strings.SplitSeq(body, "\n") {
			if restfile.IsMockSequenceDelimiter(line) {
				return "", "", errors.New("mock sequence body contains a response delimiter")
			}
		}
	}
	return "", body, nil
}
