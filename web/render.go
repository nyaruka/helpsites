package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/nyaruka/helpsites/v26/core/models"
	"github.com/nyaruka/helpsites/v26/core/search"
	"github.com/nyaruka/helpsites/v26/runtime"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// StaticFS is the site's static assets, served under /static/
var StaticFS, _ = fs.Sub(staticFS, "static")

var funcs = template.FuncMap{
	"tr":   tr,
	"trn":  trn,
	"date": func(t time.Time) string { return t.Format("2 January 2006") },
	"css":  func(s string) template.CSS { return template.CSS(s) },
}

// the pages, each parsed with the base they extend
var pages = map[string]*template.Template{}

func init() {
	for _, page := range []string{"home", "section", "article", "search", "404"} {
		pages[page] = template.Must(template.New("base.html").Funcs(funcs).ParseFS(templateFS, "templates/base.html", "templates/"+page+".html"))
	}
	pages["unavailable"] = template.Must(template.New("unavailable.html").Funcs(funcs).ParseFS(templateFS, "templates/unavailable.html"))
}

// Page is what the templates render from
type Page struct {
	Site        *models.Site
	Prefix      string
	IsPreview   bool
	SettingsURL string
	AppHost     string
	Lang        string

	Query    string
	Sections []*models.Article
	Popular  []*models.Article
	Section  *models.Article
	Articles []*models.Article
	Article  *models.Article
	Siblings []*models.Article
	HTML     template.HTML
	Results  []*search.Result
}

// NewPage returns a page for the site of the given request
func NewPage(rt *runtime.Runtime, sc *SiteContext) *Page {
	p := &Page{Prefix: "", AppHost: rt.Config.AppHost, Lang: "en"}
	if sc != nil {
		p.Site = sc.Site
		p.Prefix = sc.Prefix
		p.IsPreview = sc.IsPreview
		if sc.IsPreview {
			p.SettingsURL = settingsPath
		}
	}
	return p
}

// Render renders the given page with the given status, rendering into a buffer first so that an error in a template
// is a 500 rather than half a page
func Render(w http.ResponseWriter, status int, page string, data *Page) error {
	tpl := pages[page]
	if tpl == nil {
		return fmt.Errorf("no such page: %s", page)
	}

	buf := &bytes.Buffer{}
	if err := tpl.Execute(buf, data); err != nil {
		return fmt.Errorf("error rendering %s: %w", page, err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if data.IsPreview {
		w.Header().Set("X-Robots-Tag", "noindex")
	}
	w.WriteHeader(status)
	if _, err := w.Write(buf.Bytes()); err != nil {
		slog.Debug("error writing response", "error", err)
	}
	return nil
}
