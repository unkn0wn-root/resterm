package restwriter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
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

	content := Render(doc, opts)
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

func Render(doc *restfile.Document, opts Options) string {
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

	return b.String()
}

func renderHeader(b *strings.Builder, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	for _, line := range strings.Split(text, "\n") {
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

func renderDescription(b *strings.Builder, desc string) {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return
	}
	for _, line := range strings.Split(desc, "\n") {
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

func formatOAuthParams(params map[string]string) []string {
	if len(params) == 0 {
		return nil
	}

	ordered := []string{
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
	if strings.ContainsAny(val, " \t") && !strings.Contains(val, "\"") {
		val = "\"" + val + "\""
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
