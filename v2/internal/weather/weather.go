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

// ─── cache wrapper (per location|days|locale, 30 min) ────────────────────────

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

func (c *cached) Lookup(ctx context.Context, q domain.WeatherQuery) (*domain.Forecast, error) {
	key := strings.ToLower(strings.TrimSpace(q.Location)) + "|" + strconv.Itoa(q.Days) + "|" + q.Locale
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
	c.mu.Lock()
	c.m[key] = cacheEntry{fc: fc, exp: time.Now().Add(c.ttl)}
	c.mu.Unlock()
	return fc, nil
}
