package rts

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/unkn0wn-root/resterm/internal/watcher"
)

type FS interface {
	ReadFile(path string) ([]byte, error)
	Stat(path string) (os.FileInfo, error)
}

type OSFS struct{}

func (OSFS) ReadFile(path string) ([]byte, error)  { return os.ReadFile(path) }
func (OSFS) Stat(path string) (os.FileInfo, error) { return os.Stat(path) }

// ModCache compiles each .rts module once and recompiles it when the file changes.
type ModCache struct {
	fs  FS
	mu  sync.RWMutex
	ent map[string]*modEnt
	std func() map[string]Value
}

type modEnt struct {
	comp *Comp
	fp   modFP
	sum  [sha256.Size]byte
	// settled means fp had settled before the content was read, so a later
	// write must change fp.
	settled bool
}

type modFP struct {
	mod  time.Time
	size int64
}

// NewCache creates a module cache using fs and the provided compile-time prelude.
func NewCache(fs FS, std func() map[string]Value) *ModCache {
	if fs == nil {
		fs = OSFS{}
	}
	return &ModCache{fs: fs, ent: map[string]*modEnt{}, std: prelude(std)}
}

// SetStdlib installs the prelude used when compiling modules through the cache.
func (c *ModCache) SetStdlib(fn func() map[string]Value) {
	if fn == nil {
		return
	}
	c.mu.Lock()
	c.std = prelude(fn)
	c.mu.Unlock()
}

func (c *ModCache) Load(ctx *Ctx, base, path string) (*Comp, string, error) {
	if path == "" {
		return nil, "", fmt.Errorf("empty module path")
	}

	p, err := absPath(base, path)
	if err != nil {
		return nil, "", err
	}

	fp, err := c.stat(p)
	if err != nil {
		return nil, p, err
	}

	settled := watcher.Settled(fp.mod)
	ent, ok := c.get(p, fp)
	if ok {
		return ent.comp, p, nil
	}

	data, err := c.fs.ReadFile(p)
	if err != nil {
		return nil, p, err
	}
	sum := sha256.Sum256(data)
	if ent != nil && ent.fp == fp && ent.sum == sum {
		if settled {
			c.set(p, &modEnt{comp: ent.comp, fp: fp, sum: sum, settled: true})
		}
		return ent.comp, p, nil
	}

	mod, err := ParseModule(p, data)
	if err != nil {
		return nil, p, err
	}

	cx := ctx.CloneNoIO()
	comp, err := Exec(cx, mod, c.std())
	if err != nil {
		return nil, p, err
	}
	c.set(p, &modEnt{comp: comp, fp: fp, sum: sum, settled: settled})
	return comp, p, nil
}

// get returns the cached entry and whether fp alone proves it current.
func (c *ModCache) get(path string, fp modFP) (*modEnt, bool) {
	c.mu.RLock()
	ent := c.ent[path]
	c.mu.RUnlock()
	return ent, ent != nil && ent.settled && ent.fp == fp
}

func (c *ModCache) set(path string, ent *modEnt) {
	c.mu.Lock()
	c.ent[path] = ent
	c.mu.Unlock()
}

func (c *ModCache) stat(path string) (modFP, error) {
	info, err := c.fs.Stat(path)
	if err != nil {
		return modFP{}, err
	}
	return modFP{mod: info.ModTime(), size: info.Size()}, nil
}

func prelude(fn func() map[string]Value) func() map[string]Value {
	if fn != nil {
		return fn
	}
	return func() map[string]Value {
		return map[string]Value{}
	}
}

func absPath(base, path string) (string, error) {
	p := path
	if !filepath.IsAbs(p) && base != "" {
		p = filepath.Join(base, p)
	}
	p = filepath.Clean(p)
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return abs, nil
}
