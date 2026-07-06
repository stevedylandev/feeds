package main

import (
	"container/list"
	"context"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// feedCache is an LRU + TTL cache of fetched feed results, keyed by feed URL.
// It keeps crawler-triggered requests (page + OG image) fast after the first
// fetch warms the entry. A singleflight group collapses concurrent misses for
// the same URL into a single upstream fetch, and the bounded LRU keeps the
// entry set from growing without limit under a stream of distinct URLs.
type feedCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	max     int
	ll      *list.List // front = most recently used
	entries map[string]*list.Element
	sf      singleflight.Group
}

type feedCacheEntry struct {
	key     string
	res     *FetchResult
	etag    string
	lm      string
	expires time.Time
}

func newFeedCache(ttl time.Duration, max int) *feedCache {
	return &feedCache{
		ttl:     ttl,
		max:     max,
		ll:      list.New(),
		entries: map[string]*list.Element{},
	}
}

// fetch returns a cached result when fresh, otherwise fetches and stores it.
// Concurrent misses for the same URL share one fetch via singleflight.
func (c *feedCache) fetch(ctx context.Context, feedURL string) (*FetchResult, error) {
	if res, ok := c.lookup(feedURL); ok {
		return res, nil
	}

	v, err, _ := c.sf.Do(feedURL, func() (any, error) {
		// Another goroutine may have filled the entry while we queued.
		if res, ok := c.lookup(feedURL); ok {
			return res, nil
		}

		// Reuse any stored validators so an unchanged feed comes back as a
		// cheap 304 instead of a full download + parse.
		etag, lm, prev := c.validators(feedURL)
		res, err := fetchFeed(ctx, feedURL, etag, lm)
		if err != nil {
			return nil, err
		}
		if res.Status == http.StatusNotModified && prev != nil {
			res = prev
		}
		c.store(feedURL, res)
		return res, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*FetchResult), nil
}

// lookup returns a fresh (unexpired) cached result and marks it recently used.
func (c *feedCache) lookup(feedURL string) (*FetchResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.entries[feedURL]
	if !ok {
		return nil, false
	}
	e := el.Value.(*feedCacheEntry)
	if time.Now().After(e.expires) {
		return nil, false
	}
	c.ll.MoveToFront(el)
	return e.res, true
}

// validators returns stored conditional-GET headers and the last parsed result
// for a URL, even when the entry has expired, so a refetch can revalidate.
func (c *feedCache) validators(feedURL string) (etag, lm string, prev *FetchResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[feedURL]; ok {
		e := el.Value.(*feedCacheEntry)
		return e.etag, e.lm, e.res
	}
	return "", "", nil
}

func (c *feedCache) store(feedURL string, res *FetchResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	expires := time.Now().Add(c.ttl)
	if el, ok := c.entries[feedURL]; ok {
		e := el.Value.(*feedCacheEntry)
		e.res, e.etag, e.lm, e.expires = res, res.ETag, res.LastModified, expires
		c.ll.MoveToFront(el)
		return
	}
	el := c.ll.PushFront(&feedCacheEntry{
		key:     feedURL,
		res:     res,
		etag:    res.ETag,
		lm:      res.LastModified,
		expires: expires,
	})
	c.entries[feedURL] = el
	for c.ll.Len() > c.max {
		oldest := c.ll.Back()
		if oldest == nil {
			break
		}
		c.ll.Remove(oldest)
		delete(c.entries, oldest.Value.(*feedCacheEntry).key)
	}
}

// imageCache is an LRU + TTL cache of rendered PNG bytes, keyed by the shared
// URL set. It bounds memory (max entries) and lets repeated shares of the same
// link skip the CPU/allocation cost of re-rendering.
type imageCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	max     int
	ll      *list.List // front = most recently used
	entries map[string]*list.Element
	sf      singleflight.Group
}

type imageEntry struct {
	key     string
	png     []byte
	expires time.Time
}

func newImageCache(ttl time.Duration, max int) *imageCache {
	return &imageCache{
		ttl:     ttl,
		max:     max,
		ll:      list.New(),
		entries: map[string]*list.Element{},
	}
}

func (c *imageCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	ent := el.Value.(*imageEntry)
	if time.Now().After(ent.expires) {
		c.ll.Remove(el)
		delete(c.entries, key)
		return nil, false
	}
	c.ll.MoveToFront(el)
	return ent.png, true
}

// getOrRender returns a cached PNG or renders one via render, collapsing
// concurrent requests for the same key into a single render so duplicate
// crawler hits don't each pay the CPU/allocation cost.
func (c *imageCache) getOrRender(key string, render func() ([]byte, error)) ([]byte, error) {
	if png, ok := c.get(key); ok {
		return png, nil
	}
	v, err, _ := c.sf.Do(key, func() (any, error) {
		if png, ok := c.get(key); ok {
			return png, nil
		}
		png, err := render()
		if err != nil {
			return nil, err
		}
		c.set(key, png)
		return png, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]byte), nil
}

func (c *imageCache) set(key string, png []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[key]; ok {
		ent := el.Value.(*imageEntry)
		ent.png = png
		ent.expires = time.Now().Add(c.ttl)
		c.ll.MoveToFront(el)
		return
	}
	el := c.ll.PushFront(&imageEntry{key: key, png: png, expires: time.Now().Add(c.ttl)})
	c.entries[key] = el
	for c.ll.Len() > c.max {
		oldest := c.ll.Back()
		if oldest == nil {
			break
		}
		c.ll.Remove(oldest)
		delete(c.entries, oldest.Value.(*imageEntry).key)
	}
}
