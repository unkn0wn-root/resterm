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
	tokens, err := splitTokens(command)
	if err != nil {
		return nil, err
	}
	return parseTokens(tokens)
}

func splitTokens(input string) ([]string, error) {
	var args []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		args = append(args, current.String())
		current.Reset()
	}

	for _, r := range input {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\':
			if inSingle {
				current.WriteRune(r)
			} else {
				escaped = true
			}
		case r == '\'':
			if !inDouble {
				if inSingle {
					inSingle = false
				} else {
					inSingle = true
				}
			} else {
				current.WriteRune(r)
			}
		case r == '"':
			if !inSingle {
				if inDouble {
					inDouble = false
				} else {
					inDouble = true
				}
			} else {
				current.WriteRune(r)
			}
		case isWhitespace(r):
			if inSingle || inDouble {
				current.WriteRune(r)
			} else {
				flush()
			}
		default:
			current.WriteRune(r)
		}
	}
	if escaped {
		return nil, fmt.Errorf("unterminated escape sequence")
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quoted string")
	}
	flush()
	return args, nil
}

func parseTokens(tokens []string) (*restfile.Request, error) {
	if len(tokens) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	if stripPromptPrefix(tokens[0]) != "curl" {
		return nil, fmt.Errorf("not a curl command")
	}

	req := &restfile.Request{Method: "GET"}
	headers := make(http.Header)
	var url string
	var dataParts []string
	var basicAuth string
	compressed := false

	for i := 1; i < len(tokens); i++ {
		token := tokens[i]
		switch token {
		case "-X", "--request":
			i++
			if i >= len(tokens) {
				return nil, fmt.Errorf("missing argument for %s", token)
			}
			req.Method = strings.ToUpper(tokens[i])
		case "-H", "--header":
			i++
			if i >= len(tokens) {
				return nil, fmt.Errorf("missing header value for %s", token)
			}
			name, value := splitHeader(tokens[i])
			if name != "" {
				headers.Add(name, value)
			}
		case "-d", "--data", "--data-raw", "--data-binary", "--data-urlencode", "--data-ascii", "--data-json":
			i++
			if i >= len(tokens) {
				return nil, fmt.Errorf("missing data value for %s", token)
			}
			dataParts = append(dataParts, tokens[i])
		case "--json":
			i++
			if i >= len(tokens) {
				return nil, fmt.Errorf("missing value for --json")
			}
			dataParts = append(dataParts, tokens[i])
			if headers.Get("Content-Type") == "" {
				headers.Set("Content-Type", "application/json")
			}
		case "--url":
			i++
			if i >= len(tokens) {
				return nil, fmt.Errorf("missing argument for --url")
			}
			url = tokens[i]
		case "-u", "--user":
			i++
			if i >= len(tokens) {
				return nil, fmt.Errorf("missing credential for %s", token)
			}
			basicAuth = tokens[i]
		case "-I", "--head":
			req.Method = "HEAD"
		case "--compressed":
			compressed = true
		case "-F", "--form":
			i++
			if i >= len(tokens) {
				return nil, fmt.Errorf("missing value for %s", token)
			}
			dataParts = append(dataParts, tokens[i])
			if headers.Get("Content-Type") == "" {
				headers.Set("Content-Type", "multipart/form-data")
			}
		default:
			switch {
			case strings.HasPrefix(token, "-X") && len(token) > 2:
				req.Method = strings.ToUpper(token[2:])
			case strings.HasPrefix(token, "--request="):
				req.Method = strings.ToUpper(token[len("--request="):])
			case strings.HasPrefix(token, "-H") && len(token) > 2:
				name, value := splitHeader(token[2:])
				if name != "" {
					headers.Add(name, value)
				}
			case strings.HasPrefix(token, "--header="):
				name, value := splitHeader(token[len("--header="):])
				if name != "" {
					headers.Add(name, value)
				}
			case strings.HasPrefix(token, "--data="):
				dataParts = append(dataParts, token[len("--data="):])
			case strings.HasPrefix(token, "--data-raw="):
				dataParts = append(dataParts, token[len("--data-raw="):])
			case strings.HasPrefix(token, "--data-binary="):
				dataParts = append(dataParts, token[len("--data-binary="):])
			case strings.HasPrefix(token, "--data-urlencode="):
				dataParts = append(dataParts, token[len("--data-urlencode="):])
			case strings.HasPrefix(token, "--json="):
				value := token[len("--json="):]
				dataParts = append(dataParts, value)
				if headers.Get("Content-Type") == "" {
					headers.Set("Content-Type", "application/json")
				}
			case strings.HasPrefix(token, "--url="):
				url = token[len("--url="):]
			case (strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://")) && url == "":
				url = token
			default:
				if url == "" && !strings.HasPrefix(token, "-") {
					url = token
				}
			}
		}
	}

	if url == "" {
		return nil, fmt.Errorf("curl command missing URL")
	}

	if len(dataParts) > 0 && strings.EqualFold(req.Method, "GET") {
		req.Method = "POST"
	}

	req.URL = sanitizeURL(url)
	if len(headers) > 0 {
		req.Headers = headers
	}

	if basicAuth != "" {
		if req.Headers == nil {
			req.Headers = make(http.Header)
		}
		authValue := buildBasicAuthHeader(basicAuth)
		req.Headers.Set("Authorization", authValue)
	}

	if compressed {
		if req.Headers == nil {
			req.Headers = make(http.Header)
		}
		if req.Headers.Get("Accept-Encoding") == "" {
			req.Headers.Set("Accept-Encoding", "gzip, deflate, br")
		}
	}

	req.Body = buildBodyFromData(dataParts)
	return req, nil
}

func buildBodyFromData(parts []string) restfile.BodySource {
	if len(parts) == 0 {
		return restfile.BodySource{}
	}
	if len(parts) == 1 && strings.HasPrefix(parts[0], "@") && len(parts[0]) > 1 {
		return restfile.BodySource{FilePath: strings.TrimPrefix(parts[0], "@")}
	}
	return restfile.BodySource{Text: strings.Join(parts, "\n")}
}

func buildBasicAuthHeader(creds string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(creds))
	return fmt.Sprintf("Basic %s", encoded)
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
	prefixes := []string{"$", "%", ">", "!"}
	for _, prefix := range prefixes {
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
	return strings.Trim(raw, "\"'")
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
