package intellisense

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
	js "github.com/unkn0wn-root/resterm/internal/parser/javascript"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/scriptapi"
)

func analyzeMember(prior string, cur []rune, from, col int, lang string, writes bool) (Context, bool) {
	if from > col {
		return Context{}, false
	}
	// Include earlier lines to detect comments and template text that span lines.
	code := prior + string(cur[from:col])
	if lang == restfile.ScriptLangRTS {
		code = rts.MaskText(code)
	} else {
		code = js.MaskText(code)
	}
	code = code[len(prior):]

	i := len(code)
	for i > 0 && (code[i-1] == '.' || isMemberChar(rune(code[i-1]))) {
		i--
	}
	path := code[i:]
	dot := strings.LastIndexByte(path, '.')
	if dot < 0 || path[0] == '.' {
		return Context{}, false
	}

	var members []scriptapi.Member
	for _, m := range scriptapi.Members(path[:dot], lang) {
		if writes || !m.Writes {
			members = append(members, m)
		}
	}
	if len(members) == 0 {
		return Context{}, false
	}

	end := col
	for end < len(cur) && isMemberChar(cur[end]) {
		end++
	}
	next := skipSpace(cur, end)
	name := path[dot+1:]
	return Context{
		Kind:    KindMember,
		Query:   name,
		Start:   col - len(name),
		End:     end,
		members: members,
		call:    next < len(cur) && (cur[next] == '(' || cur[next] == '.'),
	}, true
}

// Expressions cannot change variables.
func analyzeExpr(cur []rune, from, col int) (Context, bool) {
	return analyzeMember("", cur, from, col, restfile.ScriptLangRTS, false)
}

func isMemberChar(r rune) bool {
	return r == '$' || directive.IsIdentRune(r)
}
