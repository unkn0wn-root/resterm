package runfail

import (
	"strings"
	"unicode"
)

const (
	messageConfidenceToken      = 1
	messageConfidenceTokenGroup = 3
	messageConfidencePhrase     = 3
)

const (
	messagePriorityTimeout    = 10
	messagePriorityNetwork    = 20
	messagePriorityTLS        = 30
	messagePriorityAuth       = 40
	messagePriorityFilesystem = 60
	messagePriorityProtocol   = 70
	messagePriorityRoute      = 80
)

type messageClassifier struct {
	defaultCode Code
	rules       []messageRule
}

// messageRule is only used after typed and diag-based classification fails.
// Matching picks the strongest single evidence item; weak matches never add up.
type messageRule struct {
	code        Code
	priority    int
	phrases     []string
	tokens      []string
	tokenGroups [][]string
}

var sharedMessageRules = []messageRule{
	{
		code:     CodeTimeout,
		priority: messagePriorityTimeout,
		phrases:  []string{"deadline exceeded", "timed out"},
		tokens:   []string{"timeout"},
	},
	{
		code:     CodeTLS,
		priority: messagePriorityTLS,
		tokens:   []string{"certificate", "x509", "tls", "ssl"},
	},
	{
		code:     CodeAuth,
		priority: messagePriorityAuth,
		phrases: []string{
			"access token",
			"authentication token",
			"bearer token",
			"id token",
			"refresh token",
			"authorization header",
		},
		tokens: []string{
			"auth",
			"authentication",
			"authorization",
			"credential",
			"credentials",
			"oauth",
			"unauthenticated",
			"unauthorized",
		},
		tokenGroups: [][]string{
			{"token", "expired"},
			{"token", "invalid"},
			{"token", "missing"},
			{"token", "required"},
		},
	},
	{
		code:     CodeRoute,
		priority: messagePriorityRoute,
		phrases:  []string{"port-forward", "port forward"},
		tokens:   []string{"ssh", "k8s", "kubernetes", "tunnel"},
	},
	{
		code:     CodeProtocol,
		priority: messagePriorityProtocol,
		phrases: []string{
			"decode body",
			"decode response",
			"decode stream",
			"invalid response",
			"malformed response",
			"stream transcript",
			"unexpected response",
		},
		tokens: []string{"grpc", "websocket", "sse"},
		tokenGroups: [][]string{
			{"decode", "body"},
			{"decode", "json"},
			{"decode", "response"},
			{"parse", "json"},
			{"parse", "response"},
		},
	},
}

var genericMessageRules = []messageRule{
	{
		code:     CodeFilesystem,
		priority: messagePriorityFilesystem,
		phrases: []string{
			"directory not found",
			"file not found",
			"is a directory",
			"no such directory",
			"no such file",
			"not a directory",
			"open file",
			"permission denied",
			"read file",
			"stat file",
			"write file",
		},
		tokens: []string{"filesystem"},
		tokenGroups: [][]string{
			{"no", "such", "directory"},
			{"no", "such", "file"},
			{"permission", "denied"},
		},
	},
	{
		code:     CodeNetwork,
		priority: messagePriorityNetwork,
		phrases: []string{
			"connection refused",
			"connection reset",
			"host unreachable",
			"i/o timeout",
			"network unreachable",
			"no such host",
			"temporary failure in name resolution",
		},
		tokens: []string{"dns", "network"},
		tokenGroups: [][]string{
			{"connect", "refused"},
			{"connection", "refused"},
			{"connection", "reset"},
			{"dial", "tcp"},
			{"dial", "udp"},
			{"lookup", "host"},
		},
	},
}

var httpMessageRules = []messageRule{
	{
		code:     CodeNetwork,
		priority: messagePriorityNetwork,
		phrases: []string{
			"connection refused",
			"connection reset",
			"host unreachable",
			"i/o timeout",
			"network unreachable",
			"no such host",
			"proxy connection",
			"proxy error",
			"temporary failure in name resolution",
		},
		tokens: []string{"dns", "network"},
		tokenGroups: [][]string{
			{"connect", "refused"},
			{"connection", "refused"},
			{"connection", "reset"},
			{"dial", "tcp"},
			{"dial", "udp"},
			{"lookup", "host"},
			{"proxy", "connect"},
			{"proxy", "connection"},
		},
	},
}

var genericMessageClassifier = messageClassifier{
	defaultCode: CodeUnknown,
	rules:       mergeMessageRules(sharedMessageRules, genericMessageRules),
}

var httpMessageClassifier = messageClassifier{
	defaultCode: CodeProtocol,
	rules:       mergeMessageRules(sharedMessageRules, httpMessageRules),
}

func classifyHTTPMessage(msg, source string) Failure {
	return httpMessageClassifier.classify(msg, source)
}

func classifyMessage(msg, source string) Failure {
	return genericMessageClassifier.classify(msg, source)
}

func mergeMessageRules(groups ...[]messageRule) []messageRule {
	n := 0
	for _, g := range groups {
		n += len(g)
	}
	out := make([]messageRule, 0, n)
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

func (c messageClassifier) classifyCode(msg string) Code {
	tm := newMessageTerms(msg)
	var b messageMatch
	m := false
	t := false
	for _, r := range c.rules {
		cfd := r.match(tm)
		if cfd == 0 {
			continue
		}
		next := messageMatch{
			code:       r.code,
			confidence: cfd,
			priority:   r.priority,
		}
		switch {
		case !m || next.betterThan(b):
			b = next
			m = true
			t = false
		case next.equivalentTo(b) && next.code != b.code:
			t = true
		}
	}
	if !m || t {
		return c.defaultCode
	}
	return b.code
}

func (c messageClassifier) classify(msg, source string) Failure {
	return New(c.classifyCode(msg), msg, source)
}

type messageMatch struct {
	code       Code
	confidence int
	priority   int
}

func (m messageMatch) betterThan(other messageMatch) bool {
	switch {
	case m.confidence != other.confidence:
		return m.confidence > other.confidence
	case m.priority != other.priority:
		return m.priority < other.priority
	default:
		return false
	}
}

func (m messageMatch) equivalentTo(other messageMatch) bool {
	return m.confidence == other.confidence && m.priority == other.priority
}

func (r messageRule) match(terms messageTerms) int {
	cf := 0
	for _, ph := range r.phrases {
		if terms.containsSequence(messageFields(ph)) {
			cf = max(cf, messageConfidencePhrase)
		}
	}
	for _, tok := range r.tokens {
		if terms.containsToken(tok) {
			cf = max(cf, messageConfidenceToken)
		}
	}
	for _, gr := range r.tokenGroups {
		if terms.containsAllTokens(gr...) {
			cf = max(cf, messageConfidenceTokenGroup)
		}
	}
	return cf
}

type messageTerms struct {
	ordered []string
	set     map[string]struct{}
}

func newMessageTerms(s string) messageTerms {
	ordered := messageFields(s)
	set := make(map[string]struct{}, len(ordered))
	for _, field := range ordered {
		set[field] = struct{}{}
	}
	return messageTerms{ordered: ordered, set: set}
}

func (t messageTerms) containsToken(token string) bool {
	token = strings.ToLower(token)
	_, ok := t.set[token]
	return ok
}

func (t messageTerms) containsAllTokens(words ...string) bool {
	if len(words) == 0 {
		return false
	}
	for _, w := range words {
		if !t.containsToken(w) {
			return false
		}
	}
	return true
}

func (t messageTerms) containsSequence(seq []string) bool {
	if len(seq) == 0 || len(seq) > len(t.ordered) {
		return false
	}
	for i := 0; i <= len(t.ordered)-len(seq); i++ {
		m := true
		for j, word := range seq {
			if t.ordered[i+j] != word {
				m = false
				break
			}
		}
		if m {
			return true
		}
	}
	return false
}

func messageFields(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
