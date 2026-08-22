package parser

import (
	"fmt"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/capture"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
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

func (b *documentBuilder) handleRequestMetadataDirective(d parsedDirective) directiveOutcome {
	rest := d.Args
	switch d.Name {
	case directive.RequestName:
		b.request.metadata.Name = directive.Value(rest)
		return directiveApplied
	case directive.Description:
		b.request.metadata.Description = appendDesc(b.request.metadata.Description, rest)
		return directiveApplied
	case directive.Tag:
		b.addRequestTags(rest)
		return directiveApplied
	case directive.NoLog:
		b.request.metadata.NoLog = true
		return directiveApplied
	case directive.LogSensitiveHeaders:
		allow, ok := directive.ParseSwitch(rest)
		if !ok {
			return b.reject(d, d.Spelling.Tag()+" must be true or false")
		}
		b.request.metadata.AllowSensitiveHeaders = allow
		return directiveApplied
	case directive.Settings:
		settings, err := applySettingsTokens(b.request.settings, rest, directive.Settings)
		b.request.settings = settings
		b.report(d.lines.Start, err)
		return directiveApplied
	case directive.Setting:
		settings, err := putSetting(b.request.settings, rest)
		b.request.settings = settings
		b.report(d.lines.Start, err)
		return directiveApplied
	case directive.Timeout:
		if b.request.settings == nil {
			b.request.settings = make(map[string]string)
		}
		b.request.settings["timeout"] = rest
		return directiveApplied
	case directive.Var:
		b.addRequestVar(d.lines.Start, rest)
		return directiveApplied
	case directive.Script:
		if rest == "" {
			b.request.discardScript = false
			return directiveApplied
		}
		if err := b.setScript(rest, ""); err != nil {
			b.report(d.lines.Start, err)
			return directiveRejected
		}
		return directiveApplied
	case directive.RTS:
		if err := b.setRTSScript(rest); err != nil {
			return b.reject(d, err.Error())
		}
		return directiveApplied
	case directive.Apply:
		return b.addApply(d)
	case directive.Capture:
		return b.addCapture(d)
	case directive.Assert:
		return b.addAssert(d)
	case directive.When:
		return b.setWhen(d)
	case directive.Poll:
		return b.setPoll(d)
	case directive.Retry:
		return b.setRetry(d)
	case directive.RetryWhen:
		return b.setRetryWhen(d)
	case directive.RetryBackoff:
		return b.setRetryBackoff(d)
	case directive.ForEach:
		return b.setForEach(d)
	case directive.Profile:
		spec, err := parseProfileSpec(rest)
		b.report(d.lines.Start, err)
		if spec == nil {
			return directiveRejected
		}
		b.request.metadata.Profile = spec
		return directiveApplied
	case directive.Trace:
		spec, err := parseTraceSpec(rest)
		b.report(d.lines.Start, err)
		if spec == nil {
			return directiveRejected
		}
		b.request.metadata.Trace = spec
		return directiveApplied
	case directive.Compare:
		return b.setCompare(d)
	}
	return directiveIgnored
}

func (b *documentBuilder) addRequestTags(rest string) {
	tags := strings.Fields(rest)
	if len(tags) == 0 {
		tags = strings.Split(rest, ",")
	}
	b.request.metadata.Tags = appendTagsFold(b.request.metadata.Tags, tags)
}

func (b *documentBuilder) addRequestVar(no int, rest string) {
	name, value := directive.ParseNameValue(rest)
	if name == "" {
		return
	}
	b.checkEnvRef(no, value)
	b.request.variables = append(b.request.variables, restfile.Variable{
		Name:     name,
		Value:    value,
		Line:     no,
		Scope:    directive.ScopeRequest,
		Authored: true,
	})
}

func (b *documentBuilder) addApply(d parsedDirective) directiveOutcome {
	spec, err := parseApplySpec(d.Args, d.lines.Start)
	if err != nil {
		return b.reject(d, err.Error())
	}
	d.setExprCol(&spec.Col, spec.Expression)
	b.request.metadata.Applies = append(b.request.metadata.Applies, spec)
	return directiveApplied
}

func (b *documentBuilder) addCapture(d parsedDirective) directiveOutcome {
	spec, ok := b.parseCaptureDirective(d.Args, d.lines.Start)
	if !ok {
		return directiveRejected
	}
	d.setExprCol(&spec.Col, spec.Expression)
	b.request.metadata.Captures = append(b.request.metadata.Captures, spec)
	return directiveApplied
}

func (b *documentBuilder) addAssert(d parsedDirective) directiveOutcome {
	spec, ok := b.parseAssertDirective(d.Args, d.lines.Start, d.argCol)
	if !ok {
		return b.reject(d, "@assert expression missing")
	}
	b.request.metadata.Asserts = append(b.request.metadata.Asserts, spec)
	return directiveApplied
}

func (b *documentBuilder) setWhen(d parsedDirective) directiveOutcome {
	spec, err := parseConditionSpec(d.Args, d.lines.Start, d.Spelling == directive.SkipIf)
	if err != nil {
		return b.reject(d, err.Error())
	}
	d.setExprCol(&spec.Col, spec.Expression)
	b.request.metadata.When = spec
	return directiveApplied
}

func (b *documentBuilder) setForEach(d parsedDirective) directiveOutcome {
	spec, err := parseForEachSpec(d.Args, d.lines.Start)
	if err != nil {
		return b.reject(d, err.Error())
	}
	b.request.metadata.ForEach = spec
	return directiveApplied
}

func (b *documentBuilder) setCompare(d parsedDirective) directiveOutcome {
	spec, err := parseCompareDirective(d.Args)
	if err != nil {
		return b.reject(d, err.Error())
	}
	b.request.metadata.Compare = spec
	return directiveApplied
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

func putSetting(dst map[string]string, rest string) (map[string]string, error) {
	key, val := directive.CutKey(rest)
	if key == "" {
		return dst, nil
	}
	// The @settings spelling. Read it that way instead of storing a key with an
	// equals sign in it, which no consumer would ever look up.
	if strings.Contains(key, "=") {
		return applySettingsTokens(dst, rest, directive.Setting)
	}
	// A key written on its own is a flag, the same as it is in @settings. Only
	// the parser can tell that apart from a value that was written empty.
	if val == "" {
		val = "true"
	}
	if dst == nil {
		dst = make(map[string]string)
	}
	dst[key] = val
	return dst, nil
}

// Duplicate settings are errors because the map would otherwise keep only the
// last value.
func applySettingsTokens(
	dst map[string]string,
	raw string,
	name directive.Name,
) (map[string]string, error) {
	opts, err := directive.ParseOptions(name, raw)
	if opts.Len() == 0 {
		return dst, err
	}
	if dst == nil {
		dst = make(map[string]string, opts.Len())
	}
	for k, v := range opts.All() {
		if k == "" {
			continue
		}
		dst[k] = v
	}
	return dst, err
}

func (b *documentBuilder) parseCaptureDirective(
	rest string,
	line int,
) (restfile.CaptureSpec, bool) {
	scopeToken, name, expression := cutCapture(rest)
	if scopeToken == "" {
		b.addWarning(line, "@capture missing scope (use request, file, or global)")
		return restfile.CaptureSpec{}, false
	}
	scope, secret, ok := directive.ParseSecretScope(scopeToken)
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
	if name == "" {
		b.addWarning(line, "@capture missing '<name> <expression>'")
		return restfile.CaptureSpec{}, false
	}
	if expression == "" {
		b.addWarning(line, "@capture missing expression after capture name")
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

func cutCapture(rest string) (scope, name, expr string) {
	scope, rem := directive.CutKey(rest)
	name, rem = directive.CutToken(rem)
	return scope, name, cutAssign(rem)
}

func (b *documentBuilder) parseAssertDirective(rest string, line, col int) (restfile.AssertSpec, bool) {
	expr, msg := splitAssert(rest)
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

// splitAssert ignores arrows inside strings, comments, and nested groups.
func splitAssert(text string) (expr, msg string) {
	s := strings.TrimSpace(text)
	i := strings.Index(rts.Mask(s), "=>")
	if i < 0 {
		return s, ""
	}
	return strings.TrimSpace(s[:i]), directive.TrimQuotes(strings.TrimSpace(s[i+2:]))
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
