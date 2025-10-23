package stream

type ringBuffer struct {
	items []*Event
	size  int
	count int
	head  int
}

func newRingBuffer(size int) *ringBuffer {
	if size <= 0 {
		size = 1
	}

	return &ringBuffer{
		items: make([]*Event, size),
		size:  size,
	}
}

func (r *ringBuffer) append(evt *Event) {
	if r.size == 0 {
		return
	}

	if r.count < r.size {
		idx := (r.head + r.count) % r.size
		r.items[idx] = evt
		r.count++
		return
	}

	r.items[r.head] = evt
	r.head = (r.head + 1) % r.size
}

func (r *ringBuffer) snapshot() []*Event {
	if r.count == 0 {
		return nil
	}

	out := make([]*Event, r.count)
	for i := 0; i < r.count; i++ {
		idx := (r.head + i) % r.size
		out[i] = r.items[idx]
	}
	return out
}
