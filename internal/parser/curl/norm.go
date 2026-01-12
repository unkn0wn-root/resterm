package curl

import (
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/settings"
	"github.com/unkn0wn-root/resterm/internal/util"
)

type optKind int

const (
	optNone optKind = iota
	optVal
)

type optFn func(*segState, string) error

type optDef struct {
	key  string
	kind optKind
	fn   optFn
}

type segState struct {
	m    string
	exp  bool
	hdr  http.Header
	body *bodyBuilder
	url  string
	usr  string
	zip  bool
	get  bool
	set  map[string]string
	w    []string
}

var defs = map[string]*optDef{
	"request":         {key: "request", kind: optVal, fn: optReq},
	"header":          {key: "header", kind: optVal, fn: optHdr},
	"user":            {key: "user", kind: optVal, fn: optUser},
	"user-agent":      {key: "user-agent", kind: optVal, fn: optHdrKey(headerUserAgent)},
	"referer":         {key: "referer", kind: optVal, fn: optHdrKey(headerReferer)},
	"cookie":          {key: "cookie", kind: optVal, fn: optHdrKey(headerCookie)},
	"head":            {key: "head", kind: optNone, fn: optHead},
	"compressed":      {key: "compressed", kind: optNone, fn: optComp},
	"url":             {key: "url", kind: optVal, fn: optURL},
	"json":            {key: "json", kind: optVal, fn: optJSON},
	"data-json":       {key: "data-json", kind: optVal, fn: optJSON},
	"data":            {key: "data", kind: optVal, fn: optData},
	"data-raw":        {key: "data-raw", kind: optVal, fn: optDataRaw},
	"data-binary":     {key: "data-binary", kind: optVal, fn: optDataBin},
	"data-urlencode":  {key: "data-urlencode", kind: optVal, fn: optDataURL},
	"form":            {key: "form", kind: optVal, fn: optForm},
	"form-string":     {key: "form-string", kind: optVal, fn: optFormStr},
	"upload-file":     {key: "upload-file", kind: optVal, fn: optUp},
	"get":             {key: "get", kind: optNone, fn: optGet},
	"insecure":        {key: "insecure", kind: optNone, fn: optSetConst("http-insecure", "true")},
	"proxy":           {key: "proxy", kind: optVal, fn: optSet("proxy")},
	"location":        {key: "location", kind: optNone, fn: optSetConst("followredirects", "true")},
	"max-time":        {key: "max-time", kind: optVal, fn: optSetDur("timeout")},
	"connect-timeout": {key: "connect-timeout", kind: optVal, fn: optConnTimeout},
	"max-redirs":      {key: "max-redirs", kind: optVal, fn: optMaxRedirs},
	"retry":           {key: "retry", kind: optVal, fn: optRetry},
	"retry-delay":     {key: "retry-delay", kind: optVal, fn: optWarnDur("retry-delay")},
	"retry-max-time":  {key: "retry-max-time", kind: optVal, fn: optWarnDur("retry-max-time")},
	"retry-connrefused": {
		key:  "retry-connrefused",
		kind: optNone,
		fn:   optWarn("--retry-connrefused"),
	},
	"cacert":       {key: "cacert", kind: optVal, fn: optSet("http-root-cas")},
	"cert":         {key: "cert", kind: optVal, fn: optSet("http-client-cert")},
	"key":          {key: "key", kind: optVal, fn: optSet("http-client-key")},
	"silent":       {key: "silent", kind: optNone, fn: optWarn("--silent")},
	"silent-short": {key: "silent-short", kind: optNone, fn: optWarn("-s")},
	"show-error":   {key: "show-error", kind: optNone, fn: optWarn("--show-error")},
	"show-error-short": {
		key:  "show-error-short",
		kind: optNone,
		fn:   optWarn("-S"),
	},
	"verbose":       {key: "verbose", kind: optNone, fn: optWarn("--verbose")},
	"verbose-short": {key: "verbose-short", kind: optNone, fn: optWarn("-v")},
	"include":       {key: "include", kind: optNone, fn: optWarn("--include")},
	"include-short": {key: "include-short", kind: optNone, fn: optWarn("-i")},
	"output":        {key: "output", kind: optVal, fn: optWarnVal("--output")},
	"output-short":  {key: "output-short", kind: optVal, fn: optWarnVal("-o")},
	"remote-name":   {key: "remote-name", kind: optNone, fn: optWarn("--remote-name")},
	"remote-name-short": {
		key:  "remote-name-short",
		kind: optNone,
		fn:   optWarn("-O"),
	},
	"dump-header":       {key: "dump-header", kind: optVal, fn: optWarnVal("--dump-header")},
	"dump-header-short": {key: "dump-header-short", kind: optVal, fn: optWarnVal("-D")},
	"stderr":            {key: "stderr", kind: optVal, fn: optWarnVal("--stderr")},
	"trace":             {key: "trace", kind: optVal, fn: optWarnVal("--trace")},
	"trace-ascii":       {key: "trace-ascii", kind: optVal, fn: optWarnVal("--trace-ascii")},
	"http1.1":           {key: "http1.1", kind: optNone, fn: optWarn("--http1.1")},
	"http2":             {key: "http2", kind: optNone, fn: optWarn("--http2")},
	"http2-prior-knowledge": {
		key:  "http2-prior-knowledge",
		kind: optNone,
		fn:   optWarn("--http2-prior-knowledge"),
	},
	"http3":       {key: "http3", kind: optNone, fn: optWarn("--http3")},
	"resolve":     {key: "resolve", kind: optVal, fn: optWarnVal("--resolve")},
	"connect-to":  {key: "connect-to", kind: optVal, fn: optWarnVal("--connect-to")},
	"interface":   {key: "interface", kind: optVal, fn: optWarnVal("--interface")},
	"dns-servers": {key: "dns-servers", kind: optVal, fn: optWarnVal("--dns-servers")},
}

var longDefs = map[string]*optDef{
	"request":               defs["request"],
	"header":                defs["header"],
	"user":                  defs["user"],
	"user-agent":            defs["user-agent"],
	"referer":               defs["referer"],
	"cookie":                defs["cookie"],
	"head":                  defs["head"],
	"compressed":            defs["compressed"],
	"url":                   defs["url"],
	"json":                  defs["json"],
	"data-json":             defs["data-json"],
	"data":                  defs["data"],
	"data-ascii":            defs["data"],
	"data-urlencode":        defs["data-urlencode"],
	"data-raw":              defs["data-raw"],
	"data-binary":           defs["data-binary"],
	"form":                  defs["form"],
	"form-string":           defs["form-string"],
	"upload-file":           defs["upload-file"],
	"get":                   defs["get"],
	"insecure":              defs["insecure"],
	"proxy":                 defs["proxy"],
	"location":              defs["location"],
	"max-time":              defs["max-time"],
	"connect-timeout":       defs["connect-timeout"],
	"max-redirs":            defs["max-redirs"],
	"retry":                 defs["retry"],
	"retry-delay":           defs["retry-delay"],
	"retry-max-time":        defs["retry-max-time"],
	"retry-connrefused":     defs["retry-connrefused"],
	"cacert":                defs["cacert"],
	"cert":                  defs["cert"],
	"key":                   defs["key"],
	"silent":                defs["silent"],
	"show-error":            defs["show-error"],
	"verbose":               defs["verbose"],
	"include":               defs["include"],
	"output":                defs["output"],
	"remote-name":           defs["remote-name"],
	"dump-header":           defs["dump-header"],
	"stderr":                defs["stderr"],
	"trace":                 defs["trace"],
	"trace-ascii":           defs["trace-ascii"],
	"http1.1":               defs["http1.1"],
	"http2":                 defs["http2"],
	"http2-prior-knowledge": defs["http2-prior-knowledge"],
	"http3":                 defs["http3"],
	"resolve":               defs["resolve"],
	"connect-to":            defs["connect-to"],
	"interface":             defs["interface"],
	"dns-servers":           defs["dns-servers"],
}

var shortDefs = map[rune]*optDef{
	'X': defs["request"],
	'H': defs["header"],
	'u': defs["user"],
	'A': defs["user-agent"],
	'e': defs["referer"],
	'b': defs["cookie"],
	'I': defs["head"],
	'd': defs["data"],
	'F': defs["form"],
	'G': defs["get"],
	'T': defs["upload-file"],
	'k': defs["insecure"],
	'x': defs["proxy"],
	'L': defs["location"],
	'm': defs["max-time"],
	's': defs["silent-short"],
	'S': defs["show-error-short"],
	'v': defs["verbose-short"],
	'i': defs["include-short"],
	'o': defs["output-short"],
	'O': defs["remote-name-short"],
	'D': defs["dump-header-short"],
}

type Res struct {
	Req  *restfile.Request
	Warn []string
}

func normCmd(cmd *Cmd) ([]*restfile.Request, error) {
	if cmd == nil || len(cmd.Segs) == 0 {
		return nil, nil
	}

	res, err := normCmdRes(cmd)
	if err != nil {
		return nil, err
	}

	out := make([]*restfile.Request, 0, len(res))
	for _, item := range res {
		if item.Req == nil {
			continue
		}
		out = append(out, item.Req)
	}
	return out, nil
}

func normCmdRes(cmd *Cmd) ([]Res, error) {
	if cmd == nil || len(cmd.Segs) == 0 {
		return nil, nil
	}

	out := make([]Res, 0, len(cmd.Segs))
	for _, seg := range cmd.Segs {
		item, err := normSegRes(seg)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func normSegRes(seg Seg) (Res, error) {
	req, warn, err := normSeg(seg)
	if err != nil {
		return Res{}, err
	}
	return Res{Req: req, Warn: mergeWarn(warnUnk(seg.Unk), warn)}, nil
}

func normSeg(seg Seg) (*restfile.Request, []string, error) {
	st := &segState{
		m:    "GET",
		hdr:  make(http.Header),
		body: newBodyBuilder(),
		set:  map[string]string{},
	}

	for _, it := range seg.Items {
		if it.IsOpt {
			if err := applyOpt(st, it.Opt); err != nil {
				return nil, nil, err
			}
		} else {
			if err := applyPos(st, it.Pos); err != nil {
				return nil, nil, err
			}
		}
	}

	if st.url == "" {
		return nil, nil, fmt.Errorf("curl command missing URL")
	}

	if st.get {
		if err := applyGet(st); err != nil {
			return nil, nil, err
		}
	}

	if st.body.hasContent() && !st.exp && strings.EqualFold(st.m, "GET") {
		st.m = "POST"
	}

	req := &restfile.Request{Method: st.m}
	if err := st.body.apply(req, st.hdr); err != nil {
		return nil, nil, err
	}

	req.URL = sanitizeURL(st.url)
	if len(st.hdr) > 0 {
		req.Headers = st.hdr
	}

	if st.zip {
		if req.Headers == nil {
			req.Headers = make(http.Header)
		}
		if req.Headers.Get(headerAcceptEncoding) == "" {
			req.Headers.Set(headerAcceptEncoding, acceptEncodingDefault)
		}
	}

	applyUser(req, st.usr)
	applySettings(req, st.set)
	return req, st.w, nil
}

func applyOpt(st *segState, opt Opt) error {
	def := defs[opt.Key]
	if def == nil || def.fn == nil {
		return nil
	}
	return def.fn(st, opt.Val)
}

func applyPos(st *segState, val string) error {
	if st.url == "" {
		st.url = val
		return nil
	}
	return st.body.addRaw(val)
}

func applyGet(st *segState) error {
	if !st.body.hasContent() {
		return nil
	}

	q, err := st.body.query()
	if err != nil {
		return err
	}
	st.url = addQuery(st.url, q)
	st.body = newBodyBuilder()
	return nil
}

func addQuery(raw, q string) string {
	if q == "" {
		return raw
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		sep := "?"
		if strings.Contains(raw, "?") {
			sep = "&"
		}
		return raw + sep + q
	}
	if u.RawQuery != "" {
		u.RawQuery = u.RawQuery + "&" + q
	} else {
		u.RawQuery = q
	}
	return u.String()
}

func optReq(st *segState, val string) error {
	st.m = strings.ToUpper(val)
	st.exp = true
	return nil
}

func optHdr(st *segState, val string) error {
	addHeader(st.hdr, val)
	return nil
}

func optUser(st *segState, val string) error {
	st.usr = val
	return nil
}

func optHdrKey(k string) optFn {
	return func(st *segState, val string) error {
		setHdr(st.hdr, k, val)
		return nil
	}
}

func optHead(st *segState, _ string) error {
	st.m = "HEAD"
	st.exp = true
	return nil
}

func optComp(st *segState, _ string) error {
	st.zip = true
	return nil
}

func optURL(st *segState, val string) error {
	st.url = val
	return nil
}

func optJSON(st *segState, val string) error {
	if err := st.body.addJSON(val); err != nil {
		return err
	}
	ensureJSONHeader(st.hdr)
	return nil
}

func optData(st *segState, val string) error {
	return st.body.addData(val, true)
}

func optDataRaw(st *segState, val string) error {
	return st.body.addRaw(val)
}

func optDataBin(st *segState, val string) error {
	return st.body.addBinary(val)
}

func optDataURL(st *segState, val string) error {
	return st.body.addURLEncoded(val)
}

func optForm(st *segState, val string) error {
	return st.body.addFormPart(val, false)
}

func optFormStr(st *segState, val string) error {
	return st.body.addFormPart(val, true)
}

func optUp(st *segState, val string) error {
	if !st.exp {
		st.m = "PUT"
		st.exp = true
	}
	return st.body.addFile(val)
}

func optGet(st *segState, _ string) error {
	st.get = true
	st.m = "GET"
	st.exp = true
	return nil
}

func optSet(key string) optFn {
	return func(st *segState, val string) error {
		setKV(st, key, val)
		return nil
	}
}

func optSetConst(key, val string) optFn {
	return func(st *segState, _ string) error {
		setKV(st, key, val)
		return nil
	}
}

func optSetDur(key string) optFn {
	return func(st *segState, val string) error {
		v, err := durSec(val)
		if err != nil {
			return err
		}
		setKV(st, key, v)
		return nil
	}
}

func optConnTimeout(st *segState, val string) error {
	return warnDur(st, "connect-timeout", val)
}

func optMaxRedirs(st *segState, val string) error {
	return warnInt(st, "max-redirs", val)
}

func optRetry(st *segState, val string) error {
	return warnInt(st, "retry", val)
}

func setHdr(h http.Header, k, v string) {
	if strings.TrimSpace(k) == "" {
		return
	}
	if strings.TrimSpace(v) == "" {
		return
	}
	h.Set(k, v)
}

func setKV(st *segState, k, v string) {
	if st == nil || st.set == nil {
		return
	}

	key := strings.TrimSpace(k)
	if key == "" {
		return
	}

	val := strings.TrimSpace(v)
	if val == "" {
		return
	}

	if !settings.IsHTTPKey(key) {
		addWarn(st, "unsupported setting "+key+" (ignored)")
		return
	}
	st.set[key] = val
}

func applySettings(req *restfile.Request, set map[string]string) {
	if req == nil || len(set) == 0 {
		return
	}
	if req.Settings == nil {
		req.Settings = make(map[string]string, len(set))
	}
	maps.Copy(req.Settings, set)
}

func applyUser(req *restfile.Request, usr string) {
	if req == nil || strings.TrimSpace(usr) == "" {
		return
	}
	if req.Headers != nil && req.Headers.Get(headerAuthorization) != "" {
		return
	}

	user, pass, ok := strings.Cut(usr, ":")
	if ok {
		req.Metadata.Auth = &restfile.AuthSpec{
			Type: authTypeBasic,
			Params: map[string]string{
				"username": user,
				"password": pass,
			},
		}
		return
	}
	if req.Headers == nil {
		req.Headers = make(http.Header)
	}
	req.Headers.Set(headerAuthorization, buildBasicAuthHeader(usr))
}

func warnUnk(unk []string) []string {
	if len(unk) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(unk))
	out := make([]string, 0, len(unk))
	for _, raw := range unk {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, "unsupported flag "+v)
	}
	sort.Strings(out)
	return out
}

func mergeWarn(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	for _, v := range a {
		if t := strings.TrimSpace(v); t != "" {
			out = append(out, t)
		}
	}
	for _, v := range b {
		if t := strings.TrimSpace(v); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return util.DedupeSortedStrings(out)
}

func addWarn(st *segState, msg string) {
	if st == nil {
		return
	}
	if strings.TrimSpace(msg) == "" {
		return
	}
	st.w = append(st.w, msg)
}

func warnFlagMsg(flag string) string {
	return "unsupported flag " + flag + " (ignored)"
}

func optWarn(flag string) optFn {
	return func(st *segState, _ string) error {
		addWarn(st, warnFlagMsg(flag))
		return nil
	}
}

func optWarnVal(flag string) optFn {
	return func(st *segState, val string) error {
		if strings.TrimSpace(val) == "" {
			return fmt.Errorf("empty %s", strings.TrimLeft(flag, "-"))
		}
		addWarn(st, warnFlagMsg(flag))
		return nil
	}
}

func optWarnDur(name string) optFn {
	return func(st *segState, val string) error {
		return warnDur(st, name, val)
	}
}

// Validate ignored flags so malformed values still surface during import.
func warnDur(st *segState, n, val string) error {
	if _, err := durSec(val); err != nil {
		return err
	}
	addWarn(st, warnFlagMsg("--"+n))
	return nil
}

func warnInt(st *segState, n, val string) error {
	raw := strings.TrimSpace(val)
	if raw == "" {
		return fmt.Errorf("empty %s", n)
	}
	if _, err := strconv.Atoi(raw); err != nil {
		return fmt.Errorf("invalid %s %q", n, raw)
	}
	addWarn(st, warnFlagMsg("--"+n))
	return nil
}

func durSec(val string) (string, error) {
	raw := strings.TrimSpace(val)
	if raw == "" {
		return "", fmt.Errorf("empty timeout")
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		d := time.Duration(f * float64(time.Second))
		return d.String(), nil
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return d.String(), nil
	} else {
		return "", err
	}
}
