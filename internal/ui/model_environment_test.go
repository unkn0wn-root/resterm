package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestEnvironmentSelectorRendersItems(t *testing.T) {
	cat := testCatalog(vars.EnvironmentSet{
		"dev":  {"baseUrl": "https://dev"},
		"prod": {"baseUrl": "https://prod"},
	})
	cfg := Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}}

	model := New(cfg)
	model.ready = true
	model.width = 80
	model.height = 24
	model.frameWidth = 80
	model.frameHeight = 24
	model.applyLayout()

	model.openEnvironmentSelector()
	view := model.View()

	if !containsSubstring(view, "dev") || !containsSubstring(view, "prod") {
		t.Fatalf("environment selector should list environments, got view:\n%s", view)
	}
	if model.envList.ShowTitle() {
		t.Fatal("environment list title should be rendered by the modal")
	}
	if listView := ansi.Strip(model.envList.View()); !strings.Contains(listView, "● dev") {
		t.Fatalf("active flat environment is not marked:\n%s", listView)
	}
}

func TestApplyEnvironmentSelectionResetsLatency(t *testing.T) {
	cat := testCatalog(vars.EnvironmentSet{
		"dev":  {"baseUrl": "https://dev"},
		"prod": {"baseUrl": "https://prod"},
	})
	cfg := Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}}

	model := New(cfg)
	model.latencySeries.add(120 * time.Millisecond)
	model.openEnvironmentSelector()
	for i, item := range model.envList.Items() {
		if env, ok := item.(envItem); ok && env.name == "prod" {
			model.envList.Select(i)
		}
	}

	model.applyEnvironmentSelection()
	if _, ok := model.latencySeries.summary(); ok {
		t.Fatal("expected latency series reset on environment switch")
	}
}

func TestGroupedEnvironmentSelectorChangesOneGroup(t *testing.T) {
	cat := groupedEnvironmentCatalog(t)
	model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})
	model.openEnvironmentSelector()

	summary := ansi.Strip(model.renderEnvironmentSummary(52))
	if !strings.Contains(summary, "api: dev") ||
		!strings.Contains(summary, "app: dev app 1") {
		t.Fatalf("active summary does not include the full selection: %q", summary)
	}
	active := 0
	for i, item := range model.envList.Items() {
		env, ok := item.(envItem)
		if !ok {
			continue
		}
		if env.active {
			active++
		}
		if env.group == "app" && env.profile == "dev app 2" {
			model.envList.Select(i)
		}
	}
	if active != 2 {
		t.Fatalf("active rows = %d, want one per group", active)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)
	model.applyEnvironmentSelection()
	env, err := cat.Resolve(model.ws.sel)
	if err != nil {
		t.Fatalf("resolve selection: %v", err)
	}
	if got, want := env.Label(), "api=dev, app=dev app 2"; got != want {
		t.Fatalf("selection = %q, want %q", got, want)
	}
	if got := env.Selection().Groups()["api"]; got != "dev" {
		t.Fatalf("unselected api profile changed to %q", got)
	}
	if !strings.Contains(model.statusMessage.text, env.Label()) {
		t.Fatalf("status = %q, want full selection", model.statusMessage.text)
	}
}

func TestGroupedEnvironmentSelectorStagesMultipleGroups(t *testing.T) {
	cat := groupedEnvironmentCatalog(t)
	model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})
	m := &model
	m.openEnvironmentSelector()

	selectGroupedEnvironmentItem(t, m, "api", "prod")
	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if got := m.environmentSelectionSummary(); !strings.Contains(got, "api: prod") {
		t.Fatalf("Space should stage api=prod, got %q", got)
	}
	if got, _ := m.ws.sel.Profile("api"); got != "dev" {
		t.Fatalf("staging mutated active api profile to %q", got)
	}
	if !m.showEnvSelector {
		t.Fatal("picker closed after staging the first group")
	}

	selectGroupedEnvironmentItem(t, m, "app", "dev app 2")
	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if got := m.environmentSelectionSummary(); got != "api: prod  •  app: dev app 2" {
		t.Fatalf("staged selection = %q", got)
	}
	active := map[string]string{}
	for _, item := range m.envList.Items() {
		env, ok := item.(envItem)
		if ok && env.active {
			active[env.group] = env.profile
		}
	}
	if active["api"] != "prod" || active["app"] != "dev app 2" {
		t.Fatalf("active markers do not reflect staged selection: %#v", active)
	}
	if !m.showEnvSelector {
		t.Fatal("picker closed after staging the second group")
	}

	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.showEnvSelector {
		t.Fatal("picker remained open after applying staged selection")
	}
	if got, want := m.ws.active.Label(), "api=prod, app=dev app 2"; got != want {
		t.Fatalf("applied selection = %q, want %q", got, want)
	}
}

func TestGroupedEnvironmentSelectorCancelsStagedSelection(t *testing.T) {
	cat := groupedEnvironmentCatalog(t)
	model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})
	m := &model
	m.openEnvironmentSelector()

	selectGroupedEnvironmentItem(t, m, "api", "prod")
	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.showEnvSelector {
		t.Fatal("picker remained open after cancel")
	}
	if got, want := m.ws.active.Label(), "api=dev, app=dev app 1"; got != want {
		t.Fatalf("cancel changed active selection to %q, want %q", got, want)
	}

	m.openEnvironmentSelector()
	if got := m.environmentSelectionSummary(); strings.Contains(got, "api: prod") {
		t.Fatalf("cancelled selection remained staged: %q", got)
	}
}

func TestGroupedEnvironmentSelectorStagesFilteredChoice(t *testing.T) {
	cat := groupedEnvironmentCatalog(t)
	model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})
	model.openEnvironmentSelector()
	model.envList.SetFilterText("prod")

	cmd := model.stageEnvironmentSelection()
	if cmd == nil {
		t.Fatal("staging a filtered choice should refresh filter results")
	}
	updated, _ := model.Update(cmd())
	model = updated.(Model)

	items := model.envList.VisibleItems()
	if len(items) != 1 {
		t.Fatalf("visible filtered items = %d, want 1", len(items))
	}
	env, ok := items[0].(envItem)
	if !ok || !env.active || env.group != "api" || env.profile != "prod" {
		t.Fatalf("filtered choice was not staged: %#v", items[0])
	}
}

func TestGroupedEnvironmentSelectorFilterOwnsTypedKeys(t *testing.T) {
	cat := groupedEnvironmentCatalog(t)
	model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})
	m := &model
	m.openEnvironmentSelector()
	selectGroupedEnvironmentItem(t, m, "api", "prod")

	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.envList.SettingFilter() {
		t.Fatal("/ did not activate the environment filter")
	}
	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if got := m.envList.FilterValue(); got != " ?" {
		t.Fatalf("filter value = %q, want the typed keys", got)
	}
	if got := m.environmentSelectionSummary(); strings.Contains(got, "api: prod") {
		t.Fatalf("space in the filter staged a profile: %q", got)
	}
	if m.showHelp {
		t.Fatal("? in the filter opened help")
	}

	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.showEnvSelector {
		t.Fatal("Enter applied the environment instead of accepting the filter")
	}
	if m.envList.SettingFilter() {
		t.Fatal("Enter did not accept the active filter")
	}
}

func TestGroupedEnvironmentSelectorRendersHierarchy(t *testing.T) {
	cat := groupedEnvironmentCatalog(t)
	model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})
	model.ready = true
	model.width = 100
	model.height = 30
	model.applyLayout()
	model.openEnvironmentSelector()

	view := ansi.Strip(model.envList.View())
	for _, group := range []string{"api │", "app │"} {
		if got := strings.Count(view, group); got != 1 {
			t.Fatalf("%s group rendered %d times, want once:\n%s", group, got, view)
		}
	}
	for _, want := range []string{"api", "│ ● dev", "│   prod", "app", "dev app 1", "dev app 2"} {
		if !strings.Contains(view, want) {
			t.Fatalf("grouped selector missing %q:\n%s", want, view)
		}
	}
}

func TestGroupedEnvironmentSelectorFilterShowsFullChoice(t *testing.T) {
	cat := groupedEnvironmentCatalog(t)
	model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})
	model.envList.SetSize(60, 10)
	model.envList.SetFilterText("prod")

	view := ansi.Strip(model.envList.View())
	if !strings.Contains(view, "api = prod") {
		t.Fatalf("filtered choice should include its group:\n%s", view)
	}
}

func TestGroupedEnvironmentSelectorRepeatsGroupAtPageBoundary(t *testing.T) {
	cat := groupedEnvironmentCatalog(t)
	model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})
	model.envList.SetSize(40, 4)
	model.envList.Paginator.PerPage = 1
	model.envList.Select(1)

	view := ansi.Strip(model.envList.View())
	if !strings.Contains(view, "api │") || !strings.Contains(view, "prod") {
		t.Fatalf("continued group should be labelled on a new page:\n%s", view)
	}
}

func TestEnvironmentSummaryWrapsLongSelection(t *testing.T) {
	cat, err := vars.NewGroupedCatalog(nil, []vars.Group{
		{
			Name:    "credentials",
			Default: "developer account",
			Profiles: vars.EnvironmentSet{
				"developer account": {"token": "secret"},
			},
		},
		{
			Name:    "application",
			Default: "quality assurance",
			Profiles: vars.EnvironmentSet{
				"quality assurance": {"url": "https://qa"},
			},
		},
	})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})

	view := ansi.Strip(model.renderEnvironmentSummary(28))
	if lipgloss.Height(view) < 2 {
		t.Fatalf("long selection should wrap, got %q", view)
	}
	compact := strings.Join(strings.Fields(view), " ")
	for _, want := range []string{
		"credentials: developer account",
		"application: quality assurance",
	} {
		if !strings.Contains(compact, want) {
			t.Fatalf("wrapped summary missing %q: %q", want, compact)
		}
	}
}

func TestEnvironmentSelectorFitsNarrowTerminal(t *testing.T) {
	cat := groupedEnvironmentCatalog(t)
	model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})
	model.ready = true
	model.width = 42
	model.height = 24
	model.frameWidth = 42
	model.frameHeight = 24
	model.applyLayout()
	model.openEnvironmentSelector()

	view := ansi.Strip(model.renderEnvironmentModal())
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > model.width {
			t.Fatalf("modal line width = %d, want <= %d: %q", got, model.width, line)
		}
	}
	for _, want := range []string{"prod", "Space", "Apply", "Esc", "Cancel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("narrow grouped picker missing %q:\n%s", want, view)
		}
	}
}

// MatchesForItem hands out the list's own slice, so a delegate that shifts it
// in place makes the highlight drift on every repaint.
func TestEnvironmentSelectorFilterHighlightIsStable(t *testing.T) {
	cat := groupedEnvironmentCatalog(t)
	model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})
	model.envList.SetSize(60, 10)
	model.envList.SetFilterText("prod")

	before := slices.Clone(model.envList.MatchesForItem(0))
	if len(before) == 0 {
		t.Fatal("filter produced no rune matches to highlight")
	}
	model.envList.View()
	if got := model.envList.MatchesForItem(0); !slices.Equal(got, before) {
		t.Fatalf("rendering shifted the stored filter matches to %v, want %v", got, before)
	}
}

func TestEnvironmentSelectorEscapeClearsSearchBeforeClosing(t *testing.T) {
	cat := groupedEnvironmentCatalog(t)
	for _, tc := range []struct {
		name   string
		accept bool
	}{
		{name: "while typing"},
		{name: "after accepting", accept: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := New(Config{Env: vars.Config{Catalog: cat, Selection: cat.DefaultSelection()}})
			m := &model
			m.openEnvironmentSelector()

			m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
			m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p', 'r', 'o', 'd'}})
			if tc.accept {
				m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
			}

			m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEsc})
			if !m.showEnvSelector {
				t.Fatal("Esc closed the picker instead of clearing the search")
			}
			if m.envList.SettingFilter() || m.envList.IsFiltered() {
				t.Fatalf("search survived Esc: filter %q", m.envList.FilterValue())
			}

			m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEsc})
			if m.showEnvSelector {
				t.Fatal("second Esc did not close the picker")
			}
		})
	}
}

func groupedEnvironmentCatalog(t *testing.T) vars.Catalog {
	t.Helper()
	cat, err := vars.NewGroupedCatalog(nil, []vars.Group{
		{
			Name:    "api",
			Default: "dev",
			Profiles: vars.EnvironmentSet{
				"dev":  {"api.url": "dev"},
				"prod": {"api.url": "prod"},
			},
		},
		{
			Name:    "app",
			Default: "dev app 1",
			Profiles: vars.EnvironmentSet{
				"dev app 1": {"app.url": "one"},
				"dev app 2": {"app.url": "two"},
			},
		},
	})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	return cat
}

func selectGroupedEnvironmentItem(t *testing.T, m *Model, group, profile string) {
	t.Helper()
	for i, item := range m.envList.Items() {
		env, ok := item.(envItem)
		if ok && env.group == group && env.profile == profile {
			m.envList.Select(i)
			return
		}
	}
	t.Fatalf("environment choice %s=%s not found", group, profile)
}

func containsSubstring(view, substr string) bool {
	return strings.Contains(view, substr)
}
