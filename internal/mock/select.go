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

func (rt *route) pick(r *http.Request) (*variant, *problem) {
	name := strings.TrimSpace(r.Header.Get(selectorNameHeader))
	status, err := parseStatus(r.Header.Get(selectorStatusHeader))
	if err != nil {
		return nil, err
	}
	if name != "" {
		return rt.named(name, status)
	}

	vs := rt.variants
	if status != 0 {
		vs = nil
		for _, v := range rt.variants {
			if v.status == status {
				vs = append(vs, v)
			}
		}
		if len(vs) == 0 {
			return nil, &problem{
				http.StatusNotFound,
				fmt.Sprintf("mock status %d was not found for this route", status),
			}
		}
	}

	p := &probe{r: r}
	for _, v := range vs {
		if len(v.matchers) == 0 {
			continue
		}
		ok, err := v.matches(p)
		if err != nil {
			return nil, err
		}
		if ok {
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
	return nil, &problem{http.StatusNotFound, "no mock scenario matched the request"}
}

func (rt *route) named(name string, status int) (*variant, *problem) {
	for _, v := range rt.variants {
		if v.name != name {
			continue
		}
		if status != 0 && v.status != status {
			return nil, &problem{
				status: http.StatusNotFound,
				detail: fmt.Sprintf("mock scenario %q has status %d, not %d", name, v.status, status),
			}
		}
		return v, nil
	}
	return nil, &problem{http.StatusNotFound, fmt.Sprintf("mock scenario %q was not found", name)}
}

func parseStatus(raw string) (int, *problem) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	status, err := strconv.Atoi(raw)
	if err != nil || !restfile.ValidMockStatus(status) {
		return 0, &problem{http.StatusBadRequest, selectorStatusHeader + " must be a status between 200 and 599"}
	}
	return status, nil
}
