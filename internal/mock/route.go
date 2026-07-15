package mock

import (
	"fmt"
	"net/http"
	"strconv"
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
	name     string
	def      bool
	latency  time.Duration
	match    restfile.MockMatch // only feeds the digest, matching uses matchers
	matchers []matcher
	status   int
	headers  http.Header
	body     []byte
	fixture  string
	src      loc
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

	v, err := rt.pick(r)
	if err != nil {
		event.Error = err.detail
		writeProblem(w, err.status, err.detail)
		return
	}
	event.Matched = true
	event.Scenario = v.name
	event.Source = v.src.String()

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

	hdr := w.Header()
	for name, values := range v.headers {
		hdr.Del(name)
		for _, val := range values {
			hdr.Add(name, val)
		}
	}
	if r.Method == http.MethodHead && restfile.ResponseAllowsBody(v.status) {
		hdr.Set("Content-Length", strconv.Itoa(len(v.body)))
	}
	w.WriteHeader(v.status)
	if r.Method == http.MethodHead || !restfile.ResponseAllowsBody(v.status) || len(v.body) == 0 {
		return
	}
	if _, err := w.Write(v.body); err != nil {
		event.Error = err.Error()
	}
}
