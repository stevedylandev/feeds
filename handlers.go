package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const maxFeedURLs = 20

func (a *App) privacyHandler(w http.ResponseWriter, r *http.Request) {
	render(a.Templates, w, "privacy.html", nil, a.Log)
}

func (a *App) indexHandler(w http.ResponseWriter, r *http.Request) {
	data := indexPageData{
		BaseURL:         a.BaseURL,
		MetaTitle:       "Feeds",
		MetaDescription: "An introduction to RSS",
		CanonicalURL:    a.BaseURL,
		OGImage:         a.BaseURL + "/static/og.png",
	}

	urls := feedURLsFromRequest(r)
	if len(urls) == 0 {
		render(a.Templates, w, "index.html", data, a.Log)
		return
	}
	data.CanonicalURL = a.BaseURL + r.URL.RequestURI()
	data.OGImage = a.BaseURL + "/og.png?" + r.URL.RawQuery

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	items, titles := previewURLs(ctx, urls, 0, a.Cache, a.Log)
	for _, item := range items {
		data.Items = append(data.Items, templateItem{Title: item.Title, Link: item.Link, Author: item.Author, FormattedDate: formatDate(item.Published)})
	}
	if len(data.Items) == 0 {
		data.Error = "No items could be loaded from these feeds"
	}
	for _, u := range urls {
		name := titles[u]
		if name == "" {
			name = hostName(u)
		}
		data.FeedURLs = append(data.FeedURLs, feedRef{Name: name, URL: u})
	}
	data.MetaTitle, data.MetaDescription = feedMeta(urls, titles, len(data.Items))
	render(a.Templates, w, "index.html", data, a.Log)
}

// feedURLsFromRequest extracts and normalizes the feed URLs from the "url" or
// "urls" query param, capped at maxFeedURLs.
func feedURLsFromRequest(r *http.Request) []string {
	query := r.URL.Query().Get("url")
	if query == "" {
		query = r.URL.Query().Get("urls")
	}
	urls := splitAndTrim(query)
	if len(urls) > maxFeedURLs {
		urls = urls[:maxFeedURLs]
	}
	return urls
}

// feedMeta builds an og:title and og:description from the shared feed URLs,
// their resolved titles (falling back to the URL host), and the item count.
func feedMeta(urls []string, titles map[string]string, itemCount int) (title, description string) {
	names := make([]string, 0, len(urls))
	for _, u := range urls {
		name := titles[u]
		if name == "" {
			name = hostName(u)
		}
		if name != "" {
			names = append(names, name)
		}
	}

	switch {
	case len(names) == 0:
		title = "Feeds"
	case len(names) == 1:
		title = names[0]
	default:
		title = fmt.Sprintf("%s +%d more", names[0], len(names)-1)
	}

	feedWord := "feed"
	if len(urls) != 1 {
		feedWord = "feeds"
	}
	postWord := "post"
	if itemCount != 1 {
		postWord = "posts"
	}
	description = fmt.Sprintf("%d %s from %d %s", itemCount, postWord, len(urls), feedWord)
	return title, description
}

// hostName returns the host of a URL with a leading "www." stripped.
func hostName(raw string) string {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Host, "www.")
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
