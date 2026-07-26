package wttrin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"daycore/internal/domain"
)

func TestLookup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"nearest_area":[{"areaName":[{"value":"Shanghai"}]}],"weather":[{"date":"2026-07-13","maxtempC":"31","mintempC":"22","hourly":[{"chanceofrain":"80","weatherDesc":[{"value":"Light rain"}]}]}]}`))
	}))
	t.Cleanup(srv.Close)
	p := New(nil)
	p.base = srv.URL

	f, err := p.Lookup(context.Background(), domain.WeatherQuery{Location: "Shanghai", Days: 1, Locale: "en-US"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Location != "Shanghai" || len(f.Days) != 1 {
		t.Fatalf("forecast = %+v", f)
	}
	d := f.Days[0]
	if d.TempMax != 31 || d.TempMin != 22 || d.PrecipProb != 80 || d.Text != "Light rain" {
		t.Errorf("day = %+v", d)
	}
}
