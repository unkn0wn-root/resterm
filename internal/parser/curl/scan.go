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
	line = stripPromptLine(line)
	line = stripCurlPrefixes(line)
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "curl ") || line == "curl"
}

type scanState struct {
	inS  bool
	inD  bool
	ansi bool
	esc  bool
}

func (s *scanState) consume(v string) {
	rs := []rune(v)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if s.esc {
			s.esc = false
			continue
		}
		if s.ansi {
			switch r {
			case '\\':
				s.esc = true
			case '\'':
				s.ansi = false
			}
			continue
		}
		if r == '\\' {
			if s.inS {
				continue
			}
			s.esc = true
			continue
		}
		if r == '\'' {
			if !s.inD {
				s.inS = !s.inS
			}
			continue
		}
		if r == '"' {
			if !s.inS {
				s.inD = !s.inD
			}
			continue
		}
		if !s.inS && !s.inD && r == '$' && i+1 < len(rs) && rs[i+1] == '\'' {
			s.ansi = true
			i++
			continue
		}
	}
}

func (s *scanState) open() bool {
	return s.inS || s.inD || s.ansi
}

func (s *scanState) resetEsc() {
	s.esc = false
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

func stripPromptLine(line string) string {
	line = strings.TrimSpace(line)
	for _, prefix := range []string{"$", "%", ">", "!"} {
		if strings.HasPrefix(line, prefix) {
			line = strings.TrimSpace(line[len(prefix):])
		}
	}
	return line
}

func stripCurlPrefixes(line string) string {
	line = strings.TrimSpace(line)
	for {
		tok, rest := nextTok(line)
		if tok == "" {
			return ""
		}
		switch strings.ToLower(tok) {
		case "sudo", "command", "time", "noglob":
			// skip wrapper flags/args so copied shell commands still resolve to the curl token.
			line = stripPrefix(tok, rest)
			continue
		case "env":
			// consume env exports/options first so we return the actual curl command.
			line = stripEnv(rest)
			continue
		default:
			return line
		}
	}
}

func stripEnv(line string) string {
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
			line = skipOpt(rest, tok, envOptArg(tok))
			continue
		}
		if isAssign(tok) {
			line = rest
			continue
		}
		return line
	}
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
	inS := false
	inD := false
	ansi := false
	esc := false
	i := 0
	for i < len(rs) {
		r := rs[i]
		if esc {
			b.WriteRune(r)
			esc = false
			i++
			continue
		}
		if ansi {
			switch r {
			case '\\':
				esc = true
			case '\'':
				ansi = false
			default:
				b.WriteRune(r)
			}
			i++
			continue
		}
		if r == '\\' {
			if inS {
				b.WriteRune(r)
			} else {
				esc = true
			}
			i++
			continue
		}
		if r == '\'' {
			if !inD {
				inS = !inS
			} else {
				b.WriteRune(r)
			}
			i++
			continue
		}
		if r == '"' {
			if !inS {
				inD = !inD
			} else {
				b.WriteRune(r)
			}
			i++
			continue
		}
		if !inS && !inD && r == '$' && i+1 < len(rs) && rs[i+1] == '\'' {
			ansi = true
			i += 2
			continue
		}
		if !inS && !inD && isWhitespace(r) {
			break
		}
		b.WriteRune(r)
		i++
	}
	return b.String(), strings.TrimLeft(string(rs[i:]), " \t")
}

func stripPrefix(tok, rest string) string {
	switch strings.ToLower(tok) {
	case "sudo":
		return stripSudo(rest)
	case "command":
		return stripCommand(rest)
	case "time":
		return stripTime(rest)
	case "noglob":
		return strings.TrimSpace(rest)
	default:
		return strings.TrimSpace(rest)
	}
}

func stripSudo(line string) string {
	line = strings.TrimSpace(line)
	for {
		tok, rest := nextTok(line)
		if tok == "" {
			return ""
		}
		if tok == "--" {
			return strings.TrimSpace(rest)
		}
		if !strings.HasPrefix(tok, "-") {
			return line
		}
		line = skipOpt(rest, tok, sudoOptArg(tok))
	}
}

func stripCommand(line string) string {
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
			line = rest
			continue
		}
		return line
	}
}

func stripTime(line string) string {
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
			line = skipOpt(rest, tok, timeOptArg(tok))
			continue
		}
		return line
	}
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
			return len(tok) == 2
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
			return len(tok) == 2
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
			return len(tok) == 2
		default:
			return false
		}
	}
	return false
}
