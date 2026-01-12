package curl

import (
	"encoding/base64"
	"fmt"
	"net/http"
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

func sanitizeURL(raw string) string {
	return strings.Trim(raw, urlQuoteChars)
}
