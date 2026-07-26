// Package qweather implements domain.WeatherProvider against QWeather (和风天气),
// which needs an API key. It geocodes the location to a QWeather location id,
// then fetches the 3-day daily forecast.
package qweather

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
	weather.Register("qweather", func(cfg weather.Config) domain.WeatherProvider {
		if cfg.APIKey == "" {
			return nil // no key → not usable; New() falls back to the free default
		}
		return New(cfg.HTTP, cfg.APIKey)
	})
}

// Provider fetches forecasts from QWeather.
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
		geoURL:      "https://geoapi.qweather.com/v2/city/lookup",
		forecastURL: "https://devapi.qweather.com/v7/weather/3d",
	}
}

func (p *Provider) Name() string { return "qweather" }

func (p *Provider) Lookup(ctx context.Context, q domain.WeatherQuery) (*domain.Forecast, error) {
	location := strings.TrimSpace(q.Location)
	if location == "" {
		return nil, fmt.Errorf("weather: empty location")
	}
	days := q.Days
	if days <= 0 {
		days = 2
	}
	if days > 3 {
		days = 3
	}
	lang := "en"
	if !strings.HasPrefix(strings.ToLower(q.Locale), "en") {
		lang = "zh"
	}

	id, name, err := p.geocode(ctx, location, lang)
	if err != nil {
		return nil, err
	}
	return p.forecast(ctx, id, name, days, lang)
}

func (p *Provider) geocode(ctx context.Context, location, lang string) (id, name string, err error) {
	u := fmt.Sprintf("%s?location=%s&key=%s&lang=%s&number=1", p.geoURL, url.QueryEscape(location), url.QueryEscape(p.key), lang)
	data, err := p.get(ctx, u)
	if err != nil {
		return "", "", err
	}
	var gr struct {
		Code     string `json:"code"`
		Location []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"location"`
	}
	if err := json.Unmarshal(data, &gr); err != nil {
		return "", "", fmt.Errorf("weather: qweather geocode decode: %w", err)
	}
	if gr.Code != "200" || len(gr.Location) == 0 {
		return "", "", fmt.Errorf("weather: qweather location %q not found (code %s)", location, gr.Code)
	}
	return gr.Location[0].ID, gr.Location[0].Name, nil
}

func (p *Provider) forecast(ctx context.Context, id, name string, days int, lang string) (*domain.Forecast, error) {
	u := fmt.Sprintf("%s?location=%s&key=%s&lang=%s", p.forecastURL, url.QueryEscape(id), url.QueryEscape(p.key), lang)
	data, err := p.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var fr struct {
		Code  string `json:"code"`
		Daily []struct {
			FxDate  string `json:"fxDate"`
			TempMax string `json:"tempMax"`
			TempMin string `json:"tempMin"`
			TextDay string `json:"textDay"`
			Pop     string `json:"pop"` // precip probability (%), may be empty
		} `json:"daily"`
	}
	if err := json.Unmarshal(data, &fr); err != nil {
		return nil, fmt.Errorf("weather: qweather forecast decode: %w", err)
	}
	if fr.Code != "200" {
		return nil, fmt.Errorf("weather: qweather forecast failed (code %s)", fr.Code)
	}
	fc := &domain.Forecast{Location: name}
	for i, d := range fr.Daily {
		if i >= days {
			break
		}
		fc.Days = append(fc.Days, domain.Day{
			Date: d.FxDate, Code: -1, Text: d.TextDay,
			TempMax: atof(d.TempMax), TempMin: atof(d.TempMin), PrecipProb: atoi(d.Pop),
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
		return nil, fmt.Errorf("weather: qweather http %d", resp.StatusCode)
	}
	return data, nil
}

func atof(s string) float64 { v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64); return v }
func atoi(s string) int     { v, _ := strconv.Atoi(strings.TrimSpace(s)); return v }
