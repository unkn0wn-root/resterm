package rts

import (
	"fmt"
	"strings"
)

func (e *Eng) resolveUses(cx *Ctx, cfg EvalConfig, pre map[string]Value) ([]Use, error) {
	if len(cfg.Uses) == 0 {
		return nil, nil
	}
	out := make([]Use, 0, len(cfg.Uses))
	seen := make(map[string]struct{}, len(pre)+len(cfg.Uses))
	for k := range pre {
		seen[k] = struct{}{}
	}
	for _, u := range cfg.Uses {
		u, ok := normUse(u)
		if !ok {
			continue
		}
		al := u.Alias
		if al == "" {
			nm, mp, err := e.modHead(cfg.BaseDir, u.Path)
			if err != nil {
				return nil, err
			}
			if nm == "" {
				return nil, Errf(cx, mp, "missing module name (add 'module <name>' at top of file)")
			}
			al = nm
			u.Alias = nm
		}
		if IsKeyword(al) {
			return nil, Errf(cx, u.Pos, "alias is a reserved word: %s", al)
		}
		if _, ok := seen[al]; ok {
			return nil, Errf(cx, u.Pos, "alias already defined: %s", al)
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
		return "", Pos{}, err
	}
	if ent, ok := e.C.get(p, fp); ok {
		m := ent.comp.Mod
		if m.Name == "" {
			return "", m.Start, nil
		}
		return m.Name, m.NamePos, nil
	}
	data, err := e.C.fs.ReadFile(p)
	if err != nil {
		return "", Pos{}, err
	}
	return modHead(NewFile(p, data))
}

// modHead returns the module name declared at the top of f and where it is
// declared. A file without a declaration reports its first line, where the
// declaration belongs.
func modHead(f *File) (string, Pos, error) {
	lx := newLexer(f)
	for {
		tok := lx.scan()
		switch tok.K {
		case SEMI, AUTO_SEMI:
			continue
		case EOF:
			return "", f.Start(), nil
		case ILLEGAL:
			return "", tok.P, &ParseError{Pos: tok.P, Msg: tok.Lit}
		case KW_MODULE:
			nt := lx.scan()
			if nt.K == ILLEGAL {
				return "", nt.P, &ParseError{Pos: nt.P, Msg: nt.Lit}
			}
			if nt.K.isKeyword() {
				return "", nt.P, &ParseError{
					Pos: nt.P,
					Msg: fmt.Sprintf("module name cannot be the reserved word %s", nt.K),
				}
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
			return "", f.Start(), nil
		}
	}
}

func scanMod(lx *lexer) (Pos, bool, error) {
	for {
		tok := lx.scan()
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
