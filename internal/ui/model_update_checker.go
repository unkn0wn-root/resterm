package ui

import (
	"context"
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/update"
)

const (
	updateInterval = time.Hour
	updateTimeout  = 20 * time.Second
)

func newUpdateTickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return updateTickMsg{}
	})
}

func newUpdateCheckCmd(cl update.Client, ver string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
		defer cancel()

		plat, err := update.Detect()
		if err != nil {
			return updateCheckMsg{err: err}
		}

		res, ok, err := cl.Check(ctx, ver, plat)
		if err != nil {
			// transient: assets still uploading or the shared api quota is spent. retry at next tick
			if errors.Is(err, update.ErrNoAsset) ||
				errors.Is(err, update.ErrNoDigest) ||
				errors.Is(err, update.ErrRateLimited) {
				return updateCheckMsg{}
			}
			return updateCheckMsg{err: err}
		}
		if !ok {
			return updateCheckMsg{}
		}
		return updateCheckMsg{res: &res}
	}
}

func (m *Model) enqueueUpdateCheck() tea.Cmd {
	if !m.updateEnabled || m.updateBusy || m.updateVersion == "" {
		return nil
	}
	m.updateBusy = true
	return newUpdateCheckCmd(m.updateClient, m.updateVersion)
}
