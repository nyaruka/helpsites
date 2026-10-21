package site_test

import (
	"testing"

	"github.com/nyaruka/helpsites/v26/web/site"
	"github.com/stretchr/testify/assert"
)

func TestImageURLs(t *testing.T) {
	page := "https://help.nyaruka.com/billing/invoices/"

	tcs := []struct {
		body string
		urls []string
	}{
		{``, nil},
		{`<p>No images.</p>`, nil},
		{`<img src="a.png"><img src="/img/b.png"><img src="../c.png">`, []string{
			"https://help.nyaruka.com/billing/invoices/a.png",
			"https://help.nyaruka.com/img/b.png",
			"https://help.nyaruka.com/billing/c.png",
		}},
		{`<img src="//cdn.example.com/x.png"><img src="http://example.com/y.png">`, []string{
			"https://cdn.example.com/x.png",
			"http://example.com/y.png",
		}},
		{`<img src=" a.png "><img src="a.png#size=small&amp;layout=inline"><img src="https://help.nyaruka.com/billing/invoices/a.png">`, []string{
			"https://help.nyaruka.com/billing/invoices/a.png", // whitespace trimmed, fragments dropped, listed once
		}},
		{`<img src="b.png"><img src="a.png"><img src="b.png">`, []string{
			"https://help.nyaruka.com/billing/invoices/b.png", // in the order they first appear
			"https://help.nyaruka.com/billing/invoices/a.png",
		}},
		{`<img alt="none"><img src=" "><img src="data:image/png;base64,AAAA"><img src="mailto:x@example.com">`, nil},
		{`<a href="x.png">link</a><p data-src="y.png"></p>`, nil},
	}

	for _, tc := range tcs {
		assert.Equal(t, tc.urls, site.ImageURLs(tc.body, page), "body: %s", tc.body)
	}
}
