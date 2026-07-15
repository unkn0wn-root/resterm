package mock

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

type Handler struct {
	mux       *http.ServeMux
	routes    int
	scenarios int
	digest    string
	methods   []string
	fixtures  []string
}

func (h *Handler) Routes() int { return h.routes }

func (h *Handler) Scenarios() int { return h.scenarios }

func (h *Handler) Digest() string { return h.digest }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hd, pat := h.lookup(r)
	if pat != "" {
		hd.ServeHTTP(w, r)
		return
	}

	if allowed := h.allowedMethods(r); len(allowed) > 0 {
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		writeProblem(w, http.StatusMethodNotAllowed, "mock route does not support "+r.Method)
		return
	}
	writeProblem(w, http.StatusNotFound, "mock route was not found")
}

func (h *Handler) allowedMethods(r *http.Request) []string {
	pr := r.Clone(r.Context())
	var allowed []string
	for _, m := range h.methods {
		pr.Method = m
		if _, pat := h.lookup(pr); pat == "" {
			continue
		}
		if m == http.MethodGet {
			allowed = append(allowed, http.MethodHead)
		}
		allowed = append(allowed, m)
	}
	slices.Sort(allowed)
	return slices.Compact(allowed)
}

func (h *Handler) hasRoute(r *http.Request) bool {
	_, pat := h.lookup(r)
	return pat != ""
}

func (h *Handler) lookup(r *http.Request) (http.Handler, string) {
	if !cleanPath(r.URL.EscapedPath()) {
		return nil, ""
	}
	hd, pat := h.mux.Handler(r)
	if pat != "" && missingRouteSlash(pat, r.URL.EscapedPath()) {
		return nil, ""
	}
	return hd, pat
}

// ServeMux still reports a trailing-slash pattern for the unslashed path while
// it prepares the 301 redirect. Mock routes are exact, so treat that request as
// unmatched instead of serving the redirect.
func missingRouteSlash(pat, path string) bool {
	if strings.HasSuffix(path, "/") {
		return false
	}
	if strings.HasSuffix(pat, "/{$}") {
		return true
	}
	if !strings.HasSuffix(pat, "...}") {
		return false
	}
	_, p, ok := strings.Cut(pat, " ")
	if !ok {
		return false
	}
	prefix := p[:strings.LastIndexByte(p, '{')]
	return strings.Count(path, "/") == strings.Count(strings.TrimSuffix(prefix, "/"), "/")
}

// ServeMux cleans paths like /users/../users/42 and answers with a redirect.
// Mock routes should reject those outright, so anything not already clean
// counts as no match.
func cleanPath(path string) bool {
	if path == "" || !strings.HasPrefix(path, "/") || strings.Contains(path, "//") {
		return false
	}
	for raw := range strings.SplitSeq(path, "/") {
		seg, err := url.PathUnescape(raw)
		if err != nil || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

func writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Type   string `json:"type"`
		Title  string `json:"title"`
		Status int    `json:"status"`
		Detail string `json:"detail"`
	}{"about:blank", http.StatusText(status), status, detail})
}
