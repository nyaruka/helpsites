package models_test

import (
	"testing"

	"github.com/nyaruka/helpsites/v26/core/models"
	"github.com/nyaruka/helpsites/v26/testsuite"
	"github.com/nyaruka/helpsites/v26/testsuite/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSite(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)

	testdb.AddFeature(t, rt, testdb.Org1, models.FeatureAgents)
	site := testdb.InsertSite(t, rt, testdb.Org1, "Nyaruka Help", testdb.SiteOptions{
		Domain: "help.nyaruka.com", Verified: true, Enabled: true, Tagline: "How can we help?",
		Config: map[string]string{"primary_color": "#ff0000", "header_color": "#000000"},
	})
	unverified := testdb.InsertSite(t, rt, testdb.Org2, "Other Help", testdb.SiteOptions{Domain: "help.other.com", Enabled: true})

	assert.Equal(t, "Nyaruka Help", site.Title)
	assert.Equal(t, "How can we help?", site.Tagline)
	assert.Equal(t, "help.nyaruka.com", site.Domain)
	assert.True(t, site.IsAvailable())
	assert.Equal(t, "#ff0000", site.PrimaryColor())
	assert.Equal(t, "#000000", site.HeaderColor())
	assert.Equal(t, "#ffffff", site.HeaderTextColor())
	assert.Equal(t, "", site.ChatChannel)
	assert.Equal(t, testdb.Org1, site.Org.ID)
	assert.False(t, site.Source.IsActive == false)

	assert.Equal(t, []models.ColumnColor{}, site.ColumnColors())

	assert.False(t, unverified.IsAvailable())
	assert.Equal(t, models.DefaultPrimaryColor, unverified.PrimaryColor())
	assert.Equal(t, "#1f2430", unverified.HeaderTextColor())

	// by domain - only a verified domain is a site's
	loaded, err := models.LoadSiteByDomain(ctx, rt.DB, "help.nyaruka.com")
	require.NoError(t, err)
	assert.Equal(t, site.ID, loaded.ID)

	loaded, err = models.LoadSiteByDomain(ctx, rt.DB, "help.other.com")
	require.NoError(t, err)
	assert.Nil(t, loaded)

	loaded, err = models.LoadSiteByDomain(ctx, rt.DB, "nope.com")
	require.NoError(t, err)
	assert.Nil(t, loaded)

	// by uuid
	loaded, err = models.LoadSiteByUUID(ctx, rt.DB, unverified.UUID)
	require.NoError(t, err)
	assert.Equal(t, unverified.ID, loaded.ID)

	domains, err := models.LoadVerifiedDomains(ctx, rt.DB)
	require.NoError(t, err)
	assert.Equal(t, []string{"help.nyaruka.com"}, domains)

	// availability needs the feature, an enabled site and a verified domain
	_, err = rt.DB.ExecContext(ctx, `UPDATE orgs_org SET features = '{}' WHERE id = 1`)
	require.NoError(t, err)
	loaded, err = models.LoadSiteByUUID(ctx, rt.DB, site.UUID)
	require.NoError(t, err)
	assert.False(t, loaded.IsAvailable())
}

func TestNormalizeDomain(t *testing.T) {
	assert.Equal(t, "help.nyaruka.com", models.NormalizeDomain("help.nyaruka.com"))
	assert.Equal(t, "help.nyaruka.com", models.NormalizeDomain("WWW.Help.Nyaruka.com:443"))
	assert.Equal(t, "help.nyaruka.com", models.NormalizeDomain(" www.help.nyaruka.com "))
	assert.Equal(t, "", models.NormalizeDomain(""))
}

func TestNormalizePath(t *testing.T) {
	assert.Equal(t, "/", models.NormalizePath(""))
	assert.Equal(t, "/", models.NormalizePath("/"))
	assert.Equal(t, "/hc/en-us/articles/123", models.NormalizePath("/hc/en-us/articles/123/?x=1#top"))
	assert.Equal(t, "/hc/en-us/articles/123", models.NormalizePath("HC/en-US/Articles/123"))
}

func TestGetRedirect(t *testing.T) {
	site := &models.Site{Redirects: map[string]string{"/old/page": "abc"}}
	targets := map[string]string{"abc": "/getting-started/welcome/"}

	assert.Equal(t, "/getting-started/welcome/", site.GetRedirect("/old/page/", targets))
	assert.Equal(t, "/getting-started/welcome/", site.GetRedirect("/Old/Page?ref=1", targets))
	assert.Equal(t, "", site.GetRedirect("/other", targets))
	assert.Equal(t, "", site.GetRedirect("/old/page", map[string]string{})) // not a page any more
}

func TestIsDarkColor(t *testing.T) {
	assert.True(t, models.IsDarkColor("#000000"))
	assert.True(t, models.IsDarkColor("#2f6fed"))
	assert.False(t, models.IsDarkColor("#ffffff"))
	assert.False(t, models.IsDarkColor("#ffff00"))
	assert.False(t, models.IsDarkColor("red"))
}

func TestColumnColors(t *testing.T) {
	_, rt := testsuite.Runtime(t)

	site := testdb.InsertSite(t, rt, testdb.Org1, "Nyaruka Help", testdb.SiteOptions{
		ColorStyles: map[string]models.ColorStyle{
			"10": {Fill: "#123456", Text: "#edf2f8", Border: "#07101a"},
			"2":  {Fill: "#ffe8a3", Text: "#6b581f", Border: "#d4be7d"},
			"x":  {Fill: "#ffe8a3", Text: "#6b581f", Border: "#d4be7d"}, // not an index
			"01": {Fill: "#ffe8a3", Text: "#6b581f", Border: "#d4be7d"}, // not an index as an article embeds one
			"3":  {Fill: "red;}", Text: "#6b581f", Border: "#d4be7d"},   // not a color
			"4":  {Fill: "#ffe8a3", Text: "#6b581f", Border: "url(x)"},  // nor this
		},
	})

	// what a page's stylesheet gets is only indexes and colors, in index order
	assert.Equal(t, []models.ColumnColor{
		{Index: 2, ColorStyle: models.ColorStyle{Fill: "#ffe8a3", Text: "#6b581f", Border: "#d4be7d"}},
		{Index: 10, ColorStyle: models.ColorStyle{Fill: "#123456", Text: "#edf2f8", Border: "#07101a"}},
	}, site.ColumnColors())
}
