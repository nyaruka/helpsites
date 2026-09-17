package models

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"
)

type ArticleID int

// article statuses
const (
	ArticleStatusDraft     = "D"
	ArticleStatusPublished = "P"
)

// Heading is a top level heading of an article, as its page lists them - the id its element carries, and its text.
type Heading struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// ArticleRef is what a page needs of an article's section - or of a search result's - to link to it
type ArticleRef struct {
	ID    ArticleID `json:"id"`
	UUID  string    `json:"uuid"`
	Slug  string    `json:"slug"`
	Title string    `json:"title"`
}

// Article is a section (an article with no parent) or an article in a section. Its body is markdown, rendered by
// the platform when it's published into the HTML served here, along with the headings its page lists.
type Article struct {
	ID          ArticleID   `json:"id"`
	UUID        string      `json:"uuid"`
	Parent      *ArticleRef `json:"parent"`
	SortOrder   int         `json:"sort_order"`
	Title       string      `json:"title"`
	Slug        string      `json:"slug"`
	Description string      `json:"description"`
	Language    string      `json:"language"`
	BodyHTML    string      `json:"body_html"`
	Headings    []Heading   `json:"headings"`
	ModifiedOn  time.Time   `json:"modified_on"`
	PublishedOn *time.Time  `json:"published_on"`

	NumArticles int `json:"num_articles"` // for a section, the number of published articles in it
}

// the columns a page needs of an article - including the body's HTML, which listings excerpt and search snippets
// fall back to
const sqlArticleColumns = `
a.id, a.uuid, a.sort_order, a.title, a.slug, a.description, a.language, a.body_html, a.headings, a.modified_on, a.published_on`

const sqlArticleParent = `
JSON_BUILD_OBJECT('id', p.id, 'uuid', p.uuid, 'slug', p.slug, 'title', p.title) AS parent`

// a readable article is one that's published in a section that's published
const sqlReadable = `
a.source_id = $1 AND a.is_active AND a.status = 'P' AND p.is_active AND p.status = 'P'`

const sqlSelectSections = `
SELECT ROW_TO_JSON(r) FROM (
    SELECT ` + sqlArticleColumns + `, (
        SELECT COUNT(*) FROM knowledge_article c WHERE c.parent_id = a.id AND c.is_active AND c.status = 'P'
    ) AS num_articles
      FROM knowledge_article a
     WHERE a.source_id = $1 AND a.parent_id IS NULL AND a.is_active AND a.status = 'P'
  ORDER BY a.sort_order, a.title
) r WHERE r.num_articles > 0;`

// LoadSections loads the published sections of the given helpdesk, in display order, each with the number of
// published articles under it. Sections with nothing published in them aren't listed - there'd be nothing to read
// there.
func LoadSections(ctx context.Context, db DBorTx, sourceID SourceID) ([]*Article, error) {
	sections, err := queryJSON(ctx, db, func() *Article { return &Article{} }, sqlSelectSections, sourceID)
	if err != nil {
		return nil, fmt.Errorf("error loading sections: %w", err)
	}
	return sections, nil
}

const sqlSelectSection = `
SELECT ROW_TO_JSON(r) FROM (
    SELECT ` + sqlArticleColumns + `
      FROM knowledge_article a
     WHERE a.source_id = $1 AND a.parent_id IS NULL AND a.slug = $2 AND a.is_active AND a.status = 'P'
) r;`

// LoadSection loads the published section with the given slug, or nil if there isn't one
func LoadSection(ctx context.Context, db DBorTx, sourceID SourceID, slug string) (*Article, error) {
	section, err := queryJSONOne(ctx, db, func() *Article { return &Article{} }, sqlSelectSection, sourceID, slug)
	if err != nil {
		return nil, fmt.Errorf("error loading section %s: %w", slug, err)
	}
	return section, nil
}

const sqlSelectArticles = `
SELECT ROW_TO_JSON(r) FROM (
    SELECT ` + sqlArticleColumns + `, ` + sqlArticleParent + `
      FROM knowledge_article a
      JOIN knowledge_article p ON p.id = a.parent_id
     WHERE ` + sqlReadable + ` AND a.parent_id = $2
  ORDER BY a.sort_order, a.title
) r;`

// LoadArticles loads the published articles in the given section, in display order
func LoadArticles(ctx context.Context, db DBorTx, sourceID SourceID, sectionID ArticleID) ([]*Article, error) {
	articles, err := queryJSON(ctx, db, func() *Article { return &Article{} }, sqlSelectArticles, sourceID, sectionID)
	if err != nil {
		return nil, fmt.Errorf("error loading articles: %w", err)
	}
	return articles, nil
}

const sqlSelectArticle = `
SELECT ROW_TO_JSON(r) FROM (
    SELECT ` + sqlArticleColumns + `, ` + sqlArticleParent + `
      FROM knowledge_article a
      JOIN knowledge_article p ON p.id = a.parent_id
     WHERE ` + sqlReadable + ` AND a.parent_id = $2 AND a.slug = $3
) r;`

// LoadArticle loads the published article with the given slug in the given section, with its body, or nil if there
// isn't one
func LoadArticle(ctx context.Context, db DBorTx, sourceID SourceID, sectionID ArticleID, slug string) (*Article, error) {
	article, err := queryJSONOne(ctx, db, func() *Article { return &Article{} }, sqlSelectArticle, sourceID, sectionID, slug)
	if err != nil {
		return nil, fmt.Errorf("error loading article %s: %w", slug, err)
	}
	return article, nil
}

const sqlSelectReadableByUUID = `
SELECT ROW_TO_JSON(r) FROM (
    SELECT ` + sqlArticleColumns + `, ` + sqlArticleParent + `
      FROM knowledge_article a
      JOIN knowledge_article p ON p.id = a.parent_id
     WHERE ` + sqlReadable + ` AND a.uuid::text = ANY($2)
) r;`

// LoadReadableByUUID loads the readable articles among the given UUIDs, keyed by UUID
func LoadReadableByUUID(ctx context.Context, db DBorTx, sourceID SourceID, uuids []string) (map[string]*Article, error) {
	articles, err := queryJSON(ctx, db, func() *Article { return &Article{} }, sqlSelectReadableByUUID, sourceID, pqStringArray(uuids))
	if err != nil {
		return nil, fmt.Errorf("error loading articles by UUID: %w", err)
	}
	byUUID := make(map[string]*Article, len(articles))
	for _, a := range articles {
		byUUID[a.UUID] = a
	}
	return byUUID, nil
}

const sqlSelectReadableByID = `
SELECT ROW_TO_JSON(r) FROM (
    SELECT ` + sqlArticleColumns + `, ` + sqlArticleParent + `
      FROM knowledge_article a
      JOIN knowledge_article p ON p.id = a.parent_id
     WHERE ` + sqlReadable + ` AND a.id = ANY($2)
) r;`

// LoadReadableByID loads the readable articles among the given IDs, keyed by ID
func LoadReadableByID(ctx context.Context, db DBorTx, sourceID SourceID, ids []ArticleID) (map[ArticleID]*Article, error) {
	articles, err := queryJSON(ctx, db, func() *Article { return &Article{} }, sqlSelectReadableByID, sourceID, pqIntArray(ids))
	if err != nil {
		return nil, fmt.Errorf("error loading articles by ID: %w", err)
	}
	byID := make(map[ArticleID]*Article, len(articles))
	for _, a := range articles {
		byID[a.ID] = a
	}
	return byID, nil
}

const sqlSelectLinkTargets = `
SELECT a.uuid, a.slug, p.uuid, p.slug
  FROM knowledge_article a
  JOIN knowledge_article p ON p.id = a.parent_id
 WHERE ` + sqlReadable

// LoadLinkTargets loads where every readable article and section on the site currently lives, by uuid, with the given
// prefix - what article: links in a body resolve against. Articles link by uuid rather than by address, so a
// retitled or refiled article keeps every link to it; this is the address as of now. Only a section with something
// published in it is a page.
func LoadLinkTargets(ctx context.Context, db DBorTx, sourceID SourceID, prefix string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, sqlSelectLinkTargets, sourceID)
	if err != nil {
		return nil, fmt.Errorf("error querying link targets: %w", err)
	}
	defer rows.Close()

	targets := make(map[string]string)
	for rows.Next() {
		var uuid, slug, parentUUID, parentSlug string
		if err := rows.Scan(&uuid, &slug, &parentUUID, &parentSlug); err != nil {
			return nil, err
		}
		targets[parentUUID] = fmt.Sprintf("%s/%s/", prefix, parentSlug)
		targets[uuid] = fmt.Sprintf("%s/%s/%s/", prefix, parentSlug, slug)
	}
	return targets, nil
}

const sqlSelectPopular = `
SELECT ROW_TO_JSON(r) FROM (
    SELECT ` + sqlArticleColumns + `, ` + sqlArticleParent + `
      FROM knowledge_article a
      JOIN knowledge_article p ON p.id = a.parent_id
      JOIN (
          SELECT c.article_id, SUM(c.count) AS total
            FROM knowledge_articlecount c
           WHERE c.scope = 'views' AND c.day >= $2
        GROUP BY c.article_id
      ) v ON v.article_id = a.id
     WHERE ` + sqlReadable + `
  ORDER BY v.total DESC, a.id
     LIMIT $3
) r;`

// LoadPopular loads the most viewed readable articles since the given day, most viewed first
func LoadPopular(ctx context.Context, db DBorTx, sourceID SourceID, since time.Time, limit int) ([]*Article, error) {
	articles, err := queryJSON(ctx, db, func() *Article { return &Article{} }, sqlSelectPopular, sourceID, since.Format("2006-01-02"), limit)
	if err != nil {
		return nil, fmt.Errorf("error loading popular articles: %w", err)
	}
	return articles, nil
}

const sqlSearchArticles = `
SELECT ROW_TO_JSON(r) FROM (
    SELECT ` + sqlArticleColumns + `, ` + sqlArticleParent + `, TS_RANK(v.vector, q.query) AS rank
      FROM knowledge_article a
      JOIN knowledge_article p ON p.id = a.parent_id,
   LATERAL (SELECT SETWEIGHT(TO_TSVECTOR('simple', a.title), 'A') || SETWEIGHT(TO_TSVECTOR('simple', a.body), 'B') AS vector) v,
   LATERAL (SELECT WEBSEARCH_TO_TSQUERY('simple', $2) AS query) q
     WHERE ` + sqlReadable + ` AND NOT (a.id = ANY($3)) AND v.vector @@ q.query
  ORDER BY rank DESC, a.title
     LIMIT $4
) r;`

// SearchArticles searches the readable articles' titles and bodies for the given query, best match first, leaving
// out the given articles. The simple text search configuration is used rather than a language's, since a helpdesk
// can hold articles in any language.
func SearchArticles(ctx context.Context, db DBorTx, sourceID SourceID, query string, exclude []ArticleID, limit int) ([]*Article, error) {
	articles, err := queryJSON(ctx, db, func() *Article { return &Article{} }, sqlSearchArticles, sourceID, query, pqIntArray(exclude), limit)
	if err != nil {
		return nil, fmt.Errorf("error searching articles: %w", err)
	}
	return articles, nil
}

// the scope of the daily count of an article's views
const ArticleCountScopeViews = "views"

const sqlInsertArticleView = `
INSERT INTO knowledge_articlecount(article_id, day, scope, count, is_squashed) VALUES($1, (NOW() AT TIME ZONE 'UTC')::date, 'views', 1, FALSE)`

// RecordArticleView records a view of the given article, as a delta the platform squashes periodically
func RecordArticleView(ctx context.Context, db DBorTx, articleID ArticleID) error {
	if _, err := db.ExecContext(ctx, sqlInsertArticleView, articleID); err != nil {
		return fmt.Errorf("error recording article view: %w", err)
	}
	return nil
}

var tagRegex = regexp.MustCompile(`<[^>]*>`)
var spaceRegex = regexp.MustCompile(`\s+`)

// PlainText returns the article's body as plain text - its HTML with the markup dropped and whitespace collapsed -
// for excerpts and search snippets, where the markup would only get in the way.
func (a *Article) PlainText() string {
	return PlainText(a.BodyHTML)
}

// PlainText returns the given HTML as plain text - adjacent elements are kept apart by a space, so that the end of a
// paragraph doesn't run into the start of the next
func PlainText(html_ string) string {
	text := tagRegex.ReplaceAllString(strings.ReplaceAll(html_, "><", "> <"), "")
	return strings.TrimSpace(spaceRegex.ReplaceAllString(html.UnescapeString(text), " "))
}

// Excerpt returns the article's opening, as plain text, for listing it by - a section describes itself, an article
// is read.
func (a *Article) Excerpt() string {
	return Excerpt(a.PlainText(), 160)
}

// Excerpt returns the start of the given text, cut on a word boundary where there's one past halfway
func Excerpt(text string, length int) string {
	runes := []rune(text)
	if len(runes) <= length {
		return text
	}
	cut := length
	for i := length - 1; i > length/2; i-- {
		if runes[i] == ' ' {
			cut = i
			break
		}
	}
	return strings.TrimRight(string(runes[:cut]), " ") + "…"
}
