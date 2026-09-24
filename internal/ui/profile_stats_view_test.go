package ui

import (
	"errors"
	"slices"
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
		"starting": {
			Progress: core.ProfileProgress{Status: core.ProfileRunning, Total: 12, Warmup: 2, Count: 10},
		},
		"running": {
			Progress: core.ProfileProgress{
				Status:     core.ProfileRunning,
				Total:      12,
				Done:       4,
				Warmup:     2,
				WarmupDone: 2,
				Count:      10,
				Measured:   2,
				Passed:     1,
				Failed:     1,
			},
			Stats: analysis.ComputeLatencyStats(
				[]time.Duration{120 * ms},
				analysis.DefaultProfilePercentiles(),
				10,
			),
			StatusCodes: map[int]int{200: 1, 503: 1},
			Failures: []engine.ProfileFailure{
				{Iteration: 4, Reason: "HTTP 503 Service Unavailable", StatusCode: 503},
			},
		},
		"pass": {
			Progress: core.ProfileProgress{
				Status: core.ProfilePass, Total: 9, Done: 9, Warmup: 2, WarmupDone: 2, WarmupFailed: 1,
				Count: 7, Measured: 7, Passed: 7, Elapsed: 5120 * ms,
			},
			Window:      3700 * ms,
			Active:      3500 * ms,
			Stats:       stats,
			StatusCodes: map[int]int{200: 7},
			Failures:    []engine.ProfileFailure{{Iteration: 1, Warmup: true, Reason: "HTTP 503", Duration: 20 * ms}},
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
			Stats: analysis.ComputeLatencyStats(
				[]time.Duration{100 * ms},
				analysis.DefaultProfilePercentiles(),
				10,
			),
			StatusCodes: map[int]int{0: 1, 200: 1, 503: 1},
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

func profileTestBase() *core.ProfileSnapshot {
	samples := slices.Repeat([]time.Duration{400 * time.Millisecond}, 7)
	return &core.ProfileSnapshot{
		Progress: core.ProfileProgress{Status: core.ProfilePass, Count: 7, Measured: 7, Passed: 7},
		Ended:    time.Date(2026, 9, 20, 14, 2, 0, 0, time.Local),
		Stats:    analysis.ComputeLatencyStats(samples, analysis.DefaultProfilePercentiles(), 10),
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
			for _, view := range []profileStatsView{
				{title: titles[0], env: "dev", snap: snap},
				{title: titles[1], env: "dev", snap: snap},
				{title: titles[0], env: "dev", snap: snap, base: profileTestBase()},
			} {
				v := &view
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
			snap:  "starting",
			width: 80,
			want:  []string{"Warmup", "0/2", "No successful measured requests yet."},
			skip:  []string{"stddev", "RESPONSES"},
		},
		{
			snap:  "running",
			width: 80,
			want:  []string{"Measured", "2/10", "120ms", "stddev", "200 ×1 · 503 ×1", "Run 4: HTTP 503"},
		},
		{
			snap:  "pass",
			width: 120,
			want: []string{
				"SUCCESS",
				"100%",
				"P90",
				"1.9/s wall",
				"RESPONSES measured requests",
				"200 ×7",
				"433ms-",
				"Warmup 1: HTTP 503",
				"Warmup warnings 1",
				"5.12s wall",
			},
		},
		{snap: "pass", width: 40, want: []string{"SUCCESS  100%", "P95"}},
		{
			snap:  "fail",
			width: 80,
			want:  []string{"Run 2: HTTP 503", "Run 3: Test failed", "Failures 2", "no response ×1 · 200 ×1 · 503 ×1"},
		},
		{snap: "equal", width: 80, want: []string{"50ms  ███", "100%"}},
		{snap: "canceled", width: 80, want: []string{"Profiling canceled after 4/10 runs"}},
		{snap: "skipped", width: 80, want: []string{"Profiling skipped: condition was false"}},
		{snap: "error", width: 80, want: []string{"executor gone"}},
		{snap: "legacy", width: 80, want: []string{"unavailable for this older entry"}, skip: []string{"RESPONSES"}},
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
	for line := range strings.SplitSeq(out, "\n") {
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
	for line := range strings.SplitSeq(out, "\n") {
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
		Progress: core.ProfileProgress{Status: core.ProfilePass, Count: 4, Measured: 4, Passed: 4},
		Stats:    analysis.ComputeLatencyStats([]time.Duration{100 * ms, 100 * ms, 700 * ms, 1500 * ms}, nil, 3),
	}
	pal := defaultStatsPalette()
	v := &profileStatsView{title: "GET slow", snap: snap}
	lines := strings.Split(v.render(80, pal, th), "\n")
	want := map[string]time.Duration{"100ms-": 100 * ms, "566ms-": 700 * ms, "1.033s-": 1500 * ms}
	track, _, _ := strings.Cut(pal.SubLabel.Render(barGlyphEmpty), barGlyphEmpty)
	for _, line := range lines {
		for row, d := range want {
			if strings.HasPrefix(ansi.Strip(line), row) {
				sgr, _, _ := strings.Cut(latFg(pal, th, d).Render("█"), "█")
				if !strings.Contains(line, sgr+"█") {
					t.Fatalf("row %q bar is not styled like %s latency: %q", row, d, line)
				}
				if d > 100*ms && !strings.Contains(line, track+barGlyphEmpty) {
					t.Fatalf("row %q track is not faint: %q", row, line)
				}
				delete(want, row)
			}
		}
	}
	if len(want) > 0 {
		t.Fatalf("rows not found: %v", want)
	}
	if fg := latFg(pal, th, 100*ms).GetForeground(); fg != pal.Duration.GetForeground() {
		t.Fatalf("fast bar color = %v, want the duration color %v", fg, pal.Duration.GetForeground())
	}
	ok, warn, slow := latFg(pal, th, 100*ms).Render("x"), latFg(pal, th, 700*ms).Render("x"),
		latFg(pal, th, 1500*ms).Render("x")
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
		for line := range strings.SplitSeq(v.render(80, pal, th), "\n") {
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

func TestProfileStatsViewStacksSectionsWhenWide(t *testing.T) {
	v := &profileStatsView{title: "GET getStatus", snap: profileTestSnapshots()["pass"]}
	lines := strings.Split(ansi.Strip(v.render(100, defaultStatsPalette(), theme.DefaultTheme())), "\n")
	latency, failures := -1, -1
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "LATENCY"):
			latency = i
		case strings.HasPrefix(line, "FAILURES"):
			failures = i
		case strings.HasPrefix(line, "min "):
			if !strings.Contains(line, "active") {
				t.Fatalf("summary wrapped at full width: %q", line)
			}
		}
	}
	if latency < 0 || failures <= latency || strings.Contains(lines[latency], "FAILURES") {
		t.Fatalf("FAILURES should follow LATENCY on its own row:\n%s", strings.Join(lines, "\n"))
	}
}

func TestProfileStatsViewComparesWithBase(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	pal := defaultStatsPalette()
	v := &profileStatsView{title: "GET getStatus", snap: profileTestSnapshots()["pass"], base: profileTestBase()}
	out := v.render(120, pal, theme.DefaultTheme())
	plain := ansi.Strip(out)
	for _, want := range []string{"=         +52ms", "+353ms", "vs run at", "7 measured"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("comparison is missing %q:\n%s", want, plain)
		}
	}
	sgr, _, _ := strings.Cut(pal.Warn.Render("x"), "x")
	if !strings.Contains(out, sgr+"+52ms") {
		t.Fatalf("slower p50 is not styled as worse:\n%q", out)
	}
}

func TestProfileStatsViewWrapsFailureReasons(t *testing.T) {
	v := &profileStatsView{title: "GET getStatus", snap: profileTestSnapshots()["fail"]}
	lines := strings.Split(ansi.Strip(v.render(60, defaultStatsPalette(), theme.DefaultTheme())), "\n")
	i := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "Run 2: HTTP 503") })
	if i < 0 || i+3 >= len(lines) {
		t.Fatalf("Run 2 not found:\n%s", strings.Join(lines, "\n"))
	}
	indent := strings.Repeat(" ", len("Run 2: "))
	if !strings.HasPrefix(lines[i+1], indent+"upstream") || !strings.HasPrefix(lines[i+2], indent) {
		t.Fatalf("reason does not wrap under the label:\n%s", strings.Join(lines[i:i+4], "\n"))
	}
	if !strings.HasSuffix(lines[i+2], "…") || !strings.HasPrefix(lines[i+3], "Run 3: ") {
		t.Fatalf("reason is not capped at %d lines:\n%s", profileFailureLines, strings.Join(lines[i:i+4], "\n"))
	}
}
