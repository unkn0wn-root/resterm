package parser

import (
	"fmt"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/capture"
	"github.com/unkn0wn-root/resterm/internal/parser/directive/options"
	dvalue "github.com/unkn0wn-root/resterm/internal/parser/directive/value"
	"github.com/unkn0wn-root/resterm/internal/parser/lexer"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (b *documentBuilder) handleDescriptionLine(ln line) bool {
	b.ensureRequest(ln.no)
	if b.request.http.HasMethod() {
		return false
	}
	b.request.metadata.Description = appendDesc(b.request.metadata.Description, ln.text)
	b.appendLine(ln.raw)
	return true
}

func (b *documentBuilder) handleRequestMetadataDirective(no, argCol int, key, rest string) bool {
	switch key {
	case "name":
		if rest != "" {
			b.request.metadata.Name = lexer.TrimQuotes(strings.TrimSpace(rest))
		}
		return true
	case "description", "desc":
		b.request.metadata.Description = appendDesc(b.request.metadata.Description, rest)
		return true
	case "tag", "tags":
		b.addRequestTags(rest)
		return true
	case "no-log", "nolog":
		b.request.metadata.NoLog = true
		return true
	case "log-sensitive-headers", "log-secret-headers":
		if rest == "" {
			b.request.metadata.AllowSensitiveHeaders = true
			return true
		}
		if value, ok := dvalue.ParseBool(rest); ok {
			b.request.metadata.AllowSensitiveHeaders = value
		}
		return true
	case "settings":
		b.request.settings = applySettingsTokens(b.request.settings, rest)
		return true
	case "setting":
		b.request.settings = putSetting(b.request.settings, rest)
		return true
	case "timeout":
		if b.request.settings == nil {
			b.request.settings = make(map[string]string)
		}
		b.request.settings["timeout"] = rest
		return true
	case "var":
		b.addRequestVar(no, rest)
		return true
	case "script":
		if rest != "" {
			b.setScript(rest, "")
		} else {
			b.request.discardScript = false
		}
		return true
	case "rts":
		if err := b.setRTSScript(rest); err != nil {
			b.addError(no, err.Error())
		}
		return true
	case "apply":
		b.addApply(no, argCol, rest)
		return true
	case "capture":
		b.addCapture(no, argCol, rest)
		return true
	case "assert":
		b.addAssert(no, argCol, rest)
		return true
	case "when", "skip-if":
		b.setWhen(no, argCol, key, rest)
		return true
	case "for-each":
		b.setForEach(no, rest)
		return true
	case "profile":
		if spec := parseProfileSpec(rest); spec != nil {
			b.request.metadata.Profile = spec
		}
		return true
	case "trace":
		if spec := parseTraceSpec(rest); spec != nil {
			b.request.metadata.Trace = spec
		}
		return true
	case "compare":
		b.setCompare(no, rest)
		return true
	}
	return false
}

func (b *documentBuilder) addRequestTags(rest string) {
	tags := strings.Fields(rest)
	if len(tags) == 0 {
		tags = strings.Split(rest, ",")
	}
	b.request.metadata.Tags = appendTagsFold(b.request.metadata.Tags, tags)
}

func (b *documentBuilder) addRequestVar(no int, rest string) {
	name, value := options.ParseNameValue(rest)
	if name == "" {
		return
	}
	b.request.variables = append(b.request.variables, restfile.Variable{
		Name:  name,
		Value: value,
		Line:  no,
		Scope: restfile.ScopeRequest,
	})
}

func (b *documentBuilder) addApply(no, argCol int, rest string) {
	spec, err := parseApplySpec(rest, no)
	if err != nil {
		b.addError(no, err.Error())
		return
	}
	if c := exprCol(rest, spec.Expression, argCol); c > 0 {
		spec.Col = c
	}
	b.request.metadata.Applies = append(b.request.metadata.Applies, spec)
}

func (b *documentBuilder) addCapture(no, argCol int, rest string) {
	spec, ok := b.parseCaptureDirective(rest, no)
	if !ok {
		return
	}
	if c := exprCol(rest, spec.Expression, argCol); c > 0 {
		spec.Col = c
	}
	b.request.metadata.Captures = append(b.request.metadata.Captures, spec)
}

func (b *documentBuilder) addAssert(no, argCol int, rest string) {
	spec, ok := b.parseAssertDirective(rest, no, argCol)
	if !ok {
		b.addError(no, "@assert expression missing")
		return
	}
	b.request.metadata.Asserts = append(b.request.metadata.Asserts, spec)
}

func (b *documentBuilder) setWhen(no, argCol int, key, rest string) {
	spec, err := parseConditionSpec(rest, no, key == "skip-if")
	if err != nil {
		b.addError(no, err.Error())
		return
	}
	if b.request.metadata.When != nil {
		b.addError(no, "@when directive already defined for this request")
		return
	}
	if c := exprCol(rest, spec.Expression, argCol); c > 0 {
		spec.Col = c
	}
	b.request.metadata.When = spec
}

func (b *documentBuilder) setForEach(no int, rest string) {
	spec, err := parseForEachSpec(rest, no)
	if err != nil {
		b.addError(no, err.Error())
		return
	}
	if b.request.metadata.ForEach != nil {
		b.addError(no, "@for-each directive already defined for this request")
		return
	}
	b.request.metadata.ForEach = spec
}

func (b *documentBuilder) setCompare(no int, rest string) {
	if b.request.metadata.Compare != nil {
		b.addError(no, "@compare directive already defined for this request")
		return
	}
	spec, err := parseCompareDirective(rest)
	if err != nil {
		b.addError(no, err.Error())
		return
	}
	b.request.metadata.Compare = spec
}

func appendDesc(existing, add string) string {
	if existing != "" {
		existing += "\n"
	}
	return existing + add
}

func appendTagsFold(dst, tags []string) []string {
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if !slices.ContainsFunc(dst, func(item string) bool { return strings.EqualFold(item, tag) }) {
			dst = append(dst, tag)
		}
	}
	return dst
}

func putSetting(dst map[string]string, rest string) map[string]string {
	key, value := lexer.SplitDirective(rest)
	if key == "" {
		return dst
	}
	if dst == nil {
		dst = make(map[string]string)
	}
	dst[key] = value
	return dst
}

func applySettingsTokens(dst map[string]string, raw string) map[string]string {
	opts := options.Parse(raw)
	if len(opts) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]string, len(opts))
	}
	for k, v := range opts {
		if k == "" {
			continue
		}
		dst[k] = v
	}
	return dst
}

func (b *documentBuilder) parseCaptureDirective(
	rest string,
	line int,
) (restfile.CaptureSpec, bool) {
	scopeToken, remainder := lexer.SplitDirective(rest)
	if scopeToken == "" {
		b.addWarning(line, "@capture missing scope (use request, file, or global)")
		return restfile.CaptureSpec{}, false
	}
	scope, secret, ok := parseCaptureScope(scopeToken)
	if !ok {
		b.addWarning(
			line,
			fmt.Sprintf(
				"@capture scope %q is invalid (use request, file, global, with optional -secret)",
				scopeToken,
			),
		)
		return restfile.CaptureSpec{}, false
	}
	s := strings.TrimSpace(remainder)
	if s == "" {
		b.addWarning(line, "@capture missing '<name> <expression>'")
		return restfile.CaptureSpec{}, false
	}
	nameEnd := strings.IndexAny(s, " \t")
	if nameEnd == -1 {
		b.addWarning(line, "@capture missing expression after capture name")
		return restfile.CaptureSpec{}, false
	}
	name := strings.TrimSpace(s[:nameEnd])
	expression := strings.TrimSpace(s[nameEnd:])
	if expression == "" {
		b.addWarning(line, "@capture expression missing")
		return restfile.CaptureSpec{}, false
	}
	if strings.HasPrefix(expression, "=") {
		expression = strings.TrimSpace(expression[1:])
	}
	if expression == "" {
		b.addWarning(line, "@capture expression missing after '='")
		return restfile.CaptureSpec{}, false
	}
	mode := restfile.CaptureExprModeRTS
	if capture.HasUnquotedTemplateMarker(expression) {
		mode = restfile.CaptureExprModeTemplate
	}
	return restfile.CaptureSpec{
		Scope:      scope,
		Name:       name,
		Expression: expression,
		Mode:       mode,
		Secret:     secret,
		Line:       line,
	}, true
}

func (b *documentBuilder) parseAssertDirective(rest string, line, col int) (restfile.AssertSpec, bool) {
	expr, msg := splitAssert(rest)
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return restfile.AssertSpec{}, false
	}
	return restfile.AssertSpec{
		Expression: expr,
		Message:    msg,
		Line:       line,
		Col:        col,
	}, true
}

func splitAssert(text string) (string, string) {
	s := strings.TrimSpace(text)
	if s == "" {
		return "", ""
	}

	inQuote := false
	var quote byte
	escaped := false
	for i := 0; i < len(s)-1; i++ {
		ch := s[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if inQuote {
			if ch == quote {
				inQuote = false
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuote = true
			quote = ch
			continue
		}
		if ch == '=' && s[i+1] == '>' {
			left := strings.TrimSpace(s[:i])
			right := strings.TrimSpace(s[i+2:])
			return left, lexer.TrimQuotes(right)
		}
	}
	return s, ""
}

func (b *documentBuilder) lintRequestCaptures(req *restfile.Request) {
	for _, c := range req.Metadata.Captures {
		if capture.HasJSONPathDoubleDot(c.Expression) {
			b.addWarning(
				c.Line,
				fmt.Sprintf(
					"@capture %q expression %q has double dot after json (use response.json.<field>)",
					c.Name,
					c.Expression,
				),
			)
		}
		if c.Mode == restfile.CaptureExprModeTemplate &&
			capture.MixedTemplateRTSCall(c.Expression) {
			b.addWarning(
				c.Line,
				fmt.Sprintf(
					"@capture %q mixes template markers with RTS call syntax; use pure RTS or {{= ... }}",
					c.Name,
				),
			)
		}
	}
}
