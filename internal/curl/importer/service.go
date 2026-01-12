package importer

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/curl"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/util"
)

const errWriterNotConfigured = "curlimport: writer not configured"

type DocumentWriter interface {
	WriteDocument(ctx context.Context, doc *restfile.Document, dst string, opts WriterOptions) error
}

type WriterOptions struct {
	OverwriteExisting bool
	HeaderComment     string
}

type Service struct {
	Writer DocumentWriter
}

func (s *Service) GenerateHTTPFile(ctx context.Context, cmd, dst string, opts WriterOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.Writer == nil {
		return errors.New(errWriterNotConfigured)
	}

	doc, warn, err := buildDoc(cmd)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	opts.HeaderComment = buildHeader(opts.HeaderComment, cmd, warn)
	return s.Writer.WriteDocument(ctx, doc, dst, opts)
}

func buildDoc(cmd string) (*restfile.Document, []string, error) {
	cmds := curl.SplitCommands(cmd)
	if len(cmds) == 0 {
		cmds = []string{cmd}
	}
	reqs := []*restfile.Request{}
	var warn []string
	for i, cur := range cmds {
		res, err := curl.ParseCommandsInfo(cur)
		if err != nil {
			return nil, nil, fmt.Errorf("curl command %d: %w", i+1, err)
		}
		for _, item := range res {
			if item.Req != nil {
				reqs = append(reqs, item.Req)
			}
			warn = append(warn, item.Warn...)
		}
	}
	warn = uniqSorted(warn)
	return &restfile.Document{Requests: reqs}, warn, nil
}

func buildHeader(base, cmd string, warn []string) string {
	var lines []string
	lines = appendLines(lines, base)
	lines = append(lines, sourceLines(cmd)...)
	for _, w := range warn {
		if t := strings.TrimSpace(w); t != "" {
			lines = append(lines, "Warning: "+t)
		}
	}
	return strings.Join(lines, "\n")
}

func appendLines(lines []string, raw string) []string {
	for _, line := range strings.Split(raw, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			lines = append(lines, t)
		}
	}
	return lines
}

func sourceLines(cmd string) []string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return nil
	}
	parts := strings.Split(cmd, "\n")
	out := make([]string, 0, len(parts)+1)
	out = append(out, "Source:")
	for _, part := range parts {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func uniqSorted(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if t := strings.TrimSpace(v); t != "" {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return util.DedupeSortedStrings(out)
}
