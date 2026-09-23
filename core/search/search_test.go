package search_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/helpsites/v26/core/models"
	"github.com/nyaruka/helpsites/v26/core/search"
	"github.com/nyaruka/helpsites/v26/testsuite"
	"github.com/nyaruka/helpsites/v26/testsuite/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearch(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)

	site := testdb.InsertSite(t, rt, testdb.Org1, "Help", testdb.SiteOptions{Domain: "help.nyaruka.com", Verified: true, Enabled: true})
	source := site.Source.ID

	started := testdb.InsertSection(t, rt, source, "Getting Started", "getting-started", testdb.ArticleOptions{})
	welcome := testdb.InsertArticle(t, rt, source, started, "Welcome", "welcome", testdb.ArticleOptions{BodyHTML: "<p>Welcome to the platform, where flows are built.</p>"})
	flows := testdb.InsertArticle(t, rt, source, started, "Flows", "flows", testdb.ArticleOptions{BodyHTML: "<p>A flow is a conversation you design.</p>"})
	testdb.InsertArticle(t, rt, source, started, "Secret", "secret", testdb.ArticleOptions{BodyHTML: "<p>Flows nobody should find.</p>", Draft: true})

	// nothing for an empty query
	results, err := search.Search(ctx, rt, site, "  ", 10)
	require.NoError(t, err)
	assert.Nil(t, results)

	// without an index, text search alone
	results, err = search.Search(ctx, rt, site, "flows", 10)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, flows, results[0].Article.ID) // title match ranks first
	assert.Equal(t, welcome, results[1].Article.ID)
	assert.Contains(t, string(results[1].Snippet), "<mark>flows</mark>")

	// the same search again is answered from the cache, less anything unpublished since
	_, err = rt.DB.ExecContext(ctx, `UPDATE knowledge_article SET status = 'D' WHERE id = $1`, welcome)
	require.NoError(t, err)

	results, err = search.Search(ctx, rt, site, "Flows", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, flows, results[0].Article.ID)

	// with an index and mailroom, semantic results lead and text search fills in behind them
	flowsUUID := testdb.ArticleUUID(t, rt, flows)
	var requests []map[string]any
	mailroom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/mi/knowledge/search", r.URL.Path)
		assert.Equal(t, "Token mrtoken", r.Header.Get("Authorization"))
		var req map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		requests = append(requests, req)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{
			{"knowledge_uuid": "other-source", "item_key": "x", "text": "not ours", "score": 0.9},
			{"knowledge_uuid": site.Source.UUID, "item_key": flowsUUID, "text": "A flow is a conversation you design.", "score": 0.8},
			{"knowledge_uuid": site.Source.UUID, "item_key": flowsUUID, "text": "duplicate", "score": 0.7},
		}})
	}))
	defer mailroom.Close()

	rt.Config.MailroomURL = mailroom.URL
	rt.Config.MailroomAuthToken = "mrtoken"
	_, err = rt.DB.ExecContext(ctx, `UPDATE knowledge_knowledgesource SET last_indexed_on = NOW() WHERE id = $1`, source)
	require.NoError(t, err)
	_, err = rt.DB.ExecContext(ctx, `UPDATE knowledge_article SET status = 'P' WHERE id = $1`, welcome)
	require.NoError(t, err)

	site, err = models.LoadSiteByUUID(ctx, rt.DB, site.UUID)
	require.NoError(t, err)

	results, err = search.Search(ctx, rt, site, "how do I design a conversation", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, flows, results[0].Article.ID)
	assert.Equal(t, "A flow is a <mark>conversation</mark> you <mark>design</mark>.", string(results[0].Snippet))
	require.Len(t, requests, 1)
	assert.Equal(t, float64(testdb.Org1), requests[0]["org_id"])
	assert.Equal(t, []any{site.Source.UUID}, requests[0]["source_uuids"])
	assert.Equal(t, float64(30), requests[0]["limit"])

	// mailroom being down still gives text results
	mailroom.Close()
	results, err = search.Search(ctx, rt, site, "welcome", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, welcome, results[0].Article.ID)
}
