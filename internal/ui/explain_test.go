package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestRedactExplainReportMasksSecretsAndSensitiveHeaders(t *testing.T) {
	t.Parallel()

	model := New(Config{})
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com?token=top-secret",
		Variables: []restfile.Variable{
			{Name: "token", Value: "top-secret", Secret: true},
		},
	}
	rep := &xplain.Report{
		Name:     "top-secret request",
		Method:   "GET",
		URL:      "https://example.com?token=top-secret",
		Decision: "using top-secret",
		Failure:  "top-secret failed",
		Vars: []xplain.Var{
			{Name: "token", Source: "request", Value: "top-secret"},
		},
		Stages: []xplain.Stage{
			{
				Name:    "auth",
				Status:  xplain.StageOK,
				Summary: "set top-secret auth",
				Changes: []xplain.Change{
					{Field: "header.authorization", Before: "", After: "Bearer top-secret"},
				},
				Notes: []string{"note top-secret"},
			},
		},
		Final: &xplain.Final{
			Method: "GET",
			URL:    "https://example.com?token=top-secret",
			Headers: []xplain.Header{
				{Name: "Authorization", Value: "Bearer top-secret"},
				{Name: "X-Trace", Value: "top-secret"},
			},
			Body:     "top-secret",
			BodyNote: "body top-secret",
			Settings: []xplain.Pair{
				{Key: "cert", Value: "top-secret"},
			},
			Route: &xplain.Route{
				Kind:    "ssh",
				Summary: "top-secret host",
				Notes:   []string{"note top-secret"},
			},
		},
		Warnings: []string{"warn top-secret"},
	}

	got := model.redactExplainReport(rep, "", req)
	mask := maskSecret("", true)

	if got.Final == nil {
		t.Fatalf("expected final report section")
	}
	if got.Final.Headers[0].Value != mask {
		t.Fatalf("expected authorization header to be masked, got %q", got.Final.Headers[0].Value)
	}
	if got.Final.Headers[1].Value != mask {
		t.Fatalf("expected secret header value to be redacted, got %q", got.Final.Headers[1].Value)
	}

	out := renderExplainReport(got)
	if strings.Contains(out, "top-secret") {
		t.Fatalf("expected rendered explain output to redact secrets, got %q", out)
	}
	if !strings.Contains(out, mask) {
		t.Fatalf("expected rendered explain output to include mask, got %q", out)
	}
}

func TestPaneContentBaseForExplainRendersReport(t *testing.T) {
	t.Parallel()

	model := New(Config{})
	model.responsePanes[responsePanePrimary].snapshot = &responseSnapshot{
		ready: true,
		explain: explainState{
			report: &xplain.Report{
				Status:   xplain.StatusReady,
				Method:   "GET",
				URL:      "https://example.com",
				Decision: "Request prepared",
			},
		},
	}

	out, tab := model.paneContentBaseForTab(responsePanePrimary, responseTabExplain)
	if tab != responseTabExplain {
		t.Fatalf("expected explain tab, got %v", tab)
	}
	if !strings.Contains(out, "Summary") {
		t.Fatalf("expected explain content to render summary, got %q", out)
	}
	if !strings.Contains(out, "Result: ready") {
		t.Fatalf("expected explain content to render result summary, got %q", out)
	}
	if !strings.Contains(out, "Decision") {
		t.Fatalf("expected explain content to render decision, got %q", out)
	}
	if model.responsePanes[responsePanePrimary].snapshot.explain.plain == "" {
		t.Fatalf("expected explain content to be cached on snapshot")
	}
}

func TestExplainReqChangesPreserveDisplayCaseForSettingsAndVars(t *testing.T) {
	t.Parallel()

	before := &restfile.Request{
		Settings: map[string]string{"RequestTimeout": "5s"},
		Variables: []restfile.Variable{
			{Name: "AuthToken", Value: "old"},
		},
	}
	after := &restfile.Request{
		Settings: map[string]string{"requesttimeout": "10s"},
		Variables: []restfile.Variable{
			{Name: "authtoken", Value: "new"},
		},
	}

	changes := explainReqChanges(before, after)
	fields := make(map[string]xplain.Change, len(changes))
	for _, change := range changes {
		fields[change.Field] = change
	}

	if _, ok := fields["setting.RequestTimeout"]; !ok {
		t.Fatalf("expected setting change to preserve display case, got %#v", changes)
	}
	if _, ok := fields["var.AuthToken"]; !ok {
		t.Fatalf("expected variable change to preserve display case, got %#v", changes)
	}
}

func TestRenderExplainReportUsesCompactOperatorLayout(t *testing.T) {
	t.Parallel()

	rep := &xplain.Report{
		Name:     "CreateUser",
		Method:   "POST",
		URL:      "https://example.com/source",
		Env:      "prod",
		Status:   xplain.StatusReady,
		Decision: "Request sent",
		Vars: []xplain.Var{
			{Name: "token", Source: "request", Value: "masked", Shadowed: []string{"env"}, Uses: 2},
			{Name: "$timestamp", Source: "dynamic", Value: "1710000000", Dynamic: true},
			{Name: "user_id", Missing: true},
		},
		Stages: []xplain.Stage{
			{
				Name:    "@apply",
				Status:  xplain.StageOK,
				Summary: "apply complete",
				Changes: []xplain.Change{
					{
						Field:  "url",
						Before: "https://example.com/source",
						After:  "https://example.com/final",
					},
					{
						Field:  "header.x-mode",
						Before: "",
						After:  "debug",
					},
				},
			},
			{
				Name:    "condition",
				Status:  xplain.StageSkipped,
				Summary: "condition blocked request",
			},
		},
		Final: &xplain.Final{
			Mode:     "sent",
			Method:   "POST",
			URL:      "https://example.com/final",
			Settings: []xplain.Pair{{Key: "insecure", Value: "false"}, {Key: "timeout", Value: "5s"}},
			Headers: []xplain.Header{
				{Name: "Authorization", Value: "****"},
				{Name: "X-Mode", Value: "debug"},
			},
			Body:     "{\"ok\":true}",
			BodyNote: "json body",
			Route: &xplain.Route{
				Kind:    "ssh",
				Summary: "root@bastion:22",
				Notes:   []string{"profile=bastion"},
			},
		},
		Warnings: []string{"strict_hostkey=false"},
	}

	out := renderExplainReport(rep)

	want := []string{
		"Result: sent",
		"Request: CreateUser",
		"Source: POST https://example.com/source",
		"Final: POST https://example.com/final",
		"Route: ssh via root@bastion:22",
		"Pipeline: 1 ok, 1 skipped",
		"Variables: 2 resolved, 1 missing, 1 dynamic",
		"Apply [ok]: Applied request mutations (2 change(s))",
		"Condition [skipped]: Condition skipped this request",
		"   - change url: https://example.com/source -> https://example.com/final",
		"   - set header X-Mode = debug",
		"- token <- request x2",
		"- user_id <- missing",
		"Settings: insecure=false, timeout=5s",
		"Body: json body",
		"  {\"ok\":true}",
	}
	for _, s := range want {
		if !strings.Contains(out, s) {
			t.Fatalf("expected explain output to contain %q, got %q", s, out)
		}
	}
}

func TestRenderExplainStyledUsesThemeDecorations(t *testing.T) {
	t.Parallel()

	rep := &xplain.Report{
		Name:     "CreateUser",
		Method:   "POST",
		URL:      "https://example.com/users",
		Env:      "prod",
		Status:   xplain.StatusReady,
		Decision: "Request sent",
		Final: &xplain.Final{
			Mode:   "sent",
			Method: "POST",
			URL:    "https://example.com/users",
			Route:  &xplain.Route{Kind: "ssh", Summary: "root@bastion:22"},
		},
		Stages: []xplain.Stage{{Name: "@apply", Status: xplain.StageOK, Summary: "applied"}},
		Vars:   []xplain.Var{{Name: "token", Source: "request", Value: "masked"}},
	}

	out := renderExplainStyled(rep, 80, theme.DefaultTheme())
	if out == renderExplainReport(rep) {
		t.Fatalf("expected styled explain output to differ from plain report, got %q", out)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "SUMMARY") {
		t.Fatalf("expected stripped output to include section heading, got %q", plain)
	}
	if !strings.Contains(plain, "Final: POST https://example.com/users") {
		t.Fatalf("expected stripped output to include final request line, got %q", plain)
	}
	if !strings.Contains(plain, "Apply") || strings.Contains(plain, "01 @apply") {
		t.Fatalf("expected stripped output to use display stage labels, got %q", plain)
	}
}

func TestSyncResponsePaneExplainUsesStyledRenderer(t *testing.T) {
	t.Parallel()

	model := New(Config{})
	model.ready = true
	model.width = 120
	model.height = 40
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	pane := &model.responsePanes[responsePanePrimary]
	pane.activeTab = responseTabExplain
	pane.snapshot = &responseSnapshot{
		id:    "snap-explain",
		ready: true,
		explain: explainState{
			report: &xplain.Report{
				Status: xplain.StatusReady,
				Method: "GET",
				URL:    "https://example.com",
				Final: &xplain.Final{
					Mode:   "sent",
					Method: "GET",
					URL:    "https://example.com",
				},
			},
		},
	}

	if cmd := model.syncResponsePane(responsePanePrimary); cmd != nil {
		collectMsgs(cmd)
	}

	view := pane.viewport.View()
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "SUMMARY") {
		t.Fatalf("expected explain viewport to use styled summary layout, got %q", plain)
	}
	if !strings.Contains(plain, "FINAL REQUEST") {
		t.Fatalf("expected explain viewport to include final request section, got %q", ansi.Strip(view))
	}
}

func TestSyncResponsePaneExplainFallsBackToPlainWhileSearching(t *testing.T) {
	t.Parallel()

	model := New(Config{})
	model.ready = true
	model.width = 120
	model.height = 40
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	pane := &model.responsePanes[responsePanePrimary]
	pane.activeTab = responseTabExplain
	pane.search.query = "example"
	pane.snapshot = &responseSnapshot{
		id:    "snap-explain-search",
		ready: true,
		explain: explainState{
			report: &xplain.Report{
				Status:   xplain.StatusReady,
				Method:   "GET",
				URL:      "https://example.com",
				Decision: "Request prepared",
			},
		},
	}

	if cmd := model.syncResponsePane(responsePanePrimary); cmd != nil {
		collectMsgs(cmd)
	}

	view := pane.viewport.View()
	if strings.Contains(view, "\x1b[") {
		t.Fatalf("expected search mode to use plain explain rendering, got %q", view)
	}
	if !strings.Contains(view, "Decision") {
		t.Fatalf("expected plain explain content during search, got %q", view)
	}
}

func TestRenderExplainReportNormalizesStageLabelsForUsers(t *testing.T) {
	t.Parallel()

	rep := &xplain.Report{
		Stages: []xplain.Stage{
			{Name: "route", Status: xplain.StageOK, Summary: "direct", Notes: []string{"direct connection"}},
			{Name: "settings", Status: xplain.StageOK, Summary: "effective settings merged"},
		},
	}

	out := renderExplainReport(rep)
	if strings.Contains(out, "1. route") || strings.Contains(out, "2. settings") {
		t.Fatalf("expected stage numbering to be removed from plain report, got %q", out)
	}
	if !strings.Contains(out, "Route [ok]: Direct connection") {
		t.Fatalf("expected route stage to use user-facing wording, got %q", out)
	}
	if !strings.Contains(out, "Settings [ok]: Merged environment, file, and request settings") {
		t.Fatalf("expected settings stage to use user-facing wording, got %q", out)
	}
}

func TestRenderExplainStyledWrapsCleanlyInNarrowWidth(t *testing.T) {
	t.Parallel()

	rep := &xplain.Report{
		Name:   "LongRequest",
		Method: "GET",
		URL:    "https://example.com/api/v1/resources/with/a/very/long/path",
		Status: xplain.StatusReady,
		Final: &xplain.Final{
			Mode:   "sent",
			Method: "GET",
			URL:    "https://example.com/api/v1/resources/with/a/very/long/path",
			Settings: []xplain.Pair{
				{Key: "timeout", Value: "30s"},
				{Key: "proxy", Value: "http://proxy.internal:8080"},
			},
		},
	}

	out := ansi.Strip(renderExplainStyled(rep, 44, theme.DefaultTheme()))
	if !strings.Contains(out, "SUMMARY") || !strings.Contains(out, "FINAL REQUEST") {
		t.Fatalf("expected narrow explain output to preserve section structure, got %q", out)
	}
	if !strings.Contains(out, "proxy.internal:8080") {
		t.Fatalf("expected narrow explain output to preserve wrapped content, got %q", out)
	}
}

func TestApplyResponseSearchOnExplainUsesPlainContent(t *testing.T) {
	t.Parallel()

	model := New(Config{})
	model.ready = true
	model.width = 120
	model.height = 40
	model.responsePaneFocus = responsePanePrimary
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane")
	}
	pane.activeTab = responseTabExplain
	pane.viewport.Width = 80
	pane.snapshot = &responseSnapshot{
		id:    "snap-explain-search-content",
		ready: true,
		explain: explainState{
			report: &xplain.Report{
				Status: xplain.StatusReady,
				Stages: []xplain.Stage{
					{Name: "route", Status: xplain.StageOK, Summary: "direct"},
				},
			},
		},
	}

	status := statusFromCmd(t, model.applyResponseSearch("Direct connection", false))
	if status == nil {
		t.Fatal("expected search status")
	}
	if !pane.search.active || len(pane.search.matches) == 0 {
		t.Fatalf("expected explain search matches, got active=%v matches=%d", pane.search.active, len(pane.search.matches))
	}
}

func TestSyncResponsePaneExplainCachesStyledOutputByWidthAndTheme(t *testing.T) {
	t.Parallel()

	model := New(Config{})
	model.ready = true
	model.width = 120
	model.height = 40
	model.activeThemeKey = "default"
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane")
	}
	pane.activeTab = responseTabExplain
	pane.snapshot = &responseSnapshot{
		id:    "snap-explain-cache",
		ready: true,
		explain: explainState{
			report: &xplain.Report{
				Status: xplain.StatusReady,
				Method: "GET",
				URL:    "https://example.com",
				Final: &xplain.Final{
					Mode:   "sent",
					Method: "GET",
					URL:    "https://example.com",
				},
			},
		},
	}

	if cmd := model.syncResponsePane(responsePanePrimary); cmd != nil {
		collectMsgs(cmd)
	}
	snap := pane.snapshot
	if snap.explain.cache.styled == "" {
		t.Fatalf("expected styled explain cache to be populated")
	}
	if snap.explain.cache.width == 0 || snap.explain.cache.themeKey != "default" {
		t.Fatalf(
			"expected explain cache key fields to be populated, got width=%d theme=%q",
			snap.explain.cache.width,
			snap.explain.cache.themeKey,
		)
	}
	first := snap.explain.cache.styled

	model.activeThemeKey = "nightshift"
	if cmd := model.syncResponsePane(responsePanePrimary); cmd != nil {
		collectMsgs(cmd)
	}
	if snap.explain.cache.themeKey != "nightshift" {
		t.Fatalf("expected explain cache to update for theme, got %q", snap.explain.cache.themeKey)
	}
	if snap.explain.cache.styled == "" || first == "" {
		t.Fatalf("expected styled explain cache to remain populated")
	}
}

func TestSyncResponsePanesRendersExplainInSplitView(t *testing.T) {
	t.Parallel()

	model := New(Config{})
	model.ready = true
	model.width = 140
	model.height = 40
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}
	if cmd := model.toggleResponseSplitVertical(); cmd != nil {
		collectMsgs(cmd)
	}

	left := model.pane(responsePanePrimary)
	right := model.pane(responsePaneSecondary)
	if left == nil || right == nil {
		t.Fatal("expected both response panes")
	}
	left.activeTab = responseTabExplain
	right.activeTab = responseTabExplain
	left.snapshot = &responseSnapshot{
		id:    "left-explain",
		ready: true,
		explain: explainState{
			report: &xplain.Report{
				Status: xplain.StatusReady,
				Method: "GET",
				URL:    "https://l.test/a",
				Final:  &xplain.Final{Mode: "sent", Method: "GET", URL: "https://l.test/a"},
			},
		},
	}
	right.snapshot = &responseSnapshot{
		id:    "right-explain",
		ready: true,
		explain: explainState{
			report: &xplain.Report{
				Status: xplain.StatusReady,
				Method: "GET",
				URL:    "https://r.test/b",
				Final:  &xplain.Final{Mode: "sent", Method: "GET", URL: "https://r.test/b"},
			},
		},
	}

	if cmd := model.syncResponsePanes(); cmd != nil {
		collectMsgs(cmd)
	}

	leftView := ansi.Strip(left.viewport.View())
	rightView := ansi.Strip(right.viewport.View())
	if !strings.Contains(leftView, "https://l.test") {
		t.Fatalf("expected left explain pane to render its snapshot, got %q", leftView)
	}
	if !strings.Contains(rightView, "https://r.test") {
		t.Fatalf("expected right explain pane to render its snapshot, got %q", rightView)
	}
}
