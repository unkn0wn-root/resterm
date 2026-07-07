package termcolor

import (
	"testing"

	"github.com/muesli/termenv"
)

func TestResolveAutoEnablesTTYColor(t *testing.T) {
	cfg := Resolve(Input{
		Mode:   ModeAuto,
		TTY:    true,
		Lookup: lookup(map[string]string{"TERM": "xterm-256color"}),
	})
	if !cfg.Enabled {
		t.Fatalf("expected auto color to be enabled")
	}
	if cfg.Profile != termenv.ANSI256 {
		t.Fatalf("expected ANSI256 profile, got %v", cfg.Profile)
	}
}

func TestResolveAutoDisablesForNoColor(t *testing.T) {
	cfg := Resolve(Input{
		Mode:   ModeAuto,
		TTY:    true,
		Lookup: lookup(map[string]string{"TERM": "xterm-256color", "NO_COLOR": ""}),
	})
	if cfg.Enabled {
		t.Fatalf("expected NO_COLOR to disable color")
	}
}

func TestResolveAutoDisablesForDumbTerm(t *testing.T) {
	cfg := Resolve(Input{
		Mode:   ModeAuto,
		TTY:    true,
		Lookup: lookup(map[string]string{"TERM": "dumb"}),
	})
	if cfg.Enabled {
		t.Fatalf("expected TERM=dumb to disable color")
	}
}

func TestResolveAlwaysForcesANSI(t *testing.T) {
	cfg := Resolve(Input{
		Mode:   ModeAlways,
		TTY:    false,
		Lookup: lookup(map[string]string{"TERM": "dumb", "NO_COLOR": ""}),
	})
	if !cfg.Enabled {
		t.Fatalf("expected always mode to enable color")
	}
	if cfg.Profile != termenv.ANSI {
		t.Fatalf("expected ANSI fallback, got %v", cfg.Profile)
	}
}

func TestFormatter(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "off", cfg: Config{}, want: ""},
		{name: "ansi", cfg: Enabled(termenv.ANSI), want: "terminal16"},
		{name: "ansi256", cfg: Enabled(termenv.ANSI256), want: "terminal256"},
		{name: "truecolor", cfg: Enabled(termenv.TrueColor), want: "terminal16m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Formatter(); got != tc.want {
				t.Fatalf("Formatter()=%q, want %q", got, tc.want)
			}
		})
	}
}

func lookup(vals map[string]string) Lookup {
	return func(key string) (string, bool) {
		v, ok := vals[key]
		return v, ok
	}
}

func TestConfigPaint(t *testing.T) {
	c := Enabled(termenv.ANSI)
	if got, want := c.Bold("x"), "\x1b[1mx\x1b[0m"; got != want {
		t.Fatalf("Bold=%q, want %q", got, want)
	}
	if got, want := c.Faint("x"), "\x1b[2mx\x1b[0m"; got != want {
		t.Fatalf("Faint=%q, want %q", got, want)
	}
	if got := (Config{}).Bold("x"); got != "x" {
		t.Fatalf("disabled Bold=%q, want plain", got)
	}
	if got := (Config{Enabled: true, Profile: termenv.Ascii}).Bold("x"); got != "\x1b[1mx\x1b[0m" {
		t.Fatalf("Ascii profile not promoted: %q", got)
	}
}
