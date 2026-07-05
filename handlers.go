package main

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const maxFeedURLs = 20

func (a *App) indexHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("url")
	if query == "" {
		query = r.URL.Query().Get("urls")
	}
	data := indexPageData{BaseURL: a.BaseURL}
	if query == "" {
		render(a.Templates, w, "index.html", data, a.Log)
		return
	}

	urls := splitAndTrim(query)
	if len(urls) == 0 {
		render(a.Templates, w, "index.html", data, a.Log)
		return
	}
	if len(urls) > maxFeedURLs {
		urls = urls[:maxFeedURLs]
	}
	data.FeedURLs = urls

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	for _, item := range previewURLs(ctx, urls, 0, a.Log) {
		data.Items = append(data.Items, templateItem{Title: item.Title, Link: item.Link, Author: item.Author, FormattedDate: formatDate(item.Published)})
	}
	if len(data.Items) == 0 {
		data.Error = "No items could be loaded from these feeds"
	}
	render(a.Templates, w, "index.html", data, a.Log)
}

type resolvedFeed struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Favicon string `json:"favicon,omitempty"`
}

// faviconFor discovers the favicon for a fetched feed, falling back to the
// feed URL's origin when the feed doesn't declare a site link.
func faviconFor(ctx context.Context, res *FetchResult, feedURL string) string {
	site := res.SiteURL
	if site == "" {
		if u, err := url.Parse(feedURL); err == nil && u.Host != "" {
			site = u.Scheme + "://" + u.Host
		}
	}
	if site == "" {
		return ""
	}
	return discoverFavicon(ctx, site)
}

func (a *App) resolveHandler(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("url"))
	if raw == "" {
		writeError(w, http.StatusBadRequest, "url parameter is required")
		return
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	// If the input parses as a feed, use it directly.
	if res, err := fetchFeed(ctx, raw, "", ""); err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"feeds": []resolvedFeed{{URL: raw, Title: res.Title, Favicon: faviconFor(ctx, res, raw)}}})
		return
	}

	// Otherwise treat it as a site URL and discover feeds.
	candidates, err := discoverFeeds(ctx, raw)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "no feed found at this URL")
		return
	}
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	found := map[string]resolvedFeed{}
	for _, candidate := range candidates {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := fetchFeed(ctx, candidate, "", "")
			if err != nil {
				a.Log.Warn("resolve candidate failed", "url", candidate, "err", err)
				return
			}
			favicon := faviconFor(ctx, res, candidate)
			mu.Lock()
			found[candidate] = resolvedFeed{URL: candidate, Title: res.Title, Favicon: favicon}
			mu.Unlock()
		}()
	}
	wg.Wait()

	feeds := make([]resolvedFeed, 0, len(found))
	for _, candidate := range candidates {
		if f, ok := found[candidate]; ok {
			feeds = append(feeds, f)
		}
	}
	if len(feeds) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "no feed found at this URL")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"feeds": feeds})
}
