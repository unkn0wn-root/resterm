package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"gopkg.in/yaml.v3"

	"github.com/unkn0wn-root/resterm/internal/openapi"
	"github.com/unkn0wn-root/resterm/internal/util"
)

const parRefPre = "#/components/parameters/"

var pathMs = [...]string{
	"get",
	"put",
	"post",
	"delete",
	"options",
	"head",
	"patch",
	"trace",
	"query",
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
	for _, k := range util.SortedKeys(prm) {
		h, ok := hdrParam(prm, k, map[string]bool{k: true})
		if ok {
			fx.pm[k] = h
		}
	}

	return fx
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
	for _, p := range util.SortedKeys(pts) {
		pi := mapAt(pts, p)
		if len(pi) == 0 {
			continue
		}
		fx.walkPi(pi, []string{"paths", p})
	}
}

func (fx *hdrFix) walkPi(pi map[string]any, seg []string) {
	seen := make(map[string]bool)
	add := func(key string, op map[string]any, path []string) {
		if len(op) == 0 {
			return
		}
		seen[strings.ToLower(key)] = true
		fx.walkOp(op, path)
	}

	for _, m := range pathMs {
		op := mapAt(pi, m)
		add(m, op, segAdd(seg, m))
	}

	extra := mapAt(pi, "additionalOperations")
	for _, name := range util.SortedKeys(extra) {
		op := mapAt(extra, name)
		if len(op) == 0 {
			continue
		}
		seen[strings.ToLower(name)] = true
		fx.walkOp(op, segAdd(seg, "additionalOperations", name))
	}

	for _, key := range util.SortedKeys(pi) {
		lk := strings.ToLower(key)
		if seen[lk] {
			continue
		}
		switch lk {
		case "$ref", "summary", "description", "servers", "parameters", "additionaloperations":
			continue
		}
		op := mapAt(pi, key)
		add(key, op, segAdd(seg, key))
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
	for _, code := range util.SortedKeys(rs) {
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
	for _, k := range util.SortedKeys(rbs) {
		rb := mapAt(rbs, k)
		if len(rb) == 0 {
			continue
		}
		fx.walkRb(rb, segAdd(seg, k))
	}
}

func (fx *hdrFix) walkRb(rb map[string]any, seg []string) {
	cnt := mapAt(rb, "content")
	for _, mt := range util.SortedKeys(cnt) {
		m := mapAt(cnt, mt)
		if len(m) == 0 {
			continue
		}
		enc := mapAt(m, "encoding")
		for _, fld := range util.SortedKeys(enc) {
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
	for _, cbn := range util.SortedKeys(cbs) {
		cb := mapAt(cbs, cbn)
		if len(cb) == 0 {
			continue
		}
		if _, ok := strAt(cb, "$ref"); ok {
			continue
		}
		for _, exp := range util.SortedKeys(cb) {
			pi := mapAt(cb, exp)
			if len(pi) == 0 {
				continue
			}
			fx.walkPi(pi, segAdd(seg, cbn, exp))
		}
	}
}

func (fx *hdrFix) fixHs(hs map[string]any, seg []string) {
	for _, hk := range util.SortedKeys(hs) {
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

func loadDoc(path string, opts openapi.ParseOptions) (*v3.Document, []string, error) {
	raw, rErr := os.ReadFile(path)
	if rErr != nil {
		return nil, nil, fmt.Errorf("read spec file: %w", rErr)
	}

	if bytes.Contains(raw, []byte(parRefPre)) {
		raw2, ws, n, fErr := fixHdrRefs(raw)
		if fErr == nil && n > 0 {
			doc, err := buildDoc(path, raw2, opts)
			if err == nil {
				return doc, ws, nil
			}

			doc2, err2 := buildDoc(path, raw, opts)
			if err2 == nil {
				return doc2, nil, nil
			}
			return nil, nil, compatErr(err2, "load rewritten spec", err)
		}

		doc, err := buildDoc(path, raw, opts)
		if err == nil {
			return doc, nil, nil
		}
		if fErr != nil {
			return nil, nil, compatErr(err, "rewrite spec", fErr)
		}
		if n == 0 {
			return nil, nil, compatErr(err, "no header-ref rewrite candidates found", nil)
		}
		return nil, nil, err
	}

	doc, err := buildDoc(path, raw, opts)
	if err != nil {
		return nil, nil, err
	}
	return doc, nil, nil
}

func buildDoc(path string, raw []byte, opts openapi.ParseOptions) (*v3.Document, error) {
	cfg := datamodel.NewDocumentConfiguration()
	cfg.SkipExternalRefResolution = !opts.ResolveExternalRefs
	if opts.ResolveExternalRefs {
		cp := filepath.Clean(path)
		cfg.AllowFileReferences = true
		cfg.BasePath = filepath.Dir(cp)
		cfg.SpecFilePath = cp
	}

	doc, err := libopenapi.NewDocumentWithConfiguration(raw, cfg)
	if err != nil {
		return nil, err
	}

	mod, mErr := doc.BuildV3Model()
	if mErr != nil {
		return nil, mErr
	}
	if mod == nil {
		return nil, fmt.Errorf("build OpenAPI model: nil model")
	}

	return &mod.Model, nil
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
