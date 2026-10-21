package ui

import "testing"

func TestLastMatchIndex(t *testing.T) {
	pair := []searchMatch{{start: 2, end: 5}, {start: 10, end: 13}}
	cases := []struct {
		name    string
		matches []searchMatch
		offset  int
		index   int
		wrapped bool
	}{
		{name: "Nil", matches: nil, offset: 0, index: -1, wrapped: false},
		{name: "Empty", matches: []searchMatch{}, offset: 7, index: -1, wrapped: false},
		{name: "BeforeFirst", matches: pair, offset: 0, index: 1, wrapped: true},
		{name: "AtFirstStart", matches: pair, offset: 2, index: 1, wrapped: true},
		{name: "InsideFirst", matches: pair, offset: 3, index: 0, wrapped: false},
		{name: "BetweenMatches", matches: pair, offset: 7, index: 0, wrapped: false},
		{name: "AtLastStart", matches: pair, offset: 10, index: 0, wrapped: false},
		{name: "InsideLast", matches: pair, offset: 11, index: 1, wrapped: false},
		{name: "AfterLast", matches: pair, offset: 20, index: 1, wrapped: false},
		{name: "SingleWrap", matches: pair[:1], offset: 0, index: 0, wrapped: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			index, wrapped := lastMatchIndex(tc.matches, tc.offset)
			if index != tc.index || wrapped != tc.wrapped {
				t.Fatalf(
					"lastMatchIndex(%v, %d) = %d, %t; want %d, %t",
					tc.matches,
					tc.offset,
					index,
					wrapped,
					tc.index,
					tc.wrapped,
				)
			}
		})
	}
}
