package domain

import (
	"context"
	"fmt"
	"strings"

	"daycore/internal/i18n"
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

// precipFormat is the one localized fragment in a forecast digest — the
// condition text itself arrives already localized from the provider. The whole
// fragment is in the table, spacing included: Chinese runs the label onto the
// number and Latin scripts do not, and that is the translator's call, not
// something to derive from the script at runtime.
//
// %d is the precipitation probability. A new locale must keep exactly that one
// verb.
var precipFormat = i18n.Text{"zh-CN": "降水%d%%", "en-US": "precip %d%%"}

// Summary renders a compact digest for prompt injection, e.g.
// "北京: 07-12 阴 22~31°C 降水40%; 07-13 小雨 21~28°C 降水80%".
func (f *Forecast) Summary(locale string) string {
	precip := i18n.Pick(precipFormat, locale)
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
		fmt.Fprintf(&b, "%s %s %.0f~%.0f°C ", date, d.Text, d.TempMin, d.TempMax)
		fmt.Fprintf(&b, precip, d.PrecipProb)
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
