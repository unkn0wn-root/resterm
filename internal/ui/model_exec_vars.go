package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (m *Model) documentRuntimePath(doc *restfile.Document) string {
	if doc != nil && strings.TrimSpace(doc.Path) != "" {
		return doc.Path
	}
	return m.currentFile
}

func (m *Model) showGlobalSummary() tea.Cmd {
	text := m.buildGlobalSummary()
	if strings.TrimSpace(text) == "" {
		text = "Globals: (empty)"
	}
	m.setStatusMessage(statusMsg{level: statusInfo, text: text})
	return nil
}

func (m *Model) buildGlobalSummary() string {
	var segments []string

	if snapshot := m.globalsSnapshot(); len(snapshot) > 0 {
		entries := make([]summaryEntry, 0, len(snapshot))
		for key, value := range snapshot {
			name := strings.TrimSpace(value.Name)
			if name == "" {
				name = key
			}
			entries = append(
				entries,
				summaryEntry{name: name, value: value.Value, secret: value.Secret},
			)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
		parts := make([]string, 0, len(entries))
		for _, entry := range entries {
			parts = append(
				parts,
				fmt.Sprintf("%s=%s", entry.name, maskSecret(entry.value, entry.secret)),
			)
		}
		segments = append(segments, "Globals: "+strings.Join(parts, ", "))
	}

	if doc := m.doc; doc != nil {
		entries := make([]summaryEntry, 0, len(doc.Globals))
		for _, global := range doc.Globals {
			name := strings.TrimSpace(global.Name)
			if name == "" {
				continue
			}
			entries = append(
				entries,
				summaryEntry{name: name, value: global.Value, secret: global.Secret},
			)
		}
		if len(entries) > 0 {
			sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
			parts := make([]string, 0, len(entries))
			for _, entry := range entries {
				parts = append(
					parts,
					fmt.Sprintf("%s=%s", entry.name, maskSecret(entry.value, entry.secret)),
				)
			}
			segments = append(segments, "Doc: "+strings.Join(parts, ", "))
		}
	}

	return strings.Join(segments, " | ")
}

func (m *Model) globalsSnapshot() map[string]globalValue {
	gs := m.globalsStore()
	if gs == nil {
		return nil
	}
	return gs.Snapshot(m.ws.active.Scope())
}

func (m *Model) clearGlobalValues() tea.Cmd {
	gs := m.globalsStore()
	if gs == nil {
		m.setStatusMessage(statusMsg{level: statusWarn, text: "No global store available"})
		return nil
	}

	env := m.ws.active
	gs.Clear(env.Scope())
	if cs := m.cookieStore(); cs != nil {
		cs.Clear(env.Scope())
	}
	label := env.Label()
	if strings.TrimSpace(label) == "" {
		label = "default"
	}

	m.setStatusMessage(
		statusMsg{
			level: statusInfo,
			text:  fmt.Sprintf("Cleared globals and cookies for %s", label),
		},
	)
	return nil
}

type summaryEntry struct {
	name   string
	value  string
	secret bool
}

func maskSecret(value string, secret bool) string {
	if secret {
		return "•••"
	}
	return value
}
