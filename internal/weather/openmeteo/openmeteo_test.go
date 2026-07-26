package openmeteo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"daycore/internal/domain"
)

func newTestProvider(t *testing.T) *Provider {
	t.Helper()
	geo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") == "不存在的地方" {
			_, _ = w.Write([]byte(`{"results":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"name":"上海","latitude":31.22,"longitude":121.46}]}`))
	}))
	t.Cleanup(geo.Close)
	fc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"daily":{"time":["2026-07-13","2026-07-14"],"weather_code":[0,61],"temperature_2m_max":[31.2,28.4],"temperature_2m_min":[22.1,21.0],"precipitation_probability_max":[5,80]}}`))
	}))
	t.Cleanup(fc.Close)
	p := New(nil)
	p.geoURL, p.forecastURL = geo.URL, fc.URL
	return p
}

func TestLookup(t *testing.T) {
	p := newTestProvider(t)
	f, err := p.Lookup(context.Background(), domain.WeatherQuery{Location: "上海", Days: 2, Locale: "zh-CN"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Location != "上海" || len(f.Days) != 2 {
		t.Fatalf("forecast = %+v", f)
	}
	d := f.Days[1]
	if d.Date != "2026-07-14" || d.Code != 61 || d.PrecipProb != 80 || d.TempMax != 28.4 {
		t.Errorf("day = %+v", d)
	}
	if sum := f.Summary("zh-CN"); !strings.Contains(sum, "上海") || !strings.Contains(sum, "80") {
		t.Errorf("summary = %q", sum)
	}
}

func TestLookupUnknown(t *testing.T) {
	p := newTestProvider(t)
	if _, err := p.Lookup(context.Background(), domain.WeatherQuery{Location: "不存在的地方", Days: 1}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestWMOText(t *testing.T) {
	for _, code := range []int{0, 2, 45, 61, 75, 95} {
		if WMOText(code, "zh-CN") == "" || WMOText(code, "en-US") == "" {
			t.Errorf("code %d has empty text", code)
		}
	}
	if WMOText(0, "en-US") != "Clear" {
		t.Error("expected 'Clear' for code 0 en")
	}
}
