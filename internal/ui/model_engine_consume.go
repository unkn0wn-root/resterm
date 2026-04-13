package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

func (m *Model) handleRunReqMsg(msg runReqMsg) tea.Cmd {
	if msg.err != nil {
		return m.handleRunErr(msg.err)
	}
	if msg.res.Workflow != nil || msg.res.Compare != nil || msg.res.Profile != nil {
		return m.handleRunErr(
			errdef.New(errdef.CodeUI, "unexpected aggregate run result on request path"),
		)
	}
	return m.handleResponseMessage(m.responseMsgFromRun(msg.res))
}

func (m *Model) handleRunErr(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	m.lastError = err
	m.setStatusMessage(statusMsg{text: err.Error(), level: statusError})
	return nil
}

func (m *Model) responseMsgFromRun(res engine.RequestResult) responseMsg {
	return m.responseMsgFromRunState(res, true)
}

func (m *Model) responseMsgFromRunState(res engine.RequestResult, done bool) responseMsg {
	return responseMsg{
		response:       res.Response,
		grpc:           res.GRPC,
		stream:         res.Stream,
		transcript:     append([]byte(nil), res.Transcript...),
		err:            res.Err,
		tests:          append([]scripts.TestResult(nil), res.Tests...),
		scriptErr:      res.ScriptErr,
		executed:       res.Executed,
		requestText:    res.RequestText,
		runtimeSecrets: append([]string(nil), res.RuntimeSecrets...),
		environment:    res.Environment,
		skipped:        res.Skipped,
		skipReason:     res.SkipReason,
		preview:        res.Preview,
		explain:        res.Explain,
		historyDone:    done,
	}
}

func (m *Model) applyRunSnapshot(
	sn *responseSnapshot,
	hr *httpclient.Response,
	gr *grpcclient.Response,
) {
	if sn == nil {
		return
	}
	if m.responseLatest != nil && m.responseLatest.ready {
		m.responsePrevious = m.responseLatest
	}
	m.setResponseSnapshotContent(sn)
	m.lastResponse = hr
	m.lastGRPC = gr
}

func (m *Model) syncRecordedHistory() {
	m.syncHistory()
	m.selectNewestHistoryEntry()
}

func newTextSnapshot(body, env string) *responseSnapshot {
	body = strings.TrimSpace(body)
	if body == "" {
		body = noResponseMessage
	}
	return &responseSnapshot{
		id:             nextResponseRenderToken(),
		pretty:         body,
		raw:            body,
		headers:        body,
		requestHeaders: body,
		ready:          true,
		environment:    strings.TrimSpace(env),
	}
}

func newStreamSnapshot(
	info *scripts.StreamInfo,
	raw []byte,
	env string,
) *responseSnapshot {
	body := strings.TrimSpace(string(raw))
	if body == "" && info != nil {
		body = streamSummaryText(info)
	}
	if body == "" {
		body = "<no transcript captured>"
	}
	return newTextSnapshot(body, env)
}

func streamSummaryText(info *scripts.StreamInfo) string {
	if info == nil {
		return ""
	}
	lines := []string{}
	if kind := strings.TrimSpace(info.Kind); kind != "" {
		lines = append(lines, "Stream: "+kind)
	}
	for k, v := range info.Summary {
		lines = append(lines, fmt.Sprintf("%s: %v", k, v))
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}
