package vars

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/errdef"
)

const (
	dotEnvDefaultName = "default"
)

type quoteMode int

const (
	quoteModeNone quoteMode = iota
	quoteModeSingle
	quoteModeDouble
)

// detection keeps JSON discovery stable by requiring names that intentionally look like .env files
func IsDotEnvPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".json" || strings.HasSuffix(base, ".json") {
		return false
	}
	if base == ".env" {
		return true
	}
	if strings.HasPrefix(base, ".env.") {
		return true
	}
	if strings.HasSuffix(base, ".env") {
		return true
	}
	return false
}

func loadDotEnvEnvironment(path string) (envs EnvironmentSet, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errdef.Wrap(errdef.CodeFilesystem, err, "open env file %s", path)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = errdef.Wrap(errdef.CodeFilesystem, closeErr, "close env file %s", path)
		}
	}()

	values, err := parseDotEnv(f, path)
	if err != nil {
		return nil, err
	}

	envName := deriveDotEnvName(values, path)
	if envName == "" {
		envName = dotEnvDefaultName
	}
	envs = make(EnvironmentSet, 1)
	envs[envName] = values
	return envs, nil
}

func parseDotEnv(r io.Reader, path string) (map[string]string, error) {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	values := make(map[string]string)
	workspaceSeen := false
	lineNumber := 0
	for scanner.Scan() {
		// process lines in order so interpolation can only see keys defined above,
		// matching typical dotenv and keeping cycles obvious
		lineNumber++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}

		key, rawValue, err := parseDotEnvAssignment(trimmed, lineNumber)
		if err != nil {
			return nil, err
		}

		value, mode, err := parseDotEnvValue(rawValue, lineNumber)
		if err != nil {
			return nil, err
		}

		finalValue := value
		if mode != quoteModeSingle {
			// single quotes purposely stay literal
			// so a value such as '${TOKEN}' never surprises the reader by expanding
			if expanded, err := expandDotEnvValue(value, values, lineNumber); err != nil {
				return nil, err
			} else {
				finalValue = expanded
			}
		}
		if isWorkspaceKey(key) {
			if workspaceSeen {
				return nil, errdef.New(
					errdef.CodeParse,
					"dotenv line %d: workspace defined multiple times",
					lineNumber,
				)
			}
			workspaceSeen = true
		}
		values[key] = finalValue
	}
	if err := scanner.Err(); err != nil {
		return nil, errdef.Wrap(errdef.CodeFilesystem, err, "read env file %s", path)
	}

	return values, nil
}

func parseDotEnvAssignment(line string, lineNumber int) (string, string, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", "", nil
	}

	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "export ") || strings.HasPrefix(lower, "export\t") {
		trimmed = strings.TrimSpace(trimmed[len("export"):])
	}

	idx := strings.IndexRune(trimmed, '=')
	if idx < 0 {
		return "", "", errdef.New(
			errdef.CodeParse,
			"dotenv line %d: expected KEY=value",
			lineNumber,
		)
	}

	key := strings.TrimSpace(trimmed[:idx])
	if key == "" {
		return "", "", errdef.New(errdef.CodeParse, "dotenv line %d: missing key", lineNumber)
	}

	value := trimmed[idx+1:]
	return key, value, nil
}

func parseDotEnvValue(raw string, lineNumber int) (string, quoteMode, error) {
	leadingTrimmed := strings.TrimLeft(raw, " \t")
	if leadingTrimmed == "" {
		return "", quoteModeNone, nil
	}

	switch leadingTrimmed[0] {
	case '"':
		value, _, err := parseQuotedValue(leadingTrimmed, quoteModeDouble, lineNumber)
		return value, quoteModeDouble, err
	case '\'':
		value, _, err := parseQuotedValue(leadingTrimmed, quoteModeSingle, lineNumber)
		return value, quoteModeSingle, err
	default:
		return stripInlineComment(leadingTrimmed), quoteModeNone, nil
	}
}

func parseQuotedValue(input string, mode quoteMode, lineNumber int) (string, string, error) {
	quote := byte('"')
	if mode == quoteModeSingle {
		quote = '\''
	}

	var b strings.Builder
	for i := 1; i < len(input); i++ {
		ch := input[i]
		if ch == '\\' {
			if i+1 >= len(input) {
				return "", "", errdef.New(
					errdef.CodeParse,
					"dotenv line %d: unfinished escape",
					lineNumber,
				)
			}
			i++
			next := input[i]
			if mode == quoteModeDouble {
				b.WriteByte(resolveDoubleQuoteEscape(next))
			} else {
				b.WriteByte(next)
			}
			continue
		}
		if ch == quote {
			remainder := input[i+1:]
			trimmed := strings.TrimSpace(remainder)
			if trimmed != "" && trimmed[0] != '#' && trimmed[0] != ';' {
				return "", "", errdef.New(
					errdef.CodeParse,
					"dotenv line %d: unexpected content after quoted value",
					lineNumber,
				)
			}
			return b.String(), remainder, nil
		}
		b.WriteByte(ch)
	}
	return "", "", errdef.New(
		errdef.CodeParse,
		"dotenv line %d: unterminated quoted value",
		lineNumber,
	)
}

func stripInlineComment(value string) string {
	inWhitespace := false
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case ' ', '\t':
			inWhitespace = true
		case '#', ';':
			if i == 0 || inWhitespace {
				return strings.TrimSpace(value[:i])
			}
			inWhitespace = false
		default:
			inWhitespace = false
		}
	}
	return strings.TrimSpace(value)
}

func expandDotEnvValue(value string, resolved map[string]string, lineNumber int) (string, error) {
	// single pass keeps evaluation predictable and avoids repeated expansion which could mask typos.
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if ch == '\\' && i+1 < len(value) && value[i+1] == '$' {
			b.WriteByte('$')
			i++
			continue
		}
		if ch != '$' {
			b.WriteByte(ch)
			continue
		}
		if i+1 >= len(value) {
			b.WriteByte(ch)
			continue
		}
		if value[i+1] == '{' {
			end := strings.IndexByte(value[i+2:], '}')
			if end < 0 {
				return "", errdef.New(
					errdef.CodeParse,
					"dotenv line %d: missing closing brace for ${",
					lineNumber,
				)
			}
			end += i + 2
			name := strings.TrimSpace(value[i+2 : end])
			if name == "" {
				return "", errdef.New(
					errdef.CodeParse,
					"dotenv line %d: empty variable name",
					lineNumber,
				)
			}
			replacement, err := resolveDotEnvRef(name, resolved, lineNumber)
			if err != nil {
				return "", err
			}
			b.WriteString(replacement)
			i = end
			continue
		}
		if isDotEnvNameChar(value[i+1]) {
			j := i + 1
			for j < len(value) && isDotEnvNameChar(value[j]) {
				j++
			}
			name := value[i+1 : j]
			replacement, err := resolveDotEnvRef(name, resolved, lineNumber)
			if err != nil {
				return "", err
			}
			b.WriteString(replacement)
			i = j - 1
			continue
		}
		b.WriteByte(ch)
	}
	return b.String(), nil
}

func resolveDotEnvRef(name string, resolved map[string]string, lineNumber int) (string, error) {
	if value, ok := resolved[name]; ok {
		return value, nil
	}
	// allow OS envs fallbacks so sensitive values can stay outside the dotenv file and be passed at launch time
	if envValue, ok := os.LookupEnv(name); ok {
		return envValue, nil
	}
	if envValue, ok := os.LookupEnv(strings.ToUpper(name)); ok {
		return envValue, nil
	}
	return "", errdef.New(
		errdef.CodeParse,
		"dotenv line %d: variable %q is not defined",
		lineNumber,
		name,
	)
}

func isDotEnvNameChar(ch byte) bool {
	if ch >= 'a' && ch <= 'z' {
		return true
	}
	if ch >= 'A' && ch <= 'Z' {
		return true
	}
	if ch >= '0' && ch <= '9' {
		return true
	}
	return ch == '_'
}

func resolveDoubleQuoteEscape(ch byte) byte {
	switch ch {
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	case '0':
		return 0
	case '"':
		return '"'
	case '\\':
		return '\\'
	case 'b':
		return '\b'
	case 'f':
		return '\f'
	default:
		return ch
	}
}

func deriveDotEnvName(values map[string]string, path string) string {
	// favor the workspace key so users can rename environments without touching filenames
	if name := workspaceName(values); name != "" {
		return name
	}

	base := filepath.Base(path)
	lower := strings.ToLower(base)
	switch {
	case lower == ".env":
		return dotEnvDefaultName
	case strings.HasPrefix(lower, ".env.") && len(base) > len(".env."):
		return strings.TrimSpace(base[len(".env."):])
	case strings.HasSuffix(lower, ".env") && len(base) > len(".env"):
		return strings.TrimSpace(base[:len(base)-len(".env")])
	}

	stem := strings.TrimSpace(strings.TrimSuffix(base, filepath.Ext(base)))
	if stem == "" || strings.EqualFold(stem, ".env") {
		return dotEnvDefaultName
	}
	return stem
}

func workspaceName(values map[string]string) string {
	for key, value := range values {
		if isWorkspaceKey(key) {
			trimmed := strings.TrimSpace(value)
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func isWorkspaceKey(key string) bool {
	return strings.EqualFold(strings.TrimSpace(key), "workspace")
}
