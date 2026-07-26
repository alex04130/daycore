package openweathermap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"daycore/internal/domain"
)

func TestLookup(t *testing.T) {
	geo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"Shanghai","lat":31.2,"lon":121.4}]`))
	}))
	t.Cleanup(geo.Close)
	fc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// two 3-hourly slots on the same day: min/max should aggregate, noon wins the text.
		_, _ = w.Write([]byte(`{"list":[
			{"dt_txt":"2026-07-13 09:00:00","main":{"temp_min":22,"temp_max":28},"weather":[{"description":"clouds"}],"pop":0.2},
			{"dt_txt":"2026-07-13 12:00:00","main":{"temp_min":24,"temp_max":31},"weather":[{"description":"light rain"}],"pop":0.4}
		]}`))
	}))
	t.Cleanup(fc.Close)

	p := New(nil, "key")
	p.geoURL, p.forecastURL = geo.URL, fc.URL

	f, err := p.Lookup(context.Background(), domain.WeatherQuery{Location: "Shanghai", Days: 1, Locale: "en-US"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Location != "Shanghai" || len(f.Days) != 1 {
		t.Fatalf("forecast = %+v", f)
	}
	d := f.Days[0]
	if d.TempMax != 31 || d.TempMin != 22 || d.PrecipProb != 40 || d.Text != "light rain" {
		t.Errorf("day = %+v (want max31 min22 pop40 'light rain')", d)
	}
}
