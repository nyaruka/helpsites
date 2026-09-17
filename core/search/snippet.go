package search

import (
	"html"
	"html/template"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// MakeSnippet returns a window of the given plain text around the first of the given terms it contains - or its
// start, when it contains none - with the terms marked up. Returned as HTML that's safe to render: the text is escaped
// and only our own <mark>s are markup.
func MakeSnippet(text string, terms []string, length int) template.HTML {
	runes := []rune(text)
	lowered := make([]rune, len(runes))
	for i, r := range runes {
		lowered[i] = unicode.ToLower(r)
	}

	lowerTerms := make([]string, 0, len(terms))
	for _, t := range terms {
		if t != "" {
			lowerTerms = append(lowerTerms, strings.ToLower(t))
		}
	}

	first := -1
	for _, t := range lowerTerms {
		if hit := indexRunes(lowered, []rune(t)); hit >= 0 && (first < 0 || hit < first) {
			first = hit
		}
	}

	start := 0
	if first >= 0 {
		// lead in with a little context, and start on a word boundary
		start = max(0, first-length/4)
		if start > 0 {
			if space := lastIndexRune(runes[:start], ' '); space >= 0 {
				start = space + 1
			}
		}
	}

	end := min(len(runes), start+length)
	window := runes[start:end]
	if end < len(runes) {
		if space := lastIndexRune(window, ' '); space > length/2 {
			window = window[:space]
		}
		window = append(append([]rune{}, window...), '…')
	}
	if start > 0 {
		window = append([]rune{'…'}, window...)
	}

	escaped := html.EscapeString(string(window))
	if len(lowerTerms) > 0 {
		// longest first so that a term that contains another is marked whole
		sort.Slice(lowerTerms, func(i, j int) bool { return len(lowerTerms[i]) > len(lowerTerms[j]) })
		quoted := make([]string, len(lowerTerms))
		for i, t := range lowerTerms {
			quoted[i] = regexp.QuoteMeta(t)
		}
		pattern := regexp.MustCompile("(?i)" + strings.Join(quoted, "|"))
		escaped = pattern.ReplaceAllString(escaped, "<mark>$0</mark>")
	}
	return template.HTML(escaped)
}

func indexRunes(hay, needle []rune) int {
	if len(needle) == 0 || len(needle) > len(hay) {
		return -1
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		match := true
		for j := range needle {
			if hay[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func lastIndexRune(rs []rune, r rune) int {
	for i := len(rs) - 1; i >= 0; i-- {
		if rs[i] == r {
			return i
		}
	}
	return -1
}
