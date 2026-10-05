package httpx

import (
	"bufio"
	"cmp"
	"io"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func TestSSEFieldValuesKeepWhitespace(t *testing.T) {
	values := []struct {
		name string
		raw  string
		want string
	}{
		{name: "no separator space", raw: "value", want: "value"},
		{name: "one separator space", raw: " value", want: "value"},
		{name: "two spaces", raw: "  value", want: " value"},
		{name: "tab", raw: "\tvalue", want: "\tvalue"},
		{name: "space then tab", raw: " \tvalue", want: "\tvalue"},
		{name: "tab then space", raw: "\t value", want: "\t value"},
		{name: "two tabs", raw: "\t\tvalue", want: "\t\tvalue"},
		{name: "mixed indentation", raw: " \t value", want: "\t value"},
		{name: "spaces only", raw: "   ", want: "  "},
		{name: "tab only", raw: "\t", want: "\t"},
		{name: "trailing whitespace", raw: "  value \t", want: " value \t"},
		{name: "empty", raw: "", want: ""},
	}

	for _, field := range []string{"data", "event", "id"} {
		t.Run(field, func(t *testing.T) {
			for _, tt := range values {
				t.Run(tt.name, func(t *testing.T) {
					var b sseEventBuilder
					if field != "data" {
						b.consume("data: payload")
						b.consume(field + ": old")
					}
					b.consume(field + ":" + tt.raw)
					evt, ok := b.finalize(0)
					if !ok {
						t.Fatal("field value produced no event")
					}
					var got string
					switch field {
					case "data":
						got = evt.Data
					case "event":
						got = evt.Event
					case "id":
						got = evt.ID
					}
					if got != tt.want {
						t.Fatalf("%s value = %q, want %q", field, got, tt.want)
					}
				})
			}
		})
	}
}

func TestSSEBuilderFollowsTheFieldGrammar(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  SSEEvent
		ok    bool
	}{
		{name: "bare data", lines: []string{"data"}, ok: true},
		{name: "bare data twice", lines: []string{"data", "data"}, want: SSEEvent{Data: "\n"}, ok: true},
		{name: "colon in value", lines: []string{"data:x:y"}, want: SSEEvent{Data: "x:y"}, ok: true},
		{
			name:  "bare event clears",
			lines: []string{"data: x", "event: a", "event"},
			want:  SSEEvent{Data: "x"},
			ok:    true,
		},
		{name: "bare id clears", lines: []string{"data: x", "id: 1", "id"}, want: SSEEvent{Data: "x"}, ok: true},
		{name: "id with NUL", lines: []string{"id: 1", "id: a\x00b"}, want: SSEEvent{ID: "1"}, ok: true},
		{name: "retry", lines: []string{"retry: 5"}, want: SSEEvent{Retry: 5}, ok: true},
		{name: "retry without space", lines: []string{"retry:5"}, want: SSEEvent{Retry: 5}, ok: true},
		{
			name:  "invalid retry keeps the valid one",
			lines: []string{"retry: 5", "retry: x"},
			want:  SSEEvent{Retry: 5},
			ok:    true,
		},
		{name: "retry two spaces", lines: []string{"retry:  5"}},
		{name: "retry plus sign", lines: []string{"retry: +5"}},
		{name: "retry minus sign", lines: []string{"retry: -5"}},
		{name: "retry with unit", lines: []string{"retry: 5ms"}},
		{name: "empty retry", lines: []string{"retry:"}},
		{name: "bare retry", lines: []string{"retry"}},
		{name: "retry overflow", lines: []string{"retry: 99999999999"}},
		{name: "empty comment", lines: []string{":"}, ok: true},
		{name: "comment", lines: []string{": ping"}, want: SSEEvent{Comment: "ping"}, ok: true},
		{name: "comment two spaces", lines: []string{":  ping"}, want: SSEEvent{Comment: " ping"}, ok: true},
		{name: "comment tab", lines: []string{":\tping"}, want: SSEEvent{Comment: "\tping"}, ok: true},
		{name: "capitalized name", lines: []string{"Data: x"}},
		{name: "space before name", lines: []string{" data: x"}},
		{name: "space before colon", lines: []string{"data : x"}},
		{name: "unknown name", lines: []string{"foo: x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b sseEventBuilder
			for _, line := range tt.lines {
				b.consume(line)
			}
			got, ok := b.finalize(0)
			got.Timestamp = time.Time{}
			if ok != tt.ok || got != tt.want {
				t.Fatalf("event = %+v, %v, want %+v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

type sseStep struct {
	line string
	n    int
	err  error
}

func readSSESteps(r io.Reader, limit int) []sseStep {
	sr := sseReader{br: bufio.NewReader(r)}
	var steps []sseStep
	for {
		line, n, err := sr.readLine(limit)
		steps = append(steps, sseStep{line, n, err})
		if err != nil {
			return steps
		}
	}
}

var sseChunkings = []struct {
	name string
	wrap func(io.Reader) io.Reader
}{
	{"one byte", iotest.OneByteReader},
	{"half", iotest.HalfReader},
	{"data with EOF", iotest.DataErrReader},
}

func TestSSEReaderSplitsLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		limit int
		want  []sseStep
	}{
		{name: "empty", input: "", want: []sseStep{{"", 0, io.EOF}}},
		{name: "LF", input: "a\nb\n", want: []sseStep{{"a", 2, nil}, {"b", 2, nil}, {"", 0, io.EOF}}},
		{name: "CRLF", input: "a\r\nb\r\n", want: []sseStep{{"a", 2, nil}, {"b", 3, nil}, {"", 1, io.EOF}}},
		{name: "CR", input: "a\rb\r", want: []sseStep{{"a", 2, nil}, {"b", 2, nil}, {"", 0, io.EOF}}},
		{name: "CR then CRLF", input: "a\r\r\nb", want: []sseStep{{"a", 2, nil}, {"", 1, nil}, {"b", 2, io.EOF}}},
		{
			name:  "blank lines",
			input: "\n\r\n\r",
			want:  []sseStep{{"", 1, nil}, {"", 1, nil}, {"", 2, nil}, {"", 0, io.EOF}},
		},
		{name: "unterminated", input: "a", want: []sseStep{{"a", 1, io.EOF}}},
		{name: "BOM", input: "\ufeffa\n", want: []sseStep{{"a", 5, nil}, {"", 0, io.EOF}}},
		{name: "second BOM", input: "\ufeff\ufeffa\n", want: []sseStep{{"\ufeffa", 8, nil}, {"", 0, io.EOF}}},
		{
			name:  "BOM after the first line",
			input: "a\n\ufeffb\n",
			want:  []sseStep{{"a", 2, nil}, {"\ufeffb", 5, nil}, {"", 0, io.EOF}},
		},
		{name: "line at the limit", input: "abc\n", limit: 4, want: []sseStep{{"abc", 4, nil}, {"", 0, io.EOF}}},
		{name: "end over the limit", input: "abc\n", limit: 3, want: []sseStep{{"abc", 3, errSSELineTooLong}}},
		{name: "CRLF at the limit", input: "ab\r\nc", limit: 4, want: []sseStep{{"ab", 3, nil}, {"c", 2, io.EOF}}},
		{
			name:  "LF over the limit",
			input: "abc\r\nd",
			limit: 4,
			want:  []sseStep{{"abc", 4, nil}, {"", 0, errSSELineTooLong}},
		},
		{
			name:  "line after a CRLF at the limit",
			input: "ab\r\ncde\n",
			limit: 4,
			want:  []sseStep{{"ab", 3, nil}, {"cde", 5, nil}, {"", 0, io.EOF}},
		},
		{name: "CR at the limit", input: "abc\rd", limit: 4, want: []sseStep{{"abc", 4, nil}, {"d", 1, io.EOF}}},
		{name: "line over the limit", input: "abcdef", limit: 3, want: []sseStep{{"abc", 3, errSSELineTooLong}}},
	}
	for _, tt := range tests {
		limit := cmp.Or(tt.limit, 1<<10)
		t.Run(tt.name, func(t *testing.T) {
			if got := readSSESteps(strings.NewReader(tt.input), limit); !slices.Equal(got, tt.want) {
				t.Fatalf("steps = %+v, want %+v", got, tt.want)
			}
			for _, c := range sseChunkings {
				if got := readSSESteps(c.wrap(strings.NewReader(tt.input)), limit); !slices.Equal(got, tt.want) {
					t.Fatalf("%s steps = %+v, want %+v", c.name, got, tt.want)
				}
			}
		})
	}
}

func FuzzSSEReaderMatchesSplit(f *testing.F) {
	for _, s := range []string{"", "a\nb\n", "a\r\nb\r\n", "a\rb\r", "a\r\r\nb", "\n\r\n\r", "\ufeff\ufeffa\n"} {
		f.Add(s, 3)
	}
	ends := regexp.MustCompile("\r\n|\r|\n")
	f.Fuzz(func(t *testing.T, input string, limit int) {
		var (
			lines []string
			total int
		)
		for _, s := range readSSESteps(strings.NewReader(input), len(input)+1) {
			lines = append(lines, s.line)
			total += s.n
		}
		if want := ends.Split(strings.TrimPrefix(input, "\ufeff"), -1); !slices.Equal(lines, want) {
			t.Fatalf("lines = %q, want %q", lines, want)
		}
		if total != len(input) {
			t.Fatalf("read %d bytes, want %d", total, len(input))
		}

		limit = int(uint(limit)%16) + 1
		whole := readSSESteps(strings.NewReader(input), limit)
		for _, c := range sseChunkings {
			if got := readSSESteps(c.wrap(strings.NewReader(input)), limit); !slices.Equal(got, whole) {
				t.Fatalf("%s steps = %+v, want %+v", c.name, got, whole)
			}
		}

		// Each line may use limit bytes, its own end included. A CRLF line
		// whose LF alone does not fit is reported on the next step.
		start, stop := 0, -1
		for i, m := range append(ends.FindAllStringIndex(input, -1), []int{len(input), len(input)}) {
			if m[1]-start > limit {
				stop = i
				if m[1]-m[0] == 2 && m[1]-start == limit+1 {
					stop++
				}
				break
			}
			start = m[1]
		}
		last := whole[len(whole)-1]
		if stop < 0 && last.err != io.EOF || stop >= 0 && (len(whole) != stop+1 || last.err != errSSELineTooLong) {
			t.Fatalf("limit %d steps = %+v, want the limit to end step %d", limit, whole, stop)
		}
		for i, s := range whole[:len(whole)-1] {
			if s.line != lines[i] {
				t.Fatalf("limit %d step %d = %q, want %q", limit, i, s.line, lines[i])
			}
		}
	})
}

func BenchmarkSSEReader(b *testing.B) {
	for _, bc := range []struct{ name, body string }{
		{"one 3MiB line", strings.Repeat("x", 3<<20) + "\n"},
		{"CR only", strings.Repeat("\r", 1<<20)},
		{"short CR lines", strings.Repeat("data: x\r", 1<<16)},
		{"short LF lines", strings.Repeat("data: x\n", 1<<16)},
	} {
		b.Run(bc.name, func(b *testing.B) {
			b.SetBytes(int64(len(bc.body)))
			for b.Loop() {
				r := sseReader{br: bufio.NewReader(strings.NewReader(bc.body))}
				for {
					if _, _, err := r.readLine(DefaultSSEMaxLineBytes); err != nil {
						break
					}
				}
			}
		})
	}
}
