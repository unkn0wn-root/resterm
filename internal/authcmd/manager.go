package authcmd

import (
	"context"
	"sync"
	"time"
)

type Result struct {
	Header string
	Value  string
	Token  string
	Type   string
	Expiry time.Time
}

type execFunc func(context.Context, Config) ([]byte, error)

type inflightCall struct {
	done chan struct{}
	cred credential
	cfg  Config
	err  error
}

type cacheRecord struct {
	cred credential
	cfg  Config
}

type resolvePlan struct {
	result  Result
	wait    *inflightCall
	start   *inflightCall
	seedCfg Config
	hit     bool
}

type Prepared struct {
	entryKey string
	cfg      Config
}

type Manager struct {
	mu       sync.Mutex
	cache    map[string]cacheRecord
	inflight map[string]*inflightCall
	now      func() time.Time
	exec     execFunc
}

func NewManager() *Manager {
	return &Manager{
		cache:    make(map[string]cacheRecord),
		inflight: make(map[string]*inflightCall),
		now:      time.Now,
		exec: func(ctx context.Context, cfg Config) ([]byte, error) {
			return run(ctx, cfg.command())
		},
	}
}

func (p Prepared) HeaderName() string {
	return p.cfg.HeaderName()
}

func (m *Manager) SetExecFunc(fn func(context.Context, Config) ([]byte, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if fn == nil {
		m.exec = func(ctx context.Context, cfg Config) ([]byte, error) {
			return run(ctx, cfg.command())
		}
		return
	}
	m.exec = fn
}

func (m *Manager) Prepare(env string, cfg Config) (Prepared, error) {
	entryKey, cfg, err := m.prepareConfig(env, cfg)
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{entryKey: entryKey, cfg: cfg}, nil
}

func (m *Manager) Cached(env string, cfg Config) (Result, bool, error) {
	prep, err := m.Prepare(env, cfg)
	if err != nil {
		return Result{}, false, err
	}
	return m.CachedPrepared(prep)
}

func (m *Manager) CachedPrepared(prep Prepared) (Result, bool, error) {
	entryKey := prep.entryKey
	cfg := prep.cfg
	if entryKey == "" {
		return Result{}, false, nil
	}
	return m.cachedPreparedResult(entryKey, cfg, m.now())
}

func (m *Manager) Resolve(ctx context.Context, env string, cfg Config) (Result, error) {
	prep, err := m.Prepare(env, cfg)
	if err != nil {
		return Result{}, err
	}
	return m.ResolvePrepared(ctx, prep)
}

func (m *Manager) ResolvePrepared(ctx context.Context, prep Prepared) (Result, error) {
	entryKey := prep.entryKey
	cfg := prep.cfg
	if entryKey == "" {
		cred, err := m.fetchCredential(ctx, cfg)
		if err != nil {
			return Result{}, err
		}
		return renderResult(cfg.output(), cred), nil
	}

	action, err := m.planResolve(entryKey, cfg, m.now())
	if err != nil {
		return Result{}, err
	}
	if action.hit {
		return action.result, nil
	}
	if action.wait != nil {
		return m.waitResolve(ctx, cfg, action.wait)
	}
	return m.resolveByFetch(ctx, entryKey, cfg, action.seedCfg, action.start)
}

func (m *Manager) fetchCredential(ctx context.Context, cfg Config) (credential, error) {
	out, err := m.exec(ctx, cfg)
	if err != nil {
		return credential{}, err
	}

	fetchedAt := m.now()
	cred, err := extractCredential(cfg.extract(), out, fetchedAt)
	if err != nil {
		return credential{}, err
	}
	cred.FetchedAt = fetchedAt
	return cred, nil
}

func cachedCredential(ent cacheRecord, ttl time.Duration, now time.Time) (credential, bool) {
	if ent.cred == (credential{}) {
		return credential{}, false
	}
	if !validAt(ent.cred, ttl, now) {
		return credential{}, false
	}
	return ent.cred, true
}

func resolveCacheRecord(ent cacheRecord, cfg Config, now time.Time) (credential, bool, error) {
	if field, same := ent.cfg.cacheSeedDiff(cfg); !same {
		return credential{}, false, conflictError(cfg, field)
	}
	cred, ok := cachedCredential(ent, cfg.TTL, now)
	return cred, ok, nil
}

func (m *Manager) store(key string, cfg Config, cred credential) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache[key] = cacheRecord{cred: cred, cfg: cfg}
}

func (m *Manager) lookupCacheRecord(key string) (cacheRecord, bool) {
	if key == "" {
		return cacheRecord{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ent, ok := m.cache[key]
	return ent, ok
}

func (m *Manager) prepareConfig(env string, cfg Config) (string, Config, error) {
	var err error

	cfg = cfg.normalize()
	entryKey := cacheEntryKey(env, cfg)
	if entryKey == "" {
		cfg, err = Finalize(cfg)
		return "", cfg, err
	}

	ent, ok := m.lookupCacheRecord(entryKey)
	if ok {
		cfg = cfg.inheritedFrom(ent.cfg)
	}
	cfg, err = Finalize(cfg)
	if err != nil {
		return entryKey, cfg, err
	}
	if ok {
		if field, same := ent.cfg.cacheSeedDiff(cfg); !same {
			return entryKey, cfg, conflictError(cfg, field)
		}
	}
	return entryKey, cfg, nil
}

func (m *Manager) cachedPreparedResult(
	entryKey string,
	cfg Config,
	now time.Time,
) (Result, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ent, ok := m.cache[entryKey]
	if !ok {
		return Result{}, false, nil
	}
	cred, ok, err := resolveCacheRecord(ent, cfg, now)
	if err != nil {
		return Result{}, false, err
	}
	if !ok {
		return Result{}, false, nil
	}
	return renderResult(cfg.output(), cred), true, nil
}

func (m *Manager) planResolve(entryKey string, cfg Config, now time.Time) (resolvePlan, error) {
	action := resolvePlan{seedCfg: cfg}

	m.mu.Lock()
	defer m.mu.Unlock()

	if ent, ok := m.cache[entryKey]; ok {
		cred, ok, err := resolveCacheRecord(ent, cfg, now)
		if err != nil {
			return resolvePlan{}, err
		}
		if ok {
			action.result = renderResult(cfg.output(), cred)
			action.hit = true
			return action, nil
		}
		action.seedCfg = ent.cfg
	}

	if c, ok := m.inflight[entryKey]; ok {
		if field, same := c.cfg.cacheSeedDiff(cfg); !same {
			return resolvePlan{}, conflictError(cfg, field)
		}
		action.wait = c
		return action, nil
	}

	action.start = &inflightCall{done: make(chan struct{}), cfg: action.seedCfg}
	m.inflight[entryKey] = action.start
	return action, nil
}

func (m *Manager) waitResolve(ctx context.Context, cfg Config, c *inflightCall) (Result, error) {
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-c.done:
		if c.err != nil {
			return Result{}, c.err
		}
		return renderResult(cfg.output(), c.cred), nil
	}
}

func (m *Manager) resolveByFetch(
	ctx context.Context,
	entryKey string,
	cfg Config,
	seed Config,
	c *inflightCall,
) (Result, error) {
	cred, err := m.fetchCredential(ctx, cfg)
	if err == nil {
		m.store(entryKey, seed, cred)
	}
	c.cred = cred
	c.err = err
	close(c.done)

	m.mu.Lock()
	delete(m.inflight, entryKey)
	m.mu.Unlock()

	if err != nil {
		return Result{}, err
	}
	return renderResult(cfg.output(), cred), nil
}
