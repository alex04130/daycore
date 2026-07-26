package weather

import (
	"context"
	"errors"
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
