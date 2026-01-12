package curl

type tokenOptions struct {
	decodeANSI            bool
	allowLineContinuation bool
}

type tokenStep struct {
	emit    bool
	r       rune
	handled bool
}

type TokenState struct {
	inSingle bool
	inDouble bool
	inANSI   bool
	escape   bool
	skipLF   bool
}

func (s *TokenState) Open() bool {
	return s.inSingle || s.inDouble || s.inANSI
}

func (s *TokenState) InQuote() bool {
	return s.inSingle || s.inDouble
}

func (s *TokenState) Escaping() bool {
	return s.escape
}

func (s *TokenState) ResetEscape() {
	s.escape = false
	s.skipLF = false
}

func (s *TokenState) advance(rs []rune, i *int, opts tokenOptions) (tokenStep, error) {
	r := rs[*i]

	if s.skipLF {
		s.skipLF = false
		if r == '\n' {
			return tokenStep{handled: true}, nil
		}
	}

	if s.escape {
		s.escape = false
		if s.inANSI {
			if opts.decodeANSI {
				val, err := ansiEsc(rs, i)
				if err != nil {
					return tokenStep{}, err
				}
				return tokenStep{emit: true, r: val, handled: true}, nil
			}
			return tokenStep{emit: true, r: r, handled: true}, nil
		}
		if opts.allowLineContinuation && isLineBreak(r) {
			if r == '\r' {
				s.skipLF = true
			}
			return tokenStep{handled: true}, nil
		}
		return tokenStep{emit: true, r: r, handled: true}, nil
	}

	if s.inANSI {
		switch r {
		case '\\':
			s.escape = true
			return tokenStep{handled: true}, nil
		case '\'':
			s.inANSI = false
			return tokenStep{handled: true}, nil
		default:
			return tokenStep{emit: true, r: r, handled: true}, nil
		}
	}

	if r == '\\' {
		if s.inSingle {
			return tokenStep{emit: true, r: r, handled: true}, nil
		}
		s.escape = true
		return tokenStep{handled: true}, nil
	}

	if r == '\'' {
		if !s.inDouble {
			s.inSingle = !s.inSingle
			return tokenStep{handled: true}, nil
		}
		return tokenStep{emit: true, r: r, handled: true}, nil
	}

	if r == '"' {
		if !s.inSingle {
			s.inDouble = !s.inDouble
			return tokenStep{handled: true}, nil
		}
		return tokenStep{emit: true, r: r, handled: true}, nil
	}

	if !s.inSingle && !s.inDouble && r == '$' && *i+1 < len(rs) && rs[*i+1] == '\'' {
		s.inANSI = true
		*i++
		return tokenStep{handled: true}, nil
	}

	return tokenStep{}, nil
}
