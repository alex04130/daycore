package weather

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"daycore/internal/adapters"
	"daycore/internal/domain"
)

// fake is a source implementation whose answers and failures a test controls.
type fake struct {
	name  string
	temp  float64
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fake) Name() string { return f.name }
func (f *fake) Lookup(ctx context.Context, q domain.WeatherQuery) (*domain.Forecast, error) {
	f.mu.Lock()
	f.calls++
	err := f.err
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return &domain.Forecast{Location: q.Location, Days: []domain.Day{{Date: "2026-08-09", TempMax: f.temp}}}, nil
}
func (f *fake) count() int { f.mu.Lock(); defer f.mu.Unlock(); return f.calls }

// build a Sources directly from fakes, bypassing the factory registry: these
// tests are about the SET, not about which providers exist.
func setOf(t *testing.T, fakes map[string]*fake) *Sources {
	t.Helper()
	s := &Sources{byID: map[string]*entry{}, cache: newCached(time.Hour)}
	for id, f := range fakes {
		src := adapters.Resolve(adapters.KindWeather,
			adapters.Entry{ID: id, Format: adapters.FormatBuiltin, Impl: f.name}, nil, adapters.NewHealth())
		s.byID[id] = &entry{src: src, prov: f}
		s.order = append(s.order, id)
	}
	sortStrings(s.order)
	return s
}

func sortStrings(v []string) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

// Two entries using the same implementation must not share cache slots.
//
// The key used to start with the IMPLEMENTATION's Name() — a hardcoded string
// like "qweather". With one source wrapped that was nearly harmless. With a
// set, two entries both on qweather (different keys, different regions, which
// is exactly why somebody configures two) would collide: ask the Beijing entry,
// receive the other one's half-hour-old answer under its own name.
//
// Note the fakes deliberately share a Name(): a test using two different names
// would pass against the old key too, and prove nothing.
func TestTwoEntriesOnOneImplementationDoNotShareCacheSlots(t *testing.T) {
	a := &fake{name: "qweather", temp: 10}
	b := &fake{name: "qweather", temp: 30}
	s := setOf(t, map[string]*fake{"qw-beijing": a, "qw-tokyo": b})
	q := domain.WeatherQuery{Location: "同一个地方", Days: 2, Locale: "zh-CN"}

	fa, err := s.Lookup(context.Background(), "qw-beijing", q)
	if err != nil {
		t.Fatal(err)
	}
	fb, err := s.Lookup(context.Background(), "qw-tokyo", q)
	if err != nil {
		t.Fatal(err)
	}
	if fa.Days[0].TempMax == fb.Days[0].TempMax {
		t.Error("one entry served the other's cached answer — the key ignores the config id")
	}
	if cacheKey("qw-beijing", q) == cacheKey("qw-tokyo", q) {
		t.Error("cache keys collide across entries on the same implementation")
	}
}

func TestCacheAvoidsASecondCall(t *testing.T) {
	f := &fake{name: "open-meteo", temp: 20}
	s := setOf(t, map[string]*fake{"om": f})
	q := domain.WeatherQuery{Location: "北京", Days: 2, Locale: "zh-CN"}
	for i := 0; i < 3; i++ {
		if _, err := s.Lookup(context.Background(), "om", q); err != nil {
			t.Fatal(err)
		}
	}
	if f.count() != 1 {
		t.Errorf("upstream called %d times for one cached query", f.count())
	}
}

// A cache hit must not count as a successful call.
//
// Counting it would hold a source "up" for the whole 30-minute TTL after it
// died — precisely the window in which somebody is trying to find out what
// broke. This is why the health wrapper sits outside the cache.
func TestACacheHitIsNotEvidenceOfHealth(t *testing.T) {
	f := &fake{name: "open-meteo", temp: 20}
	s := setOf(t, map[string]*fake{"om": f})
	q := domain.WeatherQuery{Location: "北京", Days: 2, Locale: "zh-CN"}
	if _, err := s.Lookup(context.Background(), "om", q); err != nil {
		t.Fatal(err)
	}
	before := s.Source("om").Health.Snapshot().LastChecked

	// Served from cache: no call, so nothing observed.
	if _, err := s.Lookup(context.Background(), "om", q); err != nil {
		t.Fatal(err)
	}
	if got := s.Source("om").Health.Snapshot().LastChecked; !got.Equal(before) {
		t.Error("a cache hit was recorded as a health observation")
	}
}

// A named source that is down is an error, not a silent substitution. Answering
// with another source's data under the same question is the cross-source chain
// this batch deleted, and the model cannot tell it happened.
func TestANamedSourceIsNeverSubstituted(t *testing.T) {
	good := &fake{name: "open-meteo", temp: 20}
	bad := &fake{name: "qweather", temp: 99, err: errors.New("upstream down")}
	s := setOf(t, map[string]*fake{"good": good, "bad": bad})

	for i := 0; i < adapters.FlipAfter; i++ {
		s.Source("bad").Health.Observe(errors.New("down"), time.Now())
	}
	_, err := s.Lookup(context.Background(), "bad",
		domain.WeatherQuery{Location: "北京", Days: 1, Locale: "zh-CN"})
	if err == nil {
		t.Fatal("a named unavailable source silently answered from elsewhere")
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Errorf("the error does not name the source that was asked for: %v", err)
	}
	if good.count() != 0 {
		t.Error("another source was called for a question about a named one")
	}
}

// An unknown id is refused with the list of real ones, so the model can correct
// itself in one turn rather than guessing.
func TestAnUnknownSourceNamesTheRealOnes(t *testing.T) {
	s := setOf(t, map[string]*fake{"open-meteo": {name: "open-meteo"}})
	_, err := s.Lookup(context.Background(), "not-a-source",
		domain.WeatherQuery{Location: "x", Days: 1})
	if err == nil {
		t.Fatal("an unknown source was accepted")
	}
	if !strings.Contains(err.Error(), "open-meteo") {
		t.Errorf("the error does not list what is available: %v", err)
	}
}

// A nameless lookup prefers the configured default and otherwise walks the
// sorted order — never a random map iteration.
func TestNamelessLookupPrefersTheDefaultThenOrder(t *testing.T) {
	a := &fake{name: "a", temp: 1}
	z := &fake{name: "z", temp: 26}
	s := setOf(t, map[string]*fake{"alpha": a, "zulu": z})
	q := domain.WeatherQuery{Location: "x", Days: 1}

	if _, err := s.Lookup(context.Background(), "", q); err != nil {
		t.Fatal(err)
	}
	if a.count() != 1 || z.count() != 0 {
		t.Errorf("nameless lookup did not take the first in order (alpha=%d zulu=%d)", a.count(), z.count())
	}

	s.SetDefault("zulu")
	if _, err := s.Lookup(context.Background(), "", domain.WeatherQuery{Location: "y", Days: 1}); err != nil {
		t.Fatal(err)
	}
	if z.count() != 1 {
		t.Error("WEATHER_PROVIDER did not promote its source to the front")
	}

	// A default that is down falls through to the order rather than failing —
	// this is a preference, not a demand.
	for i := 0; i < adapters.FlipAfter; i++ {
		s.Source("zulu").Health.Observe(errors.New("down"), time.Now())
	}
	if _, err := s.Lookup(context.Background(), "", domain.WeatherQuery{Location: "z", Days: 1}); err != nil {
		t.Fatalf("a down default made a nameless lookup fail: %v", err)
	}
}

// IDs is the tool's enum, so it must be sorted and must drop what cannot be
// used. Both properties are about prompt-cache stability, not tidiness.
func TestIDsIsSortedAndDropsUnusable(t *testing.T) {
	s := setOf(t, map[string]*fake{"zulu": {name: "z"}, "alpha": {name: "a"}, "mid": {name: "m"}})
	if got := strings.Join(s.IDs(), ","); got != "alpha,mid,zulu" {
		t.Errorf("IDs = %s, want sorted", got)
	}
	for i := 0; i < adapters.FlipAfter; i++ {
		s.Source("mid").Health.Observe(errors.New("down"), time.Now())
	}
	if got := strings.Join(s.IDs(), ","); got != "alpha,zulu" {
		t.Errorf("IDs = %s after mid went down", got)
	}
	// Repeated calls must be byte-identical: the tool band renders before the
	// system prompt, and a band that differs between rounds rewrites the
	// ephemeral breakpoint and every layer behind it.
	if strings.Join(s.IDs(), ",") != strings.Join(s.IDs(), ",") {
		t.Error("IDs is not a pure function of state")
	}
}

// With nothing usable the tool is not offered at all, rather than offered and
// always failing — a tool that cannot succeed costs a round trip and leaves its
// error in the conversation.
func TestAvailableIsFalseWhenEverythingIsDown(t *testing.T) {
	s := setOf(t, map[string]*fake{"only": {name: "o"}})
	if !s.Available() {
		t.Fatal("a healthy source reported unavailable")
	}
	for i := 0; i < adapters.FlipAfter; i++ {
		s.Source("only").Health.Observe(errors.New("down"), time.Now())
	}
	if s.Available() {
		t.Error("the capability is still offered with every source down")
	}
}

// A failing call is observed, so repeated failures eventually take the source
// out of the enum on their own.
func TestFailuresAreObserved(t *testing.T) {
	f := &fake{name: "x", err: errors.New("boom")}
	s := setOf(t, map[string]*fake{"x": f})
	for i := 0; i < adapters.FlipAfter; i++ {
		_, _ = s.Lookup(context.Background(), "x", domain.WeatherQuery{Location: "a", Days: 1, Locale: "en-US"})
	}
	if s.Source("x").Health.Up() {
		t.Error("repeated real failures did not mark the source down")
	}
}

// The bound is enforced, and expired entries are reclaimed before it fires.
//
// Kept from the old per-provider cache test, reshaped for the shared one. The
// key contains a location that arrives as free text from the model, so an
// unbounded map is an agent asking about a thousand cities away from filling
// this process's memory with multi-day forecasts.
func TestCacheIsBounded(t *testing.T) {
	c := newCached(time.Nanosecond)
	for i := 0; i < cacheMaxEntries*2; i++ {
		c.put(cacheKey("src", domain.WeatherQuery{Location: strconv.Itoa(i), Days: 1}), &domain.Forecast{})
	}
	c.mu.Lock()
	n := len(c.m)
	c.mu.Unlock()
	if n > cacheMaxEntries {
		t.Errorf("%d entries survived a 1ns TTL — expired entries are never reclaimed", n)
	}

	// And an unexpired cache still honours the ceiling.
	c2 := newCached(time.Hour)
	for i := 0; i < cacheMaxEntries*2; i++ {
		c2.put(cacheKey("src", domain.WeatherQuery{Location: strconv.Itoa(i), Days: 1}), &domain.Forecast{})
	}
	c2.mu.Lock()
	n2 := len(c2.m)
	c2.mu.Unlock()
	if n2 > cacheMaxEntries {
		t.Errorf("%d live entries — the bound is not enforced when nothing has expired", n2)
	}
}
