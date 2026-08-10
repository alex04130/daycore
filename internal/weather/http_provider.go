package weather

import (
	"context"

	"daycore/internal/adapters"
	"daycore/internal/domain"
)

// httpProvider reaches an external weather adapter over the protocol in
// docs/specs/provider-protocol.md.
//
// It satisfies domain.WeatherProvider like any built-in one, which is the whole
// claim that protocol makes: the layers above cannot tell an adapter from a
// compiled-in source. That claim is only worth anything if it is true at the
// type level, so this file is deliberately small — everything that is not
// "translate a forecast" lives in internal/adapters, shared with search,
// channels and storage.
type httpProvider struct {
	client *adapters.Client
	id     string
}

func (p *httpProvider) Name() string { return p.id }

type weatherReq struct {
	Location string `json:"location"`
	Days     int    `json:"days"`
	Locale   string `json:"locale"`
}

type weatherResp struct {
	Location string `json:"location"`
	Days     []struct {
		Date       string  `json:"date"`
		Code       *int    `json:"code"`
		Text       string  `json:"text"`
		TempMin    float64 `json:"tempMin"`
		TempMax    float64 `json:"tempMax"`
		PrecipProb int     `json:"precipProb"`
	} `json:"days"`
}

func (p *httpProvider) Lookup(ctx context.Context, q domain.WeatherQuery) (*domain.Forecast, error) {
	var r weatherResp
	if err := p.client.Do(ctx, "/v0/weather", weatherReq{
		Location: q.Location, Days: q.Days, Locale: q.Locale,
	}, &r); err != nil {
		return nil, err
	}

	// A 200 with no days is a FAILURE, not an empty forecast.
	//
	// The protocol lets an adapter answer "I do not know this place" with 200
	// and an empty array, and that is a reasonable thing for it to say. But an
	// empty forecast renders as a perfectly ordinary-looking sentence with no
	// weather in it, and it would be counted as a successful call — a source
	// that answers 200 with nothing forever would stay marked healthy and quietly
	// remove weather from every brief. This is the one shape of "200 but wrong"
	// that this protocol makes easy to produce, so it is the one that gets
	// caught here.
	if len(r.Days) == 0 {
		return nil, &adapters.Error{Kind: adapters.KindProtocol, Source: p.id}
	}

	fc := &domain.Forecast{Location: r.Location, Days: make([]domain.Day, 0, len(r.Days))}
	if fc.Location == "" {
		// Adapters that echo nothing back should not produce a forecast headed
		// by an empty string; the query is the best answer available.
		fc.Location = q.Location
	}
	for _, d := range r.Days {
		// A missing code is -1, the same sentinel the built-in providers use for
		// "this source has no condition code". Distinguishing absent from 0
		// matters: 0 is a real WMO code (clear sky).
		code := -1
		if d.Code != nil {
			code = *d.Code
		}
		fc.Days = append(fc.Days, domain.Day{
			Date: d.Date, Code: code, Text: d.Text,
			TempMin: d.TempMin, TempMax: d.TempMax, PrecipProb: d.PrecipProb,
		})
	}
	return fc, nil
}
