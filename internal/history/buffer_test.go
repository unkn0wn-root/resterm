package history

import (
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/redact"
)

var (
	errDisk   = errors.New("disk I/O error")
	errLocked = errors.New("database is locked")
)

type memStore struct {
	mu      sync.Mutex
	rows    map[string]Entry
	order   []string
	writes  map[string]int
	fail    int
	delFail int
}

func (m *memStore) Load() error { return nil }

func (m *memStore) Append(e Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail > 0 {
		m.fail--
		return errDisk
	}
	if m.rows == nil {
		m.rows, m.writes = map[string]Entry{}, map[string]int{}
	}
	if _, ok := m.rows[e.ID]; !ok {
		m.order = append(m.order, e.ID)
	}
	m.rows[e.ID] = e
	m.writes[e.ID]++
	return nil
}

func (m *memStore) Entries() ([]Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Entry
	for _, id := range m.order {
		if e, ok := m.rows[id]; ok {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *memStore) ByRequest(string) ([]Entry, error)  { return nil, nil }
func (m *memStore) ByWorkflow(string) ([]Entry, error) { return nil, nil }
func (m *memStore) ByFile(string) ([]Entry, error)     { return nil, nil }

func (m *memStore) Delete(id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.delFail > 0 {
		m.delFail--
		return false, errLocked
	}
	_, ok := m.rows[id]
	delete(m.rows, id)
	return ok, nil
}

func (m *memStore) Close() error { return nil }

func (m *memStore) row(id string) Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rows[id]
}

func (m *memStore) total() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, w := range m.writes {
		n += w
	}
	return n
}

func secretN(i int) string { return fmt.Sprintf("budget-secret-%03d-abcdef", i) }

func words(n int) string { return strings.Repeat("~~~~ ", n/5+1)[:n] }

func bodyOf(n int, s string) string {
	w := words(n)
	return w[:100] + s + w[100+len(s):]
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func pct(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

func snippets(e Entry) []string {
	out := []string{e.BodySnippet}
	if e.Compare != nil {
		for _, r := range e.Compare.Results {
			out = append(out, r.BodySnippet)
		}
	}
	return out
}

func checkHeld(t *testing.T, b *Buffer) {
	t.Helper()
	n := 0
	for _, h := range b.entries {
		for _, s := range snippets(h.Entry) {
			if s != redact.Mask {
				n += len(s)
			}
		}
	}
	if n != b.total || b.total > remaskBudget {
		t.Fatalf("held %d bytes, total %d, budget %d", n, b.total, remaskBudget)
	}
}

type budgetRun struct {
	t     *testing.T
	ms    *memStore
	b     *Buffer
	ins   []Entry
	secs  []string
	plain bool
}

func newBudgetRun(t *testing.T) *budgetRun {
	ms := &memStore{}
	return &budgetRun{t: t, ms: ms, b: &Buffer{Store: ms}}
}

func (r *budgetRun) add(n int) Entry {
	r.t.Helper()
	i := len(r.ins) + 1
	s := secretN(i)
	e := Entry{ID: fmt.Sprint(i), URL: "https://api.test/x?k=" + s, BodySnippet: strings.Repeat("y", n)}
	if n >= 100+len(s) {
		e.BodySnippet = bodyOf(n, s)
		if r.plain {
			e.URL = "https://api.test/x"
		}
	}
	r.ins, r.secs = append(r.ins, e), append(r.secs, s)
	if err := r.b.Append(e); err != nil {
		r.t.Fatal(err)
	}
	if err := r.b.Flush(); err != nil {
		r.t.Fatal(err)
	}
	checkHeld(r.t, r.b)
	return e
}

func (r *budgetRun) learn(dropped ...string) {
	r.t.Helper()
	if err := r.b.Flush(r.secs...); err != nil {
		r.t.Fatal(err)
	}
	checkHeld(r.t, r.b)
	full := redact.New(r.secs...)
	for _, e := range r.ins {
		got, ok := r.ms.rows[e.ID]
		if !ok {
			continue
		}
		want, _ := e.masked(full)
		if slices.Contains(dropped, e.ID) {
			want = dropRows(want)
		}
		if got.URL != want.URL || !slices.Equal(snippets(got), snippets(want)) {
			r.t.Errorf("entry %s: URL %q, snippets %v, want %q, %v", e.ID, got.URL, heads(got), want.URL, heads(want))
		}
		for _, s := range r.secs {
			if strings.Contains(got.URL+strings.Join(snippets(got), ""), s) {
				r.t.Errorf("entry %s holds %s", e.ID, s)
			}
		}
	}
}

func dropRows(e Entry) Entry {
	if e.BodySnippet != "" {
		e.BodySnippet = redact.Mask
	}
	if c := e.Compare; c != nil {
		cc := *c
		cc.Results = slices.Clone(c.Results)
		for i := range cc.Results {
			if cc.Results[i].BodySnippet != "" {
				cc.Results[i].BodySnippet = redact.Mask
			}
		}
		e.Compare = &cc
	}
	return e
}

func heads(e Entry) []string {
	var out []string
	for _, s := range snippets(e) {
		out = append(out, s[:min(len(s), 24)])
	}
	return out
}

func TestBufferFlushMasksPending(t *testing.T) {
	ms := &memStore{}
	b := &Buffer{Store: ms}
	a := Entry{ID: "a", URL: "https://api.test/x?k=" + secret, BodySnippet: `{"t":"` + secret + `"}`}
	if err := b.Append(a); err != nil {
		t.Fatal(err)
	}
	if err := b.Flush(secret); err != nil {
		t.Fatal(err)
	}
	got := ms.row("a")
	if strings.Contains(got.URL+got.BodySnippet, secret) || got.URL != "https://api.test/x?k="+redact.Mask {
		t.Fatalf("stored %+v, want the secret masked", got)
	}
}

func TestBufferRemasksOnNewSecret(t *testing.T) {
	const old = "older-secret-0001-abcd"
	ms := &memStore{}
	b := &Buffer{Store: ms}
	at := time.Unix(100, 0)
	a := Entry{ID: "a", ExecutedAt: at, URL: "https://api.test/x?k=" + secret, Status: "403 saw " + secret}
	cut := Entry{ID: "cut", ExecutedAt: at, BodySnippet: strings.Repeat("x", 1995) + secret + strings.Repeat("y", 50)}
	layered := Entry{ID: "5layer", ExecutedAt: at, BodySnippet: `{"k":"` + form5(secret) + `","pad":"x"}`}
	plain := Entry{ID: "b", ExecutedAt: at, URL: "https://api.test/plain"}
	older := Entry{ID: "c", ExecutedAt: at, URL: "https://api.test/x?k=" + old}
	for _, e := range []Entry{a, cut, layered, plain, older} {
		if err := b.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Flush(old); err != nil {
		t.Fatal(err)
	}
	if got := ms.row("a"); !strings.Contains(got.URL, secret) {
		t.Fatalf("first write = %+v, want it raw before the secret is known", got)
	}
	if got := ms.row("cut").BodySnippet; !strings.HasSuffix(got, secret[:5]) {
		t.Fatalf("first write of the cut body ends %q", got[len(got)-12:])
	}
	if err := b.Flush(secret); err != nil {
		t.Fatal(err)
	}
	got := ms.row("a")
	if got.ID != a.ID || !got.ExecutedAt.Equal(at) || strings.Contains(got.URL+got.Status, secret) {
		t.Errorf("rewrite = %+v, want the same entry masked", got)
	}
	want, _ := cut.masked(redact.New(old, secret))
	if got := ms.row("cut").BodySnippet; strings.Contains(got, secret[:4]) || got != want.BodySnippet {
		t.Errorf("cut body after the secret ends %q, want it masked from the raw body", got[max(0, len(got)-12):])
	}
	if got, want := ms.row("5layer").BodySnippet, `{"k":"`+redact.Mask+`"`; !strings.HasPrefix(got, want) {
		t.Errorf("five-layer body = %q, want prefix %q", got, want)
	}
	for id, n := range map[string]int{"a": 2, "cut": 2, "5layer": 2, "b": 1, "c": 1} {
		if got := ms.writes[id]; got != n {
			t.Errorf("entry %s written %d times, want %d", id, got, n)
		}
	}
}

func form5(s string) string {
	e3 := strings.ReplaceAll(pct(b64(s)), "%", "&#37;")
	e2 := strings.NewReplacer("&", "%26", "#", "%23", ";", "%3B").Replace(e3)
	return strings.ReplaceAll(e2, "%", `\\x25`)
}

func TestBufferRewritesOnlyOnNewSecret(t *testing.T) {
	ms := &memStore{}
	b := &Buffer{Store: ms}
	if err := b.Append(Entry{ID: "a", URL: "https://api.test/x?k=" + secret}); err != nil {
		t.Fatal(err)
	}
	for _, secs := range [][]string{{secret}, {secret}, {""}, nil, {secret, ""}} {
		if err := b.Flush(secs...); err != nil {
			t.Fatal(err)
		}
	}
	if got := ms.total(); got != 1 {
		t.Fatalf("store got %d writes, want 1", got)
	}
}

func TestBufferWritesInAppendOrder(t *testing.T) {
	ms := &memStore{}
	b := &Buffer{Store: ms}
	for i, id := range []string{"1", "2", "3"} {
		if err := b.Append(Entry{ID: id, URL: "https://api.test/" + id + "?k=" + secretN(i)}); err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			if err := b.Flush(); err != nil {
				t.Fatal(err)
			}
		}
	}
	for i := range 3 {
		if err := b.Flush(secretN(i)); err != nil {
			t.Fatal(err)
		}
	}
	es, _ := ms.Entries()
	var ids []string
	for _, e := range es {
		ids = append(ids, e.ID)
	}
	if !slices.Equal(ids, []string{"1", "2", "3"}) || !slices.Equal(ms.order, ids) {
		t.Fatalf("stored %v in order %v, want 1 2 3", ids, ms.order)
	}
}

func TestBufferDeleteForgets(t *testing.T) {
	for name, writes := range map[string]int{"written": 1, "pending": 0} {
		ms := &memStore{}
		b := &Buffer{Store: ms}
		if err := b.Append(Entry{ID: "a", URL: "https://api.test/x?k=" + secret}); err != nil {
			t.Fatal(err)
		}
		if writes > 0 {
			if err := b.Flush(); err != nil {
				t.Fatal(err)
			}
		}
		if ok, err := b.Delete("a"); ok != (writes > 0) || err != nil {
			t.Fatalf("%s: Delete = %v, %v", name, ok, err)
		}
		if err := b.Flush(secret); err != nil {
			t.Fatal(err)
		}
		if es, _ := ms.Entries(); len(es) != 0 || ms.total() != writes {
			t.Errorf("%s: store holds %v after %d writes, want the entry gone", name, es, ms.total())
		}
	}
}

func TestBufferFailedDeleteKeepsMasking(t *testing.T) {
	const later = "later-cap-88dd-eeff"
	for name, flush := range map[string]bool{"written": true, "pending": false} {
		t.Run(name, func(t *testing.T) {
			ms := &memStore{delFail: 1}
			b := &Buffer{Store: ms}
			if err := b.Append(Entry{ID: "a", BodySnippet: `{"t":"` + secret + `","u":"` + later + `"}`}); err != nil {
				t.Fatal(err)
			}
			if flush {
				if err := b.Flush(); err != nil {
					t.Fatal(err)
				}
			}
			if ok, err := b.Delete("a"); ok || !errors.Is(err, errLocked) {
				t.Fatalf("Delete with a locked store = %v, %v, want false, %v", ok, err, errLocked)
			}
			if err := b.Flush(secret); err != nil {
				t.Fatal(err)
			}
			if got, want := ms.row("a").BodySnippet, `{"t":"`+redact.Mask+`","u":"`+later+`"}`; got != want {
				t.Fatalf("row after the failed delete = %q, want %q", got, want)
			}
			if ok, err := b.Delete("a"); !ok || err != nil {
				t.Fatalf("Delete again = %v, %v, want true, nil", ok, err)
			}
			if err := b.Flush(later); err != nil {
				t.Fatal(err)
			}
			if es, _ := ms.Entries(); len(es) != 0 || len(b.entries) != 0 || b.total != 0 {
				t.Fatalf("store holds %v, buffer holds %d entries and %d bytes, want the entry gone",
					es, len(b.entries), b.total)
			}
		})
	}
}

func TestBufferRetriesFailedRewrite(t *testing.T) {
	for name, e := range map[string]Entry{
		"probe hit": {ID: "a", URL: "https://api.test/x?k=" + secret, BodySnippet: `{"t":"` + secret + `"}`},
		"dropped":   {ID: "a", BodySnippet: bodyOf(remaskBudget+1, secret)},
	} {
		t.Run(name, func(t *testing.T) {
			ms := &memStore{}
			b := &Buffer{Store: ms}
			if err := b.Append(e); err != nil {
				t.Fatal(err)
			}
			if err := b.Flush(); err != nil {
				t.Fatal(err)
			}
			ms.fail = 1
			if err := b.Flush(secret); !errors.Is(err, errDisk) {
				t.Fatalf("Flush with a failing store = %v, want %v", err, errDisk)
			}
			for range 2 {
				if err := b.Flush(); err != nil {
					t.Fatal(err)
				}
			}
			got := ms.row("a")
			if text := got.URL + got.BodySnippet; strings.Contains(text, secret) {
				t.Errorf("row after the failed rewrite holds the secret: %q", text[:min(len(text), 160)])
			}
			if n := ms.writes["a"]; n != 2 {
				t.Errorf("row written %d times, want the first write and one retry", n)
			}
		})
	}
}

func TestBufferForgetsFailedFirstWrite(t *testing.T) {
	ms := &memStore{fail: 1}
	b := &Buffer{Store: ms}
	if err := b.Append(Entry{ID: "a", URL: "https://api.test/x?k=" + secret, BodySnippet: words(1000)}); err != nil {
		t.Fatal(err)
	}
	if err := b.Flush(); !errors.Is(err, errDisk) {
		t.Fatalf("Flush with a failing store = %v, want %v", err, errDisk)
	}
	for _, secs := range [][]string{{secret}, nil} {
		if err := b.Flush(secs...); err != nil {
			t.Fatal(err)
		}
	}
	if len(b.entries) != 0 || b.total != 0 || ms.total() != 0 {
		t.Fatalf("buffer holds %d entries and %d bytes, store got %d writes, want the entry forgotten",
			len(b.entries), b.total, ms.total())
	}
}

func TestBufferWritesWithLearnedSecrets(t *testing.T) {
	const sa, sb = "call-a-secret-0001-abcd", "call-b-secret-0002-abcd"
	ms := &memStore{}
	b := &Buffer{Store: ms}
	for _, e := range []struct{ id, secret string }{{"A", sa}, {"B", sb}} {
		b.Learn(e.secret)
		if err := b.Append(Entry{ID: e.id, BodySnippet: `{"t":"` + e.secret + `"}`}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		if err := b.Flush(); err != nil {
			t.Error(err)
		}
	})
	wg.Wait()
	if err := b.Flush(sa, sb); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"A", "B"} {
		if got, n := ms.row(id).BodySnippet, ms.writes[id]; got != `{"t":"`+redact.Mask+`"}` || n != 1 {
			t.Errorf("row %s = %q after %d writes, want it masked on its only write", id, got, n)
		}
	}
}

type gateStore struct {
	*memStore
	entered chan struct{}
	release chan struct{}
}

func (g gateStore) Append(e Entry) error {
	g.entered <- struct{}{}
	<-g.release
	return g.memStore.Append(e)
}

func TestBufferDoesNotWaitForFlush(t *testing.T) {
	g := gateStore{memStore: &memStore{}, entered: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(g.release) })
	defer release()
	b := &Buffer{Store: g}
	if err := b.Append(Entry{ID: "a", BodySnippet: `{"t":"` + secret + `"}`}); err != nil {
		t.Fatal(err)
	}
	flushed := make(chan error, 1)
	go func() { flushed <- b.Flush(secret) }()
	<-g.entered
	done := make(chan redact.Set)
	go func() {
		b.Learn("other-secret-0001-abcd")
		_ = b.Append(Entry{ID: "b", BodySnippet: "other-secret-0001-abcd"})
		done <- b.Secrets()
	}()
	select {
	case sec := <-done:
		if got := sec.Text("other-secret-0001-abcd"); got != redact.Mask {
			t.Errorf("Secrets() during a flush masks a learned secret as %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Learn, Append and Secrets waited for a flush blocked in the store")
	}
	release()
	if err := <-flushed; err != nil {
		t.Fatal(err)
	}
}

type gateDelete struct {
	*memStore
	entered chan struct{}
	release chan struct{}
}

func (g gateDelete) Delete(id string) (bool, error) {
	ok, err := g.memStore.Delete(id)
	g.entered <- struct{}{}
	<-g.release
	return ok, err
}

func TestBufferDeleteRacingFlush(t *testing.T) {
	g := gateDelete{memStore: &memStore{}, entered: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(g.release) })
	defer release()
	b := &Buffer{Store: g}
	if err := b.Append(Entry{ID: "a", BodySnippet: `{"t":"` + secret + `"}`}); err != nil {
		t.Fatal(err)
	}
	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
	deleted := make(chan error, 1)
	go func() {
		_, err := b.Delete("a")
		deleted <- err
	}()
	<-g.entered
	done := make(chan struct{})
	go func() {
		b.Learn("other-secret-0001-abcd")
		_ = b.Append(Entry{ID: "b"})
		_ = b.Secrets()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Learn, Append and Secrets waited for a delete blocked in the store")
	}
	flushed := make(chan error, 1)
	go func() { flushed <- b.Flush(secret) }()
	time.Sleep(50 * time.Millisecond)
	release()
	if err := <-flushed; err != nil {
		t.Fatal(err)
	}
	if err := <-deleted; err != nil {
		t.Fatal(err)
	}
	if _, ok := g.rows["a"]; ok || g.writes["a"] != 1 {
		t.Fatalf("store holds row a after %d writes, want it gone", g.writes["a"])
	}
}

func TestBufferConcurrent(t *testing.T) {
	ms := &memStore{}
	b := &Buffer{Store: ms}
	const n = 64
	var all []string
	for i := range n {
		all = append(all, secretN(i))
	}
	pinned := Entry{ID: "pinned", BodySnippet: strings.Join(all, " ")}
	if err := b.Append(pinned); err != nil {
		t.Fatal(err)
	}
	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			es, _ := b.Entries()
			if !slices.ContainsFunc(es, func(e Entry) bool { return e.ID == "pinned" }) {
				t.Error("a reader saw the pinned entry missing")
				return
			}
		}
	})
	var writers sync.WaitGroup
	for w := range 4 {
		writers.Go(func() {
			for i := w; i < n; i += 4 {
				id := fmt.Sprintf("e%d", i)
				b.Learn(all[i])
				_ = b.Append(Entry{ID: id, URL: "https://api.test/x?k=" + all[i]})
				if i%2 == 0 {
					_ = b.Flush(all[i])
				} else {
					_ = b.Flush()
				}
				if b.Secrets().Text(all[i]) != redact.Mask {
					t.Errorf("Secrets() misses %s after Learn", all[i])
				}
				if i%3 == 0 {
					_, _ = b.Delete(id)
				}
			}
		})
	}
	writers.Wait()
	close(done)
	wg.Wait()
	if got := ms.row("pinned").BodySnippet; strings.Contains(got, "budget-secret") {
		t.Fatalf("pinned entry still holds a secret: %q", got[:min(len(got), 80)])
	}
	checkHeld(t, b)
}

func TestBufferRemasksWithinBudget(t *testing.T) {
	for name, sizes := range map[string][]int{
		"eight eighths": slices.Repeat([]int{remaskBudget / 8}, 8),
		"one budget":    {remaskBudget},
	} {
		t.Run(name, func(t *testing.T) {
			r := newBudgetRun(t)
			for _, n := range sizes {
				r.add(n)
			}
			r.learn()
		})
	}
}

func TestBufferEvictsOldest(t *testing.T) {
	r := newBudgetRun(t)
	for range 9 {
		r.add(remaskBudget / 8)
	}
	r.learn("1")
	if got := r.ms.row("1"); got.BodySnippet != redact.Mask || got.URL != "https://api.test/x?k="+redact.Mask {
		t.Errorf("entry 1 = %q %q, want the mask", got.URL, got.BodySnippet[:min(len(got.BodySnippet), 24)])
	}

	r = newBudgetRun(t)
	r.plain = true
	for range 8 {
		r.add(remaskBudget / 8)
	}
	r.add(1)
	r.learn("1")
}

func TestBufferDropsEntryOverBudget(t *testing.T) {
	r := newBudgetRun(t)
	r.plain = true
	r.add(1000)
	r.add(remaskBudget + 1)
	r.add(1000)
	r.learn("2")
}

func TestBufferCountsCompareRows(t *testing.T) {
	r := newBudgetRun(t)
	r.add(1000)
	s1, s2 := secretN(101), secretN(102)
	cmp := &CompareEntry{Results: []CompareResult{
		{BodySnippet: bodyOf(remaskBudget/2, s1)},
		{BodySnippet: bodyOf(remaskBudget/2+1, s2)},
		{},
	}}
	before := *cmp
	before.Results = slices.Clone(cmp.Results)
	e := Entry{ID: "cmp", URL: "https://api.test/cmp", Compare: cmp}
	r.ins, r.secs = append(r.ins, e), append(r.secs, s1, s2)
	if err := r.b.Append(e); err != nil {
		t.Fatal(err)
	}
	if err := r.b.Flush(); err != nil {
		t.Fatal(err)
	}
	checkHeld(t, r.b)
	r.learn("cmp")
	if got := snippets(r.ms.row("cmp")); !slices.Equal(got, []string{"", redact.Mask, redact.Mask, ""}) {
		t.Errorf("compare rows = %v, want the two bodies masked and the empty ones kept", heads(r.ms.row("cmp")))
	}
	if !reflect.DeepEqual(*cmp, before) {
		t.Error("the buffer changed the caller's CompareEntry")
	}
}

func TestBufferFirstWriteIsExact(t *testing.T) {
	const old = "older-secret-0001-abcd"
	zeros := strings.Repeat("\x00", 70<<10)
	run := b64(zeros + old + zeros)
	pad := words(remaskBudget)
	for name, tt := range map[string]struct{ body, want string }{
		"base64 run": {body: strings.Repeat("x", 10) + " " + run + " " + pad, want: strings.Repeat("x", 10) + " " + redact.Mask},
		"five layers": {
			body: `{"k":"` + form5(old) + `","pad":"` + pad + `"}`,
			want: `{"k":"` + redact.Mask + `"`,
		},
	} {
		ms := &memStore{}
		b := &Buffer{Store: ms}
		if err := b.Flush(old); err != nil {
			t.Fatal(err)
		}
		if err := b.Append(Entry{ID: "big", BodySnippet: tt.body}); err != nil {
			t.Fatal(err)
		}
		if err := b.Flush(); err != nil {
			t.Fatal(err)
		}
		if got := ms.row("big").BodySnippet; !strings.HasPrefix(got, tt.want) {
			t.Errorf("%s: first write = %q, want prefix %q", name, got[:min(len(got), 40)], tt.want)
		}
	}
}

func TestBufferFailsClosedOnce(t *testing.T) {
	ms := &memStore{}
	b := &Buffer{Store: ms}
	for _, e := range []Entry{
		{ID: "empty", URL: "https://api.test/empty"},
		{ID: "big", BodySnippet: words(remaskBudget/2 + 1)},
		{ID: "next", BodySnippet: words(remaskBudget / 2)},
	} {
		if err := b.Append(e); err != nil {
			t.Fatal(err)
		}
		if err := b.Flush(); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{"fresh-secret-0001-abcd", "fresh-secret-0002-abcd"} {
		if err := b.Flush(s); err != nil {
			t.Fatal(err)
		}
	}
	if got := ms.row("big").BodySnippet; got != redact.Mask {
		t.Errorf("dropped entry = %q, want the mask", got[:min(len(got), 24)])
	}
	for id, n := range map[string]int{"empty": 1, "big": 2, "next": 1} {
		if got := ms.writes[id]; got != n {
			t.Errorf("entry %s written %d times, want %d", id, got, n)
		}
	}
}

func TestBufferDeleteReleasesBytes(t *testing.T) {
	r := newBudgetRun(t)
	for range 4 {
		r.add(remaskBudget / 8)
	}
	if _, err := r.b.Delete("4"); err != nil {
		t.Fatal(err)
	}
	checkHeld(t, r.b)
	for range 5 {
		r.add(remaskBudget / 8)
	}
	r.learn()

	r = newBudgetRun(t)
	for range 9 {
		r.add(remaskBudget / 8)
	}
	if _, err := r.b.Delete("1"); err != nil {
		t.Fatal(err)
	}
	checkHeld(t, r.b)
	r.add(remaskBudget / 8)
	r.learn("2")

	r = newBudgetRun(t)
	for range 8 {
		r.add(remaskBudget / 8)
	}
	if _, err := r.b.Delete("1"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		r.add(remaskBudget / 8)
	}
	r.learn("2")
}
