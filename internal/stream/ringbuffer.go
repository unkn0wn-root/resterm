package stream

type ringBuffer struct {
	items    []*Event
	size     int
	count    int
	head     int
	bytes    int64
	maxBytes int64
	evicted  uint64
}

func newRingBuffer(size int, maxBytes int64) *ringBuffer {
	if size <= 0 {
		size = 1
	}

	return &ringBuffer{
		items:    make([]*Event, size),
		size:     size,
		maxBytes: maxBytes,
	}
}

func (r *ringBuffer) append(evt *Event) {
	if r.size == 0 {
		return
	}

	if r.count == r.size {
		r.evictOldest()
	}
	r.items[(r.head+r.count)%r.size] = evt
	r.count++
	size := evt.Size()
	r.bytes += size
	if size == 0 {
		return
	}

	// Keep the newest event even when it is larger than the byte limit.
	for r.maxBytes > 0 && r.bytes > r.maxBytes && r.count > 1 {
		r.evictOldest()
	}
}

func (r *ringBuffer) evictOldest() {
	r.bytes -= r.items[r.head].Size()
	r.items[r.head] = nil
	r.head = (r.head + 1) % r.size
	r.count--
	r.evicted++
}

func (r *ringBuffer) snapshot() []*Event {
	if r.count == 0 {
		return nil
	}

	out := make([]*Event, r.count)
	for i := range r.count {
		out[i] = r.items[(r.head+i)%r.size]
	}
	return out
}
