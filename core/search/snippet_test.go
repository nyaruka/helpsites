package search_test

import (
	"html/template"
	"testing"

	"github.com/nyaruka/helpsites/core/search"
	"github.com/stretchr/testify/assert"
)

func TestMakeSnippet(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog. It was a sunny day and the fox was happy to be out and about in the meadow with all the other animals."

	// no terms: the start of the text
	assert.Equal(t, template.HTML("The quick brown fox jumps over the lazy dog. It…"), search.MakeSnippet(text, nil, 48))

	// a term near the start: lead in with a little context, starting on a word boundary
	assert.Equal(t, template.HTML("…quick brown <mark>fox</mark> jumps over the lazy dog. It was…"), search.MakeSnippet(text, []string{"fox"}, 48))

	// a term further in, with ellipses both ends
	assert.Equal(t, template.HTML("…the fox was <mark>happy</mark> to be out and about in the…"), search.MakeSnippet(text, []string{"happy"}, 48))

	// terms are marked case-insensitively, longest first, and text is escaped
	assert.Equal(t, template.HTML("<mark>Fish</mark> &amp; <mark>chips</mark>"), search.MakeSnippet("Fish & chips", []string{"chips", "fish"}, 100))
	assert.Equal(t, template.HTML("<mark>foxes</mark> and a <mark>fox</mark>"), search.MakeSnippet("foxes and a fox", []string{"fox", "foxes"}, 100))

	// short text is returned whole
	assert.Equal(t, template.HTML("Hello"), search.MakeSnippet("Hello", []string{"nope"}, 100))

	// multibyte text is cut on characters, not bytes
	assert.Equal(t, template.HTML("…lo <mark>wörld</mark> ünd…"), search.MakeSnippet("héllo wörld ünd mehr", []string{"wörld"}, 15))
}
