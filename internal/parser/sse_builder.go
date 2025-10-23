package parser

import (
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type sseBuilder struct {
	enabled bool
	options restfile.SSEOptions
}

func newSSEBuilder() *sseBuilder {
	return &sseBuilder{}
}

func (b *sseBuilder) HandleDirective(key, rest string) bool {
	if !strings.EqualFold(key, "sse") {
		return false
	}

	trimmed := strings.TrimSpace(rest)
	if trimmed == "" {
		b.enabled = true
		return true
	}

	lowered := strings.ToLower(trimmed)
	switch lowered {
	case "0", "false", "off", "disable", "disabled":
		b.enabled = false
		b.options = restfile.SSEOptions{}
		return true
	}

	b.enabled = true
	assignments := parseOptionTokens(trimmed)
	for key, value := range assignments {
		b.applyOption(key, value)
	}
	return true
}

func (b *sseBuilder) applyOption(name, value string) {
	switch strings.ToLower(name) {
	case "duration", "timeout":
		if dur, err := time.ParseDuration(value); err == nil {
			if dur < 0 {
				return
			}
			b.options.TotalTimeout = dur
		}
	case "idle", "idle-timeout":
		if dur, err := time.ParseDuration(value); err == nil {
			if dur < 0 {
				return
			}
			b.options.IdleTimeout = dur
		}
	case "max-events":
		if n, err := parsePositiveInt(value); err == nil {
			b.options.MaxEvents = n
		}
	case "max-bytes", "limit-bytes":
		if size, err := parseByteSize(value); err == nil {
			b.options.MaxBytes = size
		}
	}
}

func (b *sseBuilder) Finalize() (*restfile.SSERequest, bool) {
	if !b.enabled {
		return nil, false
	}
	req := &restfile.SSERequest{Options: b.options}
	return req, true
}
