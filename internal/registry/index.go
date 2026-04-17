package registry

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type docSet[T any] struct {
	fs []T
	gs []T
}

type set[T any, S comparable] struct {
	mu  sync.RWMutex
	by  map[string]docSet[T]
	ks  []string
	fsc S
	gsc S
	sc  func(T) S
	nm  func(T) string
}

func newSet[T any, S comparable](
	fsc S,
	gsc S,
	sc func(T) S,
	nm func(T) string,
) *set[T, S] {
	return &set[T, S]{
		by:  make(map[string]docSet[T]),
		fsc: fsc,
		gsc: gsc,
		sc:  sc,
		nm:  nm,
	}
}

func splitByScope[T any, S comparable](xs []T, fsc, gsc S, sc func(T) S) docSet[T] {
	var out docSet[T]
	for _, x := range xs {
		switch sc(x) {
		case fsc:
			out.fs = append(out.fs, x)
		case gsc:
			out.gs = append(out.gs, x)
		}
	}
	return out
}

func (s *set[T, S]) split(xs []T) docSet[T] {
	return splitByScope(xs, s.fsc, s.gsc, s.sc)
}

func (s *set[T, S]) load(src map[string][]T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.by = make(map[string]docSet[T], len(src))
	s.ks = s.ks[:0]
	for p, xs := range src {
		ds := s.split(xs)
		if len(ds.fs) == 0 && len(ds.gs) == 0 {
			continue
		}
		s.by[p] = ds
		s.ks = append(s.ks, p)
	}
	sort.Strings(s.ks)
}

func (s *set[T, S]) sync(p string, xs []T) {
	if p == "" {
		return
	}

	ds := s.split(xs)

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(ds.fs) == 0 && len(ds.gs) == 0 {
		delete(s.by, p)
		s.reindex()
		return
	}

	if _, ok := s.by[p]; !ok {
		s.ks = append(s.ks, p)
		sort.Strings(s.ks)
	}
	s.by[p] = ds
}

func (s *set[T, S]) drop(p string) {
	if p == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.by, p)
	s.reindex()
}

func (s *set[T, S]) reindex() {
	s.ks = s.ks[:0]
	for p := range s.by {
		s.ks = append(s.ks, p)
	}
	sort.Strings(s.ks)
}

func (s *set[T, S]) layers(p string, xs []T) ([]T, []T) {
	cur := s.split(xs)
	fs := cloneSlice(cur.fs)
	gs := cloneSlice(cur.gs)

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, k := range s.ks {
		if k == p {
			continue
		}
		gs = append(gs, s.by[k].gs...)
	}
	return fs, gs
}

func (s *set[T, S]) named(p string, xs []T, n string) (T, bool) {
	key := nameKey(n)
	if key == "" {
		var z T
		return z, false
	}

	cur := s.split(xs)
	if v, ok := findNamed(cur.fs, key, s.nm); ok {
		return v, true
	}
	if v, ok := findNamed(cur.gs, key, s.nm); ok {
		return v, true
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, k := range s.ks {
		if k == p {
			continue
		}
		if v, ok := findNamed(s.by[k].gs, key, s.nm); ok {
			return v, true
		}
	}

	var z T
	return z, false
}

func (s *set[T, S]) deflt(p string, xs []T) (T, bool) {
	cur := s.split(xs)
	if v, ok := findDefault(cur.fs, s.nm); ok {
		return v, true
	}
	if v, ok := findDefault(cur.gs, s.nm); ok {
		return v, true
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, k := range s.ks {
		if k == p {
			continue
		}
		if v, ok := findDefault(s.by[k].gs, s.nm); ok {
			return v, true
		}
	}

	var z T
	return z, false
}

type Index struct {
	mu    sync.RWMutex
	root  string
	rec   bool
	ssh   *set[restfile.SSHProfile, restfile.SSHScope]
	auth  *set[restfile.AuthProfile, restfile.AuthScope]
	k8s   *set[restfile.K8sProfile, restfile.K8sScope]
	patch *set[restfile.PatchProfile, restfile.PatchScope]
}

func New() *Index {
	return &Index{
		ssh: newSet(
			restfile.SSHScopeFile,
			restfile.SSHScopeGlobal,
			func(v restfile.SSHProfile) restfile.SSHScope { return v.Scope },
			func(v restfile.SSHProfile) string { return v.Name },
		),
		auth: newSet(
			restfile.AuthScopeFile,
			restfile.AuthScopeGlobal,
			func(v restfile.AuthProfile) restfile.AuthScope { return v.Scope },
			func(v restfile.AuthProfile) string { return v.Name },
		),
		k8s: newSet(
			restfile.K8sScopeFile,
			restfile.K8sScopeGlobal,
			func(v restfile.K8sProfile) restfile.K8sScope { return v.Scope },
			func(v restfile.K8sProfile) string { return v.Name },
		),
		patch: newSet(
			restfile.PatchScopeFile,
			restfile.PatchScopeGlobal,
			func(v restfile.PatchProfile) restfile.PatchScope { return v.Scope },
			func(v restfile.PatchProfile) string { return v.Name },
		),
	}
}

func (ix *Index) Match(root string, rec bool) bool {
	if ix == nil {
		return false
	}

	root = pathKey(root)

	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.root == root && ix.rec == rec
}

func (ix *Index) Load(root string, rec bool) {
	if ix == nil {
		return
	}

	root = pathKey(root)
	if root == "" {
		ix.reset(root, rec)
		return
	}

	fs, err := filesvc.ListRequestFiles(root, rec)
	if err != nil {
		ix.reset(root, rec)
		return
	}

	sm := make(map[string][]restfile.SSHProfile, len(fs))
	am := make(map[string][]restfile.AuthProfile, len(fs))
	km := make(map[string][]restfile.K8sProfile, len(fs))
	pm := make(map[string][]restfile.PatchProfile, len(fs))

	for _, f := range fs {
		b, err := os.ReadFile(f.Path)
		if err != nil {
			continue
		}
		doc := parser.Parse(f.Path, b)
		p := pathKey(f.Path)
		sm[p] = doc.SSH
		am[p] = doc.Auth
		km[p] = doc.K8s
		pm[p] = doc.Patches
	}

	ix.ssh.load(sm)
	ix.auth.load(am)
	ix.k8s.load(km)
	ix.patch.load(pm)

	ix.mu.Lock()
	ix.root = root
	ix.rec = rec
	ix.mu.Unlock()
}

func (ix *Index) reset(root string, rec bool) {
	ix.ssh.load(nil)
	ix.auth.load(nil)
	ix.k8s.load(nil)
	ix.patch.load(nil)

	ix.mu.Lock()
	ix.root = root
	ix.rec = rec
	ix.mu.Unlock()
}

func (ix *Index) Sync(doc *restfile.Document) {
	if ix == nil {
		return
	}

	p := docPath(doc)
	if p == "" {
		return
	}

	ix.ssh.sync(p, docSSH(doc))
	ix.auth.sync(p, docAuth(doc))
	ix.k8s.sync(p, docK8s(doc))
	ix.patch.sync(p, docPatch(doc))
}

func (ix *Index) Drop(path string) {
	if ix == nil {
		return
	}

	p := pathKey(path)
	if p == "" {
		return
	}

	ix.ssh.drop(p)
	ix.auth.drop(p)
	ix.k8s.drop(p)
	ix.patch.drop(p)
}

func (ix *Index) SSH(doc *restfile.Document) ([]restfile.SSHProfile, []restfile.SSHProfile) {
	if ix == nil {
		ds := ixSplitSSH(doc)
		return ds.fs, ds.gs
	}
	return ix.ssh.layers(docPath(doc), docSSH(doc))
}

func (ix *Index) K8s(doc *restfile.Document) ([]restfile.K8sProfile, []restfile.K8sProfile) {
	if ix == nil {
		ds := ixSplitK8s(doc)
		return ds.fs, ds.gs
	}
	return ix.k8s.layers(docPath(doc), docK8s(doc))
}

func (ix *Index) Patch(doc *restfile.Document) ([]restfile.PatchProfile, []restfile.PatchProfile) {
	if ix == nil {
		ds := ixSplitPatch(doc)
		return ds.fs, ds.gs
	}
	return ix.patch.layers(docPath(doc), docPatch(doc))
}

func (ix *Index) PatchNamed(doc *restfile.Document, name string) (*restfile.PatchProfile, bool) {
	if ix == nil {
		ds := ixSplitPatch(doc)
		if v, ok := findNamed(
			ds.fs,
			nameKey(name),
			func(v restfile.PatchProfile) string { return v.Name },
		); ok {
			return &v, true
		}
		if v, ok := findNamed(
			ds.gs,
			nameKey(name),
			func(v restfile.PatchProfile) string { return v.Name },
		); ok {
			return &v, true
		}
		return nil, false
	}

	v, ok := ix.patch.named(docPath(doc), docPatch(doc), name)
	if !ok {
		return nil, false
	}
	return &v, true
}

func (ix *Index) DefaultAuth(doc *restfile.Document) (*restfile.AuthProfile, bool) {
	if ix == nil {
		ds := ixSplitAuth(doc)
		if v, ok := findDefault(ds.fs, func(v restfile.AuthProfile) string { return v.Name }); ok {
			cp := cloneAuth(v)
			return &cp, true
		}
		if v, ok := findDefault(ds.gs, func(v restfile.AuthProfile) string { return v.Name }); ok {
			cp := cloneAuth(v)
			return &cp, true
		}
		return nil, false
	}

	v, ok := ix.auth.deflt(docPath(doc), docAuth(doc))
	if !ok {
		return nil, false
	}
	cp := cloneAuth(v)
	return &cp, true
}

func cloneAuth(v restfile.AuthProfile) restfile.AuthProfile {
	v.Spec = restfile.CloneAuthSpecValue(v.Spec)
	if v.Spec.SourcePath == "" {
		v.Spec.SourcePath = v.SourcePath
	}
	return v
}

func cloneSlice[T any](xs []T) []T {
	if len(xs) == 0 {
		return nil
	}
	return append([]T(nil), xs...)
}

func findNamed[T any](xs []T, key string, nm func(T) string) (T, bool) {
	for _, x := range xs {
		if nameKey(nm(x)) == key {
			return x, true
		}
	}
	var z T
	return z, false
}

func findDefault[T any](xs []T, nm func(T) string) (T, bool) {
	for i := len(xs) - 1; i >= 0; i-- {
		if nameKey(nm(xs[i])) == "" {
			return xs[i], true
		}
	}
	var z T
	return z, false
}

func nameKey(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func pathKey(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	v = filepath.Clean(v)
	if abs, err := filepath.Abs(v); err == nil {
		return abs
	}
	return v
}

func docPath(doc *restfile.Document) string {
	if doc == nil {
		return ""
	}
	return pathKey(doc.Path)
}

func docSSH(doc *restfile.Document) []restfile.SSHProfile {
	if doc == nil {
		return nil
	}
	return doc.SSH
}

func docAuth(doc *restfile.Document) []restfile.AuthProfile {
	if doc == nil {
		return nil
	}
	return doc.Auth
}

func docK8s(doc *restfile.Document) []restfile.K8sProfile {
	if doc == nil {
		return nil
	}
	return doc.K8s
}

func docPatch(doc *restfile.Document) []restfile.PatchProfile {
	if doc == nil {
		return nil
	}
	return doc.Patches
}

func ixSplitSSH(doc *restfile.Document) docSet[restfile.SSHProfile] {
	return splitByScope(
		docSSH(doc),
		restfile.SSHScopeFile,
		restfile.SSHScopeGlobal,
		func(v restfile.SSHProfile) restfile.SSHScope { return v.Scope },
	)
}

func ixSplitAuth(doc *restfile.Document) docSet[restfile.AuthProfile] {
	return splitByScope(
		docAuth(doc),
		restfile.AuthScopeFile,
		restfile.AuthScopeGlobal,
		func(v restfile.AuthProfile) restfile.AuthScope { return v.Scope },
	)
}

func ixSplitK8s(doc *restfile.Document) docSet[restfile.K8sProfile] {
	return splitByScope(
		docK8s(doc),
		restfile.K8sScopeFile,
		restfile.K8sScopeGlobal,
		func(v restfile.K8sProfile) restfile.K8sScope { return v.Scope },
	)
}

func ixSplitPatch(doc *restfile.Document) docSet[restfile.PatchProfile] {
	return splitByScope(
		docPatch(doc),
		restfile.PatchScopeFile,
		restfile.PatchScopeGlobal,
		func(v restfile.PatchProfile) restfile.PatchScope { return v.Scope },
	)
}
