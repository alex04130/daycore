package weather

import (
	"sync"
	"time"

	"daycore/internal/domain"
)

// A shared forecast cache, keyed by config entry id plus query.
//
// # Why it moved out of the provider wrapper
//
// It used to be a decorator: newCached(provider) returned something that also
// satisfied domain.WeatherProvider. That works for exactly one provider. With a
// set, one wrapper per source means N independent caches each with its own
// bound — the bound stops meaning what it says, and the eviction that protects
// this process from a model asking about a thousand cities has to hold N times
// as many entries before it fires.
//
// One cache, one bound, and the id in the key.
//
// # Boundary: nothing here observes health
//
// A hit returns without touching the source. That is the reason the health
// accounting lives in Sources.Lookup, outside this: a cached answer proves
// nothing about whether the source still responds, and treating it as proof
// would keep a dead source marked healthy for the whole TTL — the half hour in
// which somebody is trying to work out what broke.
type cached struct {
	ttl time.Duration
	mu  sync.Mutex
	m   map[string]cacheEntry
}

type cacheEntry struct {
	fc  *domain.Forecast
	exp time.Time
}

// cacheMaxEntries bounds the map.
//
// The key contains a location that arrives as free text from the model — the
// weather tool passes the argument straight through — so without a bound, an
// agent asking about many places grows this forever, each entry holding a
// multi-day forecast. It is a cache: dropping entries costs one extra upstream
// call, never a wrong answer.
const cacheMaxEntries = 512

func newCached(ttl time.Duration) *cached {
	return &cached{ttl: ttl, m: map[string]cacheEntry{}}
}

func (c *cached) get(key string) (*domain.Forecast, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || !time.Now().Before(e.exp) {
		return nil, false
	}
	return e.fc, true
}

func (c *cached) put(key string, fc *domain.Forecast) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= cacheMaxEntries {
		c.evictLocked(now)
	}
	c.m[key] = cacheEntry{fc: fc, exp: now.Add(c.ttl)}
}

// evictLocked drops expired entries, and everything if that was not enough.
//
// Expired-first because those are free to lose. The wholesale drop after that
// is deliberate rather than an LRU: an LRU would need per-entry access
// bookkeeping to protect a thirty-minute cache of a single HTTP request, and a
// bound that is never actually enforced is the real hazard — this map once had
// no bound at all and never removed an expired entry either.
func (c *cached) evictLocked(now time.Time) {
	for k, e := range c.m {
		if !now.Before(e.exp) {
			delete(c.m, k)
		}
	}
	if len(c.m) >= cacheMaxEntries {
		c.m = make(map[string]cacheEntry, cacheMaxEntries/2)
	}
}
