package main

import (
	"container/list"
	"context"
	"sync"
	"time"
)

// feedCache is a small TTL cache of fetched feed results, keyed by feed URL.
// It keeps crawler-triggered requests (page + OG image) fast after the first
// fetch warms the entry.
type feedCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]feedCacheEntry
}

type feedCacheEntry struct {
	res     *FetchResult
	expires time.Time
}

func newFeedCache(ttl time.Duration) *feedCache {
	return &feedCache{ttl: ttl, entries: map[string]feedCacheEntry{}}
}

// fetch returns a cached result when fresh, otherwise fetches and stores it.
// Concurrent misses for the same URL may each fetch; the last write wins.
func (c *feedCache) fetch(ctx context.Context, feedURL string) (*FetchResult, error) {
	c.mu.Lock()
	if e, ok := c.entries[feedURL]; ok && time.Now().Before(e.expires) {
		c.mu.Unlock()
		return e.res, nil
	}
	c.mu.Unlock()

	res, err := fetchFeed(ctx, feedURL, "", "")
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.entries[feedURL] = feedCacheEntry{res: res, expires: time.Now().Add(c.ttl)}
	c.mu.Unlock()
	return res, nil
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
