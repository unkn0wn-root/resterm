package parser

import (
	"strings"
	"unicode"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// Splits on spaces but keeps quoted strings together.
// Quotes themselves get stripped - "hello resterm" becomes a single field: hello resterm
func splitAuthFields(input string) []string {
	var fields []string
	var current strings.Builder
	inQuote := false
	var quoteRune rune

	flush := func() {
		if current.Len() > 0 {
			fields = append(fields, current.String())
			current.Reset()
		}
	}

	for _, r := range input {
		switch {
		case inQuote:
			if r == quoteRune {
				inQuote = false
			} else {
				current.WriteRune(r)
			}
		case unicode.IsSpace(r):
			flush()
		case r == '"' || r == '\'':
			inQuote = true
			quoteRune = r
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return fields
}

func parseKeyValuePairs(fields []string) map[string]string {
	pairs := map[string]string{}
	for _, field := range fields {
		if field == "" {
			continue
		}
		if idx := strings.Index(field, "="); idx != -1 {
			key := strings.TrimSpace(field[:idx])
			value := strings.TrimSpace(field[idx+1:])
			if key == "" {
				continue
			}
			pairs[strings.ToLower(key)] = trimQuotes(value)
		}
	}
	return pairs
}

func parseBool(value string) (bool, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "true", "t", "1", "yes", "on":
		return true, true
	case "false", "f", "0", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func parseScopeToken(token string) (string, bool) {
	tok := strings.ToLower(strings.TrimSpace(token))
	if tok == "" {
		return "", false
	}
	secret := strings.HasSuffix(tok, "-secret")
	if secret {
		tok = strings.TrimSuffix(tok, "-secret")
	}
	return tok, secret
}

func isAlpha(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isIdentStartRune(r rune) bool {
	return r == '_' || isAlpha(r)
}

func isIdentRune(r rune) bool {
	return isIdentStartRune(r) || isDigit(r)
}

func isOptionKeyRune(r rune) bool {
	return r == '_' || r == '-' || r == '.' || isAlpha(r) || isDigit(r)
}

func isIdent(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	for i, r := range value {
		if i == 0 {
			if !isIdentStartRune(r) {
				return false
			}
			continue
		}
		if !isIdentRune(r) {
			return false
		}
	}
	return true
}

func splitFirst(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", ""
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return "", ""
	}
	token := fields[0]
	remainder := strings.TrimSpace(trimmed[len(token):])
	return token, remainder
}

func parseNameValue(input string) (string, string) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", ""
	}
	matches := nameValueRe.FindStringSubmatch(trimmed)
	if matches == nil {
		return "", ""
	}
	name := matches[1]
	valueCandidate := matches[2]
	if valueCandidate == "" {
		valueCandidate = matches[3]
	}
	return name, strings.TrimSpace(valueCandidate)
}

func splitDirective(text string) (string, string) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", ""
	}

	key := strings.ToLower(strings.TrimRight(fields[0], ":"))
	var rest string
	if len(text) > len(fields[0]) {
		rest = strings.TrimSpace(text[len(fields[0]):])
	}
	return key, rest
}

func splitAssert(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", ""
	}

	inQuote := false
	var quote byte
	escaped := false
	for i := 0; i < len(trimmed)-1; i++ {
		ch := trimmed[i]
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
		if ch == '=' && trimmed[i+1] == '>' {
			left := strings.TrimSpace(trimmed[:i])
			right := strings.TrimSpace(trimmed[i+2:])
			return left, trimQuotes(right)
		}
	}
	return trimmed, ""
}

func parseOptionTokens(input string) map[string]string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return map[string]string{}
	}
	tokens := tokenizeOptionTokens(trimmed)
	if len(tokens) == 0 {
		return map[string]string{}
	}
	options := make(map[string]string, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		key := token
		value := "true"
		if idx := strings.Index(token, "="); idx >= 0 {
			key = strings.TrimSpace(token[:idx])
			value = strings.TrimSpace(token[idx+1:])
		}
		if key == "" {
			continue
		}
		options[strings.ToLower(key)] = trimQuotes(value)
	}
	return options
}

func splitExprOptions(input string) (string, map[string]string) {
	tokens := splitAuthFields(strings.TrimSpace(input))
	if len(tokens) == 0 {
		return "", map[string]string{}
	}
	optIndex := -1
	for i, token := range tokens {
		if isOptionToken(token) {
			optIndex = i
			break
		}
	}
	if optIndex == -1 {
		return strings.Join(tokens, " "), map[string]string{}
	}
	expr := strings.Join(tokens[:optIndex], " ")
	opts := parseOptionTokens(strings.Join(tokens[optIndex:], " "))
	return expr, opts
}

func isOptionToken(token string) bool {
	idx := strings.Index(token, "=")
	if idx <= 0 {
		return false
	}
	key := token[:idx]
	for _, r := range key {
		if !isOptionKeyRune(r) {
			return false
		}
	}
	return true
}

func applySettingsTokens(dst map[string]string, raw string) map[string]string {
	opts := parseOptionTokens(raw)
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

// Like splitAuthFields but handles backslash escapes.
// A trailing backslash gets preserved if nothing follows it.
func tokenizeOptionTokens(input string) []string {
	var tokens []string
	var current strings.Builder
	var quote rune
	escaping := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
	}

	for _, r := range input {
		switch {
		case escaping:
			current.WriteRune(r)
			escaping = false
		case r == '\\':
			escaping = true
		case quote != 0:
			if r == quote {
				quote = 0
				break
			}
			current.WriteRune(r)
		case r == '"' || r == '\'':
			quote = r
		case unicode.IsSpace(r):
			flush()
		default:
			current.WriteRune(r)
		}
	}
	if escaping {
		current.WriteRune('\\')
	}
	flush()
	return tokens
}

func trimQuotes(value string) string {
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func parseWorkflowFailureMode(value string) (restfile.WorkflowFailureMode, bool) {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return "", false
	}
	switch trimmed {
	case "stop", "fail", "abort":
		return restfile.WorkflowOnFailureStop, true
	case "continue", "skip":
		return restfile.WorkflowOnFailureContinue, true
	default:
		return "", false
	}
}

func parseTagList(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || r == ','
	})
	var tags []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	return tags
}

func parseScriptSpec(rest string) (string, string) {
	fields := splitAuthFields(rest)
	kind := ""
	lang := ""
	for _, field := range fields {
		if strings.Contains(field, "=") {
			continue
		}
		if kind == "" {
			kind = field
			continue
		}
		if lang == "" {
			if v, ok := scriptLangToken(field); ok {
				lang = v
			}
		}
	}
	params := parseKeyValuePairs(fields)
	if v := params["lang"]; v != "" {
		lang = v
	}
	if v := params["language"]; v != "" && lang == "" {
		lang = v
	}
	return normScriptKind(kind), normScriptLang(lang)
}

func scriptLangToken(tok string) (string, bool) {
	out := strings.ToLower(strings.TrimSpace(tok))
	switch out {
	case "js", "javascript":
		return "js", true
	case "rts", "restermlang":
		return "rts", true
	default:
		return "", false
	}
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}
