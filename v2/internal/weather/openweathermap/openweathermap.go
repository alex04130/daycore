// Package openweathermap implements domain.WeatherProvider against OpenWeatherMap
// (needs an API key): geocode the location, then aggregate the 3-hourly 5-day
// forecast into daily min/max/condition.
package openweathermap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"daycore/internal/domain"
	"daycore/internal/weather"
)

func init() {
	weather.Register("openweathermap", func(cfg weather.Config) domain.WeatherProvider {
		if cfg.APIKey == "" {
			return nil
		}
		return New(cfg.HTTP, cfg.APIKey)
	})
}

// Provider fetches forecasts from OpenWeatherMap.
type Provider struct {
	http        *http.Client
	key         string
	geoURL      string
	forecastURL string
}

// New builds a Provider with the given API key.
func New(httpc *http.Client, key string) *Provider {
	if httpc == nil {
		httpc = &http.Client{Timeout: 8 * time.Second}
	}
	return &Provider{
		http:        httpc,
		key:         key,
		geoURL:      "https://api.openweathermap.org/geo/1.0/direct",
		forecastURL: "https://api.openweathermap.org/data/2.5/forecast",
	}
}

func (p *Provider) Name() string { return "openweathermap" }

func (p *Provider) Lookup(ctx context.Context, q domain.WeatherQuery) (*domain.Forecast, error) {
	location := strings.TrimSpace(q.Location)
	if location == "" {
		return nil, fmt.Errorf("weather: empty location")
	}
	days := q.Days
	if days <= 0 {
		days = 2
	}
	if days > 5 {
		days = 5
	}
	lang := "en"
	if !strings.HasPrefix(strings.ToLower(q.Locale), "en") {
		lang = "zh_cn"
	}

	lat, lon, name, err := p.geocode(ctx, location)
	if err != nil {
		return nil, err
	}
	return p.forecast(ctx, lat, lon, name, days, lang)
}

func (p *Provider) geocode(ctx context.Context, location string) (lat, lon float64, name string, err error) {
	u := fmt.Sprintf("%s?q=%s&limit=1&appid=%s", p.geoURL, url.QueryEscape(location), url.QueryEscape(p.key))
	data, err := p.get(ctx, u)
	if err != nil {
		return 0, 0, "", err
	}
	var gr []struct {
		Name string  `json:"name"`
		Lat  float64 `json:"lat"`
		Lon  float64 `json:"lon"`
	}
	if err := json.Unmarshal(data, &gr); err != nil {
		return 0, 0, "", fmt.Errorf("weather: owm geocode decode: %w", err)
	}
	if len(gr) == 0 {
		return 0, 0, "", fmt.Errorf("weather: owm location %q not found", location)
	}
	return gr[0].Lat, gr[0].Lon, gr[0].Name, nil
}

func (p *Provider) forecast(ctx context.Context, lat, lon float64, name string, days int, lang string) (*domain.Forecast, error) {
	u := fmt.Sprintf("%s?lat=%s&lon=%s&appid=%s&units=metric&lang=%s",
		p.forecastURL, strconv.FormatFloat(lat, 'f', -1, 64), strconv.FormatFloat(lon, 'f', -1, 64), url.QueryEscape(p.key), lang)
	data, err := p.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var fr struct {
		List []struct {
			DtTxt string `json:"dt_txt"` // "2026-07-13 12:00:00"
			Main  struct {
				TempMin float64 `json:"temp_min"`
				TempMax float64 `json:"temp_max"`
			} `json:"main"`
			Weather []struct {
				Description string `json:"description"`
			} `json:"weather"`
			Pop float64 `json:"pop"` // 0..1
		} `json:"list"`
	}
	if err := json.Unmarshal(data, &fr); err != nil {
		return nil, fmt.Errorf("weather: owm forecast decode: %w", err)
	}

	// Aggregate 3-hourly entries into per-day min/max, midday condition, max pop.
	type agg struct {
		min, max   float64
		set        bool
		text       string
		bestPopStr float64
		middayDiff int
	}
	byDate := map[string]*agg{}
	var order []string
	for _, e := range fr.List {
		if len(e.DtTxt) < 10 {
			continue
		}
		date := e.DtTxt[:10]
		a := byDate[date]
		if a == nil {
			a = &agg{min: e.Main.TempMin, max: e.Main.TempMax, middayDiff: 1 << 30}
			byDate[date] = a
			order = append(order, date)
		}
		if e.Main.TempMin < a.min {
			a.min = e.Main.TempMin
		}
		if e.Main.TempMax > a.max {
			a.max = e.Main.TempMax
		}
		if e.Pop > a.bestPopStr {
			a.bestPopStr = e.Pop
		}
		// pick the condition text closest to noon.
		if hh := hourOf(e.DtTxt); abs(hh-12) < a.middayDiff && len(e.Weather) > 0 {
			a.middayDiff = abs(hh - 12)
			a.text = e.Weather[0].Description
		}
		a.set = true
	}

	fc := &domain.Forecast{Location: name}
	for i, date := range order {
		if i >= days {
			break
		}
		a := byDate[date]
		fc.Days = append(fc.Days, domain.Day{
			Date: date, Code: -1, Text: a.text,
			TempMin: a.min, TempMax: a.max, PrecipProb: int(a.bestPopStr*100 + 0.5),
		})
	}
	return fc, nil
}

func (p *Provider) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("weather: owm http %d", resp.StatusCode)
	}
	return data, nil
}

func hourOf(dtTxt string) int {
	if len(dtTxt) < 13 {
		return 0
	}
	h, _ := strconv.Atoi(dtTxt[11:13])
	return h
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
