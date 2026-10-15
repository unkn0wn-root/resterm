package rts

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"slices"
)

var ErrNotJSON = errors.New("value has no json form")

type JSONOptions struct {
	Max        int
	EscapeHTML bool
	Indent     string
	// Check validates values before encoding. Object internals are not checked.
	Check func(Value) error
}

// EncodeJSON follows json.Marshal's encoding of ToIface(v).
// With o.Max > 0, encoding stops just past that byte count; the JSON may be incomplete.
func EncodeJSON(v Value, o JSONOptions) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(o.EscapeHTML)
	w := jsonWriter{JSONOptions: o, b: &b, enc: enc}
	if err := w.write(v, 0); err != nil {
		return "", err
	}
	return b.String(), nil
}

type jsonWriter struct {
	JSONOptions
	b   *bytes.Buffer
	enc *json.Encoder
}

func (w *jsonWriter) write(v Value, depth int) error {
	if w.full() {
		return nil
	}
	if w.Check != nil {
		if err := w.Check(v); err != nil {
			return err
		}
	}
	switch v.K {
	case VList:
		w.b.WriteByte('[')
		for i, it := range v.L {
			if i > 0 {
				w.b.WriteByte(',')
			}
			w.newline(depth + 1)
			if err := w.write(it, depth+1); err != nil || w.full() {
				return err
			}
		}
		if len(v.L) > 0 {
			w.newline(depth)
		}
		w.b.WriteByte(']')
		return nil
	case VDict:
		w.b.WriteByte('{')
		for i, k := range slices.Sorted(maps.Keys(v.M)) {
			if i > 0 {
				w.b.WriteByte(',')
			}
			w.newline(depth + 1)
			if err := w.encode(k); err != nil {
				return err
			}
			w.b.WriteByte(':')
			if w.Indent != "" {
				w.b.WriteByte(' ')
			}
			if err := w.write(v.M[k], depth+1); err != nil || w.full() {
				return err
			}
		}
		if len(v.M) > 0 {
			w.newline(depth)
		}
		w.b.WriteByte('}')
		return nil
	case VObj:
		if r, ok := v.O.(*resultObj); ok {
			in := *w
			in.Check = nil
			return in.write(r.dict(), depth)
		}
	}
	return w.encode(ToIface(v))
}

func (w *jsonWriter) newline(depth int) {
	if w.Indent == "" {
		return
	}
	w.b.WriteByte('\n')
	for range depth {
		w.b.WriteString(w.Indent)
	}
}

func (w *jsonWriter) full() bool {
	return w.Max > 0 && w.b.Len() > w.Max
}

func (w *jsonWriter) encode(x any) error {
	if err := w.enc.Encode(x); err != nil {
		return ErrNotJSON
	}
	// Encode ends each value with a newline.
	w.b.Truncate(w.b.Len() - 1)
	return nil
}
