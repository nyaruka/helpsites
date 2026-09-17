package web

import (
	"context"
	"net/http"

	"github.com/nyaruka/helpsites/runtime"
)

// Handler is a handler of a site request - the site is in the request's context, see GetSiteContext
type Handler func(ctx context.Context, rt *runtime.Runtime, r *http.Request, w http.ResponseWriter) error

type route struct {
	method  string
	pattern string
	handler Handler
}

var siteRoutes []*route
var siteNotFound Handler

// SiteRoute registers a route of a site's pages - served at the root of the site's domain, and under the preview
// prefix
func SiteRoute(method string, pattern string, handler Handler) {
	siteRoutes = append(siteRoutes, &route{method: method, pattern: pattern, handler: handler})
}

// SiteNotFound registers the handler for anything else asked of a site
func SiteNotFound(handler Handler) {
	siteNotFound = handler
}
