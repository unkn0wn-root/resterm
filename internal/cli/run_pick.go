package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

type RunRequestChoice struct {
	Line   int
	Method string
	Name   string
	Target string
	Label  string
}

type runRequestInfo struct {
	method string
	name   string
	target string
	label  string
}

func BuildRunRequestChoices(doc *restfile.Document) []RunRequestChoice {
	if doc == nil || len(doc.Requests) == 0 {
		return nil
	}
	out := make([]RunRequestChoice, 0, len(doc.Requests))
	for _, req := range doc.Requests {
		if req == nil {
			continue
		}
		info := runRequestFields(req)
		out = append(out, RunRequestChoice{
			Line:   reqLine(req),
			Method: info.method,
			Name:   info.name,
			Target: info.target,
			Label:  info.label,
		})
	}
	return out
}

type RunRequestPromptOptions struct {
	TTY   bool
	Color termcolor.Config
}

var ErrRunRequestChoiceCanceled = errors.New("request selection canceled")

func WriteRunRequestChoices(
	w io.Writer,
	path string,
	choices []RunRequestChoice,
) error {
	if w == nil {
		return nil
	}
	if len(choices) == 0 {
		_, err := fmt.Fprintln(w, "No requests found.")
		return err
	}
	if _, err := fmt.Fprintf(w, "Multiple requests found in %s:\n", path); err != nil {
		return err
	}
	for i, ch := range choices {
		if _, err := fmt.Fprintf(w, "  %d. %s", i+1, ch.Label); err != nil {
			return err
		}
		if ch.Line > 0 {
			if _, err := fmt.Fprintf(w, " (line %d)", ch.Line); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

func PromptRunRequestChoice(
	r io.Reader,
	w io.Writer,
	path string,
	choices []RunRequestChoice,
	opt RunRequestPromptOptions,
) (RunRequestChoice, error) {
	if len(choices) == 0 {
		return RunRequestChoice{}, errors.New("no requests found")
	}
	if opt.TTY {
		return promptRunRequestChoiceTTY(r, w, path, choices, opt)
	}
	return promptRunRequestChoiceText(r, w, path, choices)
}

func promptRunRequestChoiceText(
	r io.Reader,
	w io.Writer,
	path string,
	choices []RunRequestChoice,
) (RunRequestChoice, error) {
	if err := WriteRunRequestChoices(w, path, choices); err != nil {
		return RunRequestChoice{}, err
	}
	sc := bufio.NewScanner(r)
	for {
		if _, err := fmt.Fprintf(w, "Select request [1-%d]: ", len(choices)); err != nil {
			return RunRequestChoice{}, err
		}
		if !sc.Scan() {
			if err := sc.Err(); err != nil {
				return RunRequestChoice{}, err
			}
			return RunRequestChoice{}, io.EOF
		}
		raw := str.Trim(sc.Text())
		n, err := strconv.Atoi(raw)
		if err == nil && n >= 1 && n <= len(choices) {
			return choices[n-1], nil
		}
		if _, err := fmt.Fprintf(
			w,
			"Enter a number between 1 and %d.\n",
			len(choices),
		); err != nil {
			return RunRequestChoice{}, err
		}
	}
}

func runRequestFields(req *restfile.Request) runRequestInfo {
	if req == nil {
		return runRequestInfo{}
	}
	info := runRequestInfo{
		method: str.Trim(engine.ReqMethod(req)),
		name:   str.Trim(req.Metadata.Name),
		target: str.Trim(engine.ReqTarget(req)),
	}
	if info.name == "" {
		info.name = info.target
	}
	if info.name == "" {
		info.name = "<unnamed>"
	}
	info.label = info.name
	if info.method != "" {
		info.label = info.method + " " + info.name
	}
	return info
}

func reqLine(req *restfile.Request) int {
	if req == nil {
		return 0
	}
	if req.LineRange.Start > 0 {
		return req.LineRange.Start
	}
	return 0
}
