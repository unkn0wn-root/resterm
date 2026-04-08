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
	err  error
}

type Manager struct {
	mu       sync.Mutex
	cache    map[string]credential
	inflight map[string]*call
	now      func() time.Time
	run      runFunc
}

func NewManager() *Manager {
	return &Manager{
		cache:    make(map[string]credential),
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

func (m *Manager) Cached(env string, cfg Config) (Result, bool) {
	cfg = cfg.normalize()
	key := cacheKey(env, cfg)
	if key == "" {
		return Result{}, false
	}
	cred, ok := m.cached(key, cfg, m.now())
	if !ok {
		return Result{}, false
	}
	return renderResult(cfg, cred), true
}

func (m *Manager) Resolve(ctx context.Context, env string, cfg Config) (Result, error) {
	cfg = cfg.normalize()
	if err := validate(cfg); err != nil {
		return Result{}, err
	}
	if !cfg.usesCache() {
		cred, err := m.fetch(ctx, cfg)
		if err != nil {
			return Result{}, err
		}
		return renderResult(cfg, cred), nil
	}

	key := cacheKey(env, cfg)
	if cred, ok := m.cached(key, cfg, m.now()); ok {
		return renderResult(cfg, cred), nil
	}

	m.mu.Lock()
	if c, ok := m.inflight[key]; ok {
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
	c := &call{done: make(chan struct{})}
	m.inflight[key] = c
	m.mu.Unlock()

	cred, err := m.fetch(ctx, cfg)
	if err == nil {
		m.store(key, cred)
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

func (m *Manager) cached(key string, cfg Config, now time.Time) (credential, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.cache[key]
	if !ok {
		return credential{}, false
	}
	valid, purge := validAt(entry, cfg, now)
	if !valid {
		if purge {
			delete(m.cache, key)
		}
		return credential{}, false
	}
	return entry, true
}

func (m *Manager) store(key string, cred credential) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache[key] = cred
}
