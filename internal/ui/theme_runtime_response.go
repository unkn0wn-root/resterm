package ui

import (
	"context"
	"net/http"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

type responseRenderSource struct {
	http       *httpclient.Response
	grpc       *grpcclient.Response
	grpcMethod string
	tests      []scripts.TestResult
	scriptErr  error
}

func newHTTPResponseRenderSource(
	resp *httpclient.Response,
	tests []scripts.TestResult,
	scriptErr error,
) responseRenderSource {
	return responseRenderSource{
		http:      cloneHTTPResponse(resp),
		tests:     append([]scripts.TestResult(nil), tests...),
		scriptErr: scriptErr,
	}
}

func newGRPCResponseRenderSource(
	resp *grpcclient.Response,
	fullMethod string,
) responseRenderSource {
	return responseRenderSource{
		grpc:       cloneGRPCResponse(resp),
		grpcMethod: strings.TrimSpace(fullMethod),
	}
}

func (src responseRenderSource) hasHTTP() bool {
	return src.http != nil
}

func (src responseRenderSource) hasGRPC() bool {
	return src.grpc != nil
}

func (m *Model) collectResponseSnapshots() []*responseSnapshot {
	seen := make(map[*responseSnapshot]struct{})
	var snapshots []*responseSnapshot
	add := func(snapshot *responseSnapshot) {
		if snapshot == nil {
			return
		}
		if _, ok := seen[snapshot]; ok {
			return
		}
		seen[snapshot] = struct{}{}
		snapshots = append(snapshots, snapshot)
	}

	add(m.responseLatest)
	add(m.responsePrevious)
	add(m.responsePending)
	for _, snapshot := range m.responseTokens {
		add(snapshot)
	}
	for _, snapshot := range m.compareSnapshots {
		add(snapshot)
	}
	for i := range m.responsePanes {
		add(m.responsePanes[i].snapshot)
	}
	return snapshots
}

func (m *Model) rerenderThemedSnapshot(snapshot *responseSnapshot, renderer responseRenderer) {
	if snapshot == nil || !snapshot.ready {
		return
	}
	switch {
	case snapshot.source.hasHTTP():
		m.rerenderHTTPResponseSnapshot(snapshot, renderer)
	case snapshot.source.hasGRPC():
		m.rerenderGRPCResponseSnapshot(snapshot, renderer)
	}
}

func (m *Model) rerenderHTTPResponseSnapshot(
	snapshot *responseSnapshot,
	renderer responseRenderer,
) {
	if snapshot == nil || snapshot.source.http == nil {
		return
	}
	resp := snapshot.source.http
	views := renderer.buildHTTPResponseViews(resp, snapshot.source.tests, snapshot.source.scriptErr)
	snapshot.pretty = views.pretty
	snapshot.raw = views.raw
	snapshot.rawSummary = views.rawSummary
	snapshot.headers = views.headers
	snapshot.requestHeaders = renderer.buildHTTPRequestHeadersView(resp)
	snapshot.body = append([]byte(nil), resp.Body...)
	snapshot.bodyMeta = views.meta
	snapshot.contentType = views.contentType
	snapshot.rawText = views.rawText
	snapshot.rawHex = views.rawHex
	snapshot.rawBase64 = views.rawBase64
	if views.rawMode != 0 {
		snapshot.rawMode = views.rawMode
	} else {
		snapshot.rawMode = rawViewText
	}
	snapshot.responseHeaders = cloneHeaders(resp.Headers)
	snapshot.effectiveURL = strings.TrimSpace(resp.EffectiveURL)
	applyRawViewMode(snapshot, snapshot.rawMode)
}

func (m *Model) rerenderGRPCResponseSnapshot(
	snapshot *responseSnapshot,
	renderer responseRenderer,
) {
	if snapshot == nil || snapshot.source.grpc == nil {
		return
	}
	resp := snapshot.source.grpc
	views := renderer.buildGRPCResponseViews(resp, snapshot.source.grpcMethod)
	snapshot.pretty = views.pretty
	snapshot.raw = views.raw
	snapshot.rawSummary = views.rawSummary
	snapshot.headers = views.headers
	snapshot.body = append([]byte(nil), resp.Wire...)
	if len(snapshot.body) == 0 {
		snapshot.body = append([]byte(nil), resp.Body...)
	}
	snapshot.bodyMeta = views.meta
	snapshot.contentType = views.contentType
	snapshot.rawText = views.rawText
	snapshot.rawHex = views.rawHex
	snapshot.rawBase64 = views.rawBase64
	if views.rawMode != 0 {
		snapshot.rawMode = views.rawMode
	} else {
		snapshot.rawMode = rawViewText
	}
	snapshot.responseHeaders = grpcResponseHeaderMap(resp)
	applyRawViewMode(snapshot, snapshot.rawMode)
}

func grpcResponseHeaderMap(resp *grpcclient.Response) http.Header {
	if resp == nil || (len(resp.Headers) == 0 && len(resp.Trailers) == 0) {
		return nil
	}
	h := make(http.Header, len(resp.Headers)+len(resp.Trailers))
	for key, values := range resp.Headers {
		h[key] = append([]string(nil), values...)
	}
	for key, values := range resp.Trailers {
		h["Grpc-Trailer-"+key] = append([]string(nil), values...)
	}
	return h
}

func (m *Model) restartPendingResponseRender() tea.Cmd {
	pending := m.responsePending
	if pending == nil || pending.ready || !pending.source.hasHTTP() {
		return nil
	}

	m.abortResponseFormatting()

	// Always issue a fresh render token when restarting canceled work.
	// The snapshot identity can stay stable; the async task identity must not.
	token := nextResponseRenderToken()
	m.responsePending = pending
	m.responseLatest = pending
	if m.responseTokens == nil {
		m.responseTokens = make(map[string]*responseSnapshot)
	}
	m.responseTokens[token] = pending
	m.responseRenderToken = token
	m.responseLoading = true
	m.responseLoadingFrame = 0

	width := defaultResponseViewportWidth
	if pane := m.pane(responsePanePrimary); pane != nil && pane.viewport.Width > 0 {
		width = pane.viewport.Width
	}

	formatCtx, cancel := context.WithCancel(context.Background())
	m.responseRenderCancel = cancel
	return m.respFmtCmd(
		formatCtx,
		token,
		pending.source.http,
		pending.source.tests,
		pending.source.scriptErr,
		width,
	)
}

func (m *Model) syncThemedResponseState() tea.Cmd {
	var cmds []tea.Cmd
	if cmd := m.restartPendingResponseRender(); cmd != nil {
		cmds = append(cmds, m.respCmd(cmd))
	}
	if cmd := m.syncResponsePanes(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	switch len(cmds) {
	case 0:
		return nil
	case 1:
		return cmds[0]
	default:
		return tea.Batch(cmds...)
	}
}
