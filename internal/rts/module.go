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
	fs     FS
	mu     sync.RWMutex
	ent    map[string]*modEnt
	newEnv func() *Env
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

// NewCache creates a module cache that reads modules through fs. Each module
// runs in a new top-level scope returned by newEnv.
func NewCache(fs FS, newEnv func() *Env) *ModCache {
	if fs == nil {
		fs = OSFS{}
	}
	return &ModCache{fs: fs, ent: map[string]*modEnt{}, newEnv: newEnv}
}

func (c *ModCache) Load(ctx *Ctx, base, path string) (*Comp, error) {
	if path == "" {
		return nil, fmt.Errorf("empty module path")
	}

	p, err := newModPath(base, path)
	if err != nil {
		return nil, err
	}

	fp, err := c.stat(p.name)
	if err != nil {
		return nil, err
	}

	settled := watcher.Settled(fp.mod)
	ent, ok := c.get(p, fp)
	if ok {
		return ent.comp, nil
	}

	data, err := c.fs.ReadFile(p.name)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if ent != nil && ent.fp == fp && ent.sum == sum {
		if settled {
			c.set(p, &modEnt{comp: ent.comp, fp: fp, sum: sum, settled: true})
		}
		return ent.comp, nil
	}

	mod, err := ParseModule(NewFile(p.name, data))
	if err != nil {
		return nil, err
	}

	cx := ctx.CloneNoIO()
	comp, err := exec(cx, mod, c.newEnv())
	if err != nil {
		return nil, err
	}
	c.set(p, &modEnt{comp: comp, fp: fp, sum: sum, settled: settled})
	return comp, nil
}

// get returns the cached entry and whether fp alone proves it current.
func (c *ModCache) get(p modPath, fp modFP) (*modEnt, bool) {
	c.mu.RLock()
	ent := c.ent[p.key]
	c.mu.RUnlock()
	return ent, ent != nil && ent.settled && ent.fp == fp
}

func (c *ModCache) set(p modPath, ent *modEnt) {
	c.mu.Lock()
	c.ent[p.key] = ent
	c.mu.Unlock()
}

func (c *ModCache) stat(path string) (modFP, error) {
	info, err := c.fs.Stat(path)
	if err != nil {
		return modFP{}, err
	}
	return modFP{mod: info.ModTime(), size: info.Size()}, nil
}

// A modPath is a module path in two forms. name is the @use path joined to
// the base directory, so the module is read and shown the way the file that
// uses it is. key is the absolute form of name and keys the cache. A file used
// through both a relative and an absolute path shares one entry and shows the
// form it was last compiled under.
type modPath struct {
	name string
	key  string
}

func newModPath(base, path string) (modPath, error) {
	name := filepath.Clean(path)
	if !filepath.IsAbs(name) {
		name = filepath.Join(base, name)
	}
	key, err := filepath.Abs(name)
	if err != nil {
		return modPath{}, err
	}
	return modPath{name: name, key: key}, nil
}
