package writer

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

	"github.com/unkn0wn-root/resterm/internal/openapi"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type FileWriter struct{}

func NewFileWriter() *FileWriter {
	return &FileWriter{}
}

func (w *FileWriter) WriteDocument(
	ctx context.Context,
	doc *restfile.Document,
	destination string,
	opts openapi.WriterOptions,
) error {
	if doc == nil {
		return errors.New("writer: document is nil")
	}
	if strings.TrimSpace(destination) == "" {
		return errors.New("writer: destination path is empty")
	}

	content := renderDocument(doc, opts)
	if err := ctx.Err(); err != nil {
		return err
	}

	dir := filepath.Dir(destination)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("writer: create directory: %w", err)
	}

	if !opts.OverwriteExisting {
		if _, err := os.Stat(destination); err == nil {
			return fmt.Errorf("writer: destination %s already exists", destination)
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

	if err := os.Rename(tmpName, destination); err != nil {
		return fmt.Errorf("writer: rename temp file: %w", err)
	}
	return nil
}

func renderDocument(doc *restfile.Document, opts openapi.WriterOptions) string {
	var b strings.Builder

	if header := strings.TrimSpace(opts.HeaderComment); header != "" {
		for _, line := range strings.Split(header, "\n") {
			b.WriteString("# ")
			b.WriteString(strings.TrimSpace(line))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	renderScopeVariables(&b, doc.Variables)
	renderScopeVariables(&b, doc.Globals)

	if len(doc.Variables) > 0 || len(doc.Globals) > 0 {
		b.WriteString("\n")
	}

	for idx, req := range doc.Requests {
		if idx > 0 {
			b.WriteString("\n")
		}
		renderRequest(&b, req)
	}

	return b.String()
}

func renderScopeVariables(b *strings.Builder, vars []restfile.Variable) {
	for _, v := range vars {
		value := strings.TrimSpace(v.Value)
		switch v.Scope {
		case restfile.ScopeGlobal:
			directive := "@global"
			if v.Secret {
				directive = "@global-secret"
			}
			fmt.Fprintf(b, "# %s %s %s\n", directive, v.Name, value)
		case restfile.ScopeFile:
			scopeToken := "file"
			if v.Secret {
				scopeToken = "file-secret"
			}
			fmt.Fprintf(b, "# @var %s %s %s\n", scopeToken, v.Name, value)
		default:
			scopeToken := "request"
			if v.Secret {
				scopeToken = "request-secret"
			}
			fmt.Fprintf(b, "# @var %s %s %s\n", scopeToken, v.Name, value)
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

func renderDescription(b *strings.Builder, description string) {
	description = strings.TrimSpace(description)
	if description == "" {
		return
	}
	for _, line := range strings.Split(description, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		b.WriteString("# @description ")
		b.WriteString(trimmed)
		b.WriteString("\n")
	}
}

func renderTags(b *strings.Builder, tags []string) {
	if len(tags) == 0 {
		return
	}
	tokens := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tokens = append(tokens, tag)
		}
	}
	if len(tokens) == 0 {
		return
	}
	b.WriteString("# @tag ")
	b.WriteString(strings.Join(tokens, " "))
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
		placement := strings.TrimSpace(auth.Params["placement"])
		name := strings.TrimSpace(auth.Params["name"])
		value := strings.TrimSpace(auth.Params["value"])
		if placement == "" {
			placement = "header"
		}
		if name == "" {
			name = "X-API-Key"
		}
		b.WriteString("# @auth apikey ")
		b.WriteString(placement)
		b.WriteString(" ")
		b.WriteString(name)
		b.WriteString(" ")
		b.WriteString(value)
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

func formatOAuthParams(params map[string]string) []string {
	if len(params) == 0 {
		return nil
	}

	ordered := []string{
		openapi.OAuthParamTokenURL,
		openapi.OAuthParamClientID,
		openapi.OAuthParamClientSecret,
		openapi.OAuthParamScope,
		openapi.OAuthParamAudience,
		openapi.OAuthParamResource,
		openapi.OAuthParamGrant,
		openapi.OAuthParamUsername,
		openapi.OAuthParamPassword,
		openapi.OAuthParamClientAuth,
		openapi.OAuthParamCacheKey,
	}
	seen := make(map[string]struct{}, len(ordered))

	var parts []string
	for _, key := range ordered {
		value := strings.TrimSpace(params[key])
		if value == "" {
			continue
		}
		parts = append(parts, formatAuthParam(key, value))
		seen[key] = struct{}{}
	}
	var extras []string
	for key, raw := range params {
		lower := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
		if _, ok := seen[lower]; ok {
			continue
		}
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		extras = append(extras, formatAuthParam(lower, value))
	}
	if len(extras) > 0 {
		sort.Strings(extras)
		parts = append(parts, extras...)
	}
	return parts
}

func formatAuthParam(key, value string) string {
	if strings.ContainsAny(value, " \t") && !strings.Contains(value, "\"") {
		value = "\"" + value + "\""
	}
	return fmt.Sprintf("%s=%s", key, value)
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

func renderCaptures(b *strings.Builder, captures []restfile.CaptureSpec) {
	for _, capture := range captures {
		scope := captureScopeToken(capture)
		b.WriteString("# @capture ")
		b.WriteString(scope)
		b.WriteString(" ")
		b.WriteString(capture.Name)
		b.WriteString(" ")
		b.WriteString(strings.TrimSpace(capture.Expression))
		b.WriteString("\n")
	}
}

func reqLine(req *restfile.Request) string {
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = "GET"
	}
	return fmt.Sprintf("%s %s\n", method, strings.TrimSpace(req.URL))
}

func renderHeaders(b *strings.Builder, headers http.Header) {
	if len(headers) == 0 {
		return
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		values := headers[name]
		for _, value := range values {
			b.WriteString(name)
			b.WriteString(": ")
			b.WriteString(value)
			b.WriteString("\n")
		}
	}
}

func captureScopeToken(capture restfile.CaptureSpec) string {
	scope := ""
	switch capture.Scope {
	case restfile.CaptureScopeRequest:
		scope = "request"
	case restfile.CaptureScopeFile:
		scope = "file"
	case restfile.CaptureScopeGlobal:
		scope = "global"
	default:
		scope = "request"
	}
	if capture.Secret {
		scope += "-secret"
	}
	return scope
}
