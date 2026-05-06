package ui

import (
	"context"
	"os"

	"github.com/unkn0wn-root/resterm/internal/prerequest"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/stdlib"
	"github.com/unkn0wn-root/resterm/internal/rtspre"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (m *Model) runRTSPreRequest(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base string,
	vars map[string]string,
	globals map[string]vars.GlobalMutation,
) (prerequest.Output, error) {
	out := prerequest.Output{}
	if req == nil {
		return out, nil
	}
	eng := m.rtsEng
	if eng == nil {
		eng = rts.NewEng(stdlib.New)
		m.rtsEng = eng
	}
	uses := m.rtsUses(doc, req)
	env := m.rtsEnv(envName)
	baseDir := m.rtsBase(doc, base)
	globs := rtspre.RuntimeGlobals(globals, false)
	mut := rtspre.NewMutator(&out, m.rtsReq(req), vars, globs)
	emptyResp := &rts.Resp{}

	err := rtspre.Run(ctx, eng, rtspre.ExecInput{
		Doc:     doc,
		Scripts: req.Metadata.Scripts,
		BaseDir: baseDir,
		BuildRT: func() rts.RT {
			return rts.RT{
				Env:         env,
				Vars:        vars,
				Globals:     globs,
				Resp:        m.rtsLast(),
				Res:         emptyResp,
				Trace:       m.rtsTrace(),
				Req:         mut.Request(),
				ReqMut:      mut,
				VarsMut:     mut,
				GlobalMut:   mut,
				Uses:        uses,
				BaseDir:     baseDir,
				ReadFile:    os.ReadFile,
				AllowRandom: true,
				Site:        "@script pre-request",
			}
		},
	})
	if err != nil {
		return out, err
	}

	prerequest.Normalize(&out)
	return out, nil
}
