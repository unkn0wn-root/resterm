package termcolor

import (
	"fmt"
	"io"
	"strings"

	"github.com/muesli/termenv"
)

type Mode string

const (
	ModeAuto   Mode = "auto"
	ModeAlways Mode = "always"
	ModeNever  Mode = "never"
)

type Lookup func(string) (string, bool)

type Input struct {
	Mode   Mode
	TTY    bool
	Lookup Lookup
}

type Config struct {
	Enabled bool
	Profile termenv.Profile
}

func ParseMode(raw string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(ModeAuto):
		return ModeAuto, nil
	case string(ModeAlways):
		return ModeAlways, nil
	case string(ModeNever):
		return ModeNever, nil
	default:
		return "", fmt.Errorf("unsupported mode %q", raw)
	}
}

func Resolve(in Input) Config {
	mode, err := ParseMode(string(in.Mode))
	if err != nil {
		return Config{}
	}
	switch mode {
	case ModeNever:
		return Config{}
	case ModeAlways:
		p := profile(in.Lookup, true)
		if p == termenv.Ascii {
			p = termenv.ANSI
		}
		return Enabled(p)
	default:
		if !in.TTY || noColor(in.Lookup) || dumbTerm(in.Lookup) {
			return Config{}
		}
		p := profile(in.Lookup, true)
		if p == termenv.Ascii {
			return Config{}
		}
		return Enabled(p)
	}
}

func Enabled(p termenv.Profile) Config {
	if p == termenv.Ascii {
		p = termenv.ANSI
	}
	return Config{Enabled: true, Profile: p}
}

func TrueColor() Config {
	return Enabled(termenv.TrueColor)
}

func (c Config) Formatter() string {
	if !c.Enabled {
		return ""
	}
	switch c.Profile {
	case termenv.TrueColor:
		return "terminal16m"
	case termenv.ANSI256:
		return "terminal256"
	default:
		return "terminal16"
	}
}

func noColor(lookup Lookup) bool {
	_, ok := env(lookup, "NO_COLOR")
	return ok
}

func dumbTerm(lookup Lookup) bool {
	v, ok := env(lookup, "TERM")
	return ok && strings.EqualFold(v, "dumb")
}

func profile(lookup Lookup, tty bool) termenv.Profile {
	out := termenv.NewOutput(
		io.Discard,
		termenv.WithEnvironment(environ{lookup: lookup}),
		termenv.WithTTY(tty),
		termenv.WithProfile(termenv.Ascii),
	)
	return out.ColorProfile()
}

func env(lookup Lookup, key string) (string, bool) {
	if lookup == nil {
		return "", false
	}
	v, ok := lookup(key)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(v), true
}

type environ struct {
	lookup Lookup
}

func (e environ) Environ() []string {
	return nil
}

func (e environ) Getenv(key string) string {
	v, _ := env(e.lookup, key)
	return v
}
