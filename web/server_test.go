package web_test

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nyaruka/helpsites/core/certs"
	"github.com/nyaruka/helpsites/core/models"
	"github.com/nyaruka/helpsites/runtime"
	"github.com/nyaruka/helpsites/testsuite"
	"github.com/nyaruka/helpsites/testsuite/testdb"
	"github.com/nyaruka/helpsites/web"
	_ "github.com/nyaruka/helpsites/web/site"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// a site with a couple of sections and articles, on a workspace with the feature
func setupSite(t *testing.T, rt *runtime.Runtime) (*models.Site, map[string]models.ArticleID) {
	testdb.AddFeature(t, rt, testdb.Org1, models.FeatureAgents)
	site := testdb.InsertSite(t, rt, testdb.Org1, "Nyaruka Help", testdb.SiteOptions{
		Domain: "help.nyaruka.com", Verified: true, Enabled: true, Tagline: "Answers for everyone", Footer: "© Nyaruka",
	})
	source := site.Source.ID

	ids := map[string]models.ArticleID{}
	ids["started"] = testdb.InsertSection(t, rt, source, "Getting Started", "getting-started", testdb.ArticleOptions{Description: "The basics"})
	ids["billing"] = testdb.InsertSection(t, rt, source, "Billing", "billing", testdb.ArticleOptions{})
	ids["empty"] = testdb.InsertSection(t, rt, source, "Empty", "empty", testdb.ArticleOptions{})
	ids["welcome"] = testdb.InsertArticle(t, rt, source, ids["started"], "Welcome", "welcome", testdb.ArticleOptions{
		BodyHTML: `<h2 id="hello">Hello</h2><p>Welcome. See <a href="article:` + "SETUP" + `">setting up</a> and <a href="article:00000000-0000-0000-0000-000000000000">gone</a>.</p>`,
		Headings: []models.Heading{{ID: "hello", Text: "Hello"}},
	})
	ids["setup"] = testdb.InsertArticle(t, rt, source, ids["started"], "Setup", "setup", testdb.ArticleOptions{BodyHTML: "<p>Set things up.</p>"})
	ids["invoices"] = testdb.InsertArticle(t, rt, source, ids["billing"], "Invoices", "invoices", testdb.ArticleOptions{BodyHTML: "<p>About invoices.</p>"})

	// the welcome article links to setup by uuid
	setupUUID := testdb.ArticleUUID(t, rt, ids["setup"])
	_, err := rt.DB.ExecContext(t.Context(), `UPDATE knowledge_article SET body_html = REPLACE(body_html, 'SETUP', $2) WHERE id = $1`, ids["welcome"], setupUUID)
	require.NoError(t, err)

	testdb.SetRedirects(t, rt, site, map[string]string{"/hc/articles/123": setupUUID, "/hc/articles/999": "00000000-0000-0000-0000-000000000000"})

	return site, ids
}

func newServer(t *testing.T, rt *runtime.Runtime) *web.Server {
	manager, err := certs.NewManager(rt)
	require.NoError(t, err)
	require.NoError(t, manager.Refresh(t.Context()))

	return web.NewServer(rt, manager)
}

// get makes a request of the site's pages as the HTTPS listener would see it
func get(t *testing.T, h http.Handler, host, path string, sni string) (*http.Response, string) {
	req := httptest.NewRequest(http.MethodGet, "https://"+host+path, nil)
	req.Host = host
	if sni != "" {
		req.TLS = &tls.ConnectionState{ServerName: sni}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body, _ := io.ReadAll(rec.Result().Body)
	return rec.Result(), string(body)
}

func TestSitePages(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	_, ids := setupSite(t, rt)
	h := newServer(t, rt).SiteHandler()

	// home
	resp, body := get(t, h, "help.nyaruka.com", "/", "help.nyaruka.com")
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
	assert.Contains(t, body, "<title>Nyaruka Help</title>")
	assert.Contains(t, body, "Answers for everyone")
	assert.Contains(t, body, `href="/getting-started/"`)
	assert.Contains(t, body, "2 articles")
	assert.Contains(t, body, `href="/billing/"`)
	assert.Contains(t, body, "1 article<")
	assert.NotContains(t, body, `href="/empty/"`)
	assert.NotContains(t, body, "Popular articles")
	assert.Contains(t, body, "© Nyaruka")
	assert.NotContains(t, body, "preview-bar")
	assert.NotContains(t, body, "<temba-webchat")
	assert.Equal(t, "", resp.Header.Get("X-Robots-Tag"))

	// www. works too
	resp, _ = get(t, h, "www.help.nyaruka.com", "/", "www.help.nyaruka.com")
	assert.Equal(t, 200, resp.StatusCode)

	// section
	resp, body = get(t, h, "help.nyaruka.com", "/getting-started/", "help.nyaruka.com")
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, body, "<title>Getting Started | Nyaruka Help</title>")
	assert.Contains(t, body, "The basics")
	assert.Contains(t, body, `href="/getting-started/welcome/"`)
	assert.Contains(t, body, "Hello Welcome. See setting up and gone.") // the excerpt

	resp, body = get(t, h, "help.nyaruka.com", "/empty/", "help.nyaruka.com")
	assert.Equal(t, 404, resp.StatusCode)
	assert.Contains(t, body, "Page not found")
	assert.Contains(t, body, "Nyaruka Help") // styled as one of the site's pages

	// article, with its links resolved and a view recorded
	resp, body = get(t, h, "help.nyaruka.com", "/getting-started/welcome/", "help.nyaruka.com")
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, body, "<title>Welcome | Nyaruka Help</title>")
	assert.Contains(t, body, `<a href="/getting-started/setup/">setting up</a>`)
	assert.Contains(t, body, "and gone.")
	assert.NotContains(t, body, "article:")
	assert.Contains(t, body, `<a href="#hello">Hello</a>`)
	assert.Contains(t, body, `class="current" aria-current="page"`)
	assert.Contains(t, body, "static/js/helpsite.js")
	assert.Contains(t, body, "Last updated")

	var views int
	require.NoError(t, rt.DB.QueryRowContext(t.Context(), `SELECT COALESCE(SUM(count), 0) FROM knowledge_articlecount WHERE article_id = $1`, ids["welcome"]).Scan(&views))
	assert.Equal(t, 1, views)

	// and it's now popular
	resp, body = get(t, h, "help.nyaruka.com", "/", "help.nyaruka.com")
	assert.Contains(t, body, "Popular articles")

	resp, _ = get(t, h, "help.nyaruka.com", "/getting-started/nope/", "help.nyaruka.com")
	assert.Equal(t, 404, resp.StatusCode)

	resp, _ = get(t, h, "help.nyaruka.com", "/billing/welcome/", "help.nyaruka.com") // wrong section
	assert.Equal(t, 404, resp.StatusCode)

	// search
	resp, body = get(t, h, "help.nyaruka.com", "/search/?q=invoices", "help.nyaruka.com")
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, body, "1 result for “invoices”")
	assert.Contains(t, body, `href="/billing/invoices/"`)
	assert.Contains(t, body, "<mark>invoices</mark>")

	resp, body = get(t, h, "help.nyaruka.com", "/search/?q=xyzzy", "help.nyaruka.com")
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, body, "0 results for “xyzzy”")
	assert.Contains(t, body, "No articles matched your search")

	resp, body = get(t, h, "help.nyaruka.com", "/search/", "help.nyaruka.com")
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, body, "<h1>Search</h1>")

	// a query is escaped
	_, body = get(t, h, "help.nyaruka.com", "/search/?q=%3Cscript%3E", "help.nyaruka.com")
	assert.NotContains(t, body, "<script>")

	// trailing slashes are added
	resp, _ = get(t, h, "help.nyaruka.com", "/getting-started?x=1", "help.nyaruka.com")
	assert.Equal(t, 301, resp.StatusCode)
	assert.Equal(t, "/getting-started/?x=1", resp.Header.Get("Location"))

	// old addresses are redirected, if they're a page
	resp, _ = get(t, h, "help.nyaruka.com", "/hc/articles/123/", "help.nyaruka.com")
	assert.Equal(t, 301, resp.StatusCode)
	assert.Equal(t, "/getting-started/setup/", resp.Header.Get("Location"))

	resp, _ = get(t, h, "help.nyaruka.com", "/hc/articles/999/", "help.nyaruka.com")
	assert.Equal(t, 404, resp.StatusCode)

	resp, _ = get(t, h, "help.nyaruka.com", "/hc/articles/other/", "help.nyaruka.com")
	assert.Equal(t, 404, resp.StatusCode)

	// static
	resp, body = get(t, h, "help.nyaruka.com", "/static/css/helpsite.css", "help.nyaruka.com")
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/css")
	assert.Equal(t, "public, max-age=3600", resp.Header.Get("Cache-Control"))
	assert.True(t, strings.Contains(body, "--primary"))

	// a host that isn't the name the handshake was for
	resp, _ = get(t, h, "help.nyaruka.com", "/", "other.example.com")
	assert.Equal(t, 421, resp.StatusCode)

	// a host that isn't a site's, or a site that can't be served
	resp, body = get(t, h, "nope.example.com", "/", "nope.example.com")
	assert.Equal(t, 404, resp.StatusCode)
	assert.Contains(t, body, "This help site isn&#39;t available")

	_, err := rt.DB.ExecContext(t.Context(), `UPDATE knowledge_helpsite SET is_enabled = FALSE`)
	require.NoError(t, err)
	resp, body = get(t, h, "help.nyaruka.com", "/", "help.nyaruka.com")
	assert.Equal(t, 404, resp.StatusCode)
	assert.Contains(t, body, "This help site isn&#39;t available")
}

func TestPreview(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	site, ids := setupSite(t, rt)
	s := newServer(t, rt)
	h := s.InternalHandler()

	// the site is disabled - a preview shows it anyway
	_, err := rt.DB.ExecContext(t.Context(), `UPDATE knowledge_helpsite SET is_enabled = FALSE`)
	require.NoError(t, err)

	preview := func(path string, token string) (*http.Response, string) {
		req := httptest.NewRequest(http.MethodGet, "http://helpsites:8031/hi/preview/"+site.UUID+path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Token "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		body, _ := io.ReadAll(rec.Result().Body)
		return rec.Result(), string(body)
	}

	resp, _ := preview("/", "")
	assert.Equal(t, 401, resp.StatusCode)

	resp, _ = preview("/", "wrong")
	assert.Equal(t, 401, resp.StatusCode)

	resp, body := preview("/", "sesame")
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "noindex", resp.Header.Get("X-Robots-Tag"))
	assert.Contains(t, body, `<meta name="robots" content="noindex">`)
	assert.Contains(t, body, "preview-bar")
	assert.Contains(t, body, `href="/article/"`)
	assert.Contains(t, body, `href="/helpsite/preview/getting-started/"`)
	assert.Contains(t, body, `href="/helpsite/preview/static/css/helpsite.css"`)

	resp, body = preview("", "sesame") // the root without its slash
	assert.Equal(t, 200, resp.StatusCode)

	// links in an article resolve under the prefix, and no view is recorded
	resp, body = preview("/getting-started/welcome/", "sesame")
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, body, `<a href="/helpsite/preview/getting-started/setup/">setting up</a>`)

	var views int
	require.NoError(t, rt.DB.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM knowledge_articlecount WHERE article_id = $1`, ids["welcome"]).Scan(&views))
	assert.Equal(t, 0, views)

	resp, _ = preview("/hc/articles/123/", "sesame")
	assert.Equal(t, 301, resp.StatusCode)
	assert.Equal(t, "/helpsite/preview/getting-started/setup/", resp.Header.Get("Location"))

	resp, body = preview("/nope/", "sesame")
	assert.Equal(t, 404, resp.StatusCode)
	assert.Contains(t, body, "Page not found")

	resp, _ = preview("/static/css/helpsite.css", "sesame")
	assert.Equal(t, 200, resp.StatusCode)

	// an unknown site
	req := httptest.NewRequest(http.MethodGet, "http://helpsites:8031/hi/preview/00000000-0000-0000-0000-000000000000/", nil)
	req.Header.Set("Authorization", "Token sesame")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, 404, rec.Code)

	// health
	req = httptest.NewRequest(http.MethodGet, "http://helpsites:8031/", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), `"component":"helpsites"`)
}

func TestListeners(t *testing.T) {
	_, rt := testsuite.Runtime(t)
	setupSite(t, rt)

	// bind to free ports
	rt.Config.HTTPSAddress, rt.Config.HTTPSPort = "127.0.0.1", 0
	rt.Config.HTTPAddress, rt.Config.HTTPPort = "127.0.0.1", 0
	rt.Config.InternalAddress, rt.Config.InternalPort = "127.0.0.1", 0

	s := newServer(t, rt)
	require.NoError(t, s.Start())
	defer s.Stop()

	httpsAddr, httpAddr, _ := s.Addrs()

	// the HTTP listener answers the health check and redirects everything else to HTTPS
	resp, err := http.Get("http://" + httpAddr + "/healthz")
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequest(http.MethodGet, "http://"+httpAddr+"/getting-started/?x=1", nil)
	req.Host = "help.nyaruka.com"
	resp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 308, resp.StatusCode)
	assert.Equal(t, "https://help.nyaruka.com/getting-started/?x=1", resp.Header.Get("Location"))

	// the HTTPS listener serves a verified domain with a certificate for it, and refuses a handshake for anything else
	dial := func(serverName string) (*tls.Conn, error) {
		return tls.Dial("tcp", httpsAddr, &tls.Config{ServerName: serverName, InsecureSkipVerify: true})
	}

	conn, err := dial("help.nyaruka.com")
	require.NoError(t, err)
	assert.Equal(t, []string{"help.nyaruka.com"}, conn.ConnectionState().PeerCertificates[0].DNSNames)
	conn.Close()

	conn, err = dial("www.help.nyaruka.com")
	require.NoError(t, err)
	conn.Close()

	_, err = dial("nope.example.com")
	assert.Error(t, err)

	// and a whole request through it
	tlsClient := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, httpsAddr) // whatever the host, it's us
		},
	}}
	resp, err = tlsClient.Get("https://help.nyaruka.com/getting-started/")
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "<title>Getting Started | Nyaruka Help</title>")
}
