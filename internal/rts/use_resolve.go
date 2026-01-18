package rts

import "strings"

func (e *Eng) resolveUses(cx *Ctx, rt RT, pre map[string]Value, pos Pos) ([]Use, error) {
	if len(rt.Uses) == 0 {
		return nil, nil
	}
	out := make([]Use, 0, len(rt.Uses))
	seen := make(map[string]struct{}, len(pre)+len(rt.Uses))
	for k := range pre {
		seen[k] = struct{}{}
	}
	for _, u := range rt.Uses {
		u, ok := normUse(u)
		if !ok {
			continue
		}
		al := u.Alias
		if al == "" {
			nm, mp, err := e.modHead(rt.BaseDir, u.Path)
			if err != nil {
				return nil, err
			}
			if nm == "" {
				p := mp
				if p.Line == 0 {
					p = Pos{Path: mp.Path, Line: 1, Col: 1}
				}
				return nil, rtErr(cx, p, "missing module name (add 'module <name>' at top of file)")
			}
			al = nm
			u.Alias = nm
		}
		if _, ok := seen[al]; ok {
			return nil, rtErr(cx, pos, "alias already defined: %s", al)
		}
		seen[al] = struct{}{}
		out = append(out, u)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func normUse(u Use) (Use, bool) {
	p := strings.TrimSpace(u.Path)
	if p == "" {
		return Use{}, false
	}
	a := strings.TrimSpace(u.Alias)
	u.Path = p
	u.Alias = a
	return u, true
}

func (e *Eng) modHead(base, path string) (string, Pos, error) {
	p, err := absPath(base, path)
	if err != nil {
		return "", Pos{}, err
	}
	fp, err := e.C.stat(p)
	if err != nil {
		return "", Pos{Path: p}, err
	}
	if cp := e.C.get(p, fp); cp != nil {
		mp := cp.Mod.NamePos
		if mp.Path == "" {
			mp.Path = p
		}
		return cp.Mod.Name, mp, nil
	}
	data, err := e.C.fs.ReadFile(p)
	if err != nil {
		return "", Pos{Path: p}, err
	}
	nm, mp, err := modHead(p, data)
	if mp.Path == "" {
		mp.Path = p
	}
	return nm, mp, err
}

func modHead(path string, src []byte) (string, Pos, error) {
	lx := NewLexer(path, src)
	for {
		tok := lx.Next()
		switch tok.K {
		case SEMI, AUTO_SEMI:
			continue
		case EOF:
			return "", Pos{Path: path}, nil
		case ILLEGAL:
			return "", tok.P, &ParseError{Pos: tok.P, Msg: tok.Lit}
		case KW_MODULE:
			nt := lx.Next()
			if nt.K == ILLEGAL {
				return "", nt.P, &ParseError{Pos: nt.P, Msg: nt.Lit}
			}
			if nt.K != IDENT {
				return "", nt.P, &ParseError{Pos: nt.P, Msg: "module requires a name"}
			}
			return nt.Lit, tok.P, nil
		default:
			mp, ok, err := scanMod(lx)
			if err != nil {
				return "", mp, err
			}
			if ok {
				return "", mp, &ParseError{Pos: mp, Msg: "module must appear before statements"}
			}
			return "", Pos{Path: path}, nil
		}
	}
}

func scanMod(lx *Lexer) (Pos, bool, error) {
	for {
		tok := lx.Next()
		switch tok.K {
		case SEMI, AUTO_SEMI:
			continue
		case EOF:
			return Pos{}, false, nil
		case ILLEGAL:
			return tok.P, false, &ParseError{Pos: tok.P, Msg: tok.Lit}
		case KW_MODULE:
			return tok.P, true, nil
		}
	}
}
