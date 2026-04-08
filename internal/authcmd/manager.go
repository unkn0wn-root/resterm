package authcmd

import (
	"context"
	"sync"
	"time"
)

type runFunc func(context.Context, Config) (runOutput, error)

type call struct {
	done chan struct{}
	cred credential
	cfg  Config
	err  error
}

type entry struct {
	cred credential
	cfg  Config
}

type Prepared struct {
	key string
	cfg Config
}

type Manager struct {
	mu       sync.Mutex
	cache    map[string]entry
	inflight map[string]*call
	now      func() time.Time
	run      runFunc
}

func NewManager() *Manager {
	return &Manager{
		cache:    make(map[string]entry),
		inflight: make(map[string]*call),
		now:      time.Now,
		run:      run,
	}
}

func (m *Manager) SetRunFunc(fn func(context.Context, Config) (runOutput, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if fn == nil {
		m.run = run
		return
	}
	m.run = fn
}

func (m *Manager) SetExecFunc(fn func(context.Context, Config) ([]byte, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if fn == nil {
		m.run = run
		return
	}
	m.run = func(ctx context.Context, cfg Config) (runOutput, error) {
		out, err := fn(ctx, cfg)
		if err != nil {
			return runOutput{}, err
		}
		return runOutput{stdout: append([]byte(nil), out...)}, nil
	}
}

func (m *Manager) MergeCachedConfig(env string, cfg Config) Config {
	cfg = cfg.normalize()
	key := cacheKey(env, cfg)
	if key == "" {
		return cfg
	}
	ent, ok := m.cacheEntry(key)
	if !ok {
		return cfg
	}
	return mergeCfg(ent.cfg, cfg)
}

func (p Prepared) Config() Config {
	return p.cfg
}

func (m *Manager) Prepare(env string, cfg Config) (Prepared, error) {
	key, cfg, err := m.prepare(env, cfg)
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{key: key, cfg: cfg}, nil
}

func (m *Manager) Cached(env string, cfg Config) (Result, bool, error) {
	prep, err := m.Prepare(env, cfg)
	if err != nil {
		return Result{}, false, err
	}
	return m.CachedPrepared(prep)
}

func (m *Manager) CachedPrepared(prep Prepared) (Result, bool, error) {
	key := prep.key
	cfg := prep.cfg
	if key == "" {
		return Result{}, false, nil
	}

	now := m.now()
	m.mu.Lock()
	ent, ok := m.cache[key]
	if !ok {
		m.mu.Unlock()
		return Result{}, false, nil
	}
	if field, same := sameSeed(ent.cfg, cfg); !same {
		m.mu.Unlock()
		return Result{}, false, conflictError(cfg, field)
	}
	cred, ok := cachedEntry(ent, cfg, now)
	m.mu.Unlock()
	if !ok {
		return Result{}, false, nil
	}
	return renderResult(cfg, cred), true, nil
}

func (m *Manager) Resolve(ctx context.Context, env string, cfg Config) (Result, error) {
	prep, err := m.Prepare(env, cfg)
	if err != nil {
		return Result{}, err
	}
	return m.ResolvePrepared(ctx, prep)
}

func (m *Manager) ResolvePrepared(ctx context.Context, prep Prepared) (Result, error) {
	key := prep.key
	cfg := prep.cfg
	if key == "" {
		cred, err := m.fetch(ctx, cfg)
		if err != nil {
			return Result{}, err
		}
		return renderResult(cfg, cred), nil
	}

	now := m.now()
	m.mu.Lock()
	seed := cfg
	if ent, ok := m.cache[key]; ok {
		if field, same := sameSeed(ent.cfg, cfg); !same {
			m.mu.Unlock()
			return Result{}, conflictError(cfg, field)
		}
		if cred, ok := cachedEntry(ent, cfg, now); ok {
			m.mu.Unlock()
			return renderResult(cfg, cred), nil
		}
		seed = ent.cfg
	}
	if c, ok := m.inflight[key]; ok {
		if field, same := sameSeed(c.cfg, cfg); !same {
			m.mu.Unlock()
			return Result{}, conflictError(cfg, field)
		}
		done := c.done
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-done:
			if c.err != nil {
				return Result{}, c.err
			}
			return renderResult(cfg, c.cred), nil
		}
	}

	c := &call{done: make(chan struct{}), cfg: seed}
	m.inflight[key] = c
	m.mu.Unlock()

	cred, err := m.fetch(ctx, cfg)
	if err == nil {
		m.store(key, seed, cred)
	}
	c.cred = cred
	c.err = err
	close(c.done)

	m.mu.Lock()
	delete(m.inflight, key)
	m.mu.Unlock()

	if err != nil {
		return Result{}, err
	}
	return renderResult(cfg, cred), nil
}

func (m *Manager) fetch(ctx context.Context, cfg Config) (credential, error) {
	now := m.now()
	out, err := m.run(ctx, cfg)
	if err != nil {
		return credential{}, err
	}

	cred, err := extractCredential(cfg, out.stdout, now)
	if err != nil {
		return credential{}, err
	}
	cred.FetchedAt = now
	return cred, nil
}

func cachedEntry(ent entry, cfg Config, now time.Time) (credential, bool) {
	if ent.cred == (credential{}) {
		return credential{}, false
	}
	if !validAt(ent.cred, cfg, now) {
		return credential{}, false
	}
	return ent.cred, true
}

func (m *Manager) store(key string, cfg Config, cred credential) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache[key] = entry{cred: cred, cfg: cfg}
}

func (m *Manager) cacheEntry(key string) (entry, bool) {
	if key == "" {
		return entry{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ent, ok := m.cache[key]
	return ent, ok
}

func (m *Manager) prepare(env string, cfg Config) (string, Config, error) {
	cfg = cfg.normalize()
	key := cacheKey(env, cfg)
	if key == "" {
		cfg, err := Finalize(cfg)
		return "", cfg, err
	}

	ent, ok := m.cacheEntry(key)
	if ok {
		cfg = mergeCfg(ent.cfg, cfg)
	}
	cfg, err := Finalize(cfg)
	if err != nil {
		return key, cfg, err
	}
	if ok {
		if field, same := sameSeed(ent.cfg, cfg); !same {
			return key, cfg, conflictError(cfg, field)
		}
	}
	return key, cfg, nil
}
