package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
)

func (m *Model) cycleRawViewMode() tea.Cmd {
	pane := m.focusedPane()
	if pane == nil || pane.snapshot == nil || !pane.snapshot.ready {
		m.setStatusMessage(statusMsg{level: statusInfo, text: "No response to cycle raw view"})
		return nil
	}
	snap := pane.snapshot
	meta := ensureSnapshotMeta(snap)
	sz := len(snap.body)
	snap.rawMode = clampRawViewMode(meta, sz, snap.rawMode)
	next := nextRawViewMode(meta, sz, snap.rawMode)
	if next == rawViewHex && snap.rawHex == "" && rawHeavy(sz) {
		return m.loadRawDumpAsync(snap, rawViewHex)
	}
	if next == rawViewBase64 && snap.rawBase64 == "" && rawHeavy(sz) {
		return m.loadRawDumpAsync(snap, rawViewBase64)
	}
	return m.setRawMode(snap, next, "")
}

func (m *Model) showRawDump() tea.Cmd {
	pane := m.focusedPane()
	if pane == nil || pane.snapshot == nil || !pane.snapshot.ready {
		m.setStatusMessage(statusMsg{level: statusInfo, text: "No response to show raw dump"})
		return nil
	}
	snap := pane.snapshot
	if snap.rawHex != "" || len(snap.body) == 0 || !rawHeavy(len(snap.body)) {
		return m.setRawMode(snap, rawViewHex, rawDumpLoadedMessage(rawViewHex))
	}
	return m.loadRawDumpAsync(snap, rawViewHex)
}

func (m *Model) setRawMode(snap *responseSnapshot, mode rawViewMode, msg string) tea.Cmd {
	if snap == nil {
		return nil
	}
	applyRawViewMode(snap, mode)

	for _, id := range m.visiblePaneIDs() {
		p := m.pane(id)
		if p != nil && p.snapshot == snap {
			p.markRawViewStale()
		}
	}
	m.invalidateDiffCaches()

	if msg == "" {
		msg = fmt.Sprintf("Raw view: %s", snap.rawMode.label())
	}
	m.setStatusMessage(statusMsg{level: statusInfo, text: msg})
	return m.syncResponsePanes()
}

func (m *Model) loadRawDumpAsync(snap *responseSnapshot, mode rawViewMode) tea.Cmd {
	if snap == nil {
		return nil
	}
	if snap.rawLoading {
		m.setStatusMessage(statusMsg{level: statusInfo, text: rawDumpLoadingMessage(snap.rawLoadingMode)})
		return nil
	}
	if len(snap.body) == 0 {
		return m.setRawMode(snap, mode, rawDumpLoadedMessage(mode))
	}

	snap.rawLoading = true
	snap.rawLoadingMode = mode
	snap.rawMode = mode
	loading := rawDumpLoadingMessage(mode)
	snap.raw = joinSections(snap.rawSummary, loading)

	for _, id := range m.visiblePaneIDs() {
		p := m.pane(id)
		if p != nil && p.snapshot == snap {
			p.invalidateRawCache(mode)
		}
	}
	m.invalidateDiffCaches()

	m.setStatusMessage(statusMsg{level: statusInfo, text: loading})
	return tea.Batch(m.syncResponsePanes(), loadRawDumpCmd(snap, mode))
}

func (m *Model) handleRawDumpLoaded(msg rawDumpLoadedMsg) tea.Cmd {
	snap := msg.snapshot
	if snap == nil {
		return nil
	}
	if !snap.rawLoading || snap.rawLoadingMode != msg.mode {
		return nil
	}

	switch msg.mode {
	case rawViewHex:
		snap.rawHex = msg.content
	case rawViewBase64:
		snap.rawBase64 = msg.content
	default:
		snap.rawLoading = false
		snap.rawLoadingMode = rawViewText
		return nil
	}

	snap.rawLoading = false
	snap.rawLoadingMode = rawViewText

	if snap.rawMode == msg.mode {
		applyRawViewMode(snap, msg.mode)
	}

	visible := false
	for _, id := range m.visiblePaneIDs() {
		p := m.pane(id)
		if p != nil && p.snapshot == snap {
			visible = true
			if snap.rawMode == msg.mode {
				p.invalidateRawCache(msg.mode)
			}
		}
	}

	if visible {
		m.invalidateDiffCaches()
		m.setStatusMessage(statusMsg{level: statusInfo, text: rawDumpLoadedMessage(msg.mode)})
		if snap.rawMode == msg.mode {
			return m.syncResponsePanes()
		}
	}
	return nil
}

func (m *Model) invalidateDiffCaches() {
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil || pane.wrapCache == nil {
			continue
		}
		pane.wrapCache[responseTabDiff] = cachedWrap{}
	}
}

func rawDumpLoadingMessage(mode rawViewMode) string {
	label := strings.TrimSpace(mode.label())
	if label == "" {
		label = "raw"
	}
	return fmt.Sprintf("Loading raw dump (%s)...", label)
}

func rawDumpLoadedMessage(mode rawViewMode) string {
	label := strings.TrimSpace(mode.label())
	if label == "" {
		label = "raw"
	}
	return fmt.Sprintf("Raw dump loaded (%s)", label)
}

func loadRawDumpCmd(snap *responseSnapshot, mode rawViewMode) tea.Cmd {
	if snap == nil {
		return nil
	}
	if mode != rawViewHex && mode != rawViewBase64 {
		return nil
	}
	body := snap.body
	return func() tea.Msg {
		content := ""
		switch mode {
		case rawViewBase64:
			content = binaryview.Base64Lines(body, 76)
		default:
			content = binaryview.HexDump(body, binaryview.HexDumpBytesPerLine)
		}
		return rawDumpLoadedMsg{snapshot: snap, mode: mode, content: content}
	}
}

func applyRawViewMode(snapshot *responseSnapshot, mode rawViewMode) {
	if snapshot == nil {
		return
	}
	meta := ensureSnapshotMeta(snapshot)
	sz := len(snapshot.body)
	mode = clampRawViewMode(meta, sz, mode)
	if snapshot.rawText == "" && len(snapshot.body) > 0 && (meta.Kind == binaryview.KindText || meta.Printable) {
		snapshot.rawText = formatRawBody(snapshot.body, snapshot.contentType)
	}
	needHex := mode == rawViewHex
	needBase64 := mode == rawViewBase64
	if snapshot.rawHex == "" && needHex && len(snapshot.body) > 0 {
		snapshot.rawHex = binaryview.HexDump(snapshot.body, binaryview.HexDumpBytesPerLine)
	}
	if snapshot.rawBase64 == "" && needBase64 && len(snapshot.body) > 0 {
		snapshot.rawBase64 = binaryview.Base64Lines(snapshot.body, 76)
	}
	snapshot.rawMode = mode
	body := ""
	switch mode {
	case rawViewSummary:
		body = rawSum(meta, sz)
	case rawViewHex:
		if snapshot.rawHex != "" {
			body = snapshot.rawHex
		} else if snapshot.rawText != "" {
			body = snapshot.rawText
		}
	case rawViewBase64:
		if snapshot.rawBase64 != "" {
			body = snapshot.rawBase64
		} else if snapshot.rawText != "" {
			body = snapshot.rawText
		}
	default:
		if snapshot.rawText != "" {
			body = snapshot.rawText
		}
	}
	body = fallbackRawBody(snapshot, body)
	if snapshot.rawText == "" && mode != rawViewSummary {
		snapshot.rawText = body
	}
	snapshot.raw = joinSections(snapshot.rawSummary, body)
}

func fallbackRawBody(snapshot *responseSnapshot, body string) string {
	if body != "" {
		return body
	}
	if snapshot == nil {
		return "<empty>"
	}
	if snapshot.rawText == "" && len(snapshot.body) == 0 {
		trimmed := trimSection(snapshot.raw)
		if strings.TrimSpace(trimmed) != "" {
			if strings.TrimSpace(snapshot.rawSummary) != "" {
				summary := trimSection(snapshot.rawSummary)
				if strings.HasPrefix(trimmed, summary) {
					trimmed = strings.TrimLeft(strings.TrimPrefix(trimmed, summary), "\r\n")
				}
			}
			if strings.TrimSpace(trimmed) != "" {
				return trimmed
			}
		}
	}
	return "<empty>"
}

func ensureSnapshotMeta(snapshot *responseSnapshot) binaryview.Meta {
	if snapshot == nil {
		return binaryview.Meta{}
	}
	if snapshot.bodyMeta.Kind == binaryview.KindUnknown {
		snapshot.bodyMeta = binaryview.Analyze(snapshot.body, snapshot.contentType)
	}
	return snapshot.bodyMeta
}

func clampRawViewMode(meta binaryview.Meta, sz int, mode rawViewMode) rawViewMode {
	modes := allowedRawViewModes(meta, sz)
	for _, m := range modes {
		if m == mode {
			return mode
		}
	}
	if len(modes) == 0 {
		return rawViewText
	}
	return modes[0]
}

func nextRawViewMode(meta binaryview.Meta, sz int, current rawViewMode) rawViewMode {
	modes := allowedRawViewModes(meta, sz)
	if len(modes) == 0 {
		return current
	}
	current = clampRawViewMode(meta, sz, current)
	idx := 0
	for i, m := range modes {
		if m == current {
			idx = i
			break
		}
	}
	return modes[(idx+1)%len(modes)]
}

func allowedRawViewModes(meta binaryview.Meta, sz int) []rawViewMode {
	if meta.Kind == binaryview.KindBinary && !meta.Printable {
		if rawHeavyBin(meta, sz) {
			return []rawViewMode{rawViewSummary, rawViewHex, rawViewBase64}
		}
		return []rawViewMode{rawViewHex, rawViewBase64}
	}
	return []rawViewMode{rawViewText, rawViewHex, rawViewBase64}
}

func rawViewModeLabels(meta binaryview.Meta, sz int) []string {
	modes := allowedRawViewModes(meta, sz)
	labels := make([]string, 0, len(modes))
	for _, mode := range modes {
		labels = append(labels, mode.label())
	}
	return labels
}
