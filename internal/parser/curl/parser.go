package curl

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func ParseCommand(command string) (*restfile.Request, error) {
	tokens, err := splitTokens(command)
	if err != nil {
		return nil, err
	}
	return parseTokens(tokens)
}

// Shell-style tokenization with single quotes (literal), double quotes (escape-aware),
// and backslash escaping. Single quotes disable escaping so \'doesn\'t terminate the quote.
// Double quotes respect backslashes so you can have \"inside\" strings.
func splitTokens(input string) ([]string, error) {
	var args []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		args = append(args, current.String())
		current.Reset()
	}

	for _, r := range input {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\':
			if inSingle {
				current.WriteRune(r)
			} else {
				escaped = true
			}
		case r == '\'':
			if !inDouble {
				if inSingle {
					inSingle = false
				} else {
					inSingle = true
				}
			} else {
				current.WriteRune(r)
			}
		case r == '"':
			if !inSingle {
				if inDouble {
					inDouble = false
				} else {
					inDouble = true
				}
			} else {
				current.WriteRune(r)
			}
		case isWhitespace(r):
			if inSingle || inDouble {
				current.WriteRune(r)
			} else {
				flush()
			}
		default:
			current.WriteRune(r)
		}
	}

	if escaped {
		return nil, fmt.Errorf("unterminated escape sequence")
	}

	if inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quoted string")
	}

	flush()
	return args, nil
}

func parseTokens(tokens []string) (*restfile.Request, error) {
	idx, err := findCurlIndex(tokens)
	if err != nil {
		return nil, err
	}

	var target string
	var basic string

	req := &restfile.Request{Method: "GET"}
	headers := make(http.Header)
	body := newBodyBuilder()
	compressed := false
	explicitMethod := false
	positionalOnly := false

	for i := idx + 1; i < len(tokens); i++ {
		tok := tokens[i]
		if tok == "" {
			continue
		}

		if !positionalOnly {
			switch {
			case tok == "--":
				positionalOnly = true
				continue
			case tok == "-X" || tok == "--request":
				val, err := consumeNext(tokens, &i, tok)
				if err != nil {
					return nil, err
				}
				req.Method = strings.ToUpper(val)
				explicitMethod = true
				continue
			case strings.HasPrefix(tok, "-X") && len(tok) > 2:
				req.Method = strings.ToUpper(tok[2:])
				explicitMethod = true
				continue
			case strings.HasPrefix(tok, "--request="):
				req.Method = strings.ToUpper(tok[len("--request="):])
				explicitMethod = true
				continue
			case tok == "-H" || tok == "--header":
				val, err := consumeNext(tokens, &i, tok)
				if err != nil {
					return nil, err
				}
				addHeader(headers, val)
				continue
			case strings.HasPrefix(tok, "-H") && len(tok) > 2:
				addHeader(headers, tok[2:])
				continue
			case strings.HasPrefix(tok, "--header="):
				addHeader(headers, tok[len("--header="):])
				continue
			case tok == "-u" || tok == "--user":
				val, err := consumeNext(tokens, &i, tok)
				if err != nil {
					return nil, err
				}
				basic = val
				continue
			case strings.HasPrefix(tok, "-u") && len(tok) > 2:
				basic = tok[2:]
				continue
			case strings.HasPrefix(tok, "--user="):
				basic = tok[len("--user="):]
				continue
			case tok == "-I" || tok == "--head":
				req.Method = "HEAD"
				explicitMethod = true
				continue
			case tok == "--compressed":
				compressed = true
				continue
			case tok == "--url":
				val, err := consumeNext(tokens, &i, tok)
				if err != nil {
					return nil, err
				}
				target = val
				continue
			case strings.HasPrefix(tok, "--url="):
				target = tok[len("--url="):]
				continue
			case tok == "--json":
				val, err := consumeNext(tokens, &i, tok)
				if err != nil {
					return nil, err
				}
				if err := body.addJSON(val); err != nil {
					return nil, err
				}
				ensureJSONHeader(headers)
				continue
			case strings.HasPrefix(tok, "--json="):
				if err := body.addJSON(tok[len("--json="):]); err != nil {
					return nil, err
				}
				ensureJSONHeader(headers)
				continue
			case tok == "--data-json":
				val, err := consumeNext(tokens, &i, tok)
				if err != nil {
					return nil, err
				}
				if err := body.addJSON(val); err != nil {
					return nil, err
				}
				ensureJSONHeader(headers)
				continue
			case strings.HasPrefix(tok, "--data-json="):
				if err := body.addJSON(tok[len("--data-json="):]); err != nil {
					return nil, err
				}
				ensureJSONHeader(headers)
				continue
			case tok == "-d" || tok == "--data" || tok == "--data-ascii":
				val, err := consumeNext(tokens, &i, tok)
				if err != nil {
					return nil, err
				}
				if err := body.addData(val, true); err != nil {
					return nil, err
				}
				continue
			case strings.HasPrefix(tok, "-d") && len(tok) > 2:
				if err := body.addData(tok[2:], true); err != nil {
					return nil, err
				}
				continue
			case strings.HasPrefix(tok, "--data="):
				if err := body.addData(tok[len("--data="):], true); err != nil {
					return nil, err
				}
				continue
			case tok == "--data-urlencode":
				val, err := consumeNext(tokens, &i, tok)
				if err != nil {
					return nil, err
				}
				if err := body.addURLEncoded(val); err != nil {
					return nil, err
				}
				continue
			case strings.HasPrefix(tok, "--data-urlencode="):
				if err := body.addURLEncoded(tok[len("--data-urlencode="):]); err != nil {
					return nil, err
				}
				continue
			case tok == "--data-raw":
				val, err := consumeNext(tokens, &i, tok)
				if err != nil {
					return nil, err
				}
				if err := body.addRaw(val); err != nil {
					return nil, err
				}
				continue
			case strings.HasPrefix(tok, "--data-raw="):
				if err := body.addRaw(tok[len("--data-raw="):]); err != nil {
					return nil, err
				}
				continue
			case tok == "--data-binary":
				val, err := consumeNext(tokens, &i, tok)
				if err != nil {
					return nil, err
				}
				if err := body.addBinary(val); err != nil {
					return nil, err
				}
				continue
			case strings.HasPrefix(tok, "--data-binary="):
				if err := body.addBinary(tok[len("--data-binary="):]); err != nil {
					return nil, err
				}
				continue
			case tok == "-F" || tok == "--form":
				val, err := consumeNext(tokens, &i, tok)
				if err != nil {
					return nil, err
				}
				if err := body.addFormPart(val, false); err != nil {
					return nil, err
				}
				continue
			case strings.HasPrefix(tok, "-F") && len(tok) > 2:
				if err := body.addFormPart(tok[2:], false); err != nil {
					return nil, err
				}
				continue
			case strings.HasPrefix(tok, "--form="):
				if err := body.addFormPart(tok[len("--form="):], false); err != nil {
					return nil, err
				}
				continue
			case tok == "--form-string":
				val, err := consumeNext(tokens, &i, tok)
				if err != nil {
					return nil, err
				}
				if err := body.addFormPart(val, true); err != nil {
					return nil, err
				}
				continue
			case strings.HasPrefix(tok, "--form-string="):
				if err := body.addFormPart(tok[len("--form-string="):], true); err != nil {
					return nil, err
				}
				continue
			case (strings.HasPrefix(tok, "http://") || strings.HasPrefix(tok, "https://")) && target == "":
				target = tok
				continue
			}
		}

		if target == "" {
			target = tok
			continue
		}
		if err := body.addRaw(tok); err != nil {
			return nil, err
		}
	}

	if target == "" {
		return nil, fmt.Errorf("curl command missing URL")
	}

	if body.hasContent() && !explicitMethod && strings.EqualFold(req.Method, "GET") {
		req.Method = "POST"
	}

	if err := body.apply(req, headers); err != nil {
		return nil, err
	}

	req.URL = sanitizeURL(target)
	if len(headers) > 0 {
		req.Headers = headers
	}

	if basic != "" {
		if req.Headers == nil {
			req.Headers = make(http.Header)
		}
		if req.Headers.Get("Authorization") == "" {
			req.Headers.Set("Authorization", buildBasicAuthHeader(basic))
		}
	}

	if compressed {
		if req.Headers == nil {
			req.Headers = make(http.Header)
		}
		if req.Headers.Get("Accept-Encoding") == "" {
			req.Headers.Set("Accept-Encoding", "gzip, deflate, br")
		}
	}

	return req, nil
}

func ensureJSONHeader(h http.Header) {
	if h.Get("Content-Type") == "" {
		h.Set("Content-Type", "application/json")
	}
}

func addHeader(h http.Header, raw string) {
	name, value := splitHeader(raw)
	if name != "" {
		h.Add(name, value)
	}
}

func consumeNext(tokens []string, idx *int, flag string) (string, error) {
	*idx++
	if *idx >= len(tokens) {
		return "", fmt.Errorf("missing argument for %s", flag)
	}
	return tokens[*idx], nil
}

func findCurlIndex(tokens []string) (int, error) {
	for i, tok := range tokens {
		trimmed := strings.TrimSpace(stripPromptPrefix(tok))
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if lower == "curl" {
			return i, nil
		}
		switch lower {
		case "sudo", "env", "command", "time", "noglob":
			continue
		}
	}
	return -1, fmt.Errorf("not a curl command")
}

type bodyKind int

const (
	bodyKindNone bodyKind = iota
	bodyKindRaw
	bodyKindForm
	bodyKindMultipart
	bodyKindFile
)

type formField struct {
	name   string
	val    string
	encVal bool
}

type multipartPart struct {
	name     string
	val      string
	file     string
	ctype    string
	fname    string
	fileMode bool
}

type bodyBuilder struct {
	kind  bodyKind
	raw   []string
	form  []formField
	multi []multipartPart
	file  string
}

func newBodyBuilder() *bodyBuilder {
	return &bodyBuilder{kind: bodyKindNone}
}

func (b *bodyBuilder) ensureKind(kind bodyKind) error {
	if b.kind == bodyKindNone {
		b.kind = kind
		return nil
	}
	if b.kind != kind {
		return fmt.Errorf("conflicting body flags")
	}
	return nil
}

func (b *bodyBuilder) addData(val string, guess bool) error {
	trim := strings.TrimSpace(val)
	if guess && strings.HasPrefix(trim, "@") {
		return b.addFile(strings.TrimPrefix(trim, "@"))
	}
	if guess && looksLikeForm(val) {
		return b.addFormValues(val)
	}
	return b.addRaw(val)
}

func (b *bodyBuilder) addBinary(val string) error {
	trim := strings.TrimSpace(val)
	if strings.HasPrefix(trim, "@") {
		return b.addFile(strings.TrimPrefix(trim, "@"))
	}
	return b.addRaw(val)
}

func (b *bodyBuilder) addRaw(val string) error {
	if err := b.ensureKind(bodyKindRaw); err != nil {
		return err
	}
	b.raw = append(b.raw, val)
	return nil
}

func (b *bodyBuilder) addJSON(val string) error {
	return b.addRaw(val)
}

func (b *bodyBuilder) addURLEncoded(raw string) error {
	if err := b.ensureKind(bodyKindForm); err != nil {
		return err
	}
	for _, part := range strings.Split(raw, "&") {
		if part == "" {
			continue
		}
		if idx := strings.Index(part, "="); idx >= 0 {
			name := strings.TrimSpace(part[:idx])
			value := part[idx+1:]
			b.form = append(b.form, formField{name: name, val: value, encVal: true})
			continue
		}
		b.form = append(b.form, formField{name: "", val: part, encVal: true})
	}
	return nil
}

func (b *bodyBuilder) addFormValues(raw string) error {
	if err := b.ensureKind(bodyKindForm); err != nil {
		return err
	}
	for _, part := range strings.Split(raw, "&") {
		name, value := splitFormPair(part)
		b.form = append(b.form, formField{name: name, val: value})
	}
	return nil
}

func (b *bodyBuilder) addFormPart(raw string, literal bool) error {
	if err := b.ensureKind(bodyKindMultipart); err != nil {
		return err
	}
	part, err := parseMultipartPart(raw, literal)
	if err != nil {
		return err
	}
	b.multi = append(b.multi, part)
	return nil
}

func (b *bodyBuilder) addFile(path string) error {
	clean := strings.TrimSpace(path)
	if clean == "" {
		return fmt.Errorf("empty body file reference")
	}
	if b.kind != bodyKindNone && b.kind != bodyKindFile {
		return fmt.Errorf("file body conflicts with other data")
	}
	b.kind = bodyKindFile
	b.file = clean
	return nil
}

func (b *bodyBuilder) hasContent() bool {
	switch b.kind {
	case bodyKindRaw:
		return len(b.raw) > 0
	case bodyKindForm:
		return len(b.form) > 0
	case bodyKindMultipart:
		return len(b.multi) > 0
	case bodyKindFile:
		return b.file != ""
	default:
		return false
	}
}

func (b *bodyBuilder) apply(req *restfile.Request, headers http.Header) error {
	if !b.hasContent() {
		req.Body = restfile.BodySource{}
		return nil
	}

	switch b.kind {
	case bodyKindFile:
		req.Body = restfile.BodySource{FilePath: b.file}
	case bodyKindRaw:
		text := strings.Join(b.raw, "\n")
		req.Body = restfile.BodySource{Text: text}
	case bodyKindForm:
		pairs := make([]string, 0, len(b.form))
		for _, f := range b.form {
			pairs = append(pairs, f.encode())
		}
		if headers.Get("Content-Type") == "" {
			headers.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		body := strings.Join(pairs, "&")
		req.Body = restfile.BodySource{Text: body}
	case bodyKindMultipart:
		body, boundary := buildMultipartBody(b.multi)
		if boundary == "" {
			return fmt.Errorf("multipart body is empty")
		}
		headers.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", boundary))
		req.Body = restfile.BodySource{Text: body}
	default:
		req.Body = restfile.BodySource{}
	}

	if ct := headers.Get("Content-Type"); ct != "" {
		req.Body.MimeType = ct
	}
	return nil
}

func (f formField) encode() string {
	name := f.name
	val := f.val

	if f.encVal {
		val = url.QueryEscape(val)
	}

	if name == "" {
		return val
	}
	return name + "=" + val
}

func splitFormPair(raw string) (string, string) {
	part := raw
	id := strings.Index(part, "=")
	if id < 0 {
		return strings.TrimSpace(part), ""
	}

	name := strings.TrimSpace(part[:id])
	val := part[id+1:]
	return name, val
}

func looksLikeForm(v string) bool {
	if strings.ContainsAny(v, "\n\r") {
		return false
	}
	if strings.Contains(v, "&") {
		return true
	}
	return strings.Contains(v, "=")
}

func parseMultipartPart(raw string, literal bool) (multipartPart, error) {
	content := strings.TrimSpace(raw)
	if content == "" {
		return multipartPart{}, fmt.Errorf("empty multipart field")
	}

	idx := strings.Index(content, "=")
	if idx <= 0 {
		return multipartPart{}, fmt.Errorf("invalid multipart field %q", raw)
	}

	name := strings.TrimSpace(content[:idx])
	remain := content[idx+1:]
	segments := strings.Split(remain, ";")
	val := strings.TrimSpace(segments[0])
	part := multipartPart{name: name}

	if name == "" {
		return part, fmt.Errorf("multipart field missing name")
	}

	if !literal && len(val) > 0 && (val[0] == '@' || val[0] == '<') {
		file := strings.TrimSpace(val[1:])
		if file == "" {
			return part, fmt.Errorf("multipart file field missing path")
		}
		part.file = file
		part.fileMode = true
	} else {
		part.val = val
	}

	for _, opt := range segments[1:] {
		opt = strings.TrimSpace(opt)
		if opt == "" {
			continue
		}
		kv := strings.SplitN(opt, "=", 2)
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		value := ""
		if len(kv) == 2 {
			value = strings.TrimSpace(kv[1])
		}
		switch key {
		case "type":
			part.ctype = value
		case "filename":
			part.fname = value
		}
	}

	if part.fileMode {
		if part.fname == "" {
			part.fname = filepath.Base(part.file)
		}
		if part.ctype == "" {
			part.ctype = "application/octet-stream"
		}
	}
	return part, nil
}

func buildMultipartBody(parts []multipartPart) (string, string) {
	if len(parts) == 0 {
		return "", ""
	}

	boundary := makeBoundary()
	var b strings.Builder
	for _, p := range parts {
		b.WriteString("--")
		b.WriteString(boundary)
		b.WriteString("\r\n")
		b.WriteString("Content-Disposition: form-data; name=\"")
		b.WriteString(escapeQuotes(p.name))
		b.WriteString("\"")
		if p.fileMode {
			b.WriteString("; filename=\"")
			b.WriteString(escapeQuotes(p.fname))
			b.WriteString("\"")
		}
		b.WriteString("\r\n")
		if p.ctype != "" {
			b.WriteString("Content-Type: ")
			b.WriteString(p.ctype)
			b.WriteString("\r\n")
		}
		b.WriteString("\r\n")
		if p.fileMode {
			b.WriteString("@")
			b.WriteString(p.file)
		} else {
			b.WriteString(p.val)
		}
		b.WriteString("\r\n")
	}
	b.WriteString("--")
	b.WriteString(boundary)
	b.WriteString("--\r\n")
	return b.String(), boundary
}

func makeBoundary() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "resterm-boundary"
	}
	return "resterm-" + hex.EncodeToString(buf)
}

func escapeQuotes(v string) string {
	return strings.ReplaceAll(v, "\"", "\\\"")
}

func buildBasicAuthHeader(creds string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(creds))
	return fmt.Sprintf("Basic %s", encoded)
}

func splitHeader(header string) (string, string) {
	parts := strings.SplitN(header, ":", 2)
	if len(parts) == 0 {
		return "", ""
	}
	name := strings.TrimSpace(parts[0])
	if name == "" {
		return "", ""
	}
	value := ""
	if len(parts) > 1 {
		value = strings.TrimSpace(parts[1])
	}
	return name, value
}

func stripPromptPrefix(token string) string {
	trimmed := strings.TrimSpace(token)
	prefixes := []string{"$", "%", ">", "!"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(trimmed, prefix) {
			trimmed = strings.TrimSpace(trimmed[len(prefix):])
		}
	}
	return trimmed
}

func isWhitespace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

func sanitizeURL(raw string) string {
	return strings.Trim(raw, "\"'")
}

func VisibleHeaders(headers http.Header) []string {
	if len(headers) == 0 {
		return nil
	}
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
