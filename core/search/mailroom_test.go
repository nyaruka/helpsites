package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChunkBody(t *testing.T) {
	tcs := []struct{ name, text, expected string }{
		{"Flows", "Flows\n\nA flow is a conversation.", "A flow is a conversation."},
		{"Flows", "Flows\nA flow is a conversation.", "A flow is a conversation."},    // a single newline is accepted too
		{"Flows", "Flows \n\nA flow is a conversation.", "A flow is a conversation."}, // as is trailing whitespace
		{"Flows", "Flows", ""}, // a chunk that's only the title
		{"Flow", "Flows are conversations.", "Flows are conversations."},    // the name starting a longer word isn't the title
		{"Flows", "A flow is a conversation.", "A flow is a conversation."}, // no prefix at all
		{"", "Flows\n\nA flow is a conversation.", "Flows\n\nA flow is a conversation."},
	}
	for _, tc := range tcs {
		assert.Equal(t, tc.expected, chunkBody(&Hit{ItemName: tc.name, Text: tc.text}), "%q / %q", tc.name, tc.text)
	}
}
