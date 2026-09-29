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

	// without an index, nothing
	results, err = search.Search(ctx, rt, site, "flows", 10)
	require.NoError(t, err)
	assert.Len(t, results, 0)

	// with an index and mailroom, semantic results
	flowsUUID := testdb.ArticleUUID(t, rt, flows)
	welcomeUUID := testdb.ArticleUUID(t, rt, welcome)
	var requests []map[string]any
	mailroom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/mi/knowledge/search", r.URL.Path)
		assert.Equal(t, "Token mrtoken", r.Header.Get("Authorization"))
		var req map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		requests = append(requests, req)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{
			{"item_key": flowsUUID, "text": "Flows\n\nA **flow** is a [conversation](/flows/design/) you _design_.", "score": 0.8},
			{"item_key": flowsUUID, "text": "Flows\n\nduplicate", "score": 0.7},
			{"item_key": welcomeUUID, "text": "Welcome\n\nGetting Started\n\n- Welcome to the platform, where flows are built.", "score": 0.6},
		}})
	}))
	defer mailroom.Close()

	rt.Config.MailroomURL = mailroom.URL
	rt.Config.MailroomAuthToken = "mrtoken"
	_, err = rt.DB.ExecContext(ctx, `UPDATE knowledge_knowledgesource SET last_indexed_on = NOW() WHERE id = $1`, source)
	require.NoError(t, err)

	site, err = models.LoadSiteByUUID(ctx, rt.DB, site.UUID)
	require.NoError(t, err)

	results, err = search.Search(ctx, rt, site, "how do I design a conversation", 10)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, flows, results[0].Article.ID)
	assert.Equal(t, "A flow is a <mark>conversation</mark> you <mark>design</mark>.", string(results[0].Snippet))
	assert.Equal(t, welcome, results[1].Article.ID)
	assert.Equal(t, "Getting Started Welcome to the platform, where flows are built.", string(results[1].Snippet))
	require.Len(t, requests, 1)
	assert.Equal(t, float64(testdb.Org1), requests[0]["org_id"])
	assert.Equal(t, []any{site.Source.UUID}, requests[0]["source_uuids"])
	assert.Equal(t, float64(30), requests[0]["limit"])

	// the limit applies to articles rather than hits
	results, err = search.Search(ctx, rt, site, "flows", 1)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, flows, results[0].Article.ID)

	// the same search again is answered from the cache, less anything unpublished since
	_, err = rt.DB.ExecContext(ctx, `UPDATE knowledge_article SET status = 'D' WHERE id = $1`, flows)
	require.NoError(t, err)

	results, err = search.Search(ctx, rt, site, "How do I design a conversation", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, welcome, results[0].Article.ID)
	assert.Len(t, requests, 2)

	// mailroom failing gives no results, which aren't cached
	failing := true
	flaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		mailroom.Config.Handler.ServeHTTP(w, r)
	}))
	defer flaky.Close()
	rt.Config.MailroomURL = flaky.URL

	results, err = search.Search(ctx, rt, site, "welcome", 10)
	require.NoError(t, err)
	assert.Len(t, results, 0)

	failing = false
	results, err = search.Search(ctx, rt, site, "welcome", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, welcome, results[0].Article.ID)
}
