package rts

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FS interface {
	ReadFile(path string) ([]byte, error)
	Stat(path string) (os.FileInfo, error)
}

type OSFS struct{}

func (OSFS) ReadFile(path string) ([]byte, error)  { return os.ReadFile(path) }
func (OSFS) Stat(path string) (os.FileInfo, error) { return os.Stat(path) }

type ModCache struct {
	fs  FS
	mu  sync.RWMutex
	ent map[string]*modEnt
	std func() map[string]Value
}

type modEnt struct {
	comp *Comp
	fp   modFP
}

type modFP struct {
	mod  time.Time
	size int64
}

func NewCache(fs FS) *ModCache {
	if fs == nil {
		fs = OSFS{}
	}
	return &ModCache{fs: fs, ent: map[string]*modEnt{}, std: Stdlib}
}

func (c *ModCache) SetStdlib(fn func() map[string]Value) {
	if fn == nil {
		return
	}
	c.mu.Lock()
	c.std = fn
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

	if comp := c.get(p, fp); comp != nil {
		return comp, p, nil
	}

	data, err := c.fs.ReadFile(p)
	if err != nil {
		return nil, p, err
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
	c.set(p, fp, comp)
	return comp, p, nil
}

func (c *ModCache) get(path string, fp modFP) *Comp {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ent, ok := c.ent[path]
	if !ok {
		return nil
	}
	if ent.fp == fp {
		return ent.comp
	}
	return nil
}

func (c *ModCache) set(path string, fp modFP, comp *Comp) {
	c.mu.Lock()
	c.ent[path] = &modEnt{comp: comp, fp: fp}
	c.mu.Unlock()
}

func (c *ModCache) stat(path string) (modFP, error) {
	info, err := c.fs.Stat(path)
	if err != nil {
		return modFP{}, err
	}
	return modFP{mod: info.ModTime(), size: info.Size()}, nil
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
