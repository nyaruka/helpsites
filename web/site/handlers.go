// Package site has the handlers of a help site's pages, which are the same whether the site is being served on its
// own domain or previewed from inside the platform.
package site

import (
	"context"
	"html/template"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nyaruka/helpsites/core/models"
	"github.com/nyaruka/helpsites/core/search"
	"github.com/nyaruka/helpsites/runtime"
	"github.com/nyaruka/helpsites/web"
)

const (
	popularDays  = 30 // how far back views count towards being popular
	popularLimit = 6
	maxQueryLen  = 200
)

func init() {
	// search comes before the section pattern so it can't be shadowed by a section of that name
	web.SiteRoute(http.MethodGet, "/", handleHome)
	web.SiteRoute(http.MethodGet, "/search/", handleSearch)
	web.SiteRoute(http.MethodGet, "/{section:[\\w-]+}/", handleSection)
	web.SiteRoute(http.MethodGet, "/{section:[\\w-]+}/{article:[\\w-]+}/", handleArticle)
	web.SiteNotFound(handleNotFound)
}

func handleHome(ctx context.Context, rt *runtime.Runtime, r *http.Request, w http.ResponseWriter) error {
	sc := web.GetSiteContext(ctx)
	page := web.NewPage(rt, sc)

	var err error
	page.Sections, err = models.LoadSections(ctx, rt.DB, sc.Site.Source.ID)
	if err != nil {
		return err
	}

	since := time.Now().UTC().AddDate(0, 0, -popularDays)
	page.Popular, err = models.LoadPopular(ctx, rt.DB, sc.Site.Source.ID, since, popularLimit)
	if err != nil {
		return err
	}

	return web.Render(w, http.StatusOK, "home", page)
}

func handleSearch(ctx context.Context, rt *runtime.Runtime, r *http.Request, w http.ResponseWriter) error {
	sc := web.GetSiteContext(ctx)
	page := web.NewPage(rt, sc)

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if runes := []rune(query); len(runes) > maxQueryLen {
		query = string(runes[:maxQueryLen])
	}
	page.Query = query

	if query != "" {
		var err error
		page.Results, err = search.Search(ctx, rt, sc.Site, query, search.Limit)
		if err != nil {
			return err
		}
	}

	return web.Render(w, http.StatusOK, "search", page)
}

func handleSection(ctx context.Context, rt *runtime.Runtime, r *http.Request, w http.ResponseWriter) error {
	sc := web.GetSiteContext(ctx)
	page := web.NewPage(rt, sc)

	section, err := models.LoadSection(ctx, rt.DB, sc.Site.Source.ID, chi.URLParam(r, "section"))
	if err != nil {
		return err
	}
	if section == nil {
		return handleNotFound(ctx, rt, r, w)
	}

	articles, err := models.LoadArticles(ctx, rt.DB, sc.Site.Source.ID, section.ID)
	if err != nil {
		return err
	}
	if len(articles) == 0 {
		return handleNotFound(ctx, rt, r, w) // a section with nothing published in it isn't listed either
	}

	page.Section = section
	page.Articles = articles
	return web.Render(w, http.StatusOK, "section", page)
}

func handleArticle(ctx context.Context, rt *runtime.Runtime, r *http.Request, w http.ResponseWriter) error {
	sc := web.GetSiteContext(ctx)
	page := web.NewPage(rt, sc)

	section, err := models.LoadSection(ctx, rt.DB, sc.Site.Source.ID, chi.URLParam(r, "section"))
	if err != nil {
		return err
	}
	if section == nil {
		return handleNotFound(ctx, rt, r, w)
	}

	article, err := models.LoadArticle(ctx, rt.DB, sc.Site.Source.ID, section.ID, chi.URLParam(r, "article"))
	if err != nil {
		return err
	}
	if article == nil {
		return handleNotFound(ctx, rt, r, w)
	}

	targets, err := models.LoadLinkTargets(ctx, rt.DB, sc.Site.Source.ID, sc.Prefix)
	if err != nil {
		return err
	}

	siblings, err := models.LoadArticles(ctx, rt.DB, sc.Site.Source.ID, section.ID)
	if err != nil {
		return err
	}

	// a preview is the workspace looking at its own site, not a reader
	if !sc.IsPreview {
		if err := models.RecordArticleView(ctx, rt.DB, article.ID); err != nil {
			return err
		}
	}

	page.Section = section
	page.Article = article
	page.HTML = ResolveLinks(article.BodyHTML, targets)
	page.Siblings = siblings
	return web.Render(w, http.StatusOK, "article", page)
}

// handleNotFound answers anything else asked of the site - which is where the addresses of the site the workspace
// moved from arrive. One the site's mapping knows is sent on to where that page lives now; anything else is not
// found.
func handleNotFound(ctx context.Context, rt *runtime.Runtime, r *http.Request, w http.ResponseWriter) error {
	sc := web.GetSiteContext(ctx)

	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		targets, err := models.LoadLinkTargets(ctx, rt.DB, sc.Site.Source.ID, sc.Prefix)
		if err != nil {
			return err
		}
		if address := sc.Site.GetRedirect(r.URL.Path, targets); address != "" {
			http.Redirect(w, r, address, http.StatusMovedPermanently)
			return nil
		}
	}

	return web.Render(w, http.StatusNotFound, "404", web.NewPage(rt, sc))
}

// a link to another article in a rendered body, by the article's uuid - see the platform's ArticleLinks extension
var articleLinkRegex = regexp.MustCompile(`(?s)<a href="article:([0-9a-f-]{36})">(.*?)</a>`)

// ResolveLinks resolves the article: links in a rendered body against the given map of uuid to address - what the
// linked article's page is as of now. A link to something that isn't a page any more renders as plain text.
func ResolveLinks(html string, targets map[string]string) template.HTML {
	return template.HTML(articleLinkRegex.ReplaceAllStringFunc(html, func(link string) string {
		m := articleLinkRegex.FindStringSubmatch(link)
		if address := targets[m[1]]; address != "" {
			return `<a href="` + address + `">` + m[2] + `</a>`
		}
		return m[2]
	}))
}
