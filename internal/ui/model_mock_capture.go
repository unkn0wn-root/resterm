package ui

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/mock"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/restwriter"
)

// Keep capture bounded because rendering and validating an inline response
// requires several copies of the body on the UI event loop.
const maxInlineMockCaptureBodyBytes = 4 << 20

func (m *Model) captureMockResponse() tea.Cmd {
	spec, err := m.capturedMock()
	if err != nil {
		return statusCmd(statusWarn, err.Error())
	}

	block, err := renderCapturedMock(spec)
	if err != nil {
		return statusCmd(statusWarn, "Response cannot be captured as a mock: "+oneLine(err.Error()))
	}
	start := m.appendMockBlock(block)
	m.revealLineRangeInEditor(restfile.LineRange{
		Start: start,
		End:   start + strings.Count(block, "\n"),
	})
	m.refreshCurrentDocument([]byte(m.editor.Value()))
	m.markDirty()
	m.suppressEditorKey = true

	return batchCommands(
		m.setFocus(focusEditor),
		m.setInsertMode(false, true),
		m.scheduleMockReload(0),
		statusCmd(
			statusWarn,
			fmt.Sprintf(
				"Captured mock %s. Review headers and body for secrets before saving.",
				spec.Name,
			),
		),
	)
}

func (m *Model) capturedMock() (*restfile.Mock, error) {
	resp, effectiveURL, err := m.captureSource()
	if err != nil {
		return nil, err
	}
	method := capturedMockMethod(resp)
	if method == "" {
		return nil, errors.New("response request method is unavailable")
	}
	path, query, err := capturedMockPath(resp, effectiveURL)
	if err != nil {
		return nil, err
	}
	body, err := capturedMockBody(resp)
	if err != nil {
		return nil, err
	}
	overlay, route, err := m.captureRoute(method, path)
	if err != nil {
		return nil, err
	}

	label := capturedMockLabel(resp, m.activeRequestTitle)
	response := restfile.MockResponse{
		Status:  resp.StatusCode,
		Headers: capturedMockHeaders(resp.Headers),
		Body: restfile.BodySource{
			Text:     body,
			MimeType: resp.Headers.Get("Content-Type"),
		},
	}
	spec := &restfile.Mock{
		Title:                capturedMockTitle(resp, label),
		Name:                 nextCapturedMockName(route, label, resp.StatusCode),
		Method:               method,
		Path:                 path,
		Default:              capturedMockIsDefault(route),
		Responses:            []restfile.MockResponse{response},
		DisableInterpolation: response.HasTemplate(),
	}
	// A default scenario answers every request, so query conditions are only
	// attached once another scenario already covers the route.
	if !spec.Default && len(query) > 0 {
		spec.Match.Query = query
	}

	overlay.Mocks = append(overlay.Mocks, spec)
	if _, err := mock.Load(m.mockRoot(), m.workspaceRecursive, overlay); err != nil {
		return nil, fmt.Errorf("response cannot be captured as a mock: %s", oneLine(err.Error()))
	}
	return spec, nil
}

func (m *Model) captureSource() (*httpclient.Response, string, error) {
	if m.currentFile != "" && !filesvc.IsRequestFile(m.currentFile) {
		return nil, "", errors.New("select a .http or .rest file before capturing a mock")
	}
	snap := m.mockCaptureSnapshot()
	if snap == nil || !snap.ready || snap.source.http == nil {
		return nil, "", errors.New("no live or pinned HTTP response is available to capture")
	}
	resp := snap.source.http
	if !restfile.ValidMockStatus(resp.StatusCode) {
		return nil, "", fmt.Errorf("response status %d cannot be mocked", resp.StatusCode)
	}
	return resp, snap.effectiveURL, nil
}

func (m *Model) captureRoute(method, path string) (*restfile.Document, []*restfile.Mock, error) {
	overlay := parser.Parse(m.currentFile, []byte(m.editor.Value()))
	docs, err := mock.LoadDocuments(m.mockRoot(), m.workspaceRecursive, overlay)
	if err != nil {
		return nil, nil, fmt.Errorf("response cannot be captured as a mock: %s", oneLine(err.Error()))
	}
	if !slices.Contains(docs, overlay) {
		docs = append(docs, overlay)
	}
	return overlay, routeMocks(docs, method, path), nil
}

func capturedMockTitle(resp *httpclient.Response, label string) string {
	title := fmt.Sprintf("Mock %s - %d", label, resp.StatusCode)
	if text := http.StatusText(resp.StatusCode); text != "" {
		title += " " + text
	}
	return title
}

func capturedMockMethod(resp *httpclient.Response) string {
	method := strings.TrimSpace(resp.ReqMethod)
	if method == "" && resp.Request != nil {
		method = strings.TrimSpace(resp.Request.Method)
	}
	return strings.ToUpper(method)
}

func capturedMockPath(resp *httpclient.Response, fallback string) (string, map[string][]string, error) {
	raw := strings.TrimSpace(resp.EffectiveURL)
	if raw == "" {
		raw = strings.TrimSpace(fallback)
	}
	if raw == "" && resp.Request != nil {
		raw = strings.TrimSpace(resp.Request.URL)
	}
	if raw == "" {
		return "", nil, errors.New("response URL is unavailable")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", nil, fmt.Errorf("response URL cannot be captured: %w", err)
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "", nil, errors.New("response URL has no origin-form path")
	}
	query := u.Query()
	delete(query, "")
	return path, query, nil
}

func capturedMockBody(resp *httpclient.Response) (string, error) {
	if !restfile.ResponseAllowsBody(resp.StatusCode) {
		return "", nil
	}
	if len(resp.Body) == 0 {
		return "", nil
	}
	if len(resp.Body) > maxInlineMockCaptureBodyBytes {
		return "", fmt.Errorf(
			"response body is too large to capture inline (%d bytes; limit %d)",
			len(resp.Body),
			maxInlineMockCaptureBodyBytes,
		)
	}

	contentType := resp.Headers.Get("Content-Type")
	if !utf8.Valid(resp.Body) {
		return "", errors.New(
			"binary responses cannot be captured inline; save the body and use a file reference",
		)
	}
	if binaryview.Analyze(resp.Body, contentType).Kind == binaryview.KindBinary {
		return "", errors.New(
			"binary responses cannot be captured inline; save the body and use a file reference",
		)
	}
	body := strings.ReplaceAll(string(resp.Body), "\r\n", "\n")
	return strings.ReplaceAll(body, "\r", "\n"), nil
}

func capturedMockHeaders(src http.Header) http.Header {
	dst := make(http.Header)
	for name, values := range src {
		if restfile.IsManagedMockResponseHeader(name) || name == "Date" || name == "Set-Cookie" {
			continue
		}
		for _, value := range values {
			if !strings.ContainsAny(value, "\r\n") {
				dst.Add(name, value)
			}
		}
	}
	return dst
}

func renderCapturedMock(spec *restfile.Mock) (string, error) {
	block, err := restwriter.Render(
		&restfile.Document{Mocks: []*restfile.Mock{spec}},
		restwriter.Options{},
	)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(block) == "" {
		return "", errors.New("rendered mock is empty")
	}
	return block, nil
}

func (m *Model) appendMockBlock(block string) int {
	value := m.editor.Value()
	newline := "\n"
	if strings.Contains(value, "\r\n") {
		newline = "\r\n"
		block = strings.ReplaceAll(block, "\n", newline)
	}
	if value != "" && !strings.HasSuffix(value, newline+newline) {
		if !strings.HasSuffix(value, newline) {
			value += newline
		}
		value += newline
	}

	start := strings.Count(value, "\n") + 1
	m.editor.pushUndoSnapshot()
	m.editor.SetValue(value + block)
	m.editor.moveCursorTo(start-1, 0)
	return start
}

func (m *Model) mockCaptureSnapshot() *responseSnapshot {
	id := m.responseLastFocused
	if m.focus == focusResponse {
		id = m.responsePaneFocus
	}
	if pane := m.pane(id); pane != nil && pane.snapshot != nil {
		return pane.snapshot
	}
	return m.responseLatest
}

func capturedMockLabel(resp *httpclient.Response, fallback string) string {
	label := ""
	if resp != nil && resp.Request != nil {
		label = strings.TrimSpace(resp.Request.Metadata.Name)
	}
	if label == "" {
		label = strings.TrimSpace(fallback)
	}
	label = oneLine(label)
	if label == "" && resp != nil {
		if method := capturedMockMethod(resp); method != "" {
			label = method + " response"
		}
	}
	if label == "" {
		label = "response"
	}
	return truncateRunes(label, 80)
}

func routeMocks(docs []*restfile.Document, method, path string) []*restfile.Mock {
	var route []*restfile.Mock
	for _, doc := range docs {
		for _, spec := range doc.Mocks {
			if spec.Path == path && strings.EqualFold(spec.Method, method) {
				route = append(route, spec)
			}
		}
	}
	return route
}

func nextCapturedMockName(route []*restfile.Mock, label string, status int) string {
	base := restwriter.MockNameSlug(label)
	if base == "" {
		base = fmt.Sprintf("response-%d", status)
	}
	if len(base) > 56 {
		base = strings.Trim(base[:56], "-._")
	}

	used := make(map[string]struct{}, len(route))
	for _, spec := range route {
		used[spec.Name] = struct{}{}
		used[spec.Sequence] = struct{}{}
	}
	return restwriter.UniqueMockName(base, used)
}

func capturedMockIsDefault(route []*restfile.Mock) bool {
	for _, spec := range route {
		if spec.Default || !spec.Match.HasConditions() {
			return false
		}
	}
	return true
}
