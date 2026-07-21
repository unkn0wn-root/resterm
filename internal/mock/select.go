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
	return v.selectResponse(status, p)
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

func (v *variant) selectResponse(status int, p *probe) (selection, *problem) {
	if status != 0 {
		// variantFor already checked that the status exists on this variant
		step, _ := v.stepFor(status)
		return selection{v: v, step: step}, nil
	}

	key, err := v.sequenceKey(p)
	if err != nil {
		return selection{}, err
	}
	step, ok := v.cursor.advance(key, len(v.responses)-1)
	if !ok {
		return selection{}, &problem{
			status: http.StatusTooManyRequests,
			detail: fmt.Sprintf("mock sequence %q reached its sequence-key limit", v.sequence),
		}
	}
	return selection{v: v, step: step}, nil
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
