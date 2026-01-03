package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
)

const (
	maxSANs      = 6
	maxResolved  = 6
	expiryFormat = "2006-01-02"
)

func renderTraceDetails(
	details *nettrace.TraceDetails,
	styles timelineStyles,
	now time.Time,
) []string {
	if details == nil {
		return nil
	}
	var lines []string
	lines = appendSection(
		lines,
		styles.phase.Render("Connection"),
		renderConnDetails(details.Connection, styles),
	)
	lines = appendSection(
		lines,
		styles.phase.Render("TLS"),
		renderTLSDetails(details.TLS, styles, now),
	)
	return lines
}

func appendSection(lines []string, title string, body []string) []string {
	if len(body) == 0 {
		return lines
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	lines = append(lines, title)
	lines = append(lines, body...)
	return lines
}

func renderConnDetails(conn *nettrace.ConnDetails, styles timelineStyles) []string {
	if !hasConnDetails(conn) {
		return nil
	}
	var lines []string
	if proto := formatProto(conn.Protocol); proto != "" {
		appendDetailValue(&lines, "protocol", proto, styles)
	}
	if val, ok := reuseStatus(conn); ok {
		appendDetailValue(&lines, "reused", val, styles)
	}
	if val := dialSummary(conn); val != "" {
		appendDetailValue(&lines, "dial", val, styles)
	}
	appendDetailValue(&lines, "local", strings.TrimSpace(conn.LocalAddr), styles)
	appendDetailValue(&lines, "remote", strings.TrimSpace(conn.RemoteAddr), styles)
	if len(conn.ResolvedAddrs) > 0 {
		appendDetailValue(&lines, "resolved", formatList(conn.ResolvedAddrs, maxResolved), styles)
	}
	if conn.Proxy != "" {
		proxy := conn.Proxy
		if conn.ProxyTunnel {
			proxy += " (tunnel)"
		}
		appendDetailValue(&lines, "proxy", proxy, styles)
	}
	appendDetailValue(&lines, "ssh", strings.TrimSpace(conn.SSH), styles)
	return lines
}

func renderTLSDetails(tls *nettrace.TLSDetails, styles timelineStyles, now time.Time) []string {
	if !hasTLSDetails(tls) {
		return nil
	}
	var lines []string
	appendDetailValue(&lines, "version", strings.TrimSpace(tls.Version), styles)
	appendDetailValue(&lines, "cipher", strings.TrimSpace(tls.Cipher), styles)
	appendDetailValue(&lines, "alpn", strings.TrimSpace(tls.ALPN), styles)
	appendDetailValue(&lines, "sni", strings.TrimSpace(tls.ServerName), styles)
	if tls.Resumed {
		appendDetailValue(&lines, "resumed", "yes", styles)
	}
	switch tlsVerificationStatus(tls) {
	case tlsVerifyYes:
		appendDetailValue(&lines, "verified", "yes", styles)
	case tlsVerifyNo:
		appendDetailKV(&lines, "verified", styles.statusWarn.Render("no"), styles)
	case tlsVerifyUnknown:
		if hasTLSVerificationContext(tls) {
			label := "unknown"
			if tls.Resumed {
				label = "not rechecked (resumed)"
			}
			appendDetailKV(&lines, "verified", styles.meta.Render(label), styles)
		}
	}
	if len(tls.Certificates) == 0 {
		return lines
	}
	lines = append(
		lines,
		"  "+styles.meta.Render(fmt.Sprintf("chain (%d):", len(tls.Certificates))),
	)
	for i, cert := range tls.Certificates {
		lines = append(lines, renderCertLines(i, cert, styles, now)...)
	}
	return lines
}

func renderCertLines(
	idx int,
	cert nettrace.TLSCert,
	styles timelineStyles,
	now time.Time,
) []string {
	subject := strings.TrimSpace(cert.Subject)
	if subject == "" {
		subject = "<unknown>"
	}
	expiry, expired := formatExpiry(cert.NotAfter, now)
	expiryStyle := styles.meta
	if expired {
		expiryStyle = styles.statusWarn
	}
	title := fmt.Sprintf("%d. %s", idx+1, subject)
	if expiry != "" {
		title += " " + expiryStyle.Render("("+expiry+")")
	}
	lines := []string{"  " + styles.emph.Render(title)}
	if issuer := strings.TrimSpace(cert.Issuer); issuer != "" {
		lines = append(lines, "    "+detailKV("issuer", styles.meta.Render(issuer), styles))
	}
	if idx == 0 && len(cert.SANs) > 0 {
		sans := formatList(cert.SANs, maxSANs)
		lines = append(lines, "    "+detailKV("san", styles.meta.Render(sans), styles))
	}
	return lines
}

func detailKV(label, value string, styles timelineStyles) string {
	return fmt.Sprintf("%s %s", styles.meta.Render(label+":"), value)
}

func appendDetailValue(lines *[]string, label, value string, styles timelineStyles) {
	if value == "" {
		return
	}
	appendDetailKV(lines, label, styles.emph.Render(value), styles)
}

func appendDetailKV(lines *[]string, label, value string, styles timelineStyles) {
	if value == "" {
		return
	}
	*lines = append(*lines, "  "+detailKV(label, value, styles))
}

func reuseStatus(conn *nettrace.ConnDetails) (string, bool) {
	if conn == nil {
		return "", false
	}
	if conn.Reused {
		val := "yes"
		if conn.IdleTime > 0 {
			val += fmt.Sprintf(" (idle %s)", conn.IdleTime.Round(time.Millisecond))
		}
		return val, true
	}
	if conn.DialAddr != "" || conn.RemoteAddr != "" || conn.LocalAddr != "" {
		return "no", true
	}
	return "", false
}

func dialSummary(conn *nettrace.ConnDetails) string {
	if conn == nil {
		return ""
	}
	network := strings.TrimSpace(conn.Network)
	addr := strings.TrimSpace(conn.DialAddr)
	switch {
	case network != "" && addr != "":
		return network + " " + addr
	case addr != "":
		return addr
	default:
		return ""
	}
}

func formatList(items []string, max int) string {
	if len(items) == 0 {
		return ""
	}
	if max <= 0 || len(items) <= max {
		return strings.Join(items, ", ")
	}
	head := strings.Join(items[:max], ", ")
	return fmt.Sprintf("%s, +%d more", head, len(items)-max)
}

func formatProto(proto string) string {
	trimmed := strings.TrimSpace(proto)
	if trimmed == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(trimmed, "HTTP/2"):
		return "HTTP/2"
	case strings.HasPrefix(trimmed, "HTTP/1.1"):
		return "HTTP/1.1"
	case strings.HasPrefix(trimmed, "HTTP/1.0"):
		return "HTTP/1.0"
	default:
		return trimmed
	}
}

func formatExpiry(exp time.Time, now time.Time) (string, bool) {
	if exp.IsZero() {
		return "", false
	}
	delta := exp.Sub(now)
	rel := shortRel(delta)
	if delta < 0 {
		return fmt.Sprintf("exp %s (%s ago)", exp.Format(expiryFormat), rel), true
	}
	return fmt.Sprintf("exp %s (in %s)", exp.Format(expiryFormat), rel), false
}

func shortRel(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d >= 24*time.Hour:
		days := int(math.Round(d.Hours() / 24))
		if days == 0 {
			days = 1
		}
		return fmt.Sprintf("%dd", days)
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		secs := int(d.Seconds())
		if secs < 1 {
			secs = 1
		}
		return fmt.Sprintf("%ds", secs)
	}
}

func hasConnDetails(c *nettrace.ConnDetails) bool {
	if c == nil {
		return false
	}
	if c.Reused || c.WasIdle || c.IdleTime > 0 ||
		len(c.ResolvedAddrs) > 0 || c.ProxyTunnel {
		return true
	}
	return hasAnyString(
		c.Network,
		c.DialAddr,
		c.LocalAddr,
		c.RemoteAddr,
		c.Proxy,
		c.SSH,
		c.Protocol,
	)
}

func hasTLSDetails(t *nettrace.TLSDetails) bool {
	if t == nil {
		return false
	}
	if t.Verified {
		return true
	}
	return hasTLSVerificationContext(t)
}

type tlsVerifyStatus int

const (
	tlsVerifyUnknown tlsVerifyStatus = iota
	tlsVerifyYes
	tlsVerifyNo
)

func tlsVerificationStatus(t *nettrace.TLSDetails) tlsVerifyStatus {
	if t == nil {
		return tlsVerifyUnknown
	}
	if t.Verified {
		return tlsVerifyYes
	}
	if t.Resumed {
		// Resumed sessions do not re-verify chains, so treat as unknown unless explicitly verified.
		return tlsVerifyUnknown
	}
	if hasTLSVerificationEvidence(t) {
		return tlsVerifyNo
	}
	return tlsVerifyUnknown
}

func hasTLSVerificationEvidence(t *nettrace.TLSDetails) bool {
	if t == nil {
		return false
	}
	if len(t.Certificates) > 0 {
		return true
	}
	return hasAnyString(t.Version, t.Cipher)
}

func hasTLSVerificationContext(t *nettrace.TLSDetails) bool {
	if t == nil {
		return false
	}
	if t.Resumed || len(t.Certificates) > 0 {
		return true
	}
	return hasAnyString(t.Version, t.Cipher, t.ALPN, t.ServerName)
}

func hasAnyString(vals ...string) bool {
	for _, val := range vals {
		if val != "" {
			return true
		}
	}
	return false
}
