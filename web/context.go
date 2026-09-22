package web

import (
	"context"

	"github.com/nyaruka/helpsites/v26/core/models"
)

// the platform mounts a workspace's preview of its own site here, and proxies it to us
const PreviewPrefix = "/helpsite/preview"

// the platform's page for the settings of the site, linked from a preview
const settingsPath = "/article/"

// SiteContext is what a request carries about the site it's for: the site itself, and whether it's a preview - which
// is served under the platform's prefix rather than at the root of the site's domain
type SiteContext struct {
	Site      *models.Site
	Prefix    string
	IsPreview bool
}

type contextKey int

const siteContextKey contextKey = iota

func withSiteContext(ctx context.Context, sc *SiteContext) context.Context {
	return context.WithValue(ctx, siteContextKey, sc)
}

// GetSiteContext returns the site context of the given request context
func GetSiteContext(ctx context.Context) *SiteContext {
	sc, _ := ctx.Value(siteContextKey).(*SiteContext)
	return sc
}
