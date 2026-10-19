package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
)

func TestClientGone(t *testing.T) {
	gone, cancel := context.WithCancel(t.Context())
	cancel()
	timedOut, cancel := context.WithTimeout(t.Context(), 0)
	defer cancel()

	request := func(ctx context.Context) *http.Request {
		return httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	}
	canceledQuery := fmt.Errorf("error loading site: %w", &pq.Error{Code: "57014"})

	assert.True(t, clientGone(request(gone), fmt.Errorf("error loading site: %w", context.Canceled)))
	assert.True(t, clientGone(request(gone), canceledQuery))

	// failing for some other reason after the client has gone is still ours
	assert.False(t, clientGone(request(gone), errors.New("boom")))
	assert.False(t, clientGone(request(gone), &pq.Error{Code: "23505"}))

	// as is timing out, or the database canceling a query by itself
	assert.False(t, clientGone(request(timedOut), canceledQuery))
	assert.False(t, clientGone(request(t.Context()), canceledQuery))
}
