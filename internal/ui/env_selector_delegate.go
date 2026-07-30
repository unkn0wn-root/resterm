package ui

import (
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	envGroupMaxWidth = 24
	envGroupGap      = " │ "
	envMarkActive    = "● "
	envMarkInactive  = "  "
)

// envDelegate gives grouped catalogs a group column that only prints its label
// when the group changes or a page starts, so the profiles under it read as one
// block. groupName is the widest group name before clamping to the list width.
type envDelegate struct {
	list.DefaultDelegate
	groupName int
}

func (d envDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	env, ok := item.(envItem)
	if !ok || m.Width() <= 0 {
		return
	}

	style, _ := listRowStyles(d.Styles, m, index)
	pad := d.Styles.NormalTitle.GetPaddingLeft() + d.Styles.NormalTitle.GetPaddingRight()
	line, offset := d.itemLine(m, index, env)
	line = ansi.Truncate(line, max(m.Width()-pad, 0), "…")

	if m.FilterState() != list.Unfiltered {
		// MatchesForItem hands back the list's own slice, so shift a copy.
		matches := slices.Clone(m.MatchesForItem(index))
		for i := range matches {
			matches[i] += offset
		}
		unmatched := style.Inline(true)
		line = lipgloss.StyleRunes(line, matches, unmatched.Inherit(d.Styles.FilterMatch), unmatched)
	}
	_, _ = io.WriteString(w, style.Render(line))
}

// itemLine draws a row, and offset is how far the filter matches have to shift
// because they index into FilterValue rather than the drawn line.
func (d envDelegate) itemLine(m list.Model, index int, env envItem) (line string, offset int) {
	mark := envMarkInactive
	if env.active {
		mark = envMarkActive
	}

	// A filtered row drops the column and spells out the whole choice, so every
	// match stays next to the text the filter was typed against.
	if env.group == "" || m.FilterState() != list.Unfiltered {
		return mark + env.FilterValue(), utf8.RuneCountInString(mark)
	}

	column := d.groupColumn(m.Width())
	group := ""
	if envStartsGroup(m, index, env.group) {
		group = truncateToWidth(env.group, column)
	}
	group += strings.Repeat(" ", max(column-visibleWidth(group), 0))
	return group + envGroupGap + mark + env.profile, 0
}

// groupColumn caps the group column so the profile beside it keeps room on a
// narrow terminal.
func (d envDelegate) groupColumn(width int) int {
	return min(d.groupName, envGroupMaxWidth, max(width/3, 8), max(width-8, 0))
}

func envStartsGroup(m list.Model, index int, group string) bool {
	items := m.VisibleItems()
	start, _ := m.Paginator.GetSliceBounds(len(items))
	if index <= start {
		return true
	}
	prev, ok := items[index-1].(envItem)
	return !ok || !strings.EqualFold(prev.group, group)
}
