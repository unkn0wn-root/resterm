package mock

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"golang.org/x/net/http/httpguts"
)

func ResolveCORS(raw, addr string) (CORS, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "auto"
	}
	switch strings.ToLower(raw) {
	case "auto":
		if IsLoopbackAddr(addr) {
			return WildcardCORS(), "", nil
		}
		return CORS{}, "CORS is disabled because the mock server is not bound to loopback", nil
	case "off", "false", "none":
		return CORS{}, "", nil
	case "*":
		return WildcardCORS(), "", nil
	}

	var origins []string
	for field := range strings.SplitSeq(raw, ",") {
		origin := strings.TrimSpace(field)
		if origin == "" {
			continue
		}
		norm, ok := normOrigin(origin)
		if !ok {
			return CORS{}, "", fmt.Errorf("invalid CORS origin %q", origin)
		}
		if !slices.Contains(origins, norm) {
			origins = append(origins, norm)
		}
	}
	if len(origins) == 0 {
		return CORS{}, "", fmt.Errorf("CORS must be auto, off, *, or an origin list")
	}
	return CORS{Enabled: true, Origins: origins}, "", nil
}

func normOrigin(raw string) (string, bool) {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return "", false
	}
	if u.Opaque != "" || u.User != nil {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	if u.Host == "" {
		return "", false
	}
	if strings.ContainsAny(raw, "?#") {
		return "", false
	}
	if path := u.EscapedPath(); path != "" && path != "/" {
		return "", false
	}
	return scheme + "://" + strings.ToLower(u.Host), true
}

func IsLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return false
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) handleCORS(w http.ResponseWriter, r *http.Request, h *Handler) bool {
	cors := s.opts.CORS
	if !cors.Enabled {
		return false
	}

	origin := strings.TrimSpace(r.Header.Get("Origin"))
	allowed := cors.allow(w.Header(), origin)

	method := strings.ToUpper(strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")))
	if r.Method != http.MethodOptions || origin == "" || method == "" || h.hasRoute(r) {
		return false
	}
	preflight(w, r, h, method, allowed)
	return true
}

// allow echoes Access-Control-Allow-Origin and reports whether origin may make requests.
func (c CORS) allow(hdr http.Header, origin string) bool {
	allowed := c.Wildcard || slices.Contains(c.Origins, strings.ToLower(origin))
	if c.Wildcard {
		hdr.Set("Access-Control-Allow-Origin", "*")
	} else if origin != "" {
		addVary(hdr, "Origin")
		if allowed {
			hdr.Set("Access-Control-Allow-Origin", origin)
		}
	}
	return allowed
}

func preflight(w http.ResponseWriter, r *http.Request, h *Handler, method string, allowed bool) {
	addVary(w.Header(), "Access-Control-Request-Method")
	addVary(w.Header(), "Access-Control-Request-Headers")
	if !allowed {
		writeProblem(w, http.StatusForbidden, "CORS origin is not allowed")
		return
	}
	if !httpguts.ValidHeaderFieldName(method) {
		writeProblem(w, http.StatusBadRequest, "invalid CORS preflight method")
		return
	}
	pr := r.Clone(r.Context())
	pr.Method = method
	if !h.hasRoute(pr) {
		writeProblem(w, http.StatusNotFound, "CORS preflight route was not found")
		return
	}

	w.Header().Set("Access-Control-Allow-Methods", method)
	if hdrs := strings.TrimSpace(r.Header.Get("Access-Control-Request-Headers")); hdrs != "" {
		canon, ok := canonHeaders(hdrs)
		if !ok {
			writeProblem(w, http.StatusBadRequest, "invalid CORS preflight headers")
			return
		}
		w.Header().Set("Access-Control-Allow-Headers", canon)
	}
	w.Header().Set("Access-Control-Max-Age", "600")
	w.WriteHeader(http.StatusNoContent)
}

func canonHeaders(raw string) (string, bool) {
	fields := strings.Split(raw, ",")
	for i, field := range fields {
		field = strings.TrimSpace(field)
		if !httpguts.ValidHeaderFieldName(field) {
			return "", false
		}
		fields[i] = http.CanonicalHeaderKey(field)
	}
	return strings.Join(fields, ", "), true
}

func addVary(header http.Header, value string) {
	for _, cur := range header.Values("Vary") {
		for field := range strings.SplitSeq(cur, ",") {
			if strings.EqualFold(strings.TrimSpace(field), value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}
