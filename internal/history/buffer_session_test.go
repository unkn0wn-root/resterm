package history

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/redact"
)

type op struct {
	kind byte
	e    Entry
	secs []string
	id   string
}

func budgetSessions() map[string][]op {
	const mib = 1 << 20
	out := map[string][]op{}
	id := 0
	ent := func(body string) Entry {
		id++
		return Entry{ID: fmt.Sprint(id), BodySnippet: body, URL: "/x"}
	}
	session := func(name string, sizes []int, del map[int]string) {
		var ops []op
		var all []string
		for i, n := range sizes {
			s := secretN(i)
			ops = append(ops, op{kind: 'a', e: ent(bodyOf(n, s))}, op{kind: 'f'})
			if d, ok := del[i]; ok {
				ops = append(ops, op{kind: 'd', id: d})
			}
			all = append(all, s)
		}
		out[name] = append(ops, op{kind: 'f', secs: all})
	}
	session("20 of 1 MiB", slices.Repeat([]int{mib}, 20), nil)
	session("16 of 1 MiB", slices.Repeat([]int{mib}, 16), nil)
	session("16 of 1 MiB and a byte", append(slices.Repeat([]int{mib}, 15), mib+1), nil)
	for _, d := range []int{-1, 0, 1} {
		session(fmt.Sprintf("budget%+d then small", d), []int{remaskBudget + d, 5000}, nil)
	}
	base := id
	session("20 of 1 MiB with deletes", slices.Repeat([]int{mib}, 20),
		map[int]string{12: fmt.Sprint(base + 10), 15: fmt.Sprint(base + 13)})
	var ops []op
	var all []string
	for i := range 6 {
		e := ent("")
		e.Compare = &CompareEntry{}
		for r := range 3 {
			s := secretN(100 + 3*i + r)
			e.Compare.Results = append(e.Compare.Results, CompareResult{BodySnippet: bodyOf(3*mib, s)})
			all = append(all, s)
		}
		ops = append(ops, op{kind: 'a', e: e}, op{kind: 'f'})
	}
	out["6 compares of 3 rows of 3 MiB"] = append(ops, op{kind: 'f', secs: all})

	rng := rand.New(rand.NewPCG(442, 5))
	forms := []func(string) string{
		func(s string) string { return s },
		pct,
		b64,
	}
	for n := range 10 {
		var ops, later []op
		var ids []string
		next := 0
		body := func() (string, string) {
			var size int
			switch r := rng.IntN(100); {
			case r < 60:
				size = 1000 + rng.IntN(7000)
			case r < 85:
				size = mib/2 + rng.IntN(5*mib/2)
			default:
				size = remaskBudget - 1 + rng.IntN(3)
			}
			s := secretN(1000 + next)
			next++
			f := forms[rng.IntN(len(forms))](s)
			at := rng.IntN(min(1900, size-len(f)))
			if rng.IntN(2) == 0 {
				at = rng.IntN(size - len(f))
			}
			w := words(size)
			return w[:at] + f + w[at+len(f):], s
		}
		for range 8 + rng.IntN(13) {
			b, s := body()
			e := ent(b)
			secs := []string{s}
			if rng.IntN(10) == 0 {
				e.BodySnippet = ""
				e.Compare = &CompareEntry{}
				for range 2 + rng.IntN(2) {
					b, s := body()
					e.Compare.Results = append(e.Compare.Results, CompareResult{BodySnippet: b})
					secs = append(secs, s)
				}
			}
			ids = append(ids, e.ID)
			ops = append(ops, op{kind: 'a', e: e})
			if rng.IntN(8) == 0 {
				ops = append(ops, op{kind: 'd', id: ids[rng.IntN(len(ids))]})
			}
			f := op{kind: 'f'}
			if rng.IntN(3) == 0 {
				f.secs = secs
			} else {
				later = append(later, op{kind: 'f', secs: secs})
			}
			if rng.IntN(3) != 0 {
				ops = append(ops, f)
			}
			if len(later) > 0 && rng.IntN(3) == 0 {
				k := rng.IntN(len(later))
				ops = append(ops, later[k])
				later = slices.Delete(later, k, k+1)
			}
		}
		ops = append(ops, op{kind: 'f'})
		out[fmt.Sprintf("random %d", n)] = append(ops, later...)
	}
	return out
}

func dropModel(ops []op) []map[string]bool {
	type ref struct {
		id                               string
		size                             int
		written, deleted, dropped, shown bool
	}
	var rs []*ref
	known := map[string]bool{}
	var out []map[string]bool
	held := func() int {
		n := 0
		for _, r := range rs {
			if r.written && !r.deleted && !r.dropped {
				n += r.size
			}
		}
		return n
	}
	for _, o := range ops {
		switch o.kind {
		case 'a':
			n := 0
			for _, s := range snippets(o.e) {
				n += len(s)
			}
			rs = append(rs, &ref{id: o.e.ID, size: n})
		case 'd':
			for _, r := range rs {
				if r.id == o.id {
					r.deleted = true
				}
			}
		case 'f':
			fresh := false
			for _, s := range o.secs {
				if s != "" && !known[s] {
					known[s], fresh = true, true
				}
			}
			for _, r := range rs {
				if fresh && r.written && r.dropped {
					r.shown = true
				}
			}
			for _, r := range rs {
				if r.written || r.deleted {
					continue
				}
				r.written = true
				r.dropped = r.size > remaskBudget
				for held() > remaskBudget {
					i := slices.IndexFunc(
						rs,
						func(q *ref) bool { return q.written && !q.deleted && !q.dropped && q.size > 0 },
					)
					rs[i].dropped = true
				}
			}
		}
		m := map[string]bool{}
		for _, r := range rs {
			if r.shown && !r.deleted && r.size > 0 {
				m[r.id] = true
			}
		}
		out = append(out, m)
	}
	return out
}

func TestBufferBudgetSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("masks hundreds of MiB")
	}
	for name, ops := range budgetSessions() {
		t.Run(name, func(t *testing.T) {
			ms := &memStore{}
			b := &Buffer{Store: ms}
			raws := map[string]Entry{}
			var known []string
			model := dropModel(ops)
			for i, o := range ops {
				switch o.kind {
				case 'a':
					_ = b.Append(o.e)
					raws[o.e.ID] = o.e
				case 'd':
					_, _ = b.Delete(o.id)
					delete(raws, o.id)
				case 'f':
					pending := map[string]bool{}
					for id := range raws {
						if _, ok := ms.rows[id]; !ok {
							pending[id] = true
						}
					}
					_ = b.Flush(o.secs...)
					for _, s := range o.secs {
						if s != "" && !slices.Contains(known, s) {
							known = append(known, s)
						}
					}
					sec := redact.New(known...)
					for id := range pending {
						if want, _ := raws[id].masked(sec); !slices.Equal(snippets(ms.rows[id]), snippets(want)) {
							t.Errorf("op %d: first write of %s differs from the exact one", i, id)
						}
					}
				}
				checkHeld(t, b)
				for id, e := range ms.rows {
					for j, s := range snippets(e) {
						if snippets(raws[id])[j] != "" && (s == redact.Mask) != model[i][id] {
							t.Errorf("op %d: entry %s row %d shows the mask %v, want %v", i, id, j, s == redact.Mask,
								model[i][id])
						}
					}
				}
			}
			sec := redact.New(known...)
			for id, e := range ms.rows {
				want, _ := raws[id].masked(sec)
				for j, s := range snippets(e) {
					if s != snippets(want)[j] && s != redact.Mask {
						t.Errorf("entry %s row %d is neither exact nor the mask: %q", id, j, s[:min(len(s), 40)])
					}
				}
			}
		})
	}
}

func BenchmarkBufferRemask(b *testing.B) {
	const n = 200
	for _, size := range []int{remaskBudget / n, 900 << 10} {
		body := jsonBody(size)
		for _, k := range []int{1, 16} {
			for _, step := range []string{"first", "later"} {
				b.Run(fmt.Sprintf("%dKiB/K=%d/%s", size>>10, k, step), func(b *testing.B) {
					held := 0
					for i := 0; b.Loop(); i++ {
						b.StopTimer()
						buf := &Buffer{Store: &memStore{}}
						for j := range n {
							_ = buf.Append(Entry{
								ID:          fmt.Sprint(j),
								BodySnippet: body,
								RequestText: "GET https://api.example.test/items?page=1\nAccept: application/json\n",
								URL:         "https://api.example.test/items",
								Status:      "200 OK",
							})
						}
						_ = buf.Flush()
						fresh := func(r int) []string {
							out := make([]string, k)
							for x := range out {
								out[x] = fmt.Sprintf("fresh-%d-%d-%02d-abcdefgh", i, r, x)
							}
							return out
						}
						if step == "later" {
							_ = buf.Flush(fresh(0)...)
						}
						held = buf.total
						b.StartTimer()
						_ = buf.Flush(fresh(1)...)
					}
					b.ReportMetric(float64(held)/(1<<20), "heldMiB")
				})
			}
		}
	}
}
