package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// --- inbound limiter -------------------------------------------------------

func TestLimitInFlightRejectsOverflow(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	defer close(release) // unblock the parked handler when the test ends
	h := limitInFlight(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
	}), 1)

	// Occupy the single slot with a request parked inside the handler.
	go h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	<-entered

	// A second request must be shed immediately rather than queue.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("overflow: want 503, got %d", rec.Code)
	}
}

func TestLimitInFlightStaticBypass(t *testing.T) {
	// Capacity 0: no non-static request can ever get a slot.
	h := limitInFlight(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), 0)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/app.css", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("static bypass: want 200, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("non-static: want 503, got %d", rec.Code)
	}
}

// --- outbound fetch limiter ------------------------------------------------

func TestAcquireFetchRespectsContext(t *testing.T) {
	saved := fetchSem
	fetchSem = make(chan struct{}, 1)
	defer func() { fetchSem = saved }()

	if err := acquireFetch(context.Background()); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	// Slot is full; a cancelled context must bail instead of blocking forever.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := acquireFetch(ctx); err == nil {
		t.Fatal("want error acquiring under cancelled ctx, got nil")
	}
	releaseFetch()
	if err := acquireFetch(context.Background()); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	releaseFetch()
}

// --- request parsing -------------------------------------------------------

func TestFeedURLsFromRequestCap(t *testing.T) {
	parts := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		parts = append(parts, fmt.Sprintf("https://e%d.com/feed", i))
	}
	q := url.QueryEscape(strings.Join(parts, ","))
	req := httptest.NewRequest(http.MethodGet, "/?url="+q, nil)
	if got := len(feedURLsFromRequest(req)); got != maxFeedURLs {
		t.Fatalf("cap: want %d urls, got %d", maxFeedURLs, got)
	}
}

func TestFeedURLsFromRequestFallsBackToUrls(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?urls=https://a.com/feed,https://b.com/feed", nil)
	if got := len(feedURLsFromRequest(req)); got != 2 {
		t.Fatalf("urls param: want 2, got %d", got)
	}
}

func TestOGCacheKeyOrderIndependent(t *testing.T) {
	a := ogCacheKey([]string{"https://b.com", "https://a.com"})
	b := ogCacheKey([]string{"https://a.com", "https://b.com"})
	if a != b {
		t.Fatalf("cache key not order-independent: %q vs %q", a, b)
	}
}

// --- feed cache ------------------------------------------------------------

func TestFeedCacheEvictsLRU(t *testing.T) {
	c := newFeedCache(time.Minute, 1)
	c.store("a", &FetchResult{Title: "a"})
	c.store("b", &FetchResult{Title: "b"})
	if _, ok := c.lookup("a"); ok {
		t.Fatal("a should have been evicted")
	}
	if _, ok := c.lookup("b"); !ok {
		t.Fatal("b should still be cached")
	}
}

func TestFeedCacheExpires(t *testing.T) {
	c := newFeedCache(time.Millisecond, 8)
	c.store("a", &FetchResult{Title: "a"})
	time.Sleep(5 * time.Millisecond)
	if _, ok := c.lookup("a"); ok {
		t.Fatal("entry should have expired")
	}
}

// --- image cache -----------------------------------------------------------

func TestImageCacheRendersOnce(t *testing.T) {
	c := newImageCache(time.Minute, 4)
	var calls int32
	render := func() ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		return []byte("png"), nil
	}
	for i := 0; i < 3; i++ {
		if _, err := c.getOrRender("k", render); err != nil {
			t.Fatalf("getOrRender: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("render calls: want 1, got %d", calls)
	}
}

// --- fetchFeed -------------------------------------------------------------

const sampleRSS = `<?xml version="1.0"?>
<rss version="2.0"><channel>
<title>Example</title><link>https://example.com</link>
<item><title>Hello</title><link>https://example.com/hello</link></item>
</channel></rss>`

func TestFetchFeedParses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		fmt.Fprint(w, sampleRSS)
	}))
	defer srv.Close()

	res, err := fetchFeed(context.Background(), srv.URL, "", "")
	if err != nil {
		t.Fatalf("fetchFeed: %v", err)
	}
	if res.Title != "Example" {
		t.Fatalf("title: want Example, got %q", res.Title)
	}
	if len(res.Entries) != 1 || res.Entries[0].Link != "https://example.com/hello" {
		t.Fatalf("entries: %+v", res.Entries)
	}
	if res.ETag != `"v1"` {
		t.Fatalf("etag: want v1, got %q", res.ETag)
	}
}

func TestFetchFeedNotModified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		fmt.Fprint(w, sampleRSS)
	}))
	defer srv.Close()

	res, err := fetchFeed(context.Background(), srv.URL, `"v1"`, "")
	if err != nil {
		t.Fatalf("fetchFeed 304: %v", err)
	}
	if res.Status != http.StatusNotModified {
		t.Fatalf("status: want 304, got %d", res.Status)
	}
	if res.ETag != `"v1"` {
		t.Fatalf("etag carried forward: want v1, got %q", res.ETag)
	}
}
