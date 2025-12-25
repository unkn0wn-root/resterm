package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/bindings"
	"github.com/unkn0wn-root/resterm/internal/watcher"
)

type fileChangedMsg struct {
	path string
	kind watcher.EventKind
}

func (m *Model) startFileWatcher() {
	if m.fileWatcher == nil {
		return
	}
	m.fileWatcher.Start()
	go func() {
		for evt := range m.fileWatcher.Events() {
			m.emitFileWatchMsg(fileChangedMsg{path: evt.Path, kind: evt.Kind})
		}
	}()
}

func (m *Model) watchFile(path string, data []byte) {
	if m.fileWatcher == nil || path == "" {
		return
	}
	m.fileWatcher.Track(path, data)
	m.fileStale = false
	m.fileMissing = false
	m.pendingReloadConfirm = false
	m.closeFileChangeModal()
}

func (m *Model) forgetFileWatch(path string) {
	if m.fileWatcher != nil && path != "" {
		m.fileWatcher.Forget(path)
	}
	m.fileStale = false
	m.fileMissing = false
	m.pendingReloadConfirm = false
	m.closeFileChangeModal()
}

func (m *Model) emitFileWatchMsg(msg tea.Msg) {
	if msg == nil || m.fileWatchChan == nil {
		return
	}
	m.fileWatchChan <- msg
}

func (m *Model) nextFileWatchMsgCmd() tea.Cmd {
	if m.fileWatchChan == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-m.fileWatchChan
		if !ok {
			return nil
		}
		return msg
	}
}

func (m *Model) handleFileChangeEvent(msg fileChangedMsg) {
	if msg.path == "" || !samePath(msg.path, m.currentFile) {
		return
	}
	m.fileStale = true
	m.fileMissing = msg.kind == watcher.EventMissing
	m.pendingReloadConfirm = false
	m.showHelp = false
	name := filepath.Base(msg.path)
	if name == "" {
		name = "File"
	}
	title := fmt.Sprintf("%s changed on disk. Using current buffer.", name)
	if m.fileMissing {
		title = fmt.Sprintf("%s removed on disk. Using current buffer.", name)
	}
	text := strings.TrimSpace(title)
	m.openFileChangeModal(text)
	m.setStatusMessage(statusMsg{text: text, level: statusWarn})
}

func (m *Model) openFileChangeModal(msg string) {
	m.showFileChangeModal = true
	m.fileChangeMessage = strings.TrimSpace(msg)
	m.resetChordState()
	m.showEnvSelector = false
	m.showThemeSelector = false
	m.showOpenModal = false
	m.showNewFileModal = false
}

func (m *Model) closeFileChangeModal() {
	m.showFileChangeModal = false
	m.fileChangeMessage = ""
	m.resetChordState()
}

func (m *Model) handleReloadBinding(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.bindingsMap == nil {
		return nil, false
	}
	key := canonicalShortcutKey(msg)
	if key == "" {
		return nil, false
	}
	// Resolve pending chord first.
	if m.hasPendingChord && m.pendingChord != "" {
		prefix := m.pendingChord
		m.pendingChord = ""
		m.pendingChordMsg = tea.KeyMsg{}
		m.hasPendingChord = false
		if binding, ok := m.bindingsMap.ResolveChord(
			prefix,
			key,
		); ok &&
			binding.Action == bindings.ActionReloadFileFromDisk {
			return m.runShortcutBinding(binding, msg)
		}
	}

	if binding, ok := m.bindingsMap.MatchSingle(
		key,
	); ok &&
		binding.Action == bindings.ActionReloadFileFromDisk {
		return m.runShortcutBinding(binding, msg)
	}

	if m.isReloadChordPrefix(key) {
		m.pendingChord = key
		m.pendingChordMsg = msg
		m.hasPendingChord = true
		return nil, true
	}
	return nil, false
}

func (m *Model) isReloadChordPrefix(key string) bool {
	for _, binding := range m.bindingsMap.Bindings(bindings.ActionReloadFileFromDisk) {
		if len(binding.Steps) > 1 && binding.Steps[0] == key {
			return true
		}
	}
	return false
}
