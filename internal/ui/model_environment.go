package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/vars"
)

const (
	envModalMaxWidth = 84
	envModalMaxBody  = 20
	envModalMinList  = 4
	envLabelGap      = "  "
	envSummaryJoin   = "  •  "
)

func (m *Model) openEnvironmentSelector() {
	m.showEnvSelector = true
	m.closeHelp()
	m.showThemeSelector = false
	m.envDraft = m.ws.sel
	m.envList.ResetFilter()
	m.envList.SetItems(makeEnvItems(m.ws.cat, m.envDraft))
	m.resizeEnvList()
	m.selectActiveEnvironment()
}

func (m *Model) closeEnvironmentSelector() {
	m.showEnvSelector = false
	m.envDraft = vars.Selection{}
	m.envList.ResetFilter()
	m.envList.SetItems(makeEnvItems(m.ws.cat, m.ws.sel))
}

// envSelection is the staged draft while the picker is open, and the applied
// selection otherwise: closing the picker clears the draft.
func (m Model) envSelection() vars.Selection {
	if m.envDraft.Empty() {
		return m.ws.sel
	}
	return m.envDraft
}

func (m *Model) selectActiveEnvironment() {
	for i, item := range m.envList.Items() {
		if env, ok := item.(envItem); ok && env.active {
			m.envList.Select(i)
			return
		}
	}
	m.envList.Select(0)
}

// stageEnvironmentSelection records the highlighted profile in the draft without
// applying it, so a grouped picker can change several groups before Enter.
func (m *Model) stageEnvironmentSelection() tea.Cmd {
	item, ok := m.envList.SelectedItem().(envItem)
	if !ok || item.group == "" {
		return nil
	}

	m.envDraft = m.envSelection().WithGroup(item.group, item.profile)
	cmd := m.envList.SetItems(makeEnvItems(m.ws.cat, m.envDraft))
	m.resizeEnvList()
	return cmd
}

// handleEnvSelectorKey consumes the picker's own bindings and reports whether it
// took the key; everything else falls through to the list.
func (m *Model) handleEnvSelectorKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch key := msg.String(); {
	case key == "ctrl+q" || key == "ctrl+d":
		return tea.Quit, true
	case key == "esc":
		// Esc backs out one layer at a time: the list clears the search, and
		// only an Esc with no search left closes the picker.
		if m.envList.FilterState() != list.Unfiltered {
			return nil, false
		}
		m.closeEnvironmentSelector()
	case m.envList.SettingFilter():
		// The filter input owns every remaining key, Enter and Space included.
		return nil, false
	case key == "enter":
		m.applyEnvironmentSelection()
	case isSpaceKey(msg) && m.ws.cat.Grouped():
		return m.stageEnvironmentSelection(), true
	case key == "?" || key == "shift+/":
		m.toggleHelp()
	default:
		return nil, false
	}
	return nil, true
}

func (m *Model) applyEnvironmentSelection() {
	defer m.closeEnvironmentSelector()

	sel := m.envSelection()
	if !m.ws.cat.Grouped() {
		item, ok := m.envList.SelectedItem().(envItem)
		if !ok {
			return
		}
		var err error
		if sel, err = m.ws.cat.Select(item.name, nil); err != nil {
			m.setStatusMessage(statusMsg{level: statusError, text: err.Error()})
			return
		}
	}

	env, err := m.ws.cat.Resolve(sel)
	if err != nil {
		m.setStatusMessage(statusMsg{level: statusError, text: err.Error()})
		return
	}
	if m.ws.active.Scope() == env.Scope() {
		return
	}

	m.ws.use(env)
	m.latencySeries.reset()
	if gs := m.globalsStore(); gs != nil {
		gs.Clear(env.Scope())
	}
	if fs := m.fileStore(); fs != nil {
		fs.ClearEnv(env.Scope())
	}
	msg := fmt.Sprintf("Environment set to %s", env.Label())
	m.setStatusMessage(statusMsg{level: statusInfo, text: msg})
	m.syncRequestList(m.doc)
	m.syncHistory()
}

// envModalLayout is the modal frame plus the list height left over once the
// summary and the blank line under it are placed.
type envModalLayout struct {
	modalSize
	summary string
	list    int
}

func (m Model) environmentModalLayout() envModalLayout {
	size := m.modalSize(envModalMaxWidth, envModalMaxBody)
	summary := m.renderEnvironmentSummary(size.view)
	return envModalLayout{
		modalSize: size,
		summary:   summary,
		list:      max(size.body-lipgloss.Height(summary)-1, envModalMinList),
	}
}

// resizeEnvList keeps the list in step with the summary, which gains a line
// whenever a staged selection wraps.
func (m *Model) resizeEnvList() {
	if len(m.envList.Items()) == 0 {
		return
	}
	layout := m.environmentModalLayout()
	m.envList.SetSize(layout.view, layout.list)
}

func (m Model) renderEnvironmentModal() string {
	layout := m.environmentModalLayout()
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		layout.summary,
		"",
		m.envList.View(),
	)
	body = lipgloss.NewStyle().
		Padding(0, 2).
		Width(layout.content).
		Render(body)
	return m.renderModalBox("Environments", body, m.renderEnvironmentHints(layout.view), layout.width)
}

func (m Model) renderEnvironmentHints(width int) string {
	hint := m.theme.CommandBarHint.Render
	if !m.ws.cat.Grouped() {
		return fmt.Sprintf("%s Select  %s Search  %s Cancel", hint("Enter"), hint("/"), hint("Esc"))
	}

	stage := fmt.Sprintf("%s Choose  %s Apply", hint("Space"), hint("Enter"))
	leave := fmt.Sprintf("%s Search  %s Cancel", hint("/"), hint("Esc"))
	sep := "  "
	if visibleWidth(stage)+visibleWidth(sep)+visibleWidth(leave) > width {
		sep = "\n"
	}
	return stage + sep + leave
}

func (m Model) renderEnvironmentSummary(width int) string {
	label := "Active"
	if m.ws.cat.Grouped() {
		label = "Selection"
	}

	value := wrapToWidth(
		m.environmentSelectionSummary(),
		max(width-visibleWidth(label)-visibleWidth(envLabelGap), 8),
	)
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.themeRuntime.inputLabelStyle(m.theme).Bold(true).Render(label)+envLabelGap,
		m.theme.HeaderValue.Render(value),
	)
}

func (m Model) environmentSelectionSummary() string {
	if m.ws.unselected {
		return "none selected, " + m.ws.intent.Describe() + " is not available here"
	}
	if !m.ws.cat.Grouped() {
		return m.ws.active.Label()
	}

	choices := m.envChoices(m.envSelection())
	parts := make([]string, 0, len(choices))
	for _, choice := range choices {
		parts = append(parts, choice.group+": "+choice.profile)
	}
	return strings.Join(parts, envSummaryJoin)
}

type envChoice struct {
	group   string
	profile string
}

// envChoices pairs every group with its selected profile in declaration order,
// which is the order the picker lists them in and the order the header trusts
// when it names only the first one.
func (m Model) envChoices(sel vars.Selection) []envChoice {
	groups := m.ws.cat.Groups()
	out := make([]envChoice, 0, len(groups))
	for _, group := range groups {
		profile, _ := sel.Profile(group.Name)
		out = append(out, envChoice{group: group.Name, profile: profile})
	}
	return out
}

// headerEnvVariants renders the active environment for the header, longest
// first. A grouped selection is summarised rather than spelled out: the header
// exists for orientation, while Ctrl+E, the status bar and history entries carry
// the complete selection. The group name stays in the longest form because bare
// profile names like "personal" or "admin" are ambiguous on their own, and every
// form keeps the profile, because a header that reads the same on dev and on
// prod has given up the one thing worth glancing at before sending a request.
func (m Model) headerEnvVariants() []string {
	if m.ws.envErr != nil {
		return []string{"not loaded", "none"}
	}
	if m.ws.unselected {
		return []string{"none selected", "none"}
	}
	label := m.ws.active.Label()
	if label == "" {
		return []string{"default"}
	}
	if !m.ws.cat.Grouped() {
		return []string{label}
	}

	choices := m.envChoices(m.ws.active.Selection())
	first := choices[0]
	rest := ""
	if n := len(choices) - 1; n > 0 {
		rest = fmt.Sprintf(" +%d", n)
	}

	return []string{
		first.group + "=" + first.profile + rest,
		first.profile + rest,
	}
}

// selectEnvironment applies a selection made outside the picker, such as a
// history replay naming its environment.
func (m *Model) selectEnvironment(name string, profiles map[string]string) error {
	sel, err := m.ws.cat.Select(name, profiles)
	if err != nil {
		return err
	}
	env, err := m.ws.cat.Resolve(sel)
	if err != nil {
		return err
	}
	m.ws.use(env)
	return nil
}

func (m *Model) environment(sel vars.Selection) (vars.Environment, error) {
	if sel.Empty() {
		sel = m.ws.sel
	}
	return m.ws.cat.Resolve(sel)
}
