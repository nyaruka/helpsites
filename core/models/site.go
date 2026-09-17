package models

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type OrgID int
type SiteID int
type SourceID int

// the feature a workspace needs for its help site to be served
const FeatureAgents = "agents"

// the site config keys
const (
	ConfigPrimaryColor = "primary_color" // links, buttons, accents
	ConfigHeaderColor  = "header_color"  // the header's background
	ConfigChatChannel  = "chat_channel"  // the uuid of the WebChat channel whose widget the site embeds, if any

	DefaultPrimaryColor = "#2f6fed"
	DefaultHeaderColor  = "#ffffff"
)

// Site is a help site: the public face of a workspace's helpdesk, served on a domain of the workspace's own
type Site struct {
	ID               SiteID            `json:"id"`
	UUID             string            `json:"uuid"`
	Title            string            `json:"title"`
	Tagline          string            `json:"tagline"`
	Footer           string            `json:"footer"`
	Domain           string            `json:"domain"`
	DomainVerifiedOn *time.Time        `json:"domain_verified_on"`
	IsEnabled        bool              `json:"is_enabled"`
	Config           map[string]string `json:"config"`
	Redirects        map[string]string `json:"redirects"`    // old addresses, by path, to the uuid of the page each is now
	ChatChannel      string            `json:"chat_channel"` // the uuid of the WebChat channel the site chats through, if any and still active

	Source struct {
		ID            SourceID   `json:"id"`
		UUID          string     `json:"uuid"`
		IsActive      bool       `json:"is_active"`
		LastIndexedOn *time.Time `json:"last_indexed_on"`
	} `json:"source"`

	Org struct {
		ID       OrgID    `json:"id"`
		IsActive bool     `json:"is_active"`
		Features []string `json:"features"`
	} `json:"org"`
}

const sqlSelectSite = `
SELECT ROW_TO_JSON(r) FROM (
    SELECT
        s.id, s.uuid, s.title, s.tagline, s.footer, s.domain, s.domain_verified_on, s.is_enabled, s.config, s.redirects,
        (
            SELECT c.uuid FROM channels_channel c
             WHERE c.org_id = o.id AND c.uuid::text = s.config->>'chat_channel' AND c.channel_type = 'WCH' AND c.is_active
        ) AS chat_channel,
        JSON_BUILD_OBJECT('id', k.id, 'uuid', k.uuid, 'is_active', k.is_active, 'last_indexed_on', k.last_indexed_on) AS source,
        JSON_BUILD_OBJECT('id', o.id, 'is_active', o.is_active, 'features', o.features) AS org
      FROM knowledge_helpsite s
      JOIN knowledge_knowledgesource k ON k.id = s.source_id
      JOIN orgs_org o ON o.id = k.org_id
     WHERE %s
) r;`

// LoadSiteByDomain loads the site served on the given domain, if there is one - a domain is a site's only once it's
// verified. Returns nil if there's no such site.
func LoadSiteByDomain(ctx context.Context, db DBorTx, domain string) (*Site, error) {
	site, err := queryJSONOne(ctx, db, func() *Site { return &Site{} },
		fmt.Sprintf(sqlSelectSite, `s.domain = $1 AND s.domain_verified_on IS NOT NULL`), domain,
	)
	if err != nil {
		return nil, fmt.Errorf("error loading site for domain %s: %w", domain, err)
	}
	return site, nil
}

// LoadSiteByUUID loads the site with the given UUID, e.g. for a preview. Returns nil if there's no such site.
func LoadSiteByUUID(ctx context.Context, db DBorTx, uuid string) (*Site, error) {
	site, err := queryJSONOne(ctx, db, func() *Site { return &Site{} },
		fmt.Sprintf(sqlSelectSite, `s.uuid = $1`), uuid,
	)
	if err != nil {
		return nil, fmt.Errorf("error loading site %s: %w", uuid, err)
	}
	return site, nil
}

const sqlSelectVerifiedDomains = `SELECT domain FROM knowledge_helpsite WHERE domain IS NOT NULL AND domain_verified_on IS NOT NULL`

// LoadVerifiedDomains loads every domain that's a verified site's - the set of names we're prepared to obtain a
// certificate for.
func LoadVerifiedDomains(ctx context.Context, db DBorTx) ([]string, error) {
	rows, err := db.QueryContext(ctx, sqlSelectVerifiedDomains)
	if err != nil {
		return nil, fmt.Errorf("error querying verified domains: %w", err)
	}
	defer rows.Close()

	domains := make([]string, 0, 10)
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		domains = append(domains, d)
	}
	return domains, nil
}

// NormalizeDomain returns the form a host is matched to a site's domain in - lowercased, without a port or a leading
// www., which a site answers for as well as its bare domain.
func NormalizeDomain(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.LastIndex(host, ":"); i >= 0 && !strings.Contains(host[i:], "]") {
		host = host[:i]
	}
	return strings.TrimPrefix(host, "www.")
}

// IsAvailable returns whether the site can be served publicly - enabled, verified, and belonging to a live helpdesk
// of a workspace that still has the feature.
func (s *Site) IsAvailable() bool {
	return s.IsEnabled && s.Domain != "" && s.DomainVerifiedOn != nil && s.Source.IsActive && s.Org.IsActive &&
		hasFeature(s.Org.Features, FeatureAgents)
}

func hasFeature(features []string, f string) bool {
	for _, feature := range features {
		if feature == f {
			return true
		}
	}
	return false
}

func (s *Site) PrimaryColor() string {
	if c := s.Config[ConfigPrimaryColor]; c != "" {
		return c
	}
	return DefaultPrimaryColor
}

func (s *Site) HeaderColor() string {
	if c := s.Config[ConfigHeaderColor]; c != "" {
		return c
	}
	return DefaultHeaderColor
}

// HeaderTextColor returns what's legible on the header - the page's own dark text on a light header, white on a
// dark one.
func (s *Site) HeaderTextColor() string {
	if IsDarkColor(s.HeaderColor()) {
		return "#ffffff"
	}
	return "#1f2430"
}

// NormalizePath returns the form an old address is kept and looked up in - lowercased, without any query or
// fragment, and with a leading slash but no trailing one, so the same page reached slightly differently is still the
// same page.
func NormalizePath(path string) string {
	path = strings.TrimSpace(path)
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	return "/" + strings.Trim(strings.ToLower(path), "/")
}

// GetRedirect returns where an address of the site the workspace moved from leads now, if the mapping has it and
// it's a page in the given link targets, or empty.
func (s *Site) GetRedirect(path string, targets map[string]string) string {
	uuid := s.Redirects[NormalizePath(path)]
	if uuid == "" {
		return ""
	}
	return targets[uuid]
}

// IsDarkColor returns whether a #rrggbb color is dark enough to want light text on it, by its relative luminance.
func IsDarkColor(color string) bool {
	if len(color) != 7 || color[0] != '#' {
		return false
	}
	linear := func(hex string) float64 {
		v, err := strconv.ParseUint(hex, 16, 8)
		if err != nil {
			return 0
		}
		c := float64(v) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	r, g, b := linear(color[1:3]), linear(color[3:5]), linear(color[5:7])
	return 0.2126*r+0.7152*g+0.0722*b < 0.4
}
