package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"

	"github.com/unkn0wn-root/resterm/internal/openapi"
)

const parRefPre = "#/components/parameters/"

func loadDoc(path string, opts openapi.ParseOptions) (*openapi3.T, []string, error) {
	ld := openapi3.NewLoader()
	ld.IsExternalRefsAllowed = opts.ResolveExternalRefs

	doc, err := ld.LoadFromFile(path)
	if err == nil {
		return doc, nil, nil
	}

	raw, rErr := os.ReadFile(path)
	if rErr != nil {
		return nil, nil, compatErr(err, "read spec file", rErr)
	}
	if !bytes.Contains(raw, []byte(parRefPre)) {
		return nil, nil, err
	}

	raw2, ws, n, fErr := fixHdrRefs(raw)
	if fErr != nil {
		return nil, nil, compatErr(err, "rewrite spec", fErr)
	}
	if n == 0 {
		return nil, nil, compatErr(err, "no header-ref rewrite candidates found", nil)
	}

	ld2 := openapi3.NewLoader()
	ld2.IsExternalRefsAllowed = opts.ResolveExternalRefs

	loc := &url.URL{Path: filepath.ToSlash(path)}
	doc, dErr := ld2.LoadFromDataWithPath(raw2, loc)
	if dErr != nil {
		return nil, nil, compatErr(err, "load rewritten spec", dErr)
	}

	return doc, ws, nil
}

func fixHdrRefs(raw []byte) ([]byte, []string, int, error) {
	codec := detectCodec(raw)

	var root map[string]any
	if err := unmarshalSpec(codec, raw, &root); err != nil {
		return nil, nil, 0, err
	}
	if len(root) == 0 {
		return raw, nil, 0, nil
	}

	fx := newHdrFix(root)
	fx.walk(root)
	if fx.n == 0 {
		return raw, nil, 0, nil
	}

	out, err := marshalSpec(codec, root)
	if err != nil {
		return nil, nil, 0, err
	}
	return out, fx.warns(), fx.n, nil
}

type hdrFix struct {
	pm  map[string]map[string]any
	cnt map[string]int
	loc map[string]string
	n   int
}

func newHdrFix(root map[string]any) *hdrFix {
	fx := &hdrFix{
		pm:  make(map[string]map[string]any),
		cnt: make(map[string]int),
		loc: make(map[string]string),
	}

	com := mapAt(root, "components")
	prm := mapAt(com, "parameters")
	for _, k := range sortedKeys(prm) {
		h, ok := hdrParam(prm, k, map[string]bool{k: true})
		if ok {
			fx.pm[k] = h
		}
	}

	return fx
}

func hdrParam(prm map[string]any, key string, seen map[string]bool) (map[string]any, bool) {
	p := mapAt(prm, key)
	if len(p) == 0 {
		return nil, false
	}
	if ref, ok := strAt(p, "$ref"); ok {
		next, ok := parName(ref)
		if !ok || seen[next] {
			return nil, false
		}
		seen[next] = true
		h, ok := hdrParam(prm, next, seen)
		if !ok {
			return nil, false
		}
		h2 := cloneAnyMap(h)
		for k, v := range p {
			if strings.HasPrefix(k, "x-") {
				h2[k] = cloneVal(v)
			}
		}
		return h2, true
	}

	in, ok := strAt(p, "in")
	if !ok || !strings.EqualFold(in, "header") {
		return nil, false
	}

	h := cloneAnyMap(p)
	delete(h, "name")
	delete(h, "in")
	return h, true
}

func (fx *hdrFix) walk(root map[string]any) {
	com := mapAt(root, "components")
	if len(com) != 0 {
		if hs := mapAt(com, "headers"); len(hs) != 0 {
			fx.fixHs(hs, []string{"components", "headers"})
		}
		if rs := mapAt(com, "responses"); len(rs) != 0 {
			fx.walkRs(rs, []string{"components", "responses"})
		}
		if rbs := mapAt(com, "requestBodies"); len(rbs) != 0 {
			fx.walkRbs(rbs, []string{"components", "requestBodies"})
		}
		if cbs := mapAt(com, "callbacks"); len(cbs) != 0 {
			fx.walkCbs(cbs, []string{"components", "callbacks"})
		}
	}

	pts := mapAt(root, "paths")
	for _, p := range sortedKeys(pts) {
		pi := mapAt(pts, p)
		if len(pi) == 0 {
			continue
		}
		fx.walkPi(pi, []string{"paths", p})
	}
}

func (fx *hdrFix) walkPi(pi map[string]any, seg []string) {
	for _, m := range []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"} {
		op := mapAt(pi, m)
		if len(op) == 0 {
			continue
		}
		fx.walkOp(op, segAdd(seg, m))
	}
}

func (fx *hdrFix) walkOp(op map[string]any, seg []string) {
	if rs := mapAt(op, "responses"); len(rs) != 0 {
		fx.walkRs(rs, segAdd(seg, "responses"))
	}
	if rb := mapAt(op, "requestBody"); len(rb) != 0 {
		fx.walkRb(rb, segAdd(seg, "requestBody"))
	}
	if cbs := mapAt(op, "callbacks"); len(cbs) != 0 {
		fx.walkCbs(cbs, segAdd(seg, "callbacks"))
	}
}

func (fx *hdrFix) walkRs(rs map[string]any, seg []string) {
	for _, code := range sortedKeys(rs) {
		r := mapAt(rs, code)
		if len(r) == 0 {
			continue
		}
		if hs := mapAt(r, "headers"); len(hs) != 0 {
			fx.fixHs(hs, segAdd(seg, code, "headers"))
		}
	}
}

func (fx *hdrFix) walkRbs(rbs map[string]any, seg []string) {
	for _, k := range sortedKeys(rbs) {
		rb := mapAt(rbs, k)
		if len(rb) == 0 {
			continue
		}
		fx.walkRb(rb, segAdd(seg, k))
	}
}

func (fx *hdrFix) walkRb(rb map[string]any, seg []string) {
	cnt := mapAt(rb, "content")
	for _, mt := range sortedKeys(cnt) {
		m := mapAt(cnt, mt)
		if len(m) == 0 {
			continue
		}
		enc := mapAt(m, "encoding")
		for _, fld := range sortedKeys(enc) {
			e := mapAt(enc, fld)
			if len(e) == 0 {
				continue
			}
			hs := mapAt(e, "headers")
			if len(hs) == 0 {
				continue
			}
			fx.fixHs(hs, segAdd(seg, "content", mt, "encoding", fld, "headers"))
		}
	}
}

func (fx *hdrFix) walkCbs(cbs map[string]any, seg []string) {
	for _, cbn := range sortedKeys(cbs) {
		cb := mapAt(cbs, cbn)
		if len(cb) == 0 {
			continue
		}
		if _, ok := strAt(cb, "$ref"); ok {
			continue
		}
		for _, exp := range sortedKeys(cb) {
			pi := mapAt(cb, exp)
			if len(pi) == 0 {
				continue
			}
			fx.walkPi(pi, segAdd(seg, cbn, exp))
		}
	}
}

func (fx *hdrFix) fixHs(hs map[string]any, seg []string) {
	for _, hk := range sortedKeys(hs) {
		h := mapAt(hs, hk)
		if len(h) == 0 {
			continue
		}
		ref, ok := strAt(h, "$ref")
		if !ok {
			continue
		}
		pn, ok := parName(ref)
		if !ok {
			continue
		}
		p, ok := fx.pm[pn]
		if !ok {
			continue
		}
		h2 := cloneAnyMap(p)
		for k, v := range h {
			if strings.HasPrefix(k, "x-") {
				h2[k] = cloneVal(v)
			}
		}
		hs[hk] = h2
		fx.n++
		fx.cnt[pn]++
		if _, ok := fx.loc[pn]; !ok {
			fx.loc[pn] = ptr(segAdd(seg, hk))
		}
	}
}

func (fx *hdrFix) warns() []string {
	if len(fx.cnt) == 0 {
		return nil
	}
	ps := make([]string, 0, len(fx.cnt))
	for p := range fx.cnt {
		ps = append(ps, p)
	}
	sort.Strings(ps)

	ws := make([]string, 0, len(ps))
	for _, p := range ps {
		ref := parRefPre + ptrEsc(p)
		ws = append(
			ws,
			fmt.Sprintf(
				"OpenAPI compatibility rewrite: converted %d header $ref occurrence(s) from %q to inline Header Objects (first at %s).",
				fx.cnt[p],
				ref,
				fx.loc[p],
			),
		)
	}
	return ws
}

func mapAt(m map[string]any, k string) map[string]any {
	if len(m) == 0 {
		return nil
	}
	v, ok := m[k]
	if !ok || v == nil {
		return nil
	}
	x, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return x
}

// strAt returns a trimmed, non-empty string field.
func strAt(m map[string]any, k string) (string, bool) {
	if len(m) == 0 {
		return "", false
	}
	v, ok := m[k]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	return s, true
}

func sortedKeys[M ~map[K]V, K ~string, V any](m M) []K {
	if len(m) == 0 {
		return nil
	}
	ks := make([]K, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return ks[i] < ks[j] })
	return ks
}

func parName(ref string) (string, bool) {
	if !strings.HasPrefix(ref, parRefPre) {
		return "", false
	}
	t := ref[len(parRefPre):]
	if t == "" {
		return "", false
	}
	return ptrUnesc(t), true
}

func cloneAnyMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = cloneVal(v)
	}
	return dst
}

func cloneVal(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return cloneAnyMap(x)
	case []any:
		out := make([]any, len(x))
		for i, vv := range x {
			out[i] = cloneVal(vv)
		}
		return out
	default:
		return x
	}
}

func ptr(seg []string) string {
	if len(seg) == 0 {
		return "#"
	}
	var b strings.Builder
	b.WriteString("#")
	for _, s := range seg {
		b.WriteString("/")
		b.WriteString(ptrEsc(s))
	}
	return b.String()
}

func ptrEsc(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	return strings.ReplaceAll(s, "/", "~1")
}

func ptrUnesc(s string) string {
	s = strings.ReplaceAll(s, "~1", "/")
	return strings.ReplaceAll(s, "~0", "~")
}

type specCodec uint8

const (
	codecYAML specCodec = iota
	codecJSON
)

func detectCodec(raw []byte) specCodec {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return codecYAML
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return codecJSON
	}
	return codecYAML
}

func unmarshalSpec(c specCodec, raw []byte, out *map[string]any) error {
	switch c {
	case codecJSON:
		return json.Unmarshal(raw, out)
	default:
		return yaml.Unmarshal(raw, out)
	}
}

func marshalSpec(c specCodec, v map[string]any) ([]byte, error) {
	switch c {
	case codecJSON:
		return json.Marshal(v)
	default:
		return yaml.Marshal(v)
	}
}

func segAdd(seg []string, parts ...string) []string {
	out := make([]string, len(seg)+len(parts))
	copy(out, seg)
	copy(out[len(seg):], parts)
	return out
}

func compatErr(base error, msg string, cause error) error {
	if cause != nil {
		return fmt.Errorf("%w (compat fallback: %s: %v)", base, msg, cause)
	}
	return fmt.Errorf("%w (compat fallback: %s)", base, msg)
}
