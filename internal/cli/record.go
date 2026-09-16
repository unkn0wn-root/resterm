package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/bodyfmt"
	"github.com/unkn0wn-root/resterm/internal/bytesize"
	"github.com/unkn0wn-root/resterm/internal/recorder"
)

type RecordFlags struct {
	Config recorder.Config

	maxBytes  string
	bodyLimit string
}

func NewRecordFlags() RecordFlags {
	return RecordFlags{
		Config:    recorder.DefaultConfig(),
		maxBytes:  byteSizeText(recorder.DefaultBytes),
		bodyLimit: byteSizeText(recorder.DefaultBodyLimit),
	}
}

func byteSizeText(n int64) string {
	return strings.ReplaceAll(bodyfmt.FormatByteSize(n), " ", "")
}

func (f *RecordFlags) Bind(fs *FlagSet) {
	fs.StringVarAliases(&f.Config.Listen, f.Config.Listen, "Local HTTP listen address", "listen")
	fs.StringVarAliases(&f.Config.Upstream, "", "HTTP(S) server to forward to, without a path (required)", "upstream")
	fs.IntVarAliases(&f.Config.MaxEntries, f.Config.MaxEntries, "Maximum recordings kept in memory", "max-entries")
	fs.StringVarAliases(
		&f.maxBytes,
		f.maxBytes,
		"Byte limit for stored recordings and for capture buffers",
		"max-bytes",
	)
	fs.StringVarAliases(
		&f.bodyLimit,
		f.bodyLimit,
		"Maximum request or response body size, including after decoding",
		"body-limit",
	)
	fs.IntVarAliases(
		&f.Config.CaptureConcurrency,
		f.Config.CaptureConcurrency,
		"Maximum simultaneous captures",
		"capture-concurrency",
	)
	fs.StringListVarAliases(&f.Config.RedactHeaders, "Additional secret header (repeatable)", "redact-header")
	fs.StringListVarAliases(
		&f.Config.RedactFields,
		"Additional secret query, form, or JSON field (repeatable)",
		"redact-field",
	)
}

func (f RecordFlags) Resolve() (recorder.Config, error) {
	c := f.Config
	var err error
	if c.MaxBytes, err = parseByteLimit("max-bytes", f.maxBytes); err != nil {
		return c, err
	}
	if c.BodyLimit, err = parseByteLimit("body-limit", f.bodyLimit); err != nil {
		return c, err
	}
	// Reject explicit zero limits here. Config.Resolve treats zero as unset.
	if c.MaxEntries <= 0 || c.CaptureConcurrency <= 0 {
		return c, errors.New("recording entry and concurrency limits must be positive")
	}
	return c.Resolve()
}

func parseByteLimit(name, raw string) (int64, error) {
	n, err := bytesize.Parse(raw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("--%s requires a positive byte size", name)
	}
	return n, nil
}
