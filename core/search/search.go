package search

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"regexp"
	"strings"
	"time"

	valkey "github.com/gomodule/redigo/redis"
	"github.com/nyaruka/helpsites/core/models"
	"github.com/nyaruka/helpsites/runtime"
)

const (
	Limit         = 20
	SnippetLength = 200
	cacheTTL      = 5 * time.Minute
	cacheKey      = "helpsites:search:%d:%x"
)

// Result is an article matching a search, with a snippet of it to show
type Result struct {
	Article *models.Article
	Snippet template.HTML
}

// what's cached of a result - the snippet, and the article to load
type cachedResult struct {
	ID      models.ArticleID `json:"id"`
	Snippet template.HTML    `json:"snippet"`
}

var termRegex = regexp.MustCompile(`\W+`)

// Search searches the site's articles, returning the best first. Semantic search through mailroom leads when the
// helpdesk has been indexed, and text search over titles and bodies fills in behind it - so a search works before the
// first index, and still finds an exact phrase the embeddings rank low. The site is public, and a search costs an
// embedding and a scan of every body - so the same question asked again within a few minutes is answered from the
// last time, less anything unpublished since.
func Search(ctx context.Context, rt *runtime.Runtime, site *models.Site, query string, limit int) ([]*Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	key := fmt.Sprintf(cacheKey, site.ID, md5.Sum([]byte(fmt.Sprintf("%s|%d", strings.ToLower(query), limit))))

	if cached := readCache(rt, key); cached != nil {
		ids := make([]models.ArticleID, len(cached))
		for i, c := range cached {
			ids[i] = c.ID
		}
		byID, err := models.LoadReadableByID(ctx, rt.DB, site.Source.ID, ids)
		if err != nil {
			return nil, err
		}
		results := make([]*Result, 0, len(cached))
		for _, c := range cached {
			if a := byID[c.ID]; a != nil {
				results = append(results, &Result{Article: a, Snippet: c.Snippet})
			}
		}
		return results, nil
	}

	// terms worth marking in a snippet
	terms := make([]string, 0, 5)
	for _, t := range termRegex.Split(query, -1) {
		if len(t) > 2 {
			terms = append(terms, t)
		}
	}

	ordered := make([]*models.Article, 0, limit)
	snippets := make(map[string]template.HTML)

	if rt.Config.MailroomURL != "" && site.Source.LastIndexedOn != nil {
		// the workspace's sources are searched together, so ask for more than we need and keep what's ours
		hits, err := knowledgeSearch(ctx, rt, site.Org.ID, query, limit*3)
		if err != nil {
			slog.Error("error searching knowledge", "comp", "search", "error", err)
		}

		keys := make([]string, 0, len(hits))
		for _, h := range hits {
			if h.KnowledgeUUID == site.Source.UUID && snippets[h.ItemKey] == "" {
				keys = append(keys, h.ItemKey)
				snippets[h.ItemKey] = MakeSnippet(models.PlainText(h.Text), terms, SnippetLength)
			}
		}

		if len(keys) > 0 {
			byUUID, err := models.LoadReadableByUUID(ctx, rt.DB, site.Source.ID, keys)
			if err != nil {
				return nil, err
			}
			for _, k := range keys {
				if a := byUUID[k]; a != nil {
					ordered = append(ordered, a)
				}
			}
		}
	}

	if len(ordered) < limit {
		exclude := make([]models.ArticleID, len(ordered))
		for i, a := range ordered {
			exclude[i] = a.ID
		}
		matches, err := models.SearchArticles(ctx, rt.DB, site.Source.ID, query, exclude, limit-len(ordered))
		if err != nil {
			return nil, err
		}
		ordered = append(ordered, matches...)
	}

	if len(ordered) > limit {
		ordered = ordered[:limit]
	}

	results := make([]*Result, len(ordered))
	cached := make([]*cachedResult, len(ordered))
	for i, a := range ordered {
		snippet := snippets[a.UUID]
		if snippet == "" {
			snippet = MakeSnippet(a.PlainText(), terms, SnippetLength)
		}
		results[i] = &Result{Article: a, Snippet: snippet}
		cached[i] = &cachedResult{ID: a.ID, Snippet: snippet}
	}

	writeCache(rt, key, cached)

	return results, nil
}

func readCache(rt *runtime.Runtime, key string) []*cachedResult {
	vc := rt.VK.Get()
	defer vc.Close()

	data, err := valkey.Bytes(vc.Do("GET", key))
	if err != nil {
		if err != valkey.ErrNil {
			slog.Error("error reading search cache", "comp", "search", "error", err)
		}
		return nil
	}

	var cached []*cachedResult
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil
	}
	return cached
}

func writeCache(rt *runtime.Runtime, key string, results []*cachedResult) {
	vc := rt.VK.Get()
	defer vc.Close()

	data, _ := json.Marshal(results)
	if _, err := vc.Do("SET", key, data, "EX", int(cacheTTL.Seconds())); err != nil {
		slog.Error("error writing search cache", "comp", "search", "error", err)
	}
}
