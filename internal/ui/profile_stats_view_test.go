package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/analysis"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

func profileTestSnapshots() map[string]core.ProfileSnapshot {
	ms := time.Millisecond
	samples := []time.Duration{433 * ms, 440 * ms, 446 * ms, 452 * ms, 460 * ms, 529 * ms, 753 * ms}
	stats := analysis.ComputeLatencyStats(samples, analysis.DefaultProfilePercentiles(), 10)
	longReason := "HTTP 503 Service Unavailable " + strings.Repeat("upstream timeout ", 20)
	return map[string]core.ProfileSnapshot{
		"running": {
			Progress: core.ProfileProgress{
				Status:     core.ProfileRunning,
				Total:      12,
				Done:       3,
				Warmup:     2,
				WarmupDone: 2,
				Count:      10,
				Measured:   1,
				Passed:     1,
			},
		},
		"pass": {
			Progress: core.ProfileProgress{
				Status: core.ProfilePass, Total: 9, Done: 9, Warmup: 2, WarmupDone: 2, WarmupFailed: 1,
				Count: 7, Measured: 7, Passed: 7, Elapsed: 5120 * ms,
			},
			Window:   3700 * ms,
			Active:   3500 * ms,
			Stats:    stats,
			Failures: []engine.ProfileFailure{{Iteration: 1, Warmup: true, Reason: "HTTP 503", Duration: 20 * ms}},
		},
		"fail": {
			Progress: core.ProfileProgress{
				Status:   core.ProfileFail,
				Total:    3,
				Done:     3,
				Count:    3,
				Measured: 3,
				Passed:   1,
				Failed:   2,
			},
			Stats: analysis.ComputeLatencyStats([]time.Duration{100 * ms}, analysis.DefaultProfilePercentiles(), 10),
			Failures: []engine.ProfileFailure{
				{Iteration: 2, Reason: longReason, StatusCode: 503, Duration: 90 * ms},
				{Iteration: 3, Reason: "Test failed: 状态检查 - 期望 200"},
			},
		},
		"equal": {
			Progress: core.ProfileProgress{
				Status:   core.ProfilePass,
				Total:    3,
				Done:     3,
				Count:    3,
				Measured: 3,
				Passed:   3,
			},
			Stats: analysis.ComputeLatencyStats(
				[]time.Duration{50 * ms, 50 * ms, 50 * ms},
				analysis.DefaultProfilePercentiles(),
				10,
			),
		},
		"canceled": {
			Progress: core.ProfileProgress{
				Status:   core.ProfileCanceled,
				Total:    10,
				Done:     4,
				Count:    10,
				Measured: 4,
				Passed:   4,
			},
			Stats: stats,
		},
		"skipped": {
			Progress:   core.ProfileProgress{Status: core.ProfileSkipped, Total: 10, Count: 10},
			SkipReason: "condition was false",
		},
		"error": {
			Progress: core.ProfileProgress{
				Status:   core.ProfileError,
				Total:    10,
				Done:     1,
				Count:    10,
				Measured: 1,
				Passed:   1,
			},
			Err: errors.New("executor gone"),
		},
		"legacy": {
			Progress: core.ProfileProgress{
				Status:   core.ProfileFail,
				Total:    10,
				Done:     10,
				Count:    10,
				Measured: 10,
				Passed:   9,
				Failed:   1,
			},
			Legacy: true,
		},
	}
}

func profileTestPalettes() map[string]statsPalette {
	custom := defaultStatsPalette()
	custom.Neutral = lipgloss.NewStyle().Foreground(lipgloss.Color("#123456"))
	custom.Warn = lipgloss.NewStyle().Foreground(lipgloss.Color("#654321"))
	return map[string]statsPalette{
		"dark":   defaultStatsPalette(),
		"light":  lightStatsPalette(theme.DefaultTheme()),
		"custom": custom,
	}
}

func TestProfileStatsViewFitsWidth(t *testing.T) {
	titles := []string{"GET getStatus", "POST 注文を作成する 🚀 " + strings.Repeat("very long name ", 10)}
	for name, snap := range profileTestSnapshots() {
		for pname, pal := range profileTestPalettes() {
			for _, title := range titles {
				v := &profileStatsView{title: title, env: "dev", snap: snap}
				for _, width := range []int{40, 80, 120} {
					out := v.render(width, pal, theme.DefaultTheme())
					for i, line := range strings.Split(out, "\n") {
						if w := ansi.StringWidth(line); w > width {
							t.Fatalf(
								"%s/%s width %d line %d is %d cells: %q",
								name,
								pname,
								width,
								i,
								w,
								ansi.Strip(line),
							)
						}
					}
					plain := ansi.Strip(out)
					if !strings.Contains(plain, strings.ToUpper(snap.Progress.Status.String())) {
						t.Fatalf("%s/%s width %d has no status text:\n%s", name, pname, width, plain)
					}
				}
			}
		}
	}
}

func TestProfileStatsViewSections(t *testing.T) {
	snaps := profileTestSnapshots()
	tests := []struct {
		snap  string
		width int
		want  []string
		skip  []string
	}{
		{
			snap:  "running",
			width: 80,
			want:  []string{"Measured", "1/10", "appear when the run ends"},
			skip:  []string{"stddev"},
		},
		{
			snap:  "pass",
			width: 120,
			want: []string{
				"SUCCESS",
				"100%",
				"1.9/s wall",
				"433ms-",
				"Warmup 1: HTTP 503",
				"Warmup warnings 1",
				"5.12s wall",
			},
		},
		{snap: "pass", width: 40, want: []string{"SUCCESS  100%", "P95"}},
		{snap: "fail", width: 80, want: []string{"Run 2: HTTP 503", "Run 3: Test failed", "Failures 2"}},
		{snap: "equal", width: 80, want: []string{"50ms  ███", "100%"}},
		{snap: "canceled", width: 80, want: []string{"Profiling canceled after 4/10 runs"}},
		{snap: "skipped", width: 80, want: []string{"Profiling skipped: condition was false"}},
		{snap: "error", width: 80, want: []string{"executor gone"}},
		{snap: "legacy", width: 80, want: []string{"unavailable for this older entry"}},
	}
	for _, test := range tests {
		v := &profileStatsView{title: "GET getStatus", env: "dev", snap: snaps[test.snap]}
		out := ansi.Strip(v.render(test.width, defaultStatsPalette(), theme.DefaultTheme()))
		for _, want := range test.want {
			if !strings.Contains(out, want) {
				t.Fatalf("%s at %d is missing %q:\n%s", test.snap, test.width, want, out)
			}
		}
		for _, skip := range test.skip {
			if strings.Contains(out, skip) {
				t.Fatalf("%s at %d should not show %q:\n%s", test.snap, test.width, skip, out)
			}
		}
	}
}

func TestProfileStatsViewMarksPercentiles(t *testing.T) {
	v := &profileStatsView{title: "GET getStatus", snap: profileTestSnapshots()["pass"]}
	out := ansi.Strip(v.render(80, defaultStatsPalette(), theme.DefaultTheme()))
	want := map[string]string{"433ms-": "p50", "707ms-": "p95 p99"}
	for _, line := range strings.Split(out, "\n") {
		for row, mark := range want {
			if strings.HasPrefix(line, row) {
				if !strings.HasSuffix(line, "  "+mark) {
					t.Fatalf("row %q = %q, want mark %q", row, line, mark)
				}
				delete(want, row)
			}
		}
	}
	if len(want) > 0 {
		t.Fatalf("rows not found: %v\n%s", want, out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "478ms-") && strings.Contains(line, " p") {
			t.Fatalf("unmarked row got a mark: %q", line)
		}
	}
}

func TestProfileStatsViewColorsBarsByLatency(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	th := theme.DefaultTheme()
	ms := time.Millisecond
	snap := core.ProfileSnapshot{
		Progress: core.ProfileProgress{Status: core.ProfilePass, Count: 3, Measured: 3, Passed: 3},
		Stats:    analysis.ComputeLatencyStats([]time.Duration{100 * ms, 700 * ms, 1500 * ms}, nil, 3),
	}
	v := &profileStatsView{title: "GET slow", snap: snap}
	lines := strings.Split(v.render(80, defaultStatsPalette(), th), "\n")
	want := map[string]time.Duration{"100ms-": 100 * ms, "566ms-": 700 * ms, "1.033s-": 1500 * ms}
	for _, line := range lines {
		for row, d := range want {
			if strings.HasPrefix(ansi.Strip(line), row) {
				sgr, _, _ := strings.Cut(latFg(th, d).Render("█"), "█")
				if !strings.Contains(line, sgr+"█") {
					t.Fatalf("row %q bar is not styled like %s latency: %q", row, d, line)
				}
				delete(want, row)
			}
		}
	}
	if len(want) > 0 {
		t.Fatalf("rows not found: %v", want)
	}
	ok, warn, slow := latFg(th, 100*ms).Render("x"), latFg(th, 700*ms).Render("x"), latFg(th, 1500*ms).Render("x")
	if ok == warn || warn == slow {
		t.Fatalf("latency colors are not distinct: %q %q %q", ok, warn, slow)
	}
}

func TestProfileStatsViewProgressFollowsStatus(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	th := theme.DefaultTheme()
	pal := defaultStatsPalette()
	running := lipgloss.NewStyle().Foreground(th.HeaderValue.GetForeground())
	tests := map[string]lipgloss.Style{
		"running":  running,
		"pass":     pal.Success,
		"fail":     pal.Warn,
		"error":    pal.Warn,
		"canceled": pal.Caution,
	}
	for name, style := range tests {
		v := &profileStatsView{title: "GET getStatus", snap: profileTestSnapshots()[name]}
		found := false
		for _, line := range strings.Split(v.render(80, pal, th), "\n") {
			if !strings.HasPrefix(ansi.Strip(line), "Measured  ") {
				continue
			}
			found = true
			sgr, _, _ := strings.Cut(style.Render("█"), "█")
			if !strings.Contains(line, sgr+"█") {
				t.Fatalf("%s progress bar = %q, want the status color", name, line)
			}
		}
		if !found {
			t.Fatalf("%s has no progress bar", name)
		}
	}
}

func TestProfileStatsViewWideUsesColumns(t *testing.T) {
	v := &profileStatsView{title: "GET getStatus", snap: profileTestSnapshots()["pass"]}
	for _, line := range strings.Split(ansi.Strip(v.render(120, defaultStatsPalette(), theme.DefaultTheme())), "\n") {
		if strings.Contains(line, "LATENCY") {
			if !strings.Contains(line, "FAILURES") {
				t.Fatalf("wide layout put sections on separate rows: %q", line)
			}
			return
		}
	}
	t.Fatal("no LATENCY heading")
}
