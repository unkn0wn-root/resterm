package httpx

import (
	"net/url"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// requestScheme is the scheme family a resolved request URL must use. WebSocket
// requests resolve against the same http base-url as every other request, so ws
// and wss are applied after resolution rather than before it.
type requestScheme int

const (
	schemeHTTP requestScheme = iota
	schemeWebSocket
)

func requestSchemeOf(req *restfile.Request) requestScheme {
	if req.WebSocket != nil {
		return schemeWebSocket
	}
	return schemeHTTP
}

// resolveRequestTarget turns the request line URL into the absolute URL to send.
// A relative target resolves against base-url the way RFC 3986 resolves a URI
// reference; an absolute target ignores base-url even when it is unusable.
func resolveRequestTarget(
	rawTarget, rawBase string,
	resolver *vars.Resolver,
	scheme requestScheme,
) (string, error) {
	target, err := expandURL(rawTarget, resolver, "url")
	if err != nil {
		return "", err
	}
	if target == "" {
		return "", targetError("request url is empty")
	}

	form := classifyTarget(target)
	if form == formCredentials {
		return "", targetError("request url must not carry credentials, use @auth basic instead")
	}

	ref, err := form.parse(target)
	if err != nil {
		return "", wrapTargetError(err, "parse request url")
	}

	// url.URL cannot tell an empty fragment from an absent one, so a trailing
	// "#" has to be caught in the text before parsing drops it.
	if scheme == schemeWebSocket && strings.Contains(target, "#") {
		return "", targetError("websocket request url must not contain a fragment")
	}

	if !ref.IsAbs() {
		base, err := resolveBaseURL(rawBase, resolver)
		if err != nil {
			return "", err
		}
		ref = base.ResolveReference(ref)
	}

	if ref.Hostname() == "" {
		return "", targetError("request url host is empty")
	}
	if err := scheme.apply(ref); err != nil {
		return "", err
	}
	return ref.String(), nil
}

// targetForm separates relative references, explicit schemes, and hosts that
// need an http:// prefix.
type targetForm int

const (
	formReference targetForm = iota
	formAbsolute
	// formAuthority is a host without a scheme, such as localhost:8080/users.
	formAuthority
	// formCredentials prevents net/http from turning URL userinfo into an
	// Authorization header. For example: user@example.com:8080/path.
	formCredentials
)

// classifyTarget examines only the first path segment. A colon there separates
// either a scheme or a port; RFC relative paths cannot contain one there.
func classifyTarget(raw string) targetForm {
	front := raw
	if end := strings.IndexAny(raw, "/?#"); end >= 0 {
		front = raw[:end]
	}

	// An "@" can be part of a relative path. Treat it as userinfo only when a
	// host and port follow it: user@example.com:8080 is rejected, while
	// user@example.com/path remains relative.
	if at := strings.LastIndexByte(front, '@'); at >= 0 {
		if classifyTarget(raw[at+1:]) == formAuthority {
			return formCredentials
		}
	}
	// A bracketed IPv6 literal cannot begin a path, so it is always a host.
	if strings.HasPrefix(front, "[") {
		return formAuthority
	}

	colon := strings.IndexByte(front, ':')
	switch {
	case colon <= 0:
		return formReference
	case strings.HasPrefix(raw[colon+1:], "//"):
		return formAbsolute
	case isRequestScheme(front[:colon]):
		// Do not treat https:443/path as a host named "https" and send it over
		// plain HTTP. Known schemes without "//" remain invalid URLs.
		return formAbsolute
	case isPort(front[colon+1:]):
		return formAuthority
	default:
		return formAbsolute
	}
}

// isPort accepts one or more ASCII digits. net/url also accepts out-of-range
// values such as 99999, so this checks the syntax without parsing the number.
func isPort(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isRequestScheme(head string) bool {
	switch strings.ToLower(head) {
	case "http", "https", "ws", "wss":
		return true
	}
	return false
}

// parse adds http:// when the target is a host without a scheme.
func (f targetForm) parse(raw string) (*url.URL, error) {
	if f == formAuthority {
		return url.Parse("http://" + raw)
	}
	return url.Parse(raw)
}

// apply rejects a scheme the request cannot use and maps http and https onto ws
// and wss, so a single http base-url serves REST and WebSocket requests alike.
func (s requestScheme) apply(u *url.URL) error {
	if s == schemeHTTP {
		switch u.Scheme {
		case "http", "https":
			return nil
		case "ws", "wss":
			// A ws:// URL still needs @websocket; the method alone does not
			// start a WebSocket session.
			return targetError("websocket request url needs a @websocket directive")
		default:
			return targetError("request url scheme must be http or https")
		}
	}

	switch u.Scheme {
	case "ws", "wss":
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return targetError("websocket request url scheme must be ws, wss, http or https")
	}
	return nil
}

// The base is read only once a target actually needs it, so an unset, unresolved
// or invalid base-url never fails a request that already carries a full URL.
func resolveBaseURL(raw string, resolver *vars.Resolver) (*url.URL, error) {
	expanded, err := expandURL(raw, resolver, "base-url")
	if err != nil {
		return nil, err
	}

	expanded = strings.TrimSpace(expanded)
	if expanded == "" {
		return nil, targetError("relative request url requires a base-url setting")
	}

	base, err := url.Parse(expanded)
	if err != nil {
		return nil, wrapTargetError(err, "parse base-url")
	}
	switch {
	case base.Scheme != "http" && base.Scheme != "https":
		return nil, targetError("invalid base-url: scheme must be http or https")
	case base.Hostname() == "":
		return nil, targetError("invalid base-url: host is required")
	case base.User != nil:
		return nil, targetError("invalid base-url: userinfo is not allowed")
	case base.RawQuery != "" || base.ForceQuery:
		return nil, targetError("invalid base-url: query is not allowed")
	case strings.Contains(expanded, "#"):
		return nil, targetError("invalid base-url: fragment is not allowed")
	}
	return base, nil
}

func expandURL(raw string, resolver *vars.Resolver, label string) (string, error) {
	if resolver == nil {
		return raw, nil
	}
	expanded, err := resolver.ExpandTemplates(raw)
	if err != nil {
		return "", wrapTargetError(err, "expand "+label)
	}
	return expanded, nil
}

func targetError(msg string) error {
	return diag.New(diag.ClassProtocol, msg, diag.WithComponent(diag.ComponentHTTP))
}

func wrapTargetError(err error, op string) error {
	return diag.WrapAs(diag.ClassProtocol, err, op, diag.WithComponent(diag.ComponentHTTP))
}
