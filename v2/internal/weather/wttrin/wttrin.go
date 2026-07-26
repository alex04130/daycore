// Package wttrin implements domain.WeatherProvider against wttr.in (free, no API
// key). It is also the fallback provider in the weather chain.
package wttrin

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
	weather.Register("wttr", func(cfg weather.Config) domain.WeatherProvider {
		return New(cfg.HTTP)
	})
}

// Provider fetches forecasts from wttr.in (JSON via ?format=j1).
type Provider struct {
	http *http.Client
	base string
}

// New builds a Provider. A nil client gets an 8s-timeout default.
func New(httpc *http.Client) *Provider {
	if httpc == nil {
		httpc = &http.Client{Timeout: 8 * time.Second}
	}
	return &Provider{http: httpc, base: "https://wttr.in"}
}

func (p *Provider) Name() string { return "wttr" }

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
		days = 3 // wttr.in returns 3 days
	}
	lang := "en"
	if !strings.HasPrefix(strings.ToLower(q.Locale), "en") {
		lang = "zh"
	}
	u := fmt.Sprintf("%s/%s?format=j1&lang=%s", p.base, url.PathEscape(location), lang)
	data, err := p.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var wr struct {
		NearestArea []struct {
			AreaName []struct {
				Value string `json:"value"`
			} `json:"areaName"`
		} `json:"nearest_area"`
		Weather []struct {
			Date     string `json:"date"`
			MaxtempC string `json:"maxtempC"`
			MintempC string `json:"mintempC"`
			Hourly   []struct {
				ChanceOfRain string `json:"chanceofrain"`
				WeatherDesc  []struct {
					Value string `json:"value"`
				} `json:"weatherDesc"`
				LangZh []struct {
					Value string `json:"value"`
				} `json:"lang_zh"`
			} `json:"hourly"`
		} `json:"weather"`
	}
	if err := json.Unmarshal(data, &wr); err != nil {
		return nil, fmt.Errorf("weather: wttr decode: %w", err)
	}
	fc := &domain.Forecast{Location: location}
	if len(wr.NearestArea) > 0 && len(wr.NearestArea[0].AreaName) > 0 {
		fc.Location = wr.NearestArea[0].AreaName[0].Value
	}
	for i, wd := range wr.Weather {
		if i >= days {
			break
		}
		d := domain.Day{Date: wd.Date, Code: -1}
		d.TempMax = atof(wd.MaxtempC)
		d.TempMin = atof(wd.MintempC)
		if len(wd.Hourly) > 0 {
			h := wd.Hourly[len(wd.Hourly)/2] // midday sample
			d.PrecipProb = atoi(h.ChanceOfRain)
			if lang == "zh" && len(h.LangZh) > 0 {
				d.Text = h.LangZh[0].Value
			} else if len(h.WeatherDesc) > 0 {
				d.Text = h.WeatherDesc[0].Value
			}
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
	req.Header.Set("User-Agent", "curl/8") // wttr.in serves JSON to curl-like agents
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
		return nil, fmt.Errorf("weather: wttr http %d", resp.StatusCode)
	}
	return data, nil
}

func atof(s string) float64 { v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64); return v }
func atoi(s string) int     { v, _ := strconv.Atoi(strings.TrimSpace(s)); return v }
