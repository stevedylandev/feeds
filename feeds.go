package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
)

type ParsedEntry struct {
	GUID        string
	Title       string
	Link        string
	Author      string
	PublishedAt int64
}

type FetchResult struct {
	Status       int
	ETag         string
	LastModified string
	Title        string
	SiteURL      string
	Entries      []ParsedEntry
}

type FeedPreviewItem struct {
	Title     string
	Link      string
	Author    string
	Published int64
}

const appUserAgent = "feeds/0.1 (+https://github.com/stevedylandev/feeds)"

func buildHTTPClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

func newRequest(ctx context.Context, method, rawURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", appUserAgent)
	return req, nil
}

func fetchFeed(ctx context.Context, feedURL, etag, lastModified string) (*FetchResult, error) {
	client := buildHTTPClient()
	req, err := newRequest(ctx, http.MethodGet, feedURL)
	if err != nil {
		return nil, err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()
	result := &FetchResult{
		Status:       resp.StatusCode,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}
	if resp.StatusCode == http.StatusNotModified {
		if result.ETag == "" {
			result.ETag = etag
		}
		if result.LastModified == "" {
			result.LastModified = lastModified
		}
		return result, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream returned %d", resp.StatusCode)
	}
	parser := gofeed.NewParser()
	feed, err := parser.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("feed parse failed: %w", err)
	}
	result.Title = strings.TrimSpace(feed.Title)
	result.SiteURL = firstNonEmpty(feed.Link, firstFeedAltLink(feed))
	for _, item := range feed.Items {
		link := strings.TrimSpace(item.Link)
		if link == "" {
			continue
		}
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = deriveTitleFromHTML(firstNonEmpty(item.Description, item.Content))
		}
		if title == "" {
			title = "Untitled post"
		}
		author := ""
		if item.Author != nil {
			author = strings.TrimSpace(item.Author.Name)
		}
		guid := strings.TrimSpace(item.GUID)
		if guid == "" {
			guid = link
		}
		published := int64(0)
		switch {
		case item.PublishedParsed != nil:
			published = item.PublishedParsed.Unix()
		case item.UpdatedParsed != nil:
			published = item.UpdatedParsed.Unix()
		}
		result.Entries = append(result.Entries, ParsedEntry{
			GUID:        guid,
			Title:       title,
			Link:        link,
			Author:      author,
			PublishedAt: published,
		})
	}
	return result, nil
}

func deriveTitleFromHTML(src string) string {
	txt := strings.Join(strings.Fields(htmlToText(src)), " ")
	if txt == "" {
		return ""
	}
	const maxChars = 80
	if utf8.RuneCountInString(txt) <= maxChars {
		return txt
	}
	runes := []rune(txt)
	return strings.TrimSpace(string(runes[:maxChars])) + "…"
}

func htmlToText(src string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	node, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return src
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(node)
	return html.UnescapeString(b.String())
}

func previewURLs(ctx context.Context, urls []string, perFeed int, cache *feedCache, log *slog.Logger) ([]FeedPreviewItem, map[string]string) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	items := []FeedPreviewItem{}
	titles := map[string]string{}
	for _, raw := range urls {
		feedURL := strings.TrimSpace(raw)
		if feedURL == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := cache.fetch(ctx, feedURL)
			if err != nil {
				log.Warn("preview fetch failed", "url", feedURL, "err", err)
				return
			}
			feedTitle := res.Title
			local := make([]FeedPreviewItem, 0, len(res.Entries))
			for _, entry := range res.Entries {
				if perFeed > 0 && len(local) >= perFeed {
					break
				}
				author := feedTitle
				if entry.Author != "" && feedTitle != "" {
					author = feedTitle + " - " + entry.Author
				} else if entry.Author != "" {
					author = entry.Author
				}
				local = append(local, FeedPreviewItem{Title: entry.Title, Link: entry.Link, Author: author, Published: entry.PublishedAt})
			}
			mu.Lock()
			items = append(items, local...)
			if feedTitle != "" {
				titles[feedURL] = feedTitle
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	slices.SortFunc(items, func(a, b FeedPreviewItem) int {
		switch {
		case a.Published > b.Published:
			return -1
		case a.Published < b.Published:
			return 1
		default:
			return 0
		}
	})
	return items, titles
}

func discoverFavicon(ctx context.Context, siteURL string) string {
	parsed, err := url.Parse(siteURL)
	if err != nil {
		return ""
	}
	client := buildHTTPClient()
	req, err := newRequest(ctx, http.MethodGet, siteURL)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err == nil {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if href := findLinkHref(string(body), func(rel, typ string) bool {
			rel = strings.ToLower(rel)
			return strings.Contains(rel, "icon")
		}); href != "" {
			if resolved, err := parsed.Parse(href); err == nil {
				return resolved.String()
			}
		}
	}
	if fallback, err := parsed.Parse("/favicon.ico"); err == nil {
		return fallback.String()
	}
	return ""
}

func findLinkHref(doc string, match func(rel, typ string) bool) string {
	node, err := html.Parse(strings.NewReader(doc))
	if err != nil {
		return ""
	}
	var found string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != "" {
			return
		}
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "link") {
			attrs := attrsMap(n)
			if match(attrs["rel"], attrs["type"]) {
				found = attrs["href"]
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(node)
	return found
}

func discoverFeeds(ctx context.Context, baseURL string) ([]string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	client := buildHTTPClient()

	// Pages to scan for <link rel="alternate"> feed hints. Always include the
	// origin root: a user often pastes a deep or dead feed URL (e.g. an
	// advertised /rss.xml that 404s) while the real feed is advertised on the
	// homepage.
	scanPages := []string{baseURL}
	if root := originRoot(parsed); root != "" && root != baseURL {
		scanPages = append(scanPages, root)
	}

	candidates := []string{}
	addCandidate := func(u string) {
		if u != "" && !slices.Contains(candidates, u) {
			candidates = append(candidates, u)
		}
	}
	for _, page := range scanPages {
		req, err := newRequest(ctx, http.MethodGet, page)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			continue
		}
		for _, href := range findAlternateFeedLinks(string(body)) {
			resolved := href
			if u, err := parsed.Parse(href); err == nil {
				resolved = u.String()
			}
			addCandidate(resolved)
		}
	}

	// Fall back to well-known feed paths only when the pages advertised none.
	if len(candidates) == 0 {
		paths := []string{"/feed", "/feed.xml", "/rss", "/rss.xml", "/atom.xml", "/index.xml", "/feed/rss", "/blog/feed", "/blog/rss"}
		for _, path := range paths {
			if probe, err := parsed.Parse(path); err == nil {
				addCandidate(probe.String())
			}
		}
	}

	// A candidate is only a feed if it actually parses. Content-type is
	// unreliable — many valid feeds serve text/html or send no type at all.
	// Validate concurrently to keep discovery fast.
	valid := make([]bool, len(candidates))
	var wg sync.WaitGroup
	for i, c := range candidates {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := fetchFeed(ctx, c, "", ""); err == nil {
				valid[i] = true
			}
		}()
	}
	wg.Wait()

	feeds := []string{}
	for i, c := range candidates {
		if valid[i] {
			feeds = append(feeds, c)
		}
	}
	if len(feeds) == 0 {
		return nil, errors.New("no feeds found at this URL")
	}
	return feeds, nil
}

// originRoot returns the scheme://host/ root for a parsed URL.
func originRoot(u *url.URL) string {
	if u == nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/"
}

func findAlternateFeedLinks(doc string) []string {
	node, err := html.Parse(strings.NewReader(doc))
	if err != nil {
		return nil
	}
	links := []string{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "link") {
			attrs := attrsMap(n)
			rel := strings.ToLower(attrs["rel"])
			typ := strings.ToLower(attrs["type"])
			href := attrs["href"]
			if strings.Contains(rel, "alternate") && href != "" && (strings.Contains(typ, "rss") || strings.Contains(typ, "atom") || strings.Contains(typ, "xml")) {
				links = append(links, href)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(node)
	return links
}

func attrsMap(n *html.Node) map[string]string {
	out := make(map[string]string, len(n.Attr))
	for _, a := range n.Attr {
		out[strings.ToLower(a.Key)] = a.Val
	}
	return out
}

func firstFeedAltLink(feed *gofeed.Feed) string {
	for _, link := range feed.Links {
		if strings.TrimSpace(link) != "" {
			return link
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func formatDate(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format("Jan 2, 2006")
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := []string{}
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
