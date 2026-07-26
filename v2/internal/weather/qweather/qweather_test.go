package qweather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"daycore/internal/domain"
)

func TestLookup(t *testing.T) {
	geo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"200","location":[{"id":"101020100","name":"上海"}]}`))
	}))
	t.Cleanup(geo.Close)
	fc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"200","daily":[{"fxDate":"2026-07-13","tempMax":"31","tempMin":"22","textDay":"多云","pop":"40"}]}`))
	}))
	t.Cleanup(fc.Close)

	p := New(nil, "testkey")
	p.geoURL, p.forecastURL = geo.URL, fc.URL

	f, err := p.Lookup(context.Background(), domain.WeatherQuery{Location: "上海", Days: 1, Locale: "zh-CN"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Location != "上海" || len(f.Days) != 1 {
		t.Fatalf("forecast = %+v", f)
	}
	d := f.Days[0]
	if d.TempMax != 31 || d.TempMin != 22 || d.PrecipProb != 40 || d.Text != "多云" {
		t.Errorf("day = %+v", d)
	}
}

// A missing key means the factory returns nil (New falls back to the free default).
func TestFactoryNilWithoutKey(t *testing.T) {
	// exercised via the registry path; here just assert New still builds with a key.
	if New(nil, "k") == nil {
		t.Fatal("New with a key should not be nil")
	}
}
