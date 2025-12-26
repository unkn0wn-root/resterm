package navigator

import (
	"strings"
	"unicode"

	"github.com/unkn0wn-root/resterm/internal/ui/scroll"
)

// Kind identifies the type of a node in the navigator tree.
type Kind int

const (
	KindFile Kind = iota
	KindRequest
	KindWorkflow
	KindDir
)

type Payload[T any] struct {
	FilePath string
	Data     T
}

// Node represents a tree item.
type Node[T any] struct {
	ID       string
	Title    string
	Desc     string
	Kind     Kind
	Method   string
	Tags     []string
	Target   string
	Count    int
	Badges   []string
	HasName  bool
	Expanded bool
	Children []*Node[T]
	Payload  Payload[T]
}

// Flat is a visible row with indentation level.
type Flat[T any] struct {
	Node  *Node[T]
	Level int
}

// Model manages tree state and filtering.
type Model[T any] struct {
	nodes         []*Node[T]
	flat          []Flat[T]
	sel           int
	offset        int
	viewHeight    int
	filter        string
	methodFilters map[string]bool
	tagFilters    map[string]bool
	compact       bool
}

// New builds a navigator model.
func New[T any](nodes []*Node[T]) *Model[T] {
	m := &Model[T]{
		nodes:         nodes,
		methodFilters: make(map[string]bool),
		tagFilters:    make(map[string]bool),
	}
	m.refresh()
	return m
}

// SetNodes replaces the tree and keeps selection stable where possible.
func (m *Model[T]) SetNodes(nodes []*Node[T]) {
	m.nodes = nodes
	m.refresh()
}

// SetViewportHeight constrains the number of visible rows (0 = no limit).
func (m *Model[T]) SetViewportHeight(height int) {
	if height < 0 {
		height = 0
	}
	m.viewHeight = height
	m.ensureVisible()
}

// SetCompact toggles compact rendering hints.
func (m *Model[T]) SetCompact(v bool) {
	m.compact = v
}

// SetFilter updates text filter and refreshes visible rows.
func (m *Model[T]) SetFilter(s string) {
	if m.filter == s {
		return
	}
	m.filter = s
	m.refresh()
}

// ToggleMethodFilter flips an active method chip.
func (m *Model[T]) ToggleMethodFilter(method string) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return
	}
	if m.methodFilters[method] && len(m.methodFilters) == 1 {
		// Toggle off when it is the only active method.
		delete(m.methodFilters, method)
	} else {
		// Single-select: replace existing methods with the requested one.
		m.methodFilters = map[string]bool{method: true}
	}
	m.refresh()
}

// ClearMethodFilters removes all method chips.
func (m *Model[T]) ClearMethodFilters() {
	if len(m.methodFilters) == 0 {
		return
	}
	m.methodFilters = make(map[string]bool)
	m.refresh()
}

// ToggleTagFilter flips a tag chip.
func (m *Model[T]) ToggleTagFilter(tag string) {
	tag = strings.TrimSpace(strings.ToLower(tag))
	if tag == "" {
		return
	}
	if m.tagFilters[tag] {
		delete(m.tagFilters, tag)
	} else {
		m.tagFilters[tag] = true
	}
	m.refresh()
}

// ClearTagFilters removes all tag chips.
func (m *Model[T]) ClearTagFilters() {
	if len(m.tagFilters) == 0 {
		return
	}
	m.tagFilters = make(map[string]bool)
	m.refresh()
}

// MethodFilters returns active method chips.
func (m *Model[T]) MethodFilters() map[string]bool {
	out := make(map[string]bool, len(m.methodFilters))
	for k, v := range m.methodFilters {
		if v {
			out[k] = v
		}
	}
	return out
}

// TagFilters returns active tag chips.
func (m *Model[T]) TagFilters() map[string]bool {
	out := make(map[string]bool, len(m.tagFilters))
	for k, v := range m.tagFilters {
		if v {
			out[k] = v
		}
	}
	return out
}

// Move selection by delta, clamping to visible rows.
func (m *Model[T]) Move(delta int) {
	if len(m.flat) == 0 {
		m.sel = -1
		m.offset = 0
		return
	}
	m.sel += delta
	m.ensureVisible()
}

// SelectFirst selects the first visible row.
func (m *Model[T]) SelectFirst() {
	if len(m.flat) == 0 {
		m.sel = -1
		m.offset = 0
		return
	}
	m.sel = 0
	m.ensureVisible()
}

// SelectLast selects the last visible row.
func (m *Model[T]) SelectLast() {
	if len(m.flat) == 0 {
		m.sel = -1
		m.offset = 0
		return
	}
	m.sel = len(m.flat) - 1
	m.ensureVisible()
}

// Selected returns the active node.
func (m *Model[T]) Selected() *Node[T] {
	if m.sel < 0 || m.sel >= len(m.flat) {
		return nil
	}
	return m.flat[m.sel].Node
}

// SelectByID selects the first visible node with the given id.
func (m *Model[T]) SelectByID(id string) bool {
	if id == "" || len(m.flat) == 0 {
		return false
	}
	for i, row := range m.flat {
		if row.Node != nil && row.Node.ID == id {
			m.sel = i
			m.ensureVisible()
			return true
		}
	}
	return false
}

// ToggleExpanded toggles expansion on the selected node.
func (m *Model[T]) ToggleExpanded() {
	n := m.Selected()
	if n == nil || len(n.Children) == 0 {
		return
	}
	n.Expanded = !n.Expanded
	m.refresh()
}

// ExpandAll opens every branch.
func (m *Model[T]) ExpandAll() {
	for _, n := range m.nodes {
		expandTree(n)
	}
	m.refresh()
}

// CollapseAll closes every branch.
func (m *Model[T]) CollapseAll() {
	for _, n := range m.nodes {
		collapseTree(n)
	}
	m.refresh()
}

// Rows returns the visible flattened rows.
func (m *Model[T]) Rows() []Flat[T] {
	return m.flat
}

// VisibleRows returns rows within the viewport window.
func (m *Model[T]) VisibleRows() []Flat[T] {
	if len(m.flat) == 0 {
		return nil
	}
	if m.viewHeight <= 0 {
		return m.flat
	}
	if m.offset < 0 {
		m.offset = 0
	}

	end := m.offset + m.viewHeight
	if end > len(m.flat) {
		end = len(m.flat)
	}
	return m.flat[m.offset:end]
}

// Refresh rebuilds visible rows.
func (m *Model[T]) Refresh() {
	m.refresh()
}

// ReplaceChildren swaps children under a given node id.
func (m *Model[T]) ReplaceChildren(id string, children []*Node[T]) {
	if id == "" {
		return
	}
	for _, n := range m.nodes {
		if replaceChildren(n, id, children) {
			break
		}
	}
	m.refresh()
}

// Find finds a node by id.
func (m *Model[T]) Find(id string) *Node[T] {
	if id == "" {
		return nil
	}
	for _, n := range m.nodes {
		if found := findNode(n, id); found != nil {
			return found
		}
	}
	return nil
}

func (m *Model[T]) refresh() {
	m.flat = flatten(m.nodes, 0, m.filter, m.methodFilters, m.tagFilters)
	if len(m.flat) == 0 {
		m.sel = -1
		m.offset = 0
		return
	}
	m.ensureVisible()
}

func (m *Model[T]) ensureVisible() {
	if len(m.flat) == 0 {
		m.sel = -1
		m.offset = 0
		return
	}
	if m.sel < 0 {
		m.sel = 0
	}
	if m.sel >= len(m.flat) {
		m.sel = len(m.flat) - 1
	}
	if m.viewHeight <= 0 {
		m.offset = 0
		return
	}
	m.offset = scroll.Align(m.sel, m.offset, m.viewHeight, len(m.flat))
}

func flatten[T any](
	nodes []*Node[T],
	level int,
	filter string,
	methods map[string]bool,
	tags map[string]bool,
) []Flat[T] {
	var rows []Flat[T]
	for _, n := range nodes {
		childRows, ok := visible(n, level, filter, methods, tags)
		if ok {
			rows = append(rows, childRows...)
		}
	}
	return rows
}

func visible[T any](
	n *Node[T],
	level int,
	filter string,
	methods map[string]bool,
	tags map[string]bool,
) ([]Flat[T], bool) {
	if n == nil {
		return nil, false
	}
	matches := nodeMatches(n, filter, methods, tags)
	var childRows []Flat[T]
	childMatch := false
	expanded := n.Expanded
	if filter != "" {
		expanded = true
	}
	for _, c := range n.Children {
		rows, ok := visible(c, level+1, filter, methods, tags)
		if ok {
			childMatch = true
			if expanded {
				childRows = append(childRows, rows...)
			}
		}
	}

	if !matches && !childMatch {
		return nil, false
	}

	self := Flat[T]{Node: n, Level: level}
	if len(childRows) == 0 {
		return []Flat[T]{self}, true
	}
	return append([]Flat[T]{self}, childRows...), true
}

func nodeMatches[T any](
	n *Node[T],
	filter string,
	methods map[string]bool,
	tags map[string]bool,
) bool {
	if n == nil {
		return false
	}
	if len(methods) > 0 {
		switch n.Kind {
		case KindRequest:
			if !methods[strings.ToUpper(n.Method)] {
				return false
			}
		case KindWorkflow, KindFile, KindDir:
		default:
		}
	}

	if len(tags) > 0 && !containsAnyTag(n.Tags, tags) {
		return false
	}

	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}

	queryTokens := filterTokens(filter)
	if len(queryTokens) == 0 {
		return true
	}

	candidateTokens := nodeTokens(n)
	for _, q := range queryTokens {
		if !tokenInCandidates(q, candidateTokens) {
			return false
		}
	}
	return true
}

func expandTree[T any](n *Node[T]) {
	if n == nil {
		return
	}
	if len(n.Children) > 0 {
		n.Expanded = true
		for _, c := range n.Children {
			expandTree(c)
		}
	}
}

func collapseTree[T any](n *Node[T]) {
	if n == nil {
		return
	}
	n.Expanded = false
	for _, c := range n.Children {
		collapseTree(c)
	}
}

func replaceChildren[T any](n *Node[T], id string, children []*Node[T]) bool {
	if n == nil {
		return false
	}
	if n.ID == id {
		n.Children = children
		return true
	}
	for _, c := range n.Children {
		if replaceChildren(c, id, children) {
			return true
		}
	}
	return false
}

func findNode[T any](n *Node[T], id string) *Node[T] {
	if n == nil {
		return nil
	}
	if n.ID == id {
		return n
	}
	for _, c := range n.Children {
		if found := findNode(c, id); found != nil {
			return found
		}
	}
	return nil
}

func containsAnyTag(tags []string, active map[string]bool) bool {
	if len(active) == 0 {
		return true
	}
	for _, t := range tags {
		if active[strings.ToLower(strings.TrimSpace(t))] {
			return true
		}
	}
	return false
}

func filterTokens(s string) []string {
	return wordsFromText(strings.ToLower(s))
}

func nodeTokens[T any](n *Node[T]) []string {
	if n == nil {
		return nil
	}
	fields := []string{
		n.Title,
		n.Desc,
		n.Method,
		n.Target,
		strings.Join(n.Tags, " "),
		strings.Join(n.Badges, " "),
		n.Payload.FilePath,
		n.ID,
	}
	return wordsFromText(strings.ToLower(strings.Join(fields, " ")))
}

func tokenInCandidates(token string, candidates []string) bool {
	if token == "" {
		return true
	}
	for _, c := range candidates {
		if strings.HasPrefix(c, token) {
			return true
		}
	}
	return false
}

func wordsFromText(text string) []string {
	if text == "" {
		return nil
	}
	var tokens []string
	var buf []rune
	allowed := func(r rune) bool {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), unicode.Is(unicode.Mn, r):
			return true
		case r == '.', r == '-', r == '_', r == '#', r == ':':
			return true
		default:
			return false
		}
	}
	flush := func() {
		if len(buf) == 0 {
			return
		}
		tokens = append(tokens, string(buf))
		buf = buf[:0]
	}
	for _, r := range text {
		if allowed(r) {
			buf = append(buf, unicode.ToLower(r))
		} else {
			flush()
		}
	}
	flush()
	return tokens
}
