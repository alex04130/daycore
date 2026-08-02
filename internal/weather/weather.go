// Package weather selects and composes WeatherProvider implementations. Concrete
// providers live in subpackages (openmeteo/qweather/openweathermap/wttrin) and
// self-register via init(); New picks the configured primary and chains wttr.in
// as a free fallback, wrapped in a 30-minute cache. The Server depends only on
// domain.WeatherProvider (adapter pattern) — swapping providers is config-only.
package weather

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"daycore/internal/domain"
)

// Config is passed to a provider factory.
type Config struct {
	APIKey string
	HTTP   *http.Client
}

// Factory builds a WeatherProvider from config.
type Factory func(Config) domain.WeatherProvider

var (
	mu        sync.RWMutex
	factories = map[string]Factory{}
)

// Register adds a provider factory under a name (called from a provider init()).
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	factories[name] = f
}

// Providers lists the registered provider names (sorted), for diagnostics.
func Providers() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(factories))
	for n := range factories {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func build(name string, cfg Config) domain.WeatherProvider {
	mu.RLock()
	f, ok := factories[name]
	mu.RUnlock()
	if !ok {
		return nil
	}
	return f(cfg)
}

// Options configures New.
type Options struct {
	Provider          string // "open-meteo" (default) | "qweather" | "openweathermap" | "wttr"
	QWeatherKey       string
	OpenWeatherMapKey string
}

// New returns a WeatherProvider: the configured primary, chained to wttr.in as a
// free fallback, wrapped in a 30-minute cache. Provider packages must be
// (blank-)imported so they have registered.
func New(o Options) domain.WeatherProvider {
	httpc := &http.Client{Timeout: 8 * time.Second}
	name := o.Provider
	if name == "" {
		name = "open-meteo"
	}
	primary := build(name, Config{APIKey: keyFor(name, o), HTTP: httpc})
	if primary == nil {
		// Unknown provider or a keyed provider with no key → free default.
		primary = build("open-meteo", Config{HTTP: httpc})
	}
	fallback := build("wttr", Config{HTTP: httpc})
	return newCached(&chain{primary: primary, fallback: fallback}, 30*time.Minute)
}

func keyFor(name string, o Options) string {
	switch name {
	case "qweather":
		return o.QWeatherKey
	case "openweathermap":
		return o.OpenWeatherMapKey
	}
	return ""
}

// ─── chain: primary → fallback ───────────────────────────────────────────────

type chain struct {
	primary  domain.WeatherProvider
	fallback domain.WeatherProvider
}

func (c *chain) Name() string {
	if c.primary != nil {
		return c.primary.Name()
	}
	return "none"
}

func (c *chain) Lookup(ctx context.Context, q domain.WeatherQuery) (*domain.Forecast, error) {
	if c.primary != nil {
		if fc, err := c.primary.Lookup(ctx, q); err == nil {
			return fc, nil
		}
	}
	if c.fallback != nil {
		return c.fallback.Lookup(ctx, q)
	}
	return nil, errors.New("weather: no provider available")
}

// ─── cache wrapper (per provider|location|days|locale, 30 min) ───────────────

// cacheMaxEntries bounds the map. The key contains a location that arrives as
// free text from the model (toolGetWeather passes args.Location straight
// through), so without a bound an agent asking about many places grows this
// forever, each entry holding a multi-day forecast. It is a cache: dropping
// entries costs one extra upstream call, never a wrong answer.
const cacheMaxEntries = 512

type cached struct {
	inner domain.WeatherProvider
	ttl   time.Duration
	mu    sync.Mutex
	m     map[string]cacheEntry
}

type cacheEntry struct {
	fc  *domain.Forecast
	exp time.Time
}

func newCached(inner domain.WeatherProvider, ttl time.Duration) *cached {
	return &cached{inner: inner, ttl: ttl, m: map[string]cacheEntry{}}
}

func (c *cached) Name() string { return c.inner.Name() }

// cacheKey namespaces by the wrapped provider.
//
// Today that is nearly redundant — one provider is wrapped — but the moment a
// source becomes something the caller picks (the planned `source` parameter on
// get_weather), a shared key means one source serving another's answer: ask for
// qweather, receive a half-hour-old open-meteo forecast under its name. Keying
// on the provider costs one string concat and removes the whole class.
func (c *cached) cacheKey(q domain.WeatherQuery) string {
	return c.inner.Name() + "|" +
		strings.ToLower(strings.TrimSpace(q.Location)) + "|" +
		strconv.Itoa(q.Days) + "|" + q.Locale
}

func (c *cached) Lookup(ctx context.Context, q domain.WeatherQuery) (*domain.Forecast, error) {
	key := c.cacheKey(q)
	c.mu.Lock()
	if e, ok := c.m[key]; ok && time.Now().Before(e.exp) {
		c.mu.Unlock()
		return e.fc, nil
	}
	c.mu.Unlock()

	fc, err := c.inner.Lookup(ctx, q)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	c.mu.Lock()
	if len(c.m) >= cacheMaxEntries {
		c.evictLocked(now)
	}
	c.m[key] = cacheEntry{fc: fc, exp: now.Add(c.ttl)}
	c.mu.Unlock()
	return fc, nil
}

// evictLocked drops expired entries, and everything if that was not enough.
//
// Expired-first because those are free to lose. The wholesale drop after that is
// deliberate rather than an LRU: an LRU here would need per-entry access
// bookkeeping to protect a 30-minute cache of a call that takes one HTTP
// request, and a bound that is never actually enforced is the real hazard — this
// map previously had none at all and never removed an expired entry either.
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
