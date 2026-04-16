package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type RunRequestChoice struct {
	Line  int
	Label string
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
		out = append(out, RunRequestChoice{
			Line:  reqLine(req),
			Label: runRequestLabel(req),
		})
	}
	return out
}

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
) (RunRequestChoice, error) {
	if len(choices) == 0 {
		return RunRequestChoice{}, errors.New("no requests found")
	}
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
		raw := strings.TrimSpace(sc.Text())
		n, err := strconv.Atoi(raw)
		if err == nil && n >= 1 && n <= len(choices) {
			return choices[n-1], nil
		}
		if _, err := fmt.Fprintf(w, "Enter a number between 1 and %d.\n", len(choices)); err != nil {
			return RunRequestChoice{}, err
		}
	}
}

func runRequestLabel(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	method := strings.TrimSpace(engine.ReqMethod(req))
	name := strings.TrimSpace(req.Metadata.Name)
	if name == "" {
		name = strings.TrimSpace(engine.ReqTarget(req))
	}
	if name == "" {
		name = "<unnamed>"
	}
	if method == "" {
		return name
	}
	return method + " " + name
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
