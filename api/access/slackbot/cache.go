package main

import (
	"context"
	"sync"
	"time"

	"github.com/gravitational/teleport/api/access"
	log "github.com/sirupsen/logrus"
)

type entry struct {
	req access.Request
	exp uint
}

// RequestCache holds pending request data.
type RequestCache struct {
	sync.Mutex
	entries map[string]entry
	tainted bool
	index   uint
	next    uint
}

func NewRequestCache(ctx context.Context) *RequestCache {
	cache := &RequestCache{
		entries: make(map[string]entry),
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				cache.tick()
			case <-ctx.Done():
				cache.taint()
				return
			}
		}
	}()
	return cache
}

func (c *RequestCache) Get(reqID string) (req access.Request, ok bool) {
	c.Lock()
	defer c.Unlock()
	if c.tainted {
		panic("use of tainted cache")
	}
	entry, ok := c.entries[reqID]
	if !ok {
		return
	}
	return entry.req, true
}

func (c *RequestCache) Put(req access.Request) {
	const TTL = 60 * 60
	c.Lock()
	defer c.Unlock()
	if c.tainted {
		panic("use of tainted cache")
	}
	exp := c.index + TTL
	c.entries[req.ID] = entry{
		req: req,
		exp: exp,
	}
	if c.next == 0 || c.next > exp {
		c.next = exp
	}
}

func (c *RequestCache) Pop(reqID string) (req access.Request, ok bool) {
	c.Lock()
	defer c.Unlock()
	if c.tainted {
		panic("use of tainted cache")
	}
	e, ok := c.entries[reqID]
	if !ok {
		return
	}
	delete(c.entries, reqID)
	return e.req, true
}

func (c *RequestCache) tick() int {
	c.Lock()
	defer c.Unlock()
	c.index++
	if c.index < c.next {
		return len(c.entries)
	}
	for reqID, entry := range c.entries {
		if entry.exp < c.index {
			log.Debugf("removing expired cache entry %s...", reqID)
			delete(c.entries, reqID)
			continue
		}
		if entry.exp < c.next {
			c.next = entry.exp
		}
	}
	return len(c.entries)
}

func (c *RequestCache) taint() {
	c.Lock()
	defer c.Unlock()
	c.entries = nil
	c.tainted = true
}
