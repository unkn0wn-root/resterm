package mock

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const (
	selectorNameHeader   = "X-Resterm-Mock"
	selectorStatusHeader = "X-Resterm-Mock-Status"
)

func isSelectorHeader(name string) bool {
	return strings.EqualFold(name, selectorNameHeader) ||
		strings.EqualFold(name, selectorStatusHeader)
}

type problem struct {
	status int
	detail string
}

type selection struct {
	v    *variant
	step int
}

func (rt *route) pick(p *probe) (selection, *problem) {
	name := strings.TrimSpace(p.r.Header.Get(selectorNameHeader))
	status, err := parseStatus(p.r.Header.Get(selectorStatusHeader))
	if err != nil {
		return selection{}, err
	}
	v, err := rt.variantFor(name, status, p)
	if err != nil {
		return selection{}, err
	}
	return v.selectResponse(status), nil
}

func (rt *route) variantFor(name string, status int, p *probe) (*variant, *problem) {
	if name != "" {
		return rt.named(name, status)
	}

	vs := rt.variants
	if status != 0 {
		vs = make([]*variant, 0, len(rt.variants))
		for _, v := range rt.variants {
			if _, ok := v.stepFor(status); ok {
				vs = append(vs, v)
			}
		}
		if len(vs) == 0 {
			return nil, &problem{
				status: http.StatusNotFound,
				detail: fmt.Sprintf("mock status %d was not found for this route", status),
			}
		}
	}

	for _, v := range vs {
		if len(v.matchers) == 0 {
			continue
		}
		matched, err := v.matches(p)
		if err != nil {
			return nil, err
		}
		if matched {
			return v, nil
		}
	}
	for _, v := range vs {
		if v.def {
			return v, nil
		}
	}
	for _, v := range vs {
		if len(v.matchers) == 0 {
			return v, nil
		}
	}
	return nil, &problem{
		status: http.StatusNotFound,
		detail: "no mock scenario matched the request",
	}
}

func (rt *route) named(name string, status int) (*variant, *problem) {
	for _, v := range rt.variants {
		if v.name != name {
			continue
		}
		if status != 0 {
			if _, ok := v.stepFor(status); !ok {
				return nil, &problem{
					status: http.StatusNotFound,
					detail: fmt.Sprintf("mock scenario %q has no response with status %d", name, status),
				}
			}
		}
		return v, nil
	}
	return nil, &problem{
		status: http.StatusNotFound,
		detail: fmt.Sprintf("mock scenario %q was not found", name),
	}
}

func (v *variant) selectResponse(status int) selection {
	step := 0
	if status == 0 {
		step = v.advance()
	} else if i, ok := v.stepFor(status); ok {
		step = i
	}
	return selection{v: v, step: step}
}

// advance returns the next sequence step, sticking at the final response once
// the sequence is exhausted.
func (v *variant) advance() int {
	last := len(v.responses) - 1
	if n := v.next.Add(1) - 1; n < uint64(last) {
		return int(n)
	}
	return last
}

func (v *variant) stepFor(status int) (int, bool) {
	for i, resp := range v.responses {
		if resp.status == status {
			return i, true
		}
	}
	return 0, false
}

func parseStatus(raw string) (int, *problem) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	status, err := strconv.Atoi(raw)
	if err != nil || !restfile.ValidMockStatus(status) {
		return 0, &problem{
			status: http.StatusBadRequest,
			detail: selectorStatusHeader + " must be a status between 200 and 599",
		}
	}
	return status, nil
}
