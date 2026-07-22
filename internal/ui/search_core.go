package ui

import (
	"fmt"
	"regexp"
	"unicode/utf8"
)

type searchMatch struct {
	start int
	end   int
}

func buildSearchMatches(content, query string, isRegex bool) ([]searchMatch, error) {
	if query == "" {
		return nil, nil
	}
	if isRegex {
		rx, err := regexp.Compile(query)
		if err != nil {
			return nil, err
		}
		return regexMatches(content, rx), nil
	}
	return literalMatches(content, query), nil
}

func firstMatchIndex(matches []searchMatch, offset int) (int, bool) {
	if len(matches) == 0 {
		return -1, false
	}
	for i, match := range matches {
		if offset < match.end {
			return i, false
		}
	}
	return 0, true
}

func lastMatchIndex(matches []searchMatch, offset int) (int, bool) {
	if len(matches) == 0 {
		return -1, false
	}
	for i := len(matches) - 1; i >= 0; i-- {
		match := matches[i]
		if offset > match.start {
			return i, false
		}
	}
	return len(matches) - 1, true
}

func literalMatches(content, pattern string) []searchMatch {
	patternRunes := []rune(pattern)
	contentRunes := []rune(content)
	plen := len(patternRunes)
	if plen == 0 || len(contentRunes) < plen {
		return nil
	}

	matches := make([]searchMatch, 0)
	for i := 0; i <= len(contentRunes)-plen; i++ {
		match := true
		for j := range plen {
			if contentRunes[i+j] != patternRunes[j] {
				match = false
				break
			}
		}
		if match {
			matches = append(matches, searchMatch{start: i, end: i + plen})
		}
	}
	return matches
}

func regexMatches(content string, rx *regexp.Regexp) []searchMatch {
	indices := rx.FindAllStringIndex(content, -1)
	if len(indices) == 0 {
		return nil
	}

	matches := make([]searchMatch, 0, len(indices))
	for _, idx := range indices {
		if len(idx) != 2 {
			continue
		}
		startByte, endByte := idx[0], idx[1]
		if endByte <= startByte {
			continue
		}
		start := utf8.RuneCountInString(content[:startByte])
		end := utf8.RuneCountInString(content[:endByte])
		matches = append(matches, searchMatch{start: start, end: end})
	}
	return matches
}

func searchStatusText(index, total int, query string, wrapped bool) string {
	text := fmt.Sprintf("Match %d/%d for %q", index+1, total, query)
	if wrapped {
		text += " (wrapped)"
	}
	return text
}
