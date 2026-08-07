package sse

import (
	"errors"
	"fmt"
	"time"

	"github.com/unkn0wn-root/resterm/internal/bytesize"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/duration"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

type Builder struct {
	enabled bool
	options restfile.SSEOptions
}

func New() *Builder {
	return &Builder{}
}

func (b *Builder) HandleDirective(name directive.Name, rest string) (bool, error) {
	if name != directive.SSE {
		return false, nil
	}

	rest = str.Trim(rest)
	if rest == "" {
		b.enabled = true
		return true, nil
	}

	if directive.IsOff(rest) {
		b.enabled = false
		b.options = restfile.SSEOptions{}
		return true, nil
	}

	b.enabled = true
	assignments, err := directive.ParseOptions(directive.SSE, rest)
	errs := []error{err}
	for _, group := range sseAliases {
		assignments.Aliases(group...)
	}
	for _, key := range assignments.Keys() {
		if err := b.applyOption(key, assignments.Get(key)); err != nil {
			errs = append(errs, err)
		}
	}
	errs = append(errs, assignments.Conflicts(directive.SSE))
	return true, errors.Join(errs...)
}

var sseAliases = [][]string{
	{"duration", "timeout"},
	{"idle", "idle-timeout"},
	{"max-bytes", "limit-bytes"},
}

// ParseOptions lowercases keys, so name is already normalized.
func (b *Builder) applyOption(name, value string) error {
	switch name {
	case "duration", "timeout":
		dur, err := sseDuration(name, value)
		if err != nil {
			return err
		}
		b.options.TotalTimeout = dur
	case "idle", "idle-timeout":
		dur, err := sseDuration(name, value)
		if err != nil {
			return err
		}
		b.options.IdleTimeout = dur
	case "max-events":
		n, err := directive.ParseNonNegativeInt(value)
		if err != nil {
			return fmt.Errorf("invalid @sse %s %q: %w", name, value, err)
		}
		b.options.MaxEvents = n
	case "max-bytes", "limit-bytes":
		size, err := bytesize.Parse(value)
		if err != nil {
			return fmt.Errorf("invalid @sse %s %q: %w", name, value, err)
		}
		b.options.MaxBytes = size
	default:
		return directive.UnknownOption(directive.SSE, name)
	}
	return nil
}

func sseDuration(name, value string) (time.Duration, error) {
	dur, ok := duration.Parse(value)
	if !ok || dur < 0 {
		return 0, fmt.Errorf("invalid @sse %s %q: expected a non-negative duration", name, value)
	}
	return dur, nil
}

func (b *Builder) Finalize() (*restfile.SSERequest, bool) {
	if !b.enabled {
		return nil, false
	}
	req := &restfile.SSERequest{Options: b.options}
	return req, true
}
