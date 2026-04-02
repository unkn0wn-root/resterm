package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

type explainStyles struct {
	title         lipgloss.Style
	label         lipgloss.Style
	value         lipgloss.Style
	muted         lipgloss.Style
	sectionTitle  lipgloss.Style
	sectionBorder lipgloss.Style
	requestLine   lipgloss.Style
	headerName    lipgloss.Style
	body          lipgloss.Style
	bodyNote      lipgloss.Style
	badgeReady    lipgloss.Style
	badgeSkipped  lipgloss.Style
	badgeError    lipgloss.Style
	stageOK       lipgloss.Style
	stageSkipped  lipgloss.Style
	stageError    lipgloss.Style
	changeAdd     lipgloss.Style
	changeRemove  lipgloss.Style
	changeUpdate  lipgloss.Style
	warning       lipgloss.Style
	chip          lipgloss.Style
	chipMuted     lipgloss.Style
	chipMissing   lipgloss.Style
}

func newExplainStyles(th theme.Theme) explainStyles {
	return explainStyles{
		title:         th.ExplainValue.Bold(true),
		label:         th.ExplainLabel,
		value:         th.ExplainValue,
		muted:         th.ExplainMuted,
		sectionTitle:  th.ExplainSectionTitle,
		sectionBorder: th.ExplainSectionBorder,
		requestLine:   th.ExplainValue.Bold(true),
		headerName:    th.ExplainLabel.Bold(true),
		body:          th.ResponseContentRaw.Inherit(th.ExplainValue),
		bodyNote:      th.ExplainMuted.Italic(true),
		badgeReady:    th.ExplainBadgeReady,
		badgeSkipped:  th.ExplainBadgeSkipped,
		badgeError:    th.ExplainBadgeError,
		stageOK:       th.ExplainStageOK,
		stageSkipped:  th.ExplainStageSkipped,
		stageError:    th.ExplainStageError,
		changeAdd:     th.ExplainChangeAdd,
		changeRemove:  th.ExplainChangeRemove,
		changeUpdate:  th.ExplainChangeUpdate,
		warning:       th.ExplainWarning,
		chip:          th.ExplainSectionBorder.Padding(0, 1),
		chipMuted:     th.ExplainMuted.Padding(0, 1),
		chipMissing:   th.ExplainWarning.Padding(0, 1),
	}
}
