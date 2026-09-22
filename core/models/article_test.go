package models_test

import (
	"testing"
	"time"

	"github.com/nyaruka/helpsites/v26/core/models"
	"github.com/nyaruka/helpsites/v26/testsuite"
	"github.com/nyaruka/helpsites/v26/testsuite/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArticles(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)

	source := testdb.HelpdeskSource(t, rt, testdb.Org1)

	started := testdb.InsertSection(t, rt, source, "Getting Started", "getting-started", testdb.ArticleOptions{Description: "The basics", SortOrder: 2})
	billing := testdb.InsertSection(t, rt, source, "Billing", "billing", testdb.ArticleOptions{SortOrder: 1})
	empty := testdb.InsertSection(t, rt, source, "Empty", "empty", testdb.ArticleOptions{})
	draftSection := testdb.InsertSection(t, rt, source, "Drafts", "drafts", testdb.ArticleOptions{Draft: true})

	welcome := testdb.InsertArticle(t, rt, source, started, "Welcome", "welcome", testdb.ArticleOptions{
		BodyHTML:  "<h2 id=\"hello\">Hello</h2><p>Welcome to <strong>the</strong> platform &amp; more.</p>",
		Headings:  []models.Heading{{ID: "hello", Text: "Hello"}},
		SortOrder: 1,
	})
	setup := testdb.InsertArticle(t, rt, source, started, "Setup", "setup", testdb.ArticleOptions{BodyHTML: "<p>Set things up.</p>", SortOrder: 2})
	testdb.InsertArticle(t, rt, source, started, "Draft", "draft", testdb.ArticleOptions{Draft: true})
	testdb.InsertArticle(t, rt, source, started, "Gone", "gone", testdb.ArticleOptions{Inactive: true})
	invoices := testdb.InsertArticle(t, rt, source, billing, "Invoices", "invoices", testdb.ArticleOptions{BodyHTML: "<p>About invoices.</p>"})
	testdb.InsertArticle(t, rt, source, draftSection, "Hidden", "hidden", testdb.ArticleOptions{BodyHTML: "<p>In a draft section.</p>"})

	// another workspace's articles are never ours
	other := testdb.HelpdeskSource(t, rt, testdb.Org2)
	otherSection := testdb.InsertSection(t, rt, other, "Getting Started", "getting-started", testdb.ArticleOptions{})
	testdb.InsertArticle(t, rt, other, otherSection, "Welcome", "welcome", testdb.ArticleOptions{})

	// sections in display order, only those with published articles
	sections, err := models.LoadSections(ctx, rt.DB, source)
	require.NoError(t, err)
	require.Len(t, sections, 2)
	assert.Equal(t, billing, sections[0].ID)
	assert.Equal(t, 1, sections[0].NumArticles)
	assert.Equal(t, started, sections[1].ID)
	assert.Equal(t, 2, sections[1].NumArticles)
	assert.Equal(t, "The basics", sections[1].Description)

	section, err := models.LoadSection(ctx, rt.DB, source, "getting-started")
	require.NoError(t, err)
	assert.Equal(t, started, section.ID)

	section, err = models.LoadSection(ctx, rt.DB, source, "empty")
	require.NoError(t, err)
	assert.Equal(t, empty, section.ID) // exists, though it lists nothing

	section, err = models.LoadSection(ctx, rt.DB, source, "drafts")
	require.NoError(t, err)
	assert.Nil(t, section)

	section, err = models.LoadSection(ctx, rt.DB, source, "welcome") // an article, not a section
	require.NoError(t, err)
	assert.Nil(t, section)

	articles, err := models.LoadArticles(ctx, rt.DB, source, started)
	require.NoError(t, err)
	require.Len(t, articles, 2)
	assert.Equal(t, welcome, articles[0].ID)
	assert.Equal(t, "Welcome", articles[0].Title)
	assert.Equal(t, "getting-started", articles[0].Parent.Slug)
	assert.Equal(t, setup, articles[1].ID)

	article, err := models.LoadArticle(ctx, rt.DB, source, started, "welcome")
	require.NoError(t, err)
	assert.Equal(t, welcome, article.ID)
	assert.Equal(t, "<h2 id=\"hello\">Hello</h2><p>Welcome to <strong>the</strong> platform &amp; more.</p>", article.BodyHTML)
	assert.Equal(t, []models.Heading{{ID: "hello", Text: "Hello"}}, article.Headings)
	assert.Equal(t, "Hello Welcome to the platform & more.", article.PlainText())
	assert.Equal(t, "Hello Welcome to the platform & more.", article.Excerpt())

	article, err = models.LoadArticle(ctx, rt.DB, source, started, "draft")
	require.NoError(t, err)
	assert.Nil(t, article)

	article, err = models.LoadArticle(ctx, rt.DB, source, started, "invoices") // wrong section
	require.NoError(t, err)
	assert.Nil(t, article)

	// link targets: every readable article and its section
	welcomeUUID := testdb.ArticleUUID(t, rt, welcome)
	startedUUID := testdb.ArticleUUID(t, rt, started)
	emptyUUID := testdb.ArticleUUID(t, rt, empty)

	targets, err := models.LoadLinkTargets(ctx, rt.DB, source, "")
	require.NoError(t, err)
	assert.Equal(t, "/getting-started/welcome/", targets[welcomeUUID])
	assert.Equal(t, "/getting-started/", targets[startedUUID])
	assert.Equal(t, "", targets[emptyUUID])
	assert.Len(t, targets, 5)

	targets, err = models.LoadLinkTargets(ctx, rt.DB, source, "/helpsite/preview")
	require.NoError(t, err)
	assert.Equal(t, "/helpsite/preview/getting-started/welcome/", targets[welcomeUUID])

	// readable lookups
	byUUID, err := models.LoadReadableByUUID(ctx, rt.DB, source, []string{welcomeUUID, startedUUID})
	require.NoError(t, err)
	assert.Len(t, byUUID, 1)
	assert.Equal(t, welcome, byUUID[welcomeUUID].ID)

	byID, err := models.LoadReadableByID(ctx, rt.DB, source, []models.ArticleID{welcome, invoices, started})
	require.NoError(t, err)
	assert.Len(t, byID, 2)

	// popularity
	today := time.Now().UTC()
	testdb.InsertViews(t, rt, welcome, today, 2)
	testdb.InsertViews(t, rt, setup, today.AddDate(0, 0, -3), 5)
	testdb.InsertViews(t, rt, invoices, today.AddDate(0, 0, -60), 100) // too long ago
	require.NoError(t, models.RecordArticleView(ctx, rt.DB, welcome))

	popular, err := models.LoadPopular(ctx, rt.DB, source, today.AddDate(0, 0, -30), 6)
	require.NoError(t, err)
	require.Len(t, popular, 2)
	assert.Equal(t, setup, popular[0].ID)
	assert.Equal(t, welcome, popular[1].ID)

	popular, err = models.LoadPopular(ctx, rt.DB, source, today.AddDate(0, 0, -30), 1)
	require.NoError(t, err)
	require.Len(t, popular, 1)

	// text search
	matches, err := models.SearchArticles(ctx, rt.DB, source, "welcome", nil, 10)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, welcome, matches[0].ID)

	matches, err = models.SearchArticles(ctx, rt.DB, source, "welcome", []models.ArticleID{welcome}, 10)
	require.NoError(t, err)
	assert.Len(t, matches, 0)

	matches, err = models.SearchArticles(ctx, rt.DB, source, "invoices", nil, 10)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, invoices, matches[0].ID)

	matches, err = models.SearchArticles(ctx, rt.DB, source, "hidden", nil, 10) // in a draft section
	require.NoError(t, err)
	assert.Len(t, matches, 0)
}

func TestPlainTextAndExcerpt(t *testing.T) {
	assert.Equal(t, "", models.PlainText(""))
	assert.Equal(t, "Hello world", models.PlainText("<p>Hello</p><p>world</p>"))
	assert.Equal(t, "a & b < c", models.PlainText("<p>a &amp; b &lt; c</p>"))
	assert.Equal(t, "one two", models.PlainText("<ul><li>one</li>\n<li>two</li></ul>"))

	assert.Equal(t, "Short", models.Excerpt("Short", 160))
	assert.Equal(t, "aaaa bbbb…", models.Excerpt("aaaa bbbb cccc", 10))
	assert.Equal(t, "aaaaaaaaaa…", models.Excerpt("aaaaaaaaaaaaaaaa bbbb", 10)) // no space past halfway, so a hard cut
	assert.Equal(t, "héllo wörld…", models.Excerpt("héllo wörld again", 12))
}
