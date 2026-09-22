package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nyaruka/helpsites/v26/core/models"
	"github.com/nyaruka/helpsites/v26/runtime"
)

// Hit is a chunk of indexed knowledge matching a semantic search, naming its source and item
type Hit struct {
	KnowledgeUUID string  `json:"knowledge_uuid"`
	ItemKey       string  `json:"item_key"`
	Text          string  `json:"text"`
	Score         float64 `json:"score"`
}

// knowledgeSearch searches the workspace's indexed knowledge semantically through mailroom, returning the matching
// chunks best first
func knowledgeSearch(ctx context.Context, rt *runtime.Runtime, orgID models.OrgID, query string, limit int) ([]*Hit, error) {
	payload, _ := json.Marshal(map[string]any{"org_id": orgID, "query": query, "limit": limit})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rt.Config.MailroomURL+"/mi/knowledge/search", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Helpsites")
	if rt.Config.MailroomAuthToken != "" {
		req.Header.Set("Authorization", "Token "+rt.Config.MailroomAuthToken)
	}

	resp, err := rt.HTTP.Mailroom.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mailroom returned %d", resp.StatusCode)
	}

	body := &struct {
		Results []*Hit `json:"results"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(body); err != nil {
		return nil, fmt.Errorf("error decoding mailroom response: %w", err)
	}
	return body.Results, nil
}
