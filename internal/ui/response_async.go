package ui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

const (
	respFmtMax = 2
	respRflMax = 2
)

type respTasks struct {
	fmtLim chan struct{}
	rflLim chan struct{}
}

func newRespTasks() *respTasks {
	return &respTasks{
		fmtLim: make(chan struct{}, respFmtMax),
		rflLim: make(chan struct{}, respRflMax),
	}
}

func (m *Model) rt() *respTasks {
	if m.respTasks == nil {
		m.respTasks = newRespTasks()
	}
	return m.respTasks
}

func (t *respTasks) acq(ctx context.Context, lim chan struct{}) bool {
	if lim == nil {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case lim <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (t *respTasks) rel(lim chan struct{}) {
	if lim == nil {
		return
	}
	<-lim
}

func (t *respTasks) fmtSlot(ctx context.Context) bool {
	return t.acq(ctx, t.fmtLim)
}

func (t *respTasks) fmtRel() {
	t.rel(t.fmtLim)
}

func (t *respTasks) rflSlot(ctx context.Context) bool {
	return t.acq(ctx, t.rflLim)
}

func (t *respTasks) rflRel() {
	t.rel(t.rflLim)
}

func (m *Model) respCmd(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	if spin := m.startTabSpin(); spin != nil {
		return tea.Batch(cmd, spin)
	}
	return cmd
}

func (m *Model) respSpinStop() {
	m.stopTabSpinIfIdle()
}

func (m *Model) respFmtCmd(
	ctx context.Context,
	token string,
	resp *httpclient.Response,
	tests []scripts.TestResult,
	scriptErr error,
	width int,
) tea.Cmd {
	if resp == nil {
		return nil
	}

	rc := cloneHTTPResponse(resp)
	tc := append([]scripts.TestResult(nil), tests...)

	if width <= 0 {
		width = defaultResponseViewportWidth
	}

	w := width
	hdrs := cloneHeaders(rc.Headers)
	url := strings.TrimSpace(rc.EffectiveURL)
	rt := m.rt()

	return func() tea.Msg {
		if !rt.fmtSlot(ctx) {
			return nil
		}
		defer rt.fmtRel()

		if ctx != nil && ctx.Err() != nil {
			return nil
		}
		views := buildHTTPResponseViewsCtx(ctx, rc, tc, scriptErr)
		if ctx != nil && ctx.Err() != nil {
			return nil
		}
		return responseRenderedMsg{
			token:          token,
			pretty:         views.pretty,
			raw:            views.raw,
			rawSummary:     views.rawSummary,
			headers:        views.headers,
			requestHeaders: buildHTTPRequestHeadersView(rc),
			width:          w,
			body:           append([]byte(nil), rc.Body...),
			meta:           views.meta,
			contentType:    views.contentType,
			rawText:        views.rawText,
			rawHex:         views.rawHex,
			rawBase64:      views.rawBase64,
			rawMode:        views.rawMode,
			headersMap:     hdrs,
			effectiveURL:   url,
		}
	}
}

func (m *Model) respRflCmd(req responseReflowReq) tea.Cmd {
	return func() tea.Msg {
		rt := m.rt()
		ctx := req.ctx
		if !rt.rflSlot(ctx) {
			return nil
		}
		defer rt.rflRel()

		if ctxDone(ctx) {
			return nil
		}
		cache, ok := wrapCacheCtx(ctx, req.tab, req.content, req.width)
		if !ok || ctxDone(ctx) {
			return nil
		}
		return responseReflowDoneMsg{req: req, cache: cache}
	}
}
