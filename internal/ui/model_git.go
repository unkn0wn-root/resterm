package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/gitstatus"
)

const gitStatusRefreshTimeout = 2 * time.Second

type gitStatusMsg struct {
	seq      int
	snapshot gitstatus.Snapshot
	err      error
}

func (m Model) initialGitStatusCmd() tea.Cmd {
	return newGitStatusCmd(m.gitStatusSeq, m.workspaceRoot, m.gitStatusPaths())
}

func (m *Model) refreshGitStatusCmd() tea.Cmd {
	m.gitStatusSeq++
	paths := m.gitStatusPaths()
	if m.workspaceRoot == "" || len(paths) == 0 {
		m.gitStatus = gitstatus.Snapshot{}
		return nil
	}
	return newGitStatusCmd(m.gitStatusSeq, m.workspaceRoot, paths)
}

func newGitStatusCmd(seq int, root string, paths []string) tea.Cmd {
	if root == "" || len(paths) == 0 {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), gitStatusRefreshTimeout)
		defer cancel()

		snapshot, err := gitstatus.Load(ctx, root, paths)
		return gitStatusMsg{
			seq:      seq,
			snapshot: snapshot,
			err:      err,
		}
	}
}

func (m Model) gitStatusPaths() []string {
	entries := m.entriesFromList()
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	return paths
}

func (m *Model) handleGitStatusMsg(msg gitStatusMsg) {
	if msg.seq != m.gitStatusSeq {
		return
	}
	if msg.err != nil {
		m.gitStatus = gitstatus.Snapshot{}
	} else {
		m.gitStatus = msg.snapshot
	}
	m.rebuildNavigator(nil)
}
