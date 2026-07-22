package scroll

// Align returns a y-offset that keeps the selection away from viewport edges.
// It behaves like a lightweight scrolloff: nudge just enough to keep a small buffer.
func Align(sel, off, h, total int) int {
	buf := max(h/4, 1)
	return AlignWithBuffer(sel, off, h, total, buf)
}

// AlignWithBuffer is like Align but allows an explicit buffer size.
func AlignWithBuffer(sel, off, h, total, buf int) int {
	if h <= 0 || total <= 0 {
		return 0
	}
	if sel < 0 {
		sel = 0
	}
	if sel >= total {
		sel = total - 1
	}
	if h > total {
		h = total
	}
	maxOff := total - h
	if off < 0 {
		off = 0
	}
	if off > maxOff {
		off = maxOff
	}

	if sel >= total-1 {
		return maxOff
	}

	if buf < 0 {
		buf = 0
	}
	if buf > h-1 {
		buf = h - 1
	}
	top := off + buf
	bot := off + h - 1 - buf
	if sel < top {
		return clamp(sel-buf, 0, maxOff)
	}
	if sel > bot {
		shift := sel - bot
		return clamp(off+shift, 0, maxOff)
	}
	return off
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Reveal returns a y-offset that makes sure the span [start,end] is visible
// with a small buffer above and below when possible. If the span is already
// comfortably visible, the current offset is returned.
func Reveal(start, end, off, h, total int) int {
	if h <= 0 || total <= 0 {
		return 0
	}
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if end >= total {
		end = total - 1
	}
	if h > total {
		h = total
	}
	maxOff := max(total-h, 0)
	off = clamp(off, 0, maxOff)

	buf := max(h/5, 1)
	top := off + buf
	bot := off + h - 1 - buf
	if start >= top && end <= bot {
		return off
	}

	offset := clamp(start-buf, 0, maxOff)
	need := max(end-h+1+buf, 0)
	if offset < need {
		offset = clamp(need, 0, maxOff)
	}
	return offset
}
