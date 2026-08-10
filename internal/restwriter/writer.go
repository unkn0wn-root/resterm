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

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/parser/bodyref"
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
	w := directiveWriter{b: &b}

	renderHeader(w.b, opts.HeaderComment)
	renderScopeVariables(w, doc.Variables)
	renderScopeVariables(w, doc.Globals)
	renderSettings(w, doc.Settings)
	if err := renderPatches(w, doc.Patches); err != nil {
		return "", err
	}

	// Without this the preamble reads as part of the block below it.
	if len(doc.Variables)+len(doc.Globals)+len(doc.Settings)+len(doc.Patches) > 0 {
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
		renderRequest(w, req)
		idx++
	}
	for _, mock := range doc.Mocks {
		if mock == nil {
			continue
		}
		if idx > 0 {
			b.WriteString("\n")
		}
		if err := renderMock(w, mock); err != nil {
			return "", err
		}
		idx++
	}
	// A mock block runs to the next separator, so a workflow written straight
	// after one would be read back as part of the mock body.
	if idx > 0 && len(doc.Workflows) > 0 {
		b.WriteString("\n###\n")
	}
	for _, wf := range doc.Workflows {
		if idx > 0 {
			b.WriteString("\n")
		}
		b.WriteString(RenderWorkflow(wf, "workflow"))
		b.WriteString("\n")
		idx++
	}

	return b.String(), nil
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

// A global variable has a directive of its own, where file and request scope
// share @var and name their scope in the first word.
func renderScopeVariables(w directiveWriter, vars []restfile.Variable) {
	for _, v := range vars {
		val := strings.TrimSpace(v.Value)
		if v.Scope == directive.ScopeGlobal {
			name := directive.Global
			if v.Secret {
				name = directive.GlobalSecret
			}
			w.line(name, v.Name+" "+val)
			continue
		}
		w.line(directive.Var, scopeToken(v.Scope, v.Secret)+" "+v.Name+" "+val)
	}
}

func renderRequest(w directiveWriter, req *restfile.Request) {
	title := req.Metadata.Name
	if title == "" {
		title = fmt.Sprintf("%s %s", strings.ToUpper(req.Method), req.URL)
	}
	w.title(title)

	if req.Metadata.Name != "" {
		w.line(directive.RequestName, req.Metadata.Name)
	}
	renderDescription(w, req.Metadata.Description)
	renderTags(w, req.Metadata.Tags)
	renderLoggingDirectives(w, req.Metadata)
	renderAuth(w, req.Metadata.Auth)
	renderSettings(w, req.Settings)
	renderRequestVariables(w, req.Variables)
	writeOne(w, req.Metadata.When, conditionArg)
	writeOne(w, req.Metadata.ForEach, forEachArg)
	writeEach(w, req.Metadata.Applies, applyArg)
	writeEach(w, req.Metadata.Captures, captureArg)
	writeEach(w, req.Metadata.Asserts, assertArg)
	renderBodyOptions(w, req)

	w.b.WriteString(reqLine(req))
	renderHeaders(w.b, req.Headers)
	w.b.WriteString("\n")
	if req.Body.FilePath != "" {
		fmt.Fprintf(w.b, "< %s\n", strings.TrimSpace(req.Body.FilePath))
	} else if strings.TrimSpace(req.Body.Text) != "" {
		w.b.WriteString(req.Body.Text)
		if !strings.HasSuffix(req.Body.Text, "\n") {
			w.b.WriteString("\n")
		}
	}
}

func renderBodyOptions(w directiveWriter, req *restfile.Request) {
	if req.Body.Options.ExpandTemplates {
		w.line(directive.Body, "expand")
	}
	if req.Body.Options.ForceInline || bodyTextNeedsInlineDirective(req) {
		w.line(directive.Body, "inline")
	}
}

// An inline body whose first line reads as a file reference needs the directive
// to survive the trip back through the parser.
func bodyTextNeedsInlineDirective(req *restfile.Request) bool {
	if strings.TrimSpace(req.Body.FilePath) != "" {
		return false
	}
	text := strings.TrimSpace(req.Body.Text)
	if text == "" {
		return false
	}
	line, _, _ := strings.Cut(text, "\n")
	_, ok := bodyref.Parse(line, bodyref.Options{Location: bodyref.Line})
	return ok
}

func renderDescription(w directiveWriter, desc string) {
	for line := range strings.SplitSeq(strings.TrimSpace(desc), "\n") {
		if t := strings.TrimSpace(line); t != "" {
			w.line(directive.Description, t)
		}
	}
}

func renderTags(w directiveWriter, tags []string) {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if t := strings.TrimSpace(tag); t != "" {
			out = append(out, t)
		}
	}
	if len(out) > 0 {
		w.line(directive.Tag, strings.Join(out, " "))
	}
}

func renderLoggingDirectives(w directiveWriter, meta restfile.RequestMetadata) {
	if meta.NoLog {
		w.line(directive.NoLog, "")
	}
	if meta.AllowSensitiveHeaders {
		w.line(directive.LogSensitiveHeaders, "true")
	}
}

func renderAuth(w directiveWriter, auth *restfile.AuthSpec) {
	if auth == nil {
		return
	}
	if args := authArgs(*auth); len(args) > 0 {
		w.line(directive.Auth, strings.Join(args, " "))
	}
}

// Every form names itself first and then its parameters in the order @auth
// reads them back. An unknown form writes nothing.
func authArgs(auth restfile.AuthSpec) []string {
	p := auth.Params
	switch strings.ToLower(auth.Type) {
	case "basic":
		return []string{"basic", strings.TrimSpace(p["username"]), strings.TrimSpace(p["password"])}
	case "bearer":
		return []string{"bearer", strings.TrimSpace(p["token"])}
	case "apikey", "api-key":
		place := strings.TrimSpace(p["placement"])
		if place == "" {
			place = "header"
		}
		name := strings.TrimSpace(p["name"])
		if name == "" {
			name = "X-API-Key"
		}
		return []string{"apikey", place, name, strings.TrimSpace(p["value"])}
	case "oauth2":
		return authFormArgs("oauth2", formatOAuthParams(p))
	case "command":
		return authFormArgs("command", formatCommandParams(p))
	default:
		return nil
	}
}

// A form that resolved to no parameters at all is dropped rather than written
// as a bare name the parser would reject.
func authFormArgs(form string, params []string) []string {
	if len(params) == 0 {
		return nil
	}
	return append([]string{form}, params...)
}

func renderSettings(w directiveWriter, set map[string]string) {
	for _, key := range sortedKeys(set) {
		if val := strings.TrimSpace(set[key]); val != "" {
			w.line(directive.Setting, key+" "+val)
		}
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

func renderRequestVariables(w directiveWriter, vars []restfile.Variable) {
	for _, v := range vars {
		if v.Scope != directive.ScopeRequest {
			continue
		}
		arg := scopeToken(v.Scope, v.Secret) + " " + v.Name
		if val := strings.TrimSpace(v.Value); val != "" {
			arg += " " + val
		}
		w.line(directive.Var, arg)
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

func sortedKeys[M ~map[string]V, V any](m M) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
