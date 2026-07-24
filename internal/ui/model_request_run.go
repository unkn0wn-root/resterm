package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	rqeng "github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (m *Model) executeRequest(
	doc *restfile.Document,
	req *restfile.Request,
	opts httpclient.Options,
	env string,
	vals map[string]rts.Value,
	extras ...map[string]string,
) tea.Cmd {
	return m.executeWith(
		m.latencySeries.generation(),
		rqeng.ExecModeSend,
		doc,
		req,
		opts,
		env,
		vals,
		extras...,
	)
}

func (m *Model) executeExplain(
	doc *restfile.Document,
	req *restfile.Request,
	opts httpclient.Options,
	env string,
	vals map[string]rts.Value,
	extras ...map[string]string,
) tea.Cmd {
	return m.executeWith(
		m.latencySeries.generation(),
		rqeng.ExecModePreview,
		doc,
		req,
		opts,
		env,
		vals,
		extras...,
	)
}

// Preview bypasses the UI wrapper because it must not attach live streams to the model.
func (m *Model) executeWith(
	gen int,
	mode rqeng.ExecMode,
	doc *restfile.Document,
	req *restfile.Request,
	opts httpclient.Options,
	env string,
	vals map[string]rts.Value,
	extras ...map[string]string,
) tea.Cmd {
	if err := docErr(doc); err != nil {
		return responseErrCmd(err, req, env)
	}

	env = vars.SelectEnv(m.cfg.EnvironmentSet, env, m.cfg.EnvironmentName)
	svc := m.requestSvc(opts)
	if svc == nil {
		return nil
	}
	var rq requestRunner = svc
	if mode == rqeng.ExecModeSend {
		rq = &uiRequestEngine{Engine: svc, model: m}
	}
	extra := mergeRunExtras(extras...)
	return m.runMsg(func(ctx context.Context) tea.Msg {
		res, err := rq.ExecuteWith(doc, req, env, rqeng.ExecOptions{
			Extra:  extra,
			Values: copyRunValues(vals),
			Ctx:    ctx,
			Mode:   mode,
		})
		if err != nil {
			return responseMsg{
				err:         err,
				executed:    req.Clone(),
				environment: env,
			}
		}
		msg := m.responseMsgFromRunState(res, false)
		msg.latGen = gen
		return msg
	})
}

func responseErrCmd(err error, req *restfile.Request, env string) tea.Cmd {
	return func() tea.Msg {
		return responseMsg{
			err:         err,
			executed:    req.Clone(),
			environment: env,
		}
	}
}
