package site

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nyaruka/helpsites/v26/core/models"
	"github.com/nyaruka/helpsites/v26/runtime"
	"github.com/nyaruka/helpsites/v26/web"
	"golang.org/x/net/html"
)

// how long a crawler can keep a root file - pages are rendered fresh on every request, so not long
const rootFileCacheControl = "public, max-age=300"

// handleRobots serves the site's robots.txt, which lets crawlers have everything and tells them where the sitemap is.
// Only the site on its own domain has one - a preview isn't for crawling.
func handleRobots(ctx context.Context, rt *runtime.Runtime, r *http.Request, w http.ResponseWriter) error {
	sc := web.GetSiteContext(ctx)
	if sc.IsPreview {
		return handleNotFound(ctx, rt, r, w)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", rootFileCacheControl)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "User-agent: *\nAllow: /\n\nSitemap: %s\n", siteURL(r, "/sitemap.xml"))
	return nil
}

type sitemapURLSet struct {
	XMLName    xml.Name     `xml:"urlset"`
	XMLNS      string       `xml:"xmlns,attr"`
	XMLNSImage string       `xml:"xmlns:image,attr"`
	URLs       []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc     string         `xml:"loc"`
	LastMod string         `xml:"lastmod,omitempty"`
	Images  []sitemapImage `xml:"image:image"`
}

type sitemapImage struct {
	Loc string `xml:"image:loc"`
}

// handleSitemap serves the site's sitemap: its home page, each section and each article, with the images each
// article shows - so that search engines can find all of them. Only the site on its own domain has one.
func handleSitemap(ctx context.Context, rt *runtime.Runtime, r *http.Request, w http.ResponseWriter) error {
	sc := web.GetSiteContext(ctx)
	if sc.IsPreview {
		return handleNotFound(ctx, rt, r, w)
	}

	sections, err := models.LoadSections(ctx, rt.DB, sc.Site.Source.ID)
	if err != nil {
		return err
	}
	articles, err := models.LoadReadable(ctx, rt.DB, sc.Site.Source.ID)
	if err != nil {
		return err
	}

	// a section's page lists its articles, so it changes whenever one of them does
	sectionModified := make(map[models.ArticleID]time.Time, len(sections))
	for _, s := range sections {
		sectionModified[s.ID] = s.ModifiedOn
	}
	for _, a := range articles {
		if a.ModifiedOn.After(sectionModified[a.Parent.ID]) {
			sectionModified[a.Parent.ID] = a.ModifiedOn
		}
	}

	set := &sitemapURLSet{
		XMLNS:      "http://www.sitemaps.org/schemas/sitemap/0.9",
		XMLNSImage: "http://www.google.com/schemas/sitemap-image/1.1",
		URLs:       []sitemapURL{{Loc: siteURL(r, "/")}},
	}
	for _, s := range sections {
		set.URLs = append(set.URLs, sitemapURL{
			Loc:     siteURL(r, "/"+s.Slug+"/"),
			LastMod: sectionModified[s.ID].UTC().Format(time.RFC3339),
		})
	}
	for _, a := range articles {
		loc := siteURL(r, "/"+a.Parent.Slug+"/"+a.Slug+"/")
		u := sitemapURL{Loc: loc, LastMod: a.ModifiedOn.UTC().Format(time.RFC3339)}
		for _, src := range ImageURLs(ResolveImages(a.BodyHTML, rt.Config.StorageURL), loc) {
			u.Images = append(u.Images, sitemapImage{Loc: src})
		}
		set.URLs = append(set.URLs, u)
	}

	out, err := xml.MarshalIndent(set, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling sitemap: %w", err)
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", rootFileCacheControl)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(append([]byte(xml.Header), out...)); err != nil {
		slog.Debug("error writing response", "error", err)
	}
	return nil
}

// siteURL returns the absolute URL of the given path on the site, as the request reached it
func siteURL(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return (&url.URL{Scheme: scheme, Host: r.Host, Path: path}).String()
}

// ImageURLs returns the addresses of the images the given rendered body shows, resolved against the address of its
// page, each once and in the order they appear. A fragment is only for the page, so isn't part of the image's address.
func ImageURLs(body, pageURL string) []string {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return nil
	}

	var urls []string
	seen := map[string]bool{}
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode || n.Data != "img" {
			continue
		}
		for _, attr := range n.Attr {
			if attr.Namespace != "" || attr.Key != "src" {
				continue
			}
			src, err := base.Parse(strings.TrimSpace(attr.Val))
			if err != nil || (src.Scheme != "http" && src.Scheme != "https") {
				continue
			}
			src.Fragment, src.RawFragment = "", ""
			if address := src.String(); !seen[address] {
				seen[address] = true
				urls = append(urls, address)
			}
		}
	}
	return urls
}
