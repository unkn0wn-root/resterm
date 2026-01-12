package curl

import (
	"strings"
)

func SplitCommands(src string) []string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		if line == "" || !IsStartLine(line) {
			i++
			continue
		}
		_, e, cmd := ExtractCommand(lines, i)
		if cmd != "" {
			out = append(out, cmd)
		}
		if e <= i {
			i++
		} else {
			i = e + 1
		}
	}
	return out
}

func ExtractCommand(lines []string, cursor int) (start int, end int, cmd string) {
	start = -1
	for i := cursor; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			if i == cursor {
				continue
			}
			break
		}
		if IsStartLine(trimmed) {
			start = i
			break
		}
	}
	if start == -1 {
		return -1, -1, ""
	}

	st := &scanState{}
	var b strings.Builder
	end = start
	for i := start; i < len(lines); i++ {
		line := lines[i]
		openBefore := st.open()
		if strings.TrimSpace(line) == "" && i > start && !openBefore {
			break
		}

		seg := line
		if !openBefore {
			seg = strings.TrimLeft(seg, " \t")
		}
		if !openBefore {
			seg = strings.TrimRight(seg, " \t")
		}

		cont := lineContinues(seg)
		if cont {
			seg = seg[:len(seg)-1]
		}

		if b.Len() > 0 {
			if openBefore {
				b.WriteByte('\n')
			} else {
				b.WriteByte(' ')
			}
		}

		b.WriteString(seg)
		st.consume(seg)
		end = i
		if cont {
			st.resetEsc()
			continue
		}
		if st.open() {
			continue
		}
		break
	}

	return start, end, strings.TrimSpace(b.String())
}

func IsStartLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	line = stripPromptPrefix(line)
	line = stripCurlPrefixes(line)
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, cmdCurl+" ") || line == cmdCurl
}

type scanState struct {
	token TokenState
}

func (s *scanState) consume(v string) {
	rs := []rune(v)
	for i := 0; i < len(rs); i++ {
		_, _ = s.token.advance(rs, &i, tokenOptions{})
	}
}

func (s *scanState) open() bool {
	return s.token.Open()
}

func (s *scanState) resetEsc() {
	s.token.ResetEscape()
}

func lineContinues(v string) bool {
	if v == "" {
		return false
	}
	count := 0
	for i := len(v) - 1; i >= 0 && v[i] == '\\'; i-- {
		count++
	}
	return count%2 == 1
}

func stripCurlPrefixes(line string) string {
	line = strings.TrimSpace(line)
	for {
		tok, rest := nextTok(line)
		if tok == "" {
			return ""
		}
		switch strings.ToLower(tok) {
		case cmdSudo, cmdCommand, cmdTime, cmdNoGlob:
			// skip wrapper flags/args so copied shell commands still resolve to the curl token.
			line = stripPrefix(tok, rest)
			continue
		case cmdEnv:
			// consume env exports/options first so we return the actual curl command.
			line = stripEnv(rest)
			continue
		default:
			return line
		}
	}
}

func stripEnv(line string) string {
	return stripWithRule(line, stripRule{
		optArg:     envOptArg,
		skipAssign: true,
	})
}

func isAssign(tok string) bool {
	if tok == "" {
		return false
	}
	if strings.HasPrefix(tok, "=") {
		return false
	}
	return strings.Contains(tok, "=")
}

func nextTok(line string) (string, string) {
	line = strings.TrimLeft(line, " \t")
	if line == "" {
		return "", ""
	}
	// parse a single shell style token to handle quoted prefix args without splitting on spaces.
	rs := []rune(line)
	var b strings.Builder
	var st TokenState
	i := 0
	for i < len(rs) {
		r := rs[i]
		step, _ := st.advance(rs, &i, tokenOptions{})
		if step.handled {
			if step.emit {
				b.WriteRune(step.r)
			}
			i++
			continue
		}
		if !st.InQuote() && isWhitespace(r) {
			break
		}
		b.WriteRune(r)
		i++
	}
	return b.String(), strings.TrimLeft(string(rs[i:]), " \t")
}

func stripPrefix(tok, rest string) string {
	switch strings.ToLower(tok) {
	case cmdSudo:
		return stripSudo(rest)
	case cmdCommand:
		return stripCommand(rest)
	case cmdTime:
		return stripTime(rest)
	case cmdNoGlob:
		return strings.TrimSpace(rest)
	default:
		return strings.TrimSpace(rest)
	}
}

type stripRule struct {
	optArg     func(string) bool
	skipAssign bool
}

func stripWithRule(line string, rule stripRule) string {
	line = strings.TrimSpace(line)
	for {
		tok, rest := nextTok(line)
		if tok == "" {
			return ""
		}
		if tok == "--" {
			return strings.TrimSpace(rest)
		}
		if strings.HasPrefix(tok, "-") {
			need := false
			if rule.optArg != nil {
				need = rule.optArg(tok)
			}
			line = skipOpt(rest, tok, need)
			continue
		}
		if rule.skipAssign && isAssign(tok) {
			line = rest
			continue
		}
		return line
	}
}

func stripSudo(line string) string {
	return stripWithRule(line, stripRule{optArg: sudoOptArg})
}

func stripCommand(line string) string {
	return stripWithRule(line, stripRule{})
}

func stripTime(line string) string {
	return stripWithRule(line, stripRule{optArg: timeOptArg})
}

func skipOpt(rest, tok string, need bool) string {
	if !need || hasOptVal(tok) {
		return rest
	}
	_, rest = nextTok(rest)
	return rest
}

func hasOptVal(tok string) bool {
	if strings.HasPrefix(tok, "--") {
		return strings.Contains(tok, "=")
	}
	if len(tok) >= 3 && strings.HasPrefix(tok, "-") {
		return true
	}
	return false
}

func sudoOptArg(tok string) bool {
	if strings.HasPrefix(tok, "--") {
		name, _, ok := strings.Cut(tok, "=")
		if ok {
			return false
		}
		switch name {
		case "--user",
			"--group",
			"--host",
			"--prompt",
			"--close-from",
			"--command",
			"--chdir",
			"--login-class":
			return true
		default:
			return false
		}
	}
	if strings.HasPrefix(tok, "-") && len(tok) >= 2 {
		switch tok[1] {
		case 'u', 'g', 'h', 'p', 'C', 'c', 'U':
			return len(tok) == shortOptTokenLen
		default:
			return false
		}
	}
	return false
}

func envOptArg(tok string) bool {
	if strings.HasPrefix(tok, "--") {
		name, _, ok := strings.Cut(tok, "=")
		if ok {
			return false
		}
		switch name {
		case "--unset", "--chdir":
			return true
		default:
			return false
		}
	}
	if strings.HasPrefix(tok, "-") && len(tok) >= 2 {
		switch tok[1] {
		case 'u', 'C':
			return len(tok) == shortOptTokenLen
		default:
			return false
		}
	}
	return false
}

func timeOptArg(tok string) bool {
	if strings.HasPrefix(tok, "--") {
		name, _, ok := strings.Cut(tok, "=")
		if ok {
			return false
		}
		switch name {
		case "--format", "--output":
			return true
		default:
			return false
		}
	}
	if strings.HasPrefix(tok, "-") && len(tok) >= 2 {
		switch tok[1] {
		case 'f', 'o':
			return len(tok) == shortOptTokenLen
		default:
			return false
		}
	}
	return false
}
