package ui

import (
	"context"

	"github.com/unkn0wn-root/resterm/internal/wrap"
)

const wrapContinuationUnit = wrap.ContinuationUnit

func wrapStructuredContent(content string, width int) string {
	out, _ := wrapStructuredContentCtx(context.Background(), content, width)
	return out
}

func wrapStructuredContentCtx(ctx context.Context, content string, width int) (string, bool) {
	res, ok := wrap.Wrap(ctx, content, width, wrap.Structured, false)
	if !ok {
		return "", false
	}
	return res.S, true
}

func wrapStructuredLine(line string, width int) []string {
	segments, _ := wrapStructuredLineCtx(context.Background(), line, width)
	return segments
}

func wrapStructuredLineCtx(ctx context.Context, line string, width int) ([]string, bool) {
	return wrap.Line(ctx, line, width, wrap.Structured)
}

func detachTrailingANSIPrefix(text string) (string, string) {
	if text == "" {
		return "", ""
	}
	remaining := text
	prefix := ""
	for {
		indices := ansiSequenceRegex.FindAllStringIndex(remaining, -1)
		if len(indices) == 0 {
			break
		}
		last := indices[len(indices)-1]
		if last[1] != len(remaining) {
			break
		}
		code := remaining[last[0]:last[1]]
		prefix = code + prefix
		remaining = remaining[:last[0]]
	}
	return remaining, prefix
}
