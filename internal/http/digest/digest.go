package digest

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"io"
	"strings"

	"golang.org/x/net/http/httpguts"
)

var quoteEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

type Credentials struct {
	Username string
	Password string
}

// Challenge is a Digest challenge returned by Pick.
type Challenge struct {
	params  map[string]string
	newHash func() hash.Hash
	sess    bool
	qop     string
}

var algorithms = map[string]func() hash.Hash{
	"md5":         md5.New,
	"sha-256":     sha256.New,
	"sha-512-256": sha512.New512_256,
}

// Pick returns the first supported Digest challenge, in the server's order.
func Pick(values []string) (Challenge, bool) {
	for _, v := range values {
		for _, ch := range parseChallenges(v) {
			if !strings.EqualFold(ch.scheme, "Digest") || ch.params["nonce"] == "" {
				continue
			}

			name, ok := ch.params["algorithm"]
			if !ok {
				name = "MD5"
			}
			base, sess := strings.CutSuffix(strings.ToLower(name), "-sess")
			newHash, ok := algorithms[base]
			if !ok {
				continue
			}

			c := Challenge{params: ch.params, newHash: newHash, sess: sess}
			raw, offered := ch.params["qop"]
			switch {
			case !offered && !sess:
				// The RFC 2069 format has no cnonce, which session algorithms need.
			case httpguts.HeaderValuesContainsToken([]string{raw}, "auth"):
				c.qop = "auth"
			case httpguts.HeaderValuesContainsToken([]string{raw}, "auth-int"):
				c.qop = "auth-int"
			default:
				continue
			}
			return c, true
		}
	}
	return Challenge{}, false
}

// Authorization builds a Digest header for method and uri.
// uri includes the path and query. body is used only for qop=auth-int.
func (c Challenge) Authorization(cr Credentials, method, uri string, body []byte) string {
	return c.authorization(cr, method, uri, body, rand.Text())
}

func (c Challenge) authorization(cr Credentials, method, uri string, body []byte, cnonce string) string {
	const nc = "00000001"
	realm, nonce := c.params["realm"], c.params["nonce"]

	ha1 := c.hash(cr.Username, realm, cr.Password)
	if c.sess {
		ha1 = c.hash(ha1, nonce, cnonce)
	}
	ha2 := c.hash(method, uri)
	if c.qop == "auth-int" {
		ha2 = c.hash(method, uri, c.hash(string(body)))
	}
	response := c.hash(ha1, nonce, ha2)
	if c.qop != "" {
		response = c.hash(ha1, nonce, nc, cnonce, c.qop, ha2)
	}

	parts := []string{
		quoted("username", cr.Username),
		quoted("realm", realm),
		quoted("nonce", nonce),
		quoted("uri", uri),
	}
	if alg, ok := c.params["algorithm"]; ok {
		parts = append(parts, "algorithm="+alg)
	}

	parts = append(parts, quoted("response", response))
	if opaque, ok := c.params["opaque"]; ok {
		parts = append(parts, quoted("opaque", opaque))
	}

	if c.qop != "" {
		parts = append(parts, "qop="+c.qop, "nc="+nc, quoted("cnonce", cnonce))
	}
	return "Digest " + strings.Join(parts, ", ")
}

func (c Challenge) hash(parts ...string) string {
	h := c.newHash()
	_, _ = io.WriteString(h, strings.Join(parts, ":"))
	return hex.EncodeToString(h.Sum(nil))
}

func quoted(name, value string) string {
	return name + `="` + quoteEscaper.Replace(value) + `"`
}
