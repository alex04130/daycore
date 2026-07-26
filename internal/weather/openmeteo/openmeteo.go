// Package openmeteo implements domain.WeatherProvider against the free Open-Meteo
// API (no API key): geocode the location name, then fetch the daily forecast.
package openmeteo

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

const (
	defaultGeoURL      = "https://geocoding-api.open-meteo.com/v1/search"
	defaultForecastURL = "https://api.open-meteo.com/v1/forecast"
)

func init() {
	weather.Register("open-meteo", func(cfg weather.Config) domain.WeatherProvider {
		return New(cfg.HTTP)
	})
}

// Provider fetches forecasts from Open-Meteo.
type Provider struct {
	http        *http.Client
	geoURL      string
	forecastURL string
}

// New builds a Provider. A nil client gets an 8s-timeout default.
func New(httpc *http.Client) *Provider {
	if httpc == nil {
		httpc = &http.Client{Timeout: 8 * time.Second}
	}
	return &Provider{http: httpc, geoURL: defaultGeoURL, forecastURL: defaultForecastURL}
}

func (p *Provider) Name() string { return "open-meteo" }

func (p *Provider) Lookup(ctx context.Context, q domain.WeatherQuery) (*domain.Forecast, error) {
	location := strings.TrimSpace(q.Location)
	if location == "" {
		return nil, fmt.Errorf("weather: empty location")
	}
	days := q.Days
	switch {
	case days <= 0:
		days = 2
	case days > 7:
		days = 7
	}
	lat, lon, name, err := p.geocode(ctx, location)
	if err != nil {
		return nil, err
	}
	fc, err := p.forecast(ctx, lat, lon, days, q.Locale)
	if err != nil {
		return nil, err
	}
	fc.Location = name
	return fc, nil
}

func (p *Provider) geocode(ctx context.Context, location string) (lat, lon float64, name string, err error) {
	q := url.Values{"name": {location}, "count": {"1"}, "language": {"zh"}}
	data, err := p.get(ctx, p.geoURL+"?"+q.Encode())
	if err != nil {
		return 0, 0, "", err
	}
	var gr struct {
		Results []struct {
			Name      string  `json:"name"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &gr); err != nil {
		return 0, 0, "", fmt.Errorf("weather: geocode decode: %w", err)
	}
	if len(gr.Results) == 0 {
		return 0, 0, "", fmt.Errorf("weather: location %q not found", location)
	}
	r := gr.Results[0]
	return r.Latitude, r.Longitude, r.Name, nil
}

func (p *Provider) forecast(ctx context.Context, lat, lon float64, days int, locale string) (*domain.Forecast, error) {
	q := url.Values{
		"latitude":      {strconv.FormatFloat(lat, 'f', -1, 64)},
		"longitude":     {strconv.FormatFloat(lon, 'f', -1, 64)},
		"daily":         {"weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max"},
		"timezone":      {"auto"},
		"forecast_days": {strconv.Itoa(days)},
	}
	data, err := p.get(ctx, p.forecastURL+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var fr struct {
		Daily struct {
			Time       []string  `json:"time"`
			Code       []int     `json:"weather_code"`
			TempMax    []float64 `json:"temperature_2m_max"`
			TempMin    []float64 `json:"temperature_2m_min"`
			PrecipProb []int     `json:"precipitation_probability_max"`
		} `json:"daily"`
	}
	if err := json.Unmarshal(data, &fr); err != nil {
		return nil, fmt.Errorf("weather: forecast decode: %w", err)
	}
	fc := &domain.Forecast{}
	for i, date := range fr.Daily.Time {
		d := domain.Day{Date: date, Code: -1}
		if i < len(fr.Daily.Code) {
			d.Code = fr.Daily.Code[i]
			d.Text = WMOText(d.Code, locale)
		}
		if i < len(fr.Daily.TempMax) {
			d.TempMax = fr.Daily.TempMax[i]
		}
		if i < len(fr.Daily.TempMin) {
			d.TempMin = fr.Daily.TempMin[i]
		}
		if i < len(fr.Daily.PrecipProb) {
			d.PrecipProb = fr.Daily.PrecipProb[i]
		}
		fc.Days = append(fc.Days, d)
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
		return nil, fmt.Errorf("weather: http %d", resp.StatusCode)
	}
	return data, nil
}

// codeTexts maps WMO weather interpretation codes to {zh-CN, en-US} labels.
var codeTexts = map[int][2]string{
	0: {"晴", "Clear"}, 1: {"大致晴", "Mostly clear"}, 2: {"局部多云", "Partly cloudy"},
	3: {"阴", "Overcast"}, 45: {"雾", "Fog"}, 48: {"冻雾", "Rime fog"},
	51: {"细毛毛雨", "Light drizzle"}, 53: {"毛毛雨", "Drizzle"}, 55: {"浓毛毛雨", "Dense drizzle"},
	56: {"冻毛毛雨", "Freezing drizzle"}, 57: {"强冻毛毛雨", "Dense freezing drizzle"},
	61: {"小雨", "Light rain"}, 63: {"中雨", "Rain"}, 65: {"大雨", "Heavy rain"},
	66: {"冻雨", "Freezing rain"}, 67: {"强冻雨", "Heavy freezing rain"},
	71: {"小雪", "Light snow"}, 73: {"中雪", "Snow"}, 75: {"大雪", "Heavy snow"},
	77: {"雪粒", "Snow grains"}, 80: {"小阵雨", "Light showers"}, 81: {"阵雨", "Showers"},
	82: {"强阵雨", "Violent showers"}, 85: {"小阵雪", "Light snow showers"}, 86: {"阵雪", "Snow showers"},
	95: {"雷暴", "Thunderstorm"}, 96: {"雷暴伴冰雹", "Thunderstorm with hail"}, 99: {"强雷暴伴冰雹", "Thunderstorm with heavy hail"},
}

// WMOText returns the label for a WMO weather code in the given locale.
func WMOText(code int, locale string) string {
	idx := 0
	if strings.HasPrefix(strings.ToLower(locale), "en") {
		idx = 1
	}
	if t, ok := codeTexts[code]; ok {
		return t[idx]
	}
	if idx == 1 {
		return "Unknown"
	}
	return "未知"
}
