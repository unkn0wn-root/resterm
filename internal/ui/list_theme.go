package ui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

func listItemStylesForTheme(th theme.Theme) list.DefaultItemStyles {
	styles := list.NewDefaultItemStyles()
	styles.NormalTitle = mergeListStyle(styles.NormalTitle, th.ListItemTitle)
	styles.NormalDesc = mergeListStyle(styles.NormalDesc, th.ListItemDescription)
	styles.SelectedTitle = mergeListStyle(styles.SelectedTitle, th.ListItemSelectedTitle)
	styles.SelectedDesc = mergeListStyle(styles.SelectedDesc, th.ListItemSelectedDescription)
	styles.DimmedTitle = mergeListStyle(styles.DimmedTitle, th.ListItemDimmedTitle)
	styles.DimmedDesc = mergeListStyle(styles.DimmedDesc, th.ListItemDimmedDescription)
	styles.FilterMatch = mergeListStyle(styles.FilterMatch, th.ListItemFilterMatch)
	return styles
}

func mergeListStyle(base, override lipgloss.Style) lipgloss.Style {
	merged := override.Inherit(base)
	pt, pr, pb, pl := base.GetPadding()
	merged = merged.Padding(pt, pr, pb, pl)
	mt, mr, mb, ml := base.GetMargin()
	merged = merged.Margin(mt, mr, mb, ml)
	return merged
}

func listDelegateForTheme(th theme.Theme, showDescription bool, height int) list.DefaultDelegate {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = showDescription
	if showDescription && height > 0 {
		delegate.SetHeight(height)
	}
	delegate.Styles = listItemStylesForTheme(th)
	return delegate
}

func historyDelegateForTheme(
	th theme.Theme,
	height int,
	selected map[string]struct{},
) historyDelegate {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	if height > 0 {
		delegate.SetHeight(height)
	}
	delegate.Styles = listItemStylesForTheme(th)
	return historyDelegate{DefaultDelegate: delegate, th: th, selected: selected}
}

func applyListTheme(th theme.Theme, model *list.Model, showDescription bool, height int) {
	delegate := listDelegateForTheme(th, showDescription, height)
	model.SetDelegate(delegate)
}

func applyHistoryListTheme(
	th theme.Theme,
	model *list.Model,
	height int,
	selected map[string]struct{},
) {
	delegate := historyDelegateForTheme(th, height, selected)
	model.SetDelegate(delegate)
}

func (m *Model) applyThemeToLists() {
	applyListTheme(m.theme, &m.fileList, false, 0)
	applyListTheme(m.theme, &m.requestList, !m.reqCompactMode(), 3)
	applyListTheme(m.theme, &m.workflowList, !m.wfCompactMode(), 3)
	applyHistoryListTheme(m.theme, &m.historyList, 2, m.historySelected)
	m.historyList.Styles.PaginationStyle = mergeListStyle(
		list.DefaultStyles().PaginationStyle,
		m.theme.ListItemDescription,
	)
	m.historyList.Styles.ArabicPagination = mergeListStyle(
		list.DefaultStyles().ArabicPagination,
		m.theme.ListItemDescription,
	)
	applyListTheme(m.theme, &m.envList, false, 0)
	applyListTheme(m.theme, &m.themeList, true, 3)
}
