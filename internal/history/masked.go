package history

import (
	"bytes"
	"slices"
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/redact"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// History keeps the start of a body, not the response.
const maxSnippet = 2000

// masked returns e with sec masked in every field but identities and lookup keys, and whether any field changed.
func (e Entry) masked(sec redact.Set) (Entry, bool) {
	hit := false
	text := func(s string) string {
		m := sec.Text(s)
		hit = hit || m != s
		return m
	}
	each := func(xs []string) []string {
		xs = slices.Clone(xs)
		for i := range xs {
			xs[i] = text(xs[i])
		}
		return xs
	}
	body := func(s string) string {
		b := []byte(s)
		m := sec.JSON(b)
		hit = hit || !bytes.Equal(m, b)
		n := len(m)
		if n > maxSnippet {
			n = maxSnippet
			// Back up to a rune start, so the snippet stays valid UTF-8.
			for i := n; i > n-utf8.UTFMax; i-- {
				if utf8.RuneStart(m[i]) {
					n = i
					break
				}
			}
		}
		return string(m[:n])
	}

	e.RequestName = text(e.RequestName)
	e.URL = text(e.URL)
	e.Status = text(e.Status)
	// A workflow's snippet is its report, kept whole.
	if e.Method == restfile.HistoryMethodWorkflow {
		e.BodySnippet = text(e.BodySnippet)
	} else {
		e.BodySnippet = body(e.BodySnippet)
	}
	e.RequestText = text(e.RequestText)
	e.Description = text(e.Description)
	e.Tags = each(e.Tags)
	if p := e.ProfileResults; p != nil {
		c := *p
		c.Error = text(c.Error)
		c.Failures = slices.Clone(c.Failures)
		for i := range c.Failures {
			c.Failures[i].Reason = text(c.Failures[i].Reason)
		}
		e.ProfileResults = &c
	}
	if t := e.Trace; t != nil {
		c := *t
		c.Error = text(c.Error)
		c.Phases = slices.Clone(c.Phases)
		for i := range c.Phases {
			p := &c.Phases[i]
			p.Error = text(p.Error)
			p.Meta.Addr = text(p.Meta.Addr)
		}
		if d := c.Details; d != nil {
			dc := *d
			if cn := dc.Connection; cn != nil {
				x := *cn
				x.DialAddr = text(x.DialAddr)
				x.LocalAddr = text(x.LocalAddr)
				x.RemoteAddr = text(x.RemoteAddr)
				x.ResolvedAddrs = each(x.ResolvedAddrs)
				x.Proxy = text(x.Proxy)
				x.SSH = text(x.SSH)
				x.K8s = text(x.K8s)
				dc.Connection = &x
			}
			if tl := dc.TLS; tl != nil {
				x := *tl
				x.ServerName = text(x.ServerName)
				x.Certificates = slices.Clone(x.Certificates)
				for i := range x.Certificates {
					ct := &x.Certificates[i]
					ct.Subject = text(ct.Subject)
					ct.Issuer = text(ct.Issuer)
					ct.SANs = each(ct.SANs)
					ct.Serial = text(ct.Serial)
				}
				dc.TLS = &x
			}
			c.Details = &dc
		}
		e.Trace = &c
	}
	if cm := e.Compare; cm != nil {
		c := *cm
		c.Results = slices.Clone(c.Results)
		for i := range c.Results {
			r := &c.Results[i]
			r.Status = text(r.Status)
			r.BodySnippet = body(r.BodySnippet)
			r.RequestText = text(r.RequestText)
			r.Error = text(r.Error)
		}
		e.Compare = &c
	}
	return e, hit
}
