package curl

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func ParseCommand(command string) (*restfile.Request, error) {
	reqs, err := ParseCommands(command)
	if err != nil {
		return nil, err
	}
	if len(reqs) == 0 {
		return nil, fmt.Errorf("curl command missing URL")
	}
	return reqs[0], nil
}

func ParseCommands(command string) ([]*restfile.Request, error) {
	tok, err := splitTokens(command)
	if err != nil {
		return nil, err
	}

	cmd, err := parseCmd(tok)
	if err != nil {
		return nil, err
	}
	return normCmd(cmd)
}

type lexState struct {
	token TokenState
	buf   strings.Builder
	out   []string
}

func (st *lexState) add(r rune) {
	st.buf.WriteRune(r)
}

func (st *lexState) flush() {
	if st.buf.Len() == 0 {
		return
	}
	st.out = append(st.out, st.buf.String())
	st.buf.Reset()
}

// Shell-style tokenization with single quotes (literal), double quotes (escape-aware),
// ANSI-C $'...' quoting, and backslash escaping. Single quotes disable escaping.
// Double quotes respect backslashes so you can have \"inside\" strings.
func splitTokens(input string) ([]string, error) {
	st := &lexState{}
	rs := []rune(input)
	opts := tokenOptions{decodeANSI: true, allowLineContinuation: true}

	for i := 0; i < len(rs); i++ {
		r := rs[i]
		step, err := st.token.advance(rs, &i, opts)
		if err != nil {
			return nil, err
		}
		if step.handled {
			if step.emit {
				st.add(step.r)
			}
			continue
		}

		if isWhitespace(r) {
			if st.token.InQuote() {
				st.add(r)
			} else {
				st.flush()
			}
			continue
		}

		st.add(r)
	}

	if st.token.Escaping() {
		return nil, fmt.Errorf("unterminated escape sequence")
	}

	if st.token.Open() {
		return nil, fmt.Errorf("unterminated quoted string")
	}

	st.flush()
	return st.out, nil
}

func ansiEsc(rs []rune, i *int) (rune, error) {
	if *i >= len(rs) {
		return 0, fmt.Errorf("unterminated escape sequence")
	}
	r := rs[*i]
	switch r {
	case 'n':
		return '\n', nil
	case 'r':
		return '\r', nil
	case 't':
		return '\t', nil
	case '\\':
		return '\\', nil
	case '\'':
		return '\'', nil
	case '"':
		return '"', nil
	case 'x':
		return readHex(rs, i, 2)
	case 'u':
		return readHex(rs, i, 4)
	default:
		return r, nil
	}
}

func readHex(rs []rune, i *int, n int) (rune, error) {
	if *i+n >= len(rs) {
		return 0, fmt.Errorf("invalid hex escape")
	}
	val := 0
	for j := 0; j < n; j++ {
		r := rs[*i+1+j]
		d, ok := hexVal(r)
		if !ok {
			return 0, fmt.Errorf("invalid hex escape")
		}
		val = val*16 + d
	}
	*i += n
	return rune(val), nil
}

func hexVal(r rune) (int, bool) {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0'), true
	case r >= 'a' && r <= 'f':
		return int(r-'a') + 10, true
	case r >= 'A' && r <= 'F':
		return int(r-'A') + 10, true
	default:
		return 0, false
	}
}

func isLineBreak(r rune) bool {
	return r == '\n' || r == '\r'
}

func ensureJSONHeader(h http.Header) {
	if h.Get(headerContentType) == "" {
		h.Set(headerContentType, mimeJSON)
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

func findCurlIndex(tokens []string) (int, bool) {
	for i, tok := range tokens {
		trimmed := strings.TrimSpace(stripPromptPrefix(tok))
		if trimmed == "" {
			continue
		}

		lower := strings.ToLower(trimmed)
		if lower == cmdCurl {
			return i, true
		}

		switch lower {
		case cmdSudo, cmdEnv, cmdCommand, cmdTime, cmdNoGlob:
			continue
		}
	}
	return 0, false
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

func (b *bodyBuilder) query() (string, error) {
	switch b.kind {
	case bodyKindNone:
		return "", nil
	case bodyKindRaw:
		return strings.Join(b.raw, "&"), nil
	case bodyKindForm:
		pairs := make([]string, 0, len(b.form))
		for _, f := range b.form {
			pairs = append(pairs, f.encode())
		}
		return strings.Join(pairs, "&"), nil
	case bodyKindMultipart:
		return "", fmt.Errorf("multipart body cannot be mapped to query")
	case bodyKindFile:
		return "", fmt.Errorf("file body cannot be mapped to query")
	default:
		return "", nil
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
		if headers.Get(headerContentType) == "" {
			headers.Set(headerContentType, mimeFormURLEncoded)
		}
		body := strings.Join(pairs, "&")
		req.Body = restfile.BodySource{Text: body}
	case bodyKindMultipart:
		body, boundary := buildMultipartBody(b.multi)
		if boundary == "" {
			return fmt.Errorf("multipart body is empty")
		}
		headers.Set(headerContentType, mimeMultipartForm+"; boundary="+boundary)
		req.Body = restfile.BodySource{Text: body}
	default:
		req.Body = restfile.BodySource{}
	}

	if ct := headers.Get(headerContentType); ct != "" {
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
			part.ctype = mimeOctetStream
		}
	}
	return part, nil
}

func buildMultipartBody(parts []multipartPart) (string, string) {
	if len(parts) == 0 {
		return "", ""
	}

	boundary := makeBoundary(parts)
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

func makeBoundary(parts []multipartPart) string {
	if len(parts) == 0 {
		return multipartBoundaryDefault
	}
	h := sha256.New()
	for _, p := range parts {
		addHash(h, p.name)
		addHash(h, p.val)
		addHash(h, p.file)
		addHash(h, p.ctype)
		addHash(h, p.fname)
		if p.fileMode {
			_, _ = h.Write([]byte{1})
		} else {
			_, _ = h.Write([]byte{0})
		}
	}
	sum := h.Sum(nil)
	return multipartBoundaryPrefix + hex.EncodeToString(sum[:12])
}

func addHash(h hash.Hash, v string) {
	if v == "" {
		_, _ = h.Write([]byte{0})
		return
	}
	_, _ = h.Write([]byte(v))
	_, _ = h.Write([]byte{0})
}

func escapeQuotes(v string) string {
	return strings.ReplaceAll(v, "\"", "\\\"")
}

func buildBasicAuthHeader(creds string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(creds))
	return authHeaderBasicPrefix + encoded
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
	for _, prefix := range promptPrefixes {
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
	return strings.Trim(raw, urlQuoteChars)
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
