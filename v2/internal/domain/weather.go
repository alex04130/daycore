package domain

import (
	"context"
	"fmt"
	"strings"
)

// WeatherQuery is the input to a WeatherProvider lookup.
type WeatherQuery struct {
	Location string // free-text place name, e.g. "北京", "New York"
	Days     int    // requested forecast days (each provider clamps to its range)
	Locale   string // BCP-47 locale for the localized Day.Text
}

// Day is one day of forecast. Text is already localized by the provider.
type Day struct {
	Date       string  `json:"date"`
	Code       int     `json:"code"` // provider condition code (-1 when the provider has none)
	Text       string  `json:"text"`
	TempMin    float64 `json:"tempMin"`
	TempMax    float64 `json:"tempMax"`
	PrecipProb int     `json:"precipProb"`
}

// Forecast is a resolved location plus its daily forecast.
type Forecast struct {
	Location string `json:"location"`
	Days     []Day  `json:"days"`
}

// Summary renders a compact digest for prompt injection, e.g.
// "北京: 07-12 阴 22~31°C 降水40%; 07-13 小雨 21~28°C 降水80%".
func (f *Forecast) Summary(locale string) string {
	en := strings.HasPrefix(strings.ToLower(locale), "en")
	var b strings.Builder
	b.WriteString(f.Location)
	b.WriteString(": ")
	for i, d := range f.Days {
		if i > 0 {
			b.WriteString("; ")
		}
		date := d.Date
		if len(date) == len("2006-01-02") {
			date = date[5:]
		}
		if en {
			fmt.Fprintf(&b, "%s %s %.0f~%.0f°C precip %d%%", date, d.Text, d.TempMin, d.TempMax, d.PrecipProb)
		} else {
			fmt.Fprintf(&b, "%s %s %.0f~%.0f°C 降水%d%%", date, d.Text, d.TempMin, d.TempMax, d.PrecipProb)
		}
	}
	return b.String()
}

// WeatherProvider fetches short daily forecasts. Implementations live in
// internal/weather/<provider> and self-register with the weather registry, so
// the Server depends only on this interface (adapter pattern, §2.5).
type WeatherProvider interface {
	Name() string
	Lookup(ctx context.Context, q WeatherQuery) (*Forecast, error)
}
