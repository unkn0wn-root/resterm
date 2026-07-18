package restwriter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/parser/bodyref"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/util"
)

type Options struct {
	OverwriteExisting bool
	HeaderComment     string
}

func WriteDocument(ctx context.Context, doc *restfile.Document, dst string, opts Options) error {
	if doc == nil {
		return errors.New("writer: document is nil")
	}
	if strings.TrimSpace(dst) == "" {
		return errors.New("writer: destination path is empty")
	}

	content, err := Render(doc, opts)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return writeFile(dst, content, opts.OverwriteExisting)
}

func writeFile(dst, content string, overwrite bool) error {
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("writer: create directory: %w", err)
	}

	if !overwrite {
		if _, err := os.Stat(dst); err == nil {
			return fmt.Errorf("writer: destination %s already exists", dst)
		}
	}

	tmp, err := os.CreateTemp(dir, "resterm-*.http")
	if err != nil {
		return fmt.Errorf("writer: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := io.WriteString(tmp, content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writer: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writer: close temp file: %w", err)
	}

	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("writer: rename temp file: %w", err)
	}
	return nil
}

func Render(doc *restfile.Document, opts Options) (string, error) {
	var b strings.Builder

	renderHeader(&b, opts.HeaderComment)
	renderScopeVariables(&b, doc.Variables)
	renderScopeVariables(&b, doc.Globals)
	renderSettings(&b, doc.Settings)

	if len(doc.Variables) > 0 || len(doc.Globals) > 0 || len(doc.Settings) > 0 {
		b.WriteString("\n")
	}

	idx := 0
	for _, req := range doc.Requests {
		if req == nil {
			continue
		}
		if idx > 0 {
			b.WriteString("\n")
		}
		renderRequest(&b, req)
		idx++
	}
	for _, mock := range doc.Mocks {
		if mock == nil {
			continue
		}
		if idx > 0 {
			b.WriteString("\n")
		}
		if err := renderMock(&b, mock); err != nil {
			return "", err
		}
		idx++
	}

	return b.String(), nil
}

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
	b.WriteString("\n# @mock method=")
	b.WriteString(strings.ToUpper(strings.TrimSpace(mock.Method)))
	b.WriteString(" path=")
	b.WriteString(strings.TrimSpace(mock.Path))
	if mock.Sequence != "" {
		b.WriteString(" sequence=")
		b.WriteString(mock.Sequence)
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
	renderMockMatch(b, mock.Match)

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
	lines := strings.Split(body, "\n")
	for _, line := range lines {
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

func renderMockMatch(b *strings.Builder, match restfile.MockMatch) {
	var fields []string
	if len(match.Query) > 0 {
		data, _ := json.Marshal(match.Query)
		fields = append(fields, "query="+string(data))
	}
	if len(match.Headers) > 0 {
		data, _ := json.Marshal(match.Headers)
		fields = append(fields, "headers="+string(data))
	}
	if len(match.JSON) > 0 {
		fields = append(fields, "json="+formatMockJSON(match.JSON))
	}
	if len(fields) == 0 {
		return
	}
	b.WriteString("# @match ")
	b.WriteString(strings.Join(fields, " "))
	b.WriteString("\n")
}

func formatMockJSON(raw []byte) string {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err == nil {
		raw = compact.Bytes()
	}
	return strconv.Quote(string(raw))
}

func renderHeader(b *strings.Builder, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	for line := range strings.SplitSeq(text, "\n") {
		b.WriteString("# ")
		b.WriteString(strings.TrimSpace(line))
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func renderScopeVariables(b *strings.Builder, vars []restfile.Variable) {
	for _, v := range vars {
		val := strings.TrimSpace(v.Value)
		switch v.Scope {
		case restfile.ScopeGlobal:
			dir := "@global"
			if v.Secret {
				dir = "@global-secret"
			}
			fmt.Fprintf(b, "# %s %s %s\n", dir, v.Name, val)
		case restfile.ScopeFile:
			scope := "file"
			if v.Secret {
				scope = "file-secret"
			}
			fmt.Fprintf(b, "# @var %s %s %s\n", scope, v.Name, val)
		default:
			scope := "request"
			if v.Secret {
				scope = "request-secret"
			}
			fmt.Fprintf(b, "# @var %s %s %s\n", scope, v.Name, val)
		}
	}
}

func renderRequest(b *strings.Builder, req *restfile.Request) {
	title := req.Metadata.Name
	if title == "" {
		title = fmt.Sprintf("%s %s", strings.ToUpper(req.Method), req.URL)
	}
	b.WriteString("### ")
	b.WriteString(title)
	b.WriteString("\n")

	if req.Metadata.Name != "" {
		b.WriteString("# @name ")
		b.WriteString(req.Metadata.Name)
		b.WriteString("\n")
	}

	renderDescription(b, req.Metadata.Description)
	renderTags(b, req.Metadata.Tags)
	renderLoggingDirectives(b, req.Metadata)
	renderAuth(b, req.Metadata.Auth)
	renderSettings(b, req.Settings)
	renderRequestVariables(b, req.Variables)
	renderCaptures(b, req.Metadata.Captures)
	renderBodyOptions(b, req)

	b.WriteString(reqLine(req))
	renderHeaders(b, req.Headers)
	b.WriteString("\n")
	if req.Body.FilePath != "" {
		b.WriteString("< ")
		b.WriteString(strings.TrimSpace(req.Body.FilePath))
		b.WriteString("\n")
	} else if strings.TrimSpace(req.Body.Text) != "" {
		b.WriteString(req.Body.Text)
		if !strings.HasSuffix(req.Body.Text, "\n") {
			b.WriteString("\n")
		}
	}
}

func renderBodyOptions(b *strings.Builder, req *restfile.Request) {
	if req == nil {
		return
	}
	opt := req.Body.Options
	if opt.ExpandTemplates {
		b.WriteString("# @body expand\n")
	}
	if opt.ForceInline || bodyTextNeedsInlineDirective(req) {
		b.WriteString("# @body inline\n")
	}
}

func bodyTextNeedsInlineDirective(req *restfile.Request) bool {
	if req == nil || strings.TrimSpace(req.Body.FilePath) != "" {
		return false
	}
	text := strings.TrimSpace(req.Body.Text)
	if text == "" {
		return false
	}
	line, _, _ := strings.Cut(text, "\n")
	opt := bodyref.Options{
		Location: bodyref.Line,
	}
	if _, ok := bodyref.Parse(line, opt); ok {
		return true
	}
	return false
}

func renderDescription(b *strings.Builder, desc string) {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return
	}
	for line := range strings.SplitSeq(desc, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		b.WriteString("# @description ")
		b.WriteString(t)
		b.WriteString("\n")
	}
}

func renderTags(b *strings.Builder, tags []string) {
	if len(tags) == 0 {
		return
	}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		t := strings.TrimSpace(tag)
		if t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return
	}
	b.WriteString("# @tag ")
	b.WriteString(strings.Join(out, " "))
	b.WriteString("\n")
}

func renderLoggingDirectives(b *strings.Builder, meta restfile.RequestMetadata) {
	if meta.NoLog {
		b.WriteString("# @no-log\n")
	}
	if meta.AllowSensitiveHeaders {
		b.WriteString("# @log-sensitive-headers true\n")
	}
}

func renderAuth(b *strings.Builder, auth *restfile.AuthSpec) {
	if auth == nil || auth.Type == "" {
		return
	}
	switch strings.ToLower(auth.Type) {
	case "basic":
		b.WriteString("# @auth basic ")
		b.WriteString(strings.TrimSpace(auth.Params["username"]))
		b.WriteString(" ")
		b.WriteString(strings.TrimSpace(auth.Params["password"]))
	case "bearer":
		b.WriteString("# @auth bearer ")
		b.WriteString(strings.TrimSpace(auth.Params["token"]))
	case "apikey", "api-key":
		place := strings.TrimSpace(auth.Params["placement"])
		name := strings.TrimSpace(auth.Params["name"])
		val := strings.TrimSpace(auth.Params["value"])
		if place == "" {
			place = "header"
		}
		if name == "" {
			name = "X-API-Key"
		}
		b.WriteString("# @auth apikey ")
		b.WriteString(place)
		b.WriteString(" ")
		b.WriteString(name)
		b.WriteString(" ")
		b.WriteString(val)
	case "oauth2":
		formatted := formatOAuthParams(auth.Params)
		if len(formatted) == 0 {
			return
		}
		b.WriteString("# @auth oauth2 ")
		b.WriteString(strings.Join(formatted, " "))
	case "command":
		formatted := formatCommandParams(auth.Params)
		if len(formatted) == 0 {
			return
		}
		b.WriteString("# @auth command ")
		b.WriteString(strings.Join(formatted, " "))
	default:
		return
	}
	b.WriteString("\n")
}

func renderSettings(b *strings.Builder, set map[string]string) {
	if len(set) == 0 {
		return
	}
	keys := sortedKeys(set)
	for _, key := range keys {
		val := strings.TrimSpace(set[key])
		if val == "" {
			continue
		}
		b.WriteString("# @setting ")
		b.WriteString(key)
		b.WriteString(" ")
		b.WriteString(val)
		b.WriteString("\n")
	}
}

var oauthParamOrder = []string{
	"token_url",
	"auth_url",
	"redirect_uri",
	"client_id",
	"client_secret",
	"scope",
	"audience",
	"resource",
	"grant",
	"username",
	"password",
	"client_auth",
	"cache_key",
	"code_verifier",
	"code_challenge_method",
	"state",
}

var commandParamOrder = []string{
	"argv",
	"format",
	"header",
	"scheme",
	"token_path",
	"type_path",
	"expiry_path",
	"expires_in_path",
	"cache_key",
	"ttl",
	"timeout",
}

func formatOAuthParams(params map[string]string) []string {
	return formatOrderedParams(params, oauthParamOrder)
}

func formatCommandParams(params map[string]string) []string {
	return formatOrderedParams(params, commandParamOrder)
}

func formatOrderedParams(params map[string]string, ordered []string) []string {
	if len(params) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(ordered))

	var parts []string
	for _, key := range ordered {
		val := strings.TrimSpace(params[key])
		if val == "" {
			continue
		}
		parts = append(parts, formatAuthParam(key, val))
		seen[key] = struct{}{}
	}

	var extra []string
	for key, raw := range params {
		lower := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
		if _, ok := seen[lower]; ok {
			continue
		}
		val := strings.TrimSpace(raw)
		if val == "" {
			continue
		}
		extra = append(extra, formatAuthParam(lower, val))
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		parts = append(parts, extra...)
	}
	return parts
}

func formatAuthParam(key, val string) string {
	if strings.ContainsAny(val, " \t") {
		switch {
		case !strings.Contains(val, "'"):
			val = "'" + val + "'"
		case !strings.Contains(val, "\""):
			val = `"` + val + `"`
		}
	}
	return fmt.Sprintf("%s=%s", key, val)
}

func renderRequestVariables(b *strings.Builder, vars []restfile.Variable) {
	for _, v := range vars {
		if v.Scope != restfile.ScopeRequest {
			continue
		}
		scope := "request"
		if v.Secret {
			scope = "request-secret"
		}
		b.WriteString("# @var ")
		b.WriteString(scope)
		b.WriteString(" ")
		b.WriteString(v.Name)
		if strings.TrimSpace(v.Value) != "" {
			b.WriteString(" ")
			b.WriteString(strings.TrimSpace(v.Value))
		}
		b.WriteString("\n")
	}
}

func renderCaptures(b *strings.Builder, caps []restfile.CaptureSpec) {
	for _, c := range caps {
		scope := captureScopeToken(c)
		b.WriteString("# @capture ")
		b.WriteString(scope)
		b.WriteString(" ")
		b.WriteString(c.Name)
		b.WriteString(" ")
		b.WriteString(strings.TrimSpace(c.Expression))
		b.WriteString("\n")
	}
}

func reqLine(req *restfile.Request) string {
	m := strings.ToUpper(strings.TrimSpace(req.Method))
	if m == "" {
		m = "GET"
	}
	return fmt.Sprintf("%s %s\n", m, strings.TrimSpace(req.URL))
}

func renderHeaders(b *strings.Builder, hdr http.Header) {
	if len(hdr) == 0 {
		return
	}
	for _, name := range sortedKeys(hdr) {
		for _, val := range hdr[name] {
			b.WriteString(name)
			b.WriteString(": ")
			b.WriteString(val)
			b.WriteString("\n")
		}
	}
}

func captureScopeToken(c restfile.CaptureSpec) string {
	scope := ""
	switch c.Scope {
	case restfile.CaptureScopeRequest:
		scope = "request"
	case restfile.CaptureScopeFile:
		scope = "file"
	case restfile.CaptureScopeGlobal:
		scope = "global"
	default:
		scope = "request"
	}
	if c.Secret {
		scope += "-secret"
	}
	return scope
}

func sortedKeys[M ~map[string]V, V any](m M) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
