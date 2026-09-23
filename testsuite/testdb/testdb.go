// Package testdb has helpers for putting help sites and their articles into a test database, on top of the
// workspaces and users the dump seeds it with.
package testdb

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nyaruka/helpsites/v26/core/models"
	"github.com/nyaruka/helpsites/v26/runtime"
	"github.com/stretchr/testify/require"
)

// the workspaces and users in the dump
const (
	Org1  models.OrgID = 1
	Org2  models.OrgID = 2
	Admin              = 3
)

// HelpdeskSource returns the id of the workspace's helpdesk source, which every workspace has
func HelpdeskSource(t *testing.T, rt *runtime.Runtime, orgID models.OrgID) models.SourceID {
	t.Helper()

	var id models.SourceID
	err := rt.DB.QueryRowContext(context.Background(),
		`SELECT id FROM knowledge_knowledgesource WHERE org_id = $1 AND source_type = 'helpdesk' AND is_system`, orgID,
	).Scan(&id)
	require.NoError(t, err, "error looking up helpdesk source")
	return id
}

// AddFeature gives the workspace the given feature
func AddFeature(t *testing.T, rt *runtime.Runtime, orgID models.OrgID, feature string) {
	t.Helper()

	_, err := rt.DB.ExecContext(context.Background(),
		`UPDATE orgs_org SET features = ARRAY_APPEND(ARRAY_REMOVE(features, $2), $2) WHERE id = $1`, orgID, feature,
	)
	require.NoError(t, err)
}

// SiteOptions is what a test can vary about a site
type SiteOptions struct {
	Domain   string
	Verified bool
	Enabled  bool
	Tagline  string
	Footer   string
	Config   map[string]string

	// what the helpdesk's palette entries look like, as the platform keeps them alongside the palette
	ColorStyles map[string]models.ColorStyle
}

// InsertSite inserts a help site for the workspace's helpdesk and returns it
func InsertSite(t *testing.T, rt *runtime.Runtime, orgID models.OrgID, title string, opts SiteOptions) *models.Site {
	t.Helper()

	source := HelpdeskSource(t, rt, orgID)

	var domain *string
	if opts.Domain != "" {
		domain = &opts.Domain
	}
	var verifiedOn *time.Time
	if opts.Verified {
		now := time.Now()
		verifiedOn = &now
	}
	config := opts.Config
	if config == nil {
		config = map[string]string{}
	}
	configJSON, _ := json.Marshal(config)

	siteUUID := uuid.NewString()

	_, err := rt.DB.ExecContext(context.Background(),
		`INSERT INTO knowledge_helpsite(uuid, source_id, title, tagline, footer, domain, domain_token, domain_verified_on, is_enabled, config, redirects, created_by_id, created_on, modified_by_id, modified_on)
		 VALUES($1, $2, $3, $4, $5, $6, 'token', $7, $8, $9, '{}', $10, NOW(), $10, NOW())`,
		siteUUID, source, title, opts.Tagline, opts.Footer, domain, verifiedOn, opts.Enabled, configJSON, Admin,
	)
	require.NoError(t, err)

	if opts.ColorStyles != nil {
		stylesJSON, _ := json.Marshal(opts.ColorStyles)
		_, err = rt.DB.ExecContext(context.Background(),
			`UPDATE knowledge_knowledgesource SET config = config || JSONB_BUILD_OBJECT('color_styles', $2::jsonb) WHERE id = $1`, source, stylesJSON,
		)
		require.NoError(t, err)
	}

	site, err := models.LoadSiteByUUID(context.Background(), rt.DB, siteUUID)
	require.NoError(t, err)
	return site
}

// SetRedirects sets the site's mapping of old addresses to the uuids of the pages they are now
func SetRedirects(t *testing.T, rt *runtime.Runtime, site *models.Site, redirects map[string]string) {
	t.Helper()

	redirectsJSON, _ := json.Marshal(redirects)
	_, err := rt.DB.ExecContext(context.Background(), `UPDATE knowledge_helpsite SET redirects = $2 WHERE id = $1`, site.ID, redirectsJSON)
	require.NoError(t, err)
}

// ArticleOptions is what a test can vary about an article
type ArticleOptions struct {
	Description string
	Body        string // the markdown source; derived from BodyHTML when not given
	BodyHTML    string
	Headings    []models.Heading
	Language    string // ISO-639-3, English when not given
	Draft       bool
	Inactive    bool
	SortOrder   int
}

// InsertSection inserts a section (an article with no parent) into the helpdesk and returns its id
func InsertSection(t *testing.T, rt *runtime.Runtime, source models.SourceID, title, slug string, opts ArticleOptions) models.ArticleID {
	t.Helper()

	return insertArticle(t, rt, source, nil, title, slug, opts)
}

// InsertArticle inserts an article into the given section and returns its id
func InsertArticle(t *testing.T, rt *runtime.Runtime, source models.SourceID, section models.ArticleID, title, slug string, opts ArticleOptions) models.ArticleID {
	t.Helper()

	return insertArticle(t, rt, source, &section, title, slug, opts)
}

func insertArticle(t *testing.T, rt *runtime.Runtime, source models.SourceID, parent *models.ArticleID, title, slug string, opts ArticleOptions) models.ArticleID {
	t.Helper()

	status := models.ArticleStatusPublished
	var publishedOn *time.Time
	if opts.Draft {
		status = models.ArticleStatusDraft
	} else {
		now := time.Now()
		publishedOn = &now
	}
	headings := opts.Headings
	if headings == nil {
		headings = []models.Heading{}
	}
	headingsJSON, _ := json.Marshal(headings)
	body := opts.Body
	if body == "" {
		body = models.PlainText(opts.BodyHTML)
	}
	language := opts.Language
	if language == "" {
		language = "eng"
	}

	var id models.ArticleID
	err := rt.DB.QueryRowContext(context.Background(),
		`INSERT INTO knowledge_article(uuid, source_id, parent_id, sort_order, title, slug, body, body_html, headings, description, language, status, published_on, is_active, created_by_id, created_on, modified_by_id, modified_on)
		 VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW(), $15, NOW()) RETURNING id`,
		uuid.NewString(), source, parent, opts.SortOrder, title, slug, body, opts.BodyHTML, headingsJSON, opts.Description, language, status, publishedOn, !opts.Inactive, Admin,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

// ArticleUUID returns the uuid of the given article
func ArticleUUID(t *testing.T, rt *runtime.Runtime, id models.ArticleID) string {
	t.Helper()

	var u string
	require.NoError(t, rt.DB.QueryRowContext(context.Background(), `SELECT uuid FROM knowledge_article WHERE id = $1`, id).Scan(&u))
	return u
}

// InsertViews records the given number of views of an article on the given day
func InsertViews(t *testing.T, rt *runtime.Runtime, article models.ArticleID, day time.Time, count int) {
	t.Helper()

	_, err := rt.DB.ExecContext(context.Background(),
		`INSERT INTO knowledge_articlecount(article_id, day, scope, count, is_squashed) VALUES($1, $2, 'views', $3, FALSE)`,
		article, day.Format("2006-01-02"), count,
	)
	require.NoError(t, err)
}
