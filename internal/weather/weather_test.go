package weather

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"daycore/internal/domain"
)

type fakeProvider struct {
	name  string
	fc    *domain.Forecast
	err   error
	calls int
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Lookup(ctx context.Context, q domain.WeatherQuery) (*domain.Forecast, error) {
	f.calls++
	return f.fc, f.err
}

// The chain must fall through to wttr.in (or any fallback) when the primary errors.
func TestChainFallsBack(t *testing.T) {
	primary := &fakeProvider{name: "p", err: errors.New("down")}
	fallback := &fakeProvider{name: "f", fc: &domain.Forecast{Location: "X"}}
	c := &chain{primary: primary, fallback: fallback}

	fc, err := c.Lookup(context.Background(), domain.WeatherQuery{})
	if err != nil || fc.Location != "X" {
		t.Fatalf("fc=%+v err=%v", fc, err)
	}
	if fallback.calls != 1 {
		t.Fatalf("fallback calls = %d, want 1", fallback.calls)
	}
}

// A repeated lookup within the TTL must hit the cache, not the provider.
func TestCachedDedups(t *testing.T) {
	inner := &fakeProvider{name: "p", fc: &domain.Forecast{Location: "Y"}}
	c := newCached(inner, time.Hour)
	q := domain.WeatherQuery{Location: "Y", Days: 2, Locale: "zh-CN"}

	if _, err := c.Lookup(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Lookup(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if inner.calls != 1 {
		t.Fatalf("inner calls = %d, want 1 (second lookup should be cached)", inner.calls)
	}
}

// The cache must be bounded and must namespace by provider.
//
// Both were found by an adversarial design review rather than by use, and they
// fail in opposite ways: the missing bound grows silently until something else
// notices, and the missing namespace would have returned one source's forecast
// under another source's name the moment `source` became a tool parameter.
func TestCacheIsBoundedAndPerProvider(t *testing.T) {
	t.Run("bounded by entry count", func(t *testing.T) {
		c := newCached(countingProvider{name: "p"}, time.Hour)
		// The location is model-supplied free text, so this is the shape of the
		// real growth path, not a synthetic one.
		for i := 0; i < cacheMaxEntries*3; i++ {
			if _, err := c.Lookup(context.Background(), domain.WeatherQuery{
				Location: "city-" + strconv.Itoa(i), Days: 2, Locale: "zh-CN",
			}); err != nil {
				t.Fatal(err)
			}
		}
		c.mu.Lock()
		n := len(c.m)
		c.mu.Unlock()
		if n >= cacheMaxEntries*2 {
			t.Errorf("cache holds %d entries after %d distinct locations — it is unbounded",
				n, cacheMaxEntries*3)
		}
	})

	t.Run("expired entries are reclaimed", func(t *testing.T) {
		c := newCached(countingProvider{name: "p"}, time.Nanosecond)
		for i := 0; i < cacheMaxEntries+10; i++ {
			if _, err := c.Lookup(context.Background(), domain.WeatherQuery{
				Location: "city-" + strconv.Itoa(i), Days: 1,
			}); err != nil {
				t.Fatal(err)
			}
		}
		c.mu.Lock()
		n := len(c.m)
		c.mu.Unlock()
		// Everything expires instantly, so a sweep should keep this tiny.
		if n > cacheMaxEntries {
			t.Errorf("%d entries survived with a 1ns TTL — expired entries are never reclaimed", n)
		}
	})

	t.Run("two providers do not share a cache slot", func(t *testing.T) {
		a := newCached(countingProvider{name: "alpha", temp: 1}, time.Hour)
		b := newCached(countingProvider{name: "beta", temp: 2}, time.Hour)
		q := domain.WeatherQuery{Location: "同一个地方", Days: 2, Locale: "zh-CN"}

		fa, err := a.Lookup(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		fb, err := b.Lookup(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		if fa.Days[0].TempMax == fb.Days[0].TempMax {
			t.Error("two providers returned the same forecast for the same query — the cache key ignores the provider")
		}
		// And the keys themselves must differ, which is what makes it safe when
		// one cache instance is shared.
		if a.cacheKey(q) == b.cacheKey(q) {
			t.Errorf("cache keys collide across providers: %q", a.cacheKey(q))
		}
	})
}

// countingProvider returns a distinguishable forecast per provider name.
type countingProvider struct {
	name string
	temp float64
}

func (p countingProvider) Name() string { return p.name }
func (p countingProvider) Lookup(context.Context, domain.WeatherQuery) (*domain.Forecast, error) {
	return &domain.Forecast{Days: []domain.Day{{TempMax: p.temp}}}, nil
}
