package curl

import (
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

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
	warn *WarningCollector
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
	return Res{Req: req, Warn: warn}, nil
}

func normSeg(seg Seg) (*restfile.Request, []string, error) {
	st := &segState{
		m:    "GET",
		hdr:  make(http.Header),
		body: newBodyBuilder(),
		set:  map[string]string{},
		warn: newWarningCollector(),
	}
	st.warn.UnknownFlags(seg.Unk)

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
	return req, st.warn.List(), nil
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
