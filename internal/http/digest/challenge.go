package digest

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/http/header"
)

type challenge struct {
	scheme string
	params map[string]string
}

// Commas separate both challenges and parameters. An item starting with
// "name=" belongs to the previous challenge; quoted commas stay in the value.
func parseChallenges(value string) []challenge {
	var items []string
	start, quoted := 0, false
	for i := 0; i <= len(value); i++ {
		switch {
		case i == len(value) || value[i] == ',' && !quoted:
			if s := strings.TrimSpace(value[start:i]); s != "" {
				items = append(items, s)
			}
			start = i + 1
		case value[i] == '\\' && quoted && i+1 < len(value):
			i++
		case value[i] == '"':
			quoted = !quoted
		}
	}

	var out []challenge
	for _, item := range items {
		i := strings.IndexAny(item, " \t=")
		if i >= 0 && strings.HasPrefix(strings.TrimLeft(item[i:], " \t"), "=") {
			if len(out) > 0 {
				addParam(out[len(out)-1].params, item)
			}
			continue
		}
		ch := challenge{scheme: item, params: map[string]string{}}
		if i >= 0 {
			ch.scheme = item[:i]
			addParam(ch.params, strings.TrimSpace(item[i:]))
		}
		out = append(out, ch)
	}
	return out
}

func addParam(params map[string]string, item string) {
	name, value, ok := strings.Cut(item, "=")
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	if !ok || !header.Valid(name) || value == "" || value[0] == '=' {
		return
	}
	if value[0] == '"' {
		if len(value) < 2 || value[len(value)-1] != '"' {
			return
		}
		var b strings.Builder
		for i := 1; i < len(value)-1; i++ {
			if value[i] == '\\' && i+1 < len(value)-1 {
				i++
			}
			b.WriteByte(value[i])
		}
		value = b.String()
	}
	params[strings.ToLower(name)] = value
}
