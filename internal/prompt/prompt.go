package prompt

import "fmt"

type Edit struct {
	Start int
	End   int
	Text  string
}

func (e Edit) Apply(input string) (string, int, error) {
	runes := []rune(input)
	if e.Start < 0 || e.Start > e.End || e.End > len(runes) {
		return input, min(max(e.Start, 0), len(runes)), fmt.Errorf(
			"invalid completion span %d:%d for %d runes",
			e.Start,
			e.End,
			len(runes),
		)
	}

	text := []rune(e.Text)
	out := make([]rune, 0, len(runes)-(e.End-e.Start)+len(text))
	out = append(out, runes[:e.Start]...)
	out = append(out, text...)
	out = append(out, runes[e.End:]...)
	return string(out), e.Start + len(text), nil
}

type Item struct {
	Label    string
	Summary  string
	Edit     Edit
	Continue bool
}
