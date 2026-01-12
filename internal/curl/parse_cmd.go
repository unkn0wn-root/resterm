package curl

import (
	"strings"
)

func parseCmd(tok []string) (*Cmd, error) {
	idx, ok := findCurlIndex(tok)
	if !ok {
		return nil, errNotCurlCommand
	}

	cmd := &Cmd{}
	seg := Seg{}
	posOnly := false

	for i := idx + 1; i < len(tok); i++ {
		t := tok[i]
		if t == "" {
			continue
		}

		if !posOnly {
			if t == "--" {
				posOnly = true
				continue
			}
			if t == "--next" {
				addSeg(cmd, seg)
				seg = Seg{}
				posOnly = false
				continue
			}
			if strings.HasPrefix(t, "--") {
				it, ok, err := parseLong(t, tok, &i)
				if err != nil {
					return nil, err
				}
				if ok {
					seg.Items = append(seg.Items, it)
				} else {
					seg.Unk = append(seg.Unk, t)
				}
				continue
			}
			if strings.HasPrefix(t, "-") && t != "-" {
				its, unk, ok, err := parseShort(t, tok, &i)
				if err != nil {
					return nil, err
				}
				if ok {
					seg.Items = append(seg.Items, its...)
				}
				if len(unk) > 0 {
					seg.Unk = append(seg.Unk, unk...)
				}
				if ok || len(unk) > 0 {
					continue
				}
			}
		}

		seg.Items = append(seg.Items, Item{Pos: t})
	}

	addSeg(cmd, seg)
	return cmd, nil
}

func addSeg(cmd *Cmd, seg Seg) {
	if len(seg.Items) == 0 && len(seg.Unk) == 0 {
		return
	}
	cmd.Segs = append(cmd.Segs, seg)
}

func parseLong(t string, tok []string, i *int) (Item, bool, error) {
	name, val, hasVal := splitLong(t)
	if name == "" {
		return Item{}, false, nil
	}

	def := longDefs[name]
	if def == nil {
		return Item{}, false, nil
	}

	if def.kind == optVal && !hasVal {
		nv, err := consumeNext(tok, i, "--"+name)
		if err != nil {
			return Item{}, false, err
		}
		val = nv
	}

	return optItem(def.key, val), true, nil
}

func splitLong(t string) (string, string, bool) {
	if !strings.HasPrefix(t, "--") || len(t) < 3 {
		return "", "", false
	}

	raw := t[2:]
	if raw == "" {
		return "", "", false
	}

	if idx := strings.Index(raw, "="); idx >= 0 {
		return raw[:idx], raw[idx+1:], true
	}

	return raw, "", false
}

func parseShort(t string, tok []string, i *int) ([]Item, []string, bool, error) {
	if len(t) < 2 || !strings.HasPrefix(t, "-") {
		return nil, nil, false, nil
	}
	raw := t[1:]
	var its []Item
	var unk []string
	ok := false

	for j := 0; j < len(raw); j++ {
		ch := rune(raw[j])
		def := shortDefs[ch]
		if def == nil {
			unk = append(unk, "-"+string(ch))
			continue
		}

		ok = true
		if def.kind == optNone {
			its = append(its, optItem(def.key, ""))
			continue
		}

		val := ""
		if j+1 < len(raw) {
			val = raw[j+1:]
		} else {
			nv, err := consumeNext(tok, i, "-"+string(ch))
			if err != nil {
				return nil, nil, false, err
			}
			val = nv
		}
		its = append(its, optItem(def.key, val))
		break
	}
	return its, unk, ok, nil
}

func optItem(key, val string) Item {
	return Item{Opt: Opt{Key: key, Val: val}, IsOpt: true}
}
