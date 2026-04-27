package sse

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/duration"
	"github.com/unkn0wn-root/resterm/internal/parser/directive/options"
	dvalue "github.com/unkn0wn-root/resterm/internal/parser/directive/value"
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

func (b *Builder) HandleDirective(key, rest string) bool {
	if !strings.EqualFold(key, "sse") {
		return false
	}

	trimmed := str.Trim(rest)
	if trimmed == "" {
		b.enabled = true
		return true
	}

	lowered := str.LowerTrim(trimmed)
	switch lowered {
	case "0", "false", "off", "disable", "disabled":
		b.enabled = false
		b.options = restfile.SSEOptions{}
		return true
	}

	b.enabled = true
	assignments := options.Parse(trimmed)
	for key, value := range assignments {
		b.applyOption(key, value)
	}
	return true
}

func (b *Builder) applyOption(name, value string) {
	switch str.LowerTrim(name) {
	case "duration", "timeout":
		if dur, ok := duration.Parse(value); ok {
			if dur < 0 {
				return
			}
			b.options.TotalTimeout = dur
		}
	case "idle", "idle-timeout":
		if dur, ok := duration.Parse(value); ok {
			if dur < 0 {
				return
			}
			b.options.IdleTimeout = dur
		}
	case "max-events":
		if n, err := dvalue.ParsePositiveInt(value); err == nil {
			b.options.MaxEvents = n
		}
	case "max-bytes", "limit-bytes":
		if size, err := dvalue.ParseByteSize(value); err == nil {
			b.options.MaxBytes = size
		}
	}
}

func (b *Builder) Finalize() (*restfile.SSERequest, bool) {
	if !b.enabled {
		return nil, false
	}
	req := &restfile.SSERequest{Options: b.options}
	return req, true
}
