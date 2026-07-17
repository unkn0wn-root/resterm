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

func (rt *route) pick(r *http.Request, p *probe) (selection, *problem) {
	name := strings.TrimSpace(r.Header.Get(selectorNameHeader))
	status, err := parseStatus(r.Header.Get(selectorStatusHeader))
	if err != nil {
		return selection{}, err
	}
	if name != "" {
		v, selectErr := rt.named(name, status)
		if selectErr != nil {
			return selection{}, selectErr
		}
		return v.selectResponse(status), nil
	}

	vs := rt.variants
	if status != 0 {
		vs = make([]*variant, 0, len(rt.variants))
		for _, v := range rt.variants {
			if v.hasStatus(status) {
				vs = append(vs, v)
			}
		}
		if len(vs) == 0 {
			return selection{}, &problem{
				status: http.StatusNotFound,
				detail: fmt.Sprintf("mock status %d was not found for this route", status),
			}
		}
	}

	for _, v := range vs {
		if len(v.matchers) == 0 {
			continue
		}
		matched, matchErr := v.matches(p)
		if matchErr != nil {
			return selection{}, matchErr
		}
		if matched {
			return v.selectResponse(status), nil
		}
	}
	for _, v := range vs {
		if v.def {
			return v.selectResponse(status), nil
		}
	}
	for _, v := range vs {
		if len(v.matchers) == 0 {
			return v.selectResponse(status), nil
		}
	}
	return selection{}, &problem{
		status: http.StatusNotFound,
		detail: "no mock scenario matched the request",
	}
}

func (rt *route) named(name string, status int) (*variant, *problem) {
	for _, v := range rt.variants {
		if v.name != name {
			continue
		}
		if status != 0 && !v.hasStatus(status) {
			return nil, &problem{
				status: http.StatusNotFound,
				detail: fmt.Sprintf("mock scenario %q has no response with status %d", name, status),
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
		step = v.nextResponse()
	} else {
		for i := range v.responses {
			if v.responses[i].status == status {
				step = i
				break
			}
		}
	}
	return selection{v: v, step: step}
}

func (v *variant) nextResponse() int {
	last := uint64(len(v.responses) - 1)
	for {
		current := v.next.Load()
		if current >= last {
			return int(last)
		}
		if v.next.CompareAndSwap(current, current+1) {
			return int(current)
		}
	}
}

func (v *variant) hasStatus(status int) bool {
	for _, resp := range v.responses {
		if resp.status == status {
			return true
		}
	}
	return false
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
