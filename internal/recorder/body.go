package recorder

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limit nesting to prevent stack exhaustion.
const maxJSONDepth = 64

type bodyKind uint8

const (
	bodyUnsupported bodyKind = iota
	bodyText
	bodyJSON
	bodyForm
)

// value holds decoded JSON or form fields. Text bodies use data only.
type body struct {
	data    []byte
	kind    bodyKind
	value   any
	issue   Issue
	decoded bool
}

func (b body) ok() bool { return b.issue.ok() }

func decodeBody(m Message, limit int64) body {
	b := body{data: m.Body, issue: m.Issue}
	if !b.ok() {
		b.data = nil
		return b
	}
	if len(b.data) == 0 {
		return b
	}

	data, issue := decompress(b.data, m.Headers.Get("Content-Encoding"), limit)
	if !issue.ok() {
		b.issue = issue
		return b
	}
	if data != nil {
		b.data, b.decoded = data, true
	}
	if !utf8.Valid(b.data) {
		b.issue = issueBinary
		return b
	}

	b.kind, b.value, b.issue = classify(b.data, m.Headers.Get("Content-Type"))
	return b
}

// decompress returns nil for unencoded bodies. limit applies after decoding.
func decompress(data []byte, encoding string, limit int64) ([]byte, Issue) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "identity":
		return nil, ""
	case "gzip":
	default:
		return nil, issueEncoding
	}

	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, issueGzip
	}
	out, err := io.ReadAll(io.LimitReader(r, limit+1))
	closeErr := r.Close()
	if err != nil || closeErr != nil || int64(len(out)) > limit {
		return nil, issueDecodedBody
	}
	return out, ""
}

func classify(data []byte, contentType string) (bodyKind, any, Issue) {
	media, _, err := mime.ParseMediaType(contentType)
	if err != nil && contentType != "" {
		return bodyUnsupported, nil, issueContentType
	}

	switch {
	case media == "application/json" || strings.HasSuffix(media, "+json"):
		value, err := parseJSON(data)
		if err != nil {
			return bodyJSON, nil, issueJSON
		}
		return bodyJSON, value, ""
	case media == "application/x-www-form-urlencoded":
		values, err := url.ParseQuery(string(data))
		if err != nil {
			return bodyForm, nil, issueForm
		}
		return bodyForm, values, ""
	case media == "", strings.HasPrefix(media, "text/"),
		media == "application/xml", strings.HasSuffix(media, "+xml"),
		media == "application/javascript":
		if hasControlRunes(data) {
			return bodyText, nil, issueBinary
		}
		return bodyText, nil, ""
	default:
		return bodyUnsupported, nil, issueMediaType
	}
}

func hasControlRunes(data []byte) bool {
	for _, r := range string(data) {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return true
		}
	}
	return false
}

// Read tokens to reject duplicate keys and preserve the original numbers.
func parseJSON(data []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	v, err := readJSON(d, 0)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing JSON")
	}
	return v, nil
}

func readJSON(d *json.Decoder, depth int) (any, error) {
	if depth > maxJSONDepth {
		return nil, errors.New("JSON nesting limit")
	}
	t, err := d.Token()
	if err != nil {
		return nil, err
	}

	switch t {
	case json.Delim('{'):
		return readJSONObject(d, depth)
	case json.Delim('['):
		return readJSONArray(d, depth)
	default:
		return t, nil
	}
}

func readJSONObject(d *json.Decoder, depth int) (map[string]any, error) {
	object := make(map[string]any)
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("invalid JSON key")
		}
		if _, ok := object[key]; ok {
			return nil, errors.New("duplicate JSON key")
		}
		value, err := readJSON(d, depth+1)
		if err != nil {
			return nil, err
		}
		object[key] = value
	}
	_, err := d.Token()
	return object, err
}

func readJSONArray(d *json.Decoder, depth int) ([]any, error) {
	array := make([]any, 0)
	for d.More() {
		value, err := readJSON(d, depth+1)
		if err != nil {
			return nil, err
		}
		array = append(array, value)
	}
	_, err := d.Token()
	return array, err
}
