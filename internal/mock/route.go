package mock

import (
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type loc struct {
	path string
	line int
}

func (l loc) String() string {
	path := l.path
	if path == "" {
		path = "<document>"
	}
	if l.line > 0 {
		return fmt.Sprintf("%s:%d", path, l.line)
	}
	return path
}

type variant struct {
	name       string
	def        bool
	latency    time.Duration
	match      restfile.MockMatch // only feeds the digest, matching uses matchers
	matchers   []matcher
	responses  []response
	pathParams map[string]string
	next       atomic.Uint64
	src        loc
}

type response struct {
	status      int
	headers     http.Header
	body        []byte
	fixture     string
	interpolate bool
}

type route struct {
	method   string
	path     string
	pattern  string
	label    string
	variants []*variant
}

func (rt *route) serveHTTP(w http.ResponseWriter, r *http.Request) {
	event := requestEvent(r)
	if event == nil {
		event = new(Event)
	}
	event.Route = rt.label

	p := &probe{r: r}
	sel, err := rt.pick(r, p)
	if err != nil {
		event.Error = err.detail
		writeProblem(w, err.status, err.detail)
		return
	}
	v := sel.v
	resp := &v.responses[sel.step]
	total := len(v.responses)
	event.Matched = true
	event.Scenario = v.name
	event.Source = v.src.String()
	if total > 1 {
		event.SequenceStep = sel.step + 1
		event.SequenceTotal = total
	}

	if v.latency > 0 {
		timer := time.NewTimer(v.latency)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-r.Context().Done():
			event.Error = "request canceled during mock latency"
			return
		}
	}

	rendered, renderErr := renderMockResponse(v, resp, r, p)
	if renderErr != nil {
		event.Error = renderErr.detail
		writeProblem(w, renderErr.status, renderErr.detail)
		return
	}
	hdr := w.Header()
	for name, values := range rendered.headers {
		hdr.Del(name)
		for _, val := range values {
			hdr.Add(name, val)
		}
	}
	if r.Method == http.MethodHead && restfile.ResponseAllowsBody(resp.status) {
		hdr.Set("Content-Length", strconv.Itoa(len(rendered.body)))
	}
	w.WriteHeader(resp.status)
	if r.Method == http.MethodHead || !restfile.ResponseAllowsBody(resp.status) || len(rendered.body) == 0 {
		return
	}
	if _, err := w.Write(rendered.body); err != nil {
		event.Error = err.Error()
	}
}
