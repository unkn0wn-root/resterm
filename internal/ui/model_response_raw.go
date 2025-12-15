package ui

import (
	"fmt"

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
	snap.rawMode = clampRawViewMode(meta, snap.rawMode)
	next := nextRawViewMode(meta, snap.rawMode)
	applyRawViewMode(snap, next)

	for _, id := range m.visiblePaneIDs() {
		p := m.pane(id)
		if p != nil && p.snapshot == snap {
			p.invalidateCaches()
		}
	}

	m.setStatusMessage(statusMsg{
		level: statusInfo,
		text:  fmt.Sprintf("Raw view: %s", snap.rawMode.label()),
	})
	return tea.Batch(tea.ClearScreen, m.syncResponsePanes())
}

func applyRawViewMode(snapshot *responseSnapshot, mode rawViewMode) {
	if snapshot == nil {
		return
	}
	meta := ensureSnapshotMeta(snapshot)
	mode = clampRawViewMode(meta, mode)
	if snapshot.rawText == "" && len(snapshot.body) > 0 {
		snapshot.rawText = formatRawBody(snapshot.body, snapshot.contentType)
	}
	needHex := mode == rawViewHex
	needBase64 := mode == rawViewBase64
	if snapshot.rawHex == "" && needHex && len(snapshot.body) > 0 {
		snapshot.rawHex = binaryview.HexDump(snapshot.body, 16)
	}
	if snapshot.rawBase64 == "" && needBase64 && len(snapshot.body) > 0 {
		snapshot.rawBase64 = binaryview.Base64Lines(snapshot.body, 76)
	}
	if snapshot.rawText == "" {
		snapshot.rawText = snapshot.raw
	}
	snapshot.rawMode = mode
	body := ""
	switch mode {
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
	if body == "" {
		body = snapshot.raw
	}
	snapshot.raw = joinSections(snapshot.rawSummary, body)
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

func clampRawViewMode(meta binaryview.Meta, mode rawViewMode) rawViewMode {
	modes := allowedRawViewModes(meta)
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

func nextRawViewMode(meta binaryview.Meta, current rawViewMode) rawViewMode {
	modes := allowedRawViewModes(meta)
	if len(modes) == 0 {
		return current
	}
	current = clampRawViewMode(meta, current)
	idx := 0
	for i, m := range modes {
		if m == current {
			idx = i
			break
		}
	}
	return modes[(idx+1)%len(modes)]
}

func allowedRawViewModes(meta binaryview.Meta) []rawViewMode {
	if meta.Kind == binaryview.KindBinary && !meta.Printable {
		return []rawViewMode{rawViewHex, rawViewBase64}
	}
	return []rawViewMode{rawViewText, rawViewHex, rawViewBase64}
}

func rawViewModeLabels(meta binaryview.Meta) []string {
	modes := allowedRawViewModes(meta)
	labels := make([]string, 0, len(modes))
	for _, mode := range modes {
		labels = append(labels, mode.label())
	}
	return labels
}
