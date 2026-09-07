package recorder

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/restwriter"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type Mode string

const (
	Requests Mode = "requests"
	Mocks    Mode = "mocks"
	Both     Mode = "both"
)

const baseName = "recordedBaseUrl"

var errMode = errors.New("record mode must be requests, mocks, or both")

func ParseMode(raw string) (Mode, error) {
	m := Mode(raw)
	if !m.valid() {
		return "", errMode
	}
	return m, nil
}

func (m Mode) valid() bool { return m == Requests || m == Mocks || m == Both }

func (m Mode) Wants(kind Mode) bool { return m == kind || m == Both }

type ExportOptions struct {
	Mode     Mode
	Path     string
	Upstream string
	Existing *restfile.Document
}

type Exclusion struct {
	ID     uint64
	Mode   Mode
	Reason Issue
}

// Shadow identifies a mock with the same match rules as an earlier mock.
// By is the earlier capture ID, or zero for a mock already in the document.
type Shadow struct {
	ID uint64
	By uint64
}

type Export struct {
	ID    uint64
	Names []string
}

type Fixture struct {
	Path string
	Data []byte
}

// Plan holds an export with fixture paths relative to Document.Path.
// Build does not write files. Call PublishFixtures before inserting Text.
type Plan struct {
	Document   *restfile.Document
	Text       string
	Fixtures   []Fixture
	Excluded   []Exclusion
	Shadowed   []Shadow
	Exported   []Export
	Redactions int
}

func Build(entries []Entry, opts ExportOptions) (*Plan, error) {
	if !opts.Mode.valid() {
		return nil, errMode
	}
	if opts.Path == "" {
		return nil, errors.New("save the request file before exporting recordings")
	}
	dir, err := fixtureDir()
	if err != nil {
		return nil, err
	}

	b := &builder{
		plan:  &Plan{Document: &restfile.Document{Path: opts.Path}},
		opts:  opts,
		dir:   dir,
		names: DeclaredNames(opts.Existing),
		keys:  declaredMatches(opts.Existing),
	}
	b.base = b.plan.declareBase(opts)
	for _, e := range entries {
		b.add(e)
	}
	return b.finish()
}

type builder struct {
	plan  *Plan
	opts  ExportOptions
	base  string
	dir   string
	names map[string]struct{}
	keys  map[string]uint64

	blocks []string
}

func (b *builder) add(e Entry) {
	b.plan.Redactions += e.Request.Redactions + e.Response.Redactions

	var names []string
	if b.opts.Mode.Wants(Requests) {
		if name, ok := b.addRequest(e); ok {
			names = append(names, name)
		}
	}
	if b.opts.Mode.Wants(Mocks) {
		if name, ok := b.addMock(e); ok {
			names = append(names, name)
		}
	}
	if len(names) > 0 {
		b.plan.Exported = append(b.plan.Exported, Export{ID: e.ID, Names: names})
	}
}

func (b *builder) addRequest(e Entry) (string, bool) {
	r, issue := entryRequest(e, b.base, b.opts.Upstream, b.names)
	if !issue.ok() {
		b.exclude(e.ID, Requests, issue)
		return "", false
	}

	doc := &restfile.Document{Path: b.plan.Document.Path, Requests: []*restfile.Request{r}}
	fixture := fmt.Sprintf("%s/%d-request.body", b.dir, e.ID)
	if err := b.render(doc, &r.Body, e.Request.Body, fixture); err != nil {
		b.exclude(e.ID, Requests, issueRequestRender)
		return "", false
	}
	b.plan.Document.Requests = append(b.plan.Document.Requests, r)
	return r.Metadata.Name, true
}

func (b *builder) addMock(e Entry) (string, bool) {
	m, issue := entryMock(e, b.names)
	if !issue.ok() {
		b.exclude(e.ID, Mocks, issue)
		return "", false
	}

	doc := &restfile.Document{Path: b.plan.Document.Path, Mocks: []*restfile.Mock{m}}
	fixture := fmt.Sprintf("%s/%d-response.body", b.dir, e.ID)
	if err := b.render(doc, &m.Responses[0].Body, e.Response.Body, fixture); err != nil {
		b.exclude(e.ID, Mocks, issueMockRender)
		return "", false
	}
	b.claim(e.ID, m)
	b.plan.Document.Mocks = append(b.plan.Document.Mocks, m)
	return m.Name, true
}

func (b *builder) render(doc *restfile.Document, dst *restfile.BodySource, data []byte, fixture string) error {
	if len(data) > 0 && inlinable(data) {
		dst.Text = string(data)
		switch text, err := renderBlock(doc); {
		case err == nil:
			b.blocks = append(b.blocks, text)
			return nil
		case !errors.Is(err, errInlineBody):
			return err
		}
		dst.Text = ""
	}
	if len(data) > 0 {
		dst.FilePath = filepath.ToSlash(fixture)
	}

	text, err := renderBlock(doc)
	if err != nil {
		return err
	}
	if dst.FilePath != "" {
		b.plan.Fixtures = append(b.plan.Fixtures, Fixture{Path: dst.FilePath, Data: bytes.Clone(data)})
	}
	b.blocks = append(b.blocks, text)
	return nil
}

func (b *builder) exclude(id uint64, mode Mode, reason Issue) {
	b.plan.Excluded = append(b.plan.Excluded, Exclusion{ID: id, Mode: mode, Reason: reason})
}

func (b *builder) claim(id uint64, m *restfile.Mock) {
	key := matchKey(m)
	if prior, taken := b.keys[key]; taken {
		b.plan.Shadowed = append(b.plan.Shadowed, Shadow{ID: id, By: prior})
		return
	}
	b.keys[key] = id
}

func (b *builder) finish() (*Plan, error) {
	p := b.plan
	if len(p.Document.Requests) == 0 {
		p.Document.Variables = nil
	}
	if len(b.blocks) == 0 {
		return p, nil
	}

	var text strings.Builder
	if len(p.Document.Variables) > 0 {
		preamble, err := restwriter.Render(
			&restfile.Document{Path: p.Document.Path, Variables: p.Document.Variables},
			restwriter.Options{},
		)
		if err != nil {
			return nil, errors.New("recording cannot be rendered safely")
		}
		text.WriteString(preamble)
	}
	for _, block := range b.blocks {
		text.WriteString(block)
	}

	p.Text = text.String()
	if err := verifyJoin(p); err != nil {
		return nil, err
	}
	return p, nil
}

func verifyJoin(p *Plan) error {
	doc := parser.Parse(p.Document.Path, []byte(p.Text))
	if parser.Check(doc) != nil ||
		len(doc.Requests) != len(p.Document.Requests) ||
		len(doc.Mocks) != len(p.Document.Mocks) ||
		len(doc.Variables) != len(p.Document.Variables) {
		return errors.New("recording does not round-trip through the request parser")
	}
	return nil
}

func fixtureDir() (string, error) {
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("name recording fixtures: %w", err)
	}
	return "resterm-record-" + hex.EncodeToString(nonce[:]), nil
}

func DeclaredNames(doc *restfile.Document) map[string]struct{} {
	used := make(map[string]struct{})
	if doc == nil {
		return used
	}
	for _, r := range doc.Requests {
		used[r.Metadata.Name] = struct{}{}
	}
	for _, m := range doc.Mocks {
		used[m.Name] = struct{}{}
		used[m.Sequence] = struct{}{}
	}
	return used
}

func declaredMatches(doc *restfile.Document) map[string]uint64 {
	keys := make(map[string]uint64)
	if doc == nil {
		return keys
	}
	for _, m := range doc.Mocks {
		keys[matchKey(m)] = 0
	}
	return keys
}

func matchKey(m *restfile.Mock) string {
	var compact bytes.Buffer
	if json.Compact(&compact, m.Match.JSON) != nil {
		compact.Reset()
	}
	key, _ := json.Marshal(struct {
		Method, Path string
		Query        map[string]restfile.MockQueryRule
		Headers      map[string]restfile.MockHeaderRule
		JSON         string
	}{m.Method, m.Path, m.Match.Query, m.Match.Headers, compact.String()})
	return string(key)
}

// Variable names are case-insensitive, so new declarations can shadow existing bindings.
func (p *Plan) declareBase(opts ExportOptions) string {
	bound := boundValues(opts.Existing)
	for n := 1; ; n++ {
		name := baseName
		if n > 1 {
			name += strconv.Itoa(n)
		}
		value, exists := bound.Get(name)
		if !exists {
			p.Document.Variables = []restfile.Variable{
				{Name: name, Value: opts.Upstream, Scope: directive.ScopeFile},
			}
			return name
		}
		if value == opts.Upstream {
			return name
		}
	}
}

// Write lower-precedence sources first so the winning binding survives.
func boundValues(doc *restfile.Document) vars.NameMap[string] {
	var bound vars.NameMap[string]
	if doc == nil {
		return bound
	}
	for _, v := range slices.Concat(doc.Variables, doc.Globals) {
		bound.Set(v.Name, v.Value)
	}
	for _, c := range doc.Constants {
		bound.Set(c.Name, c.Value)
	}
	return bound
}
