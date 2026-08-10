package main

import (
	"context"
	"fmt"
	"log/slog"

	"daycore/internal/adapters"
	"daycore/internal/config"
	"daycore/internal/domain"
	"daycore/internal/weather"
	"daycore/internal/websearch"
)

// buildSources assembles the external capability sources from their three
// layers.
//
//	config/providers.yaml    identity and wiring (boot: the process builds a
//	                         client out of it, so it cannot change at runtime)
//	provider_overrides       enabled / description / approved (runtime)
//	the environment          credentials, which never go in a committed file
//
// # Why one function rather than each package fetching its own
//
// providers.yaml would otherwise be parsed twice, and two parsers of one file
// is the definition of drift. It also means the override table is read once:
// it is small, read at boot and after each console write, and reading it per
// capability would triple that for no benefit.
//
// # Boundary: a failure here is a warning, never fatal
//
// A weather adapter with a bad base_url must not stop the process. The
// deployment's other sources still work, and refusing to boot over one
// misconfigured source would take away the console that could fix it — the same
// reasoning as degraded storage boot, applied one layer up.
func buildSources(cfg *config.Config, store domain.Store, log *slog.Logger) (*weather.Sources, *websearch.Sources, []error) {
	var warnings []error

	file, err := adapters.Load(cfg.ProvidersConfigPath)
	if err != nil {
		// A providers.yaml that will not parse is different from one that is
		// absent: absent means "use the defaults", unparseable means somebody
		// wrote something and it is being ignored. Say so loudly and carry on
		// with the defaults, because the alternative is a deployment that will
		// not start over a config file it did not have yesterday.
		warnings = append(warnings, fmt.Errorf("providers config ignored: %w", err))
		file = &adapters.File{}
	}

	overrides := map[string]*domain.ProviderOverride{}
	if store != nil {
		rows, err := store.ProviderOverrides().All(context.Background())
		if err != nil {
			warnings = append(warnings, fmt.Errorf("provider overrides unreadable, using the file as-is: %w", err))
		}
		for i := range rows {
			overrides[rows[i].Kind+"/"+rows[i].ID] = &rows[i]
		}
	}

	wOpts := weather.Options{QWeatherKey: cfg.QWeatherKey, OpenWeatherMapKey: cfg.OpenWeatherMapKey}
	sOpts := websearch.Options{TavilyKey: cfg.TavilyKey}

	wEntries := file.Entries(adapters.KindWeather)
	if len(wEntries) == 0 {
		// No providers.yaml, or none declared for this capability: synthesise
		// what the previous release had access to, so no existing deployment
		// changes behaviour on upgrade.
		wEntries = weather.DefaultEntries(wOpts)
	}
	sEntries := file.Entries(adapters.KindSearch)
	if len(sEntries) == 0 {
		sEntries = websearch.DefaultEntries(sOpts)
	}

	wSources, wProblems := weather.NewSources(resolve(adapters.KindWeather, wEntries, overrides), wOpts)
	sSources, sProblems := websearch.NewSources(resolve(adapters.KindSearch, sEntries, overrides), sOpts)
	warnings = append(warnings, wProblems...)
	warnings = append(warnings, sProblems...)

	// WEATHER_PROVIDER survives, with a new meaning: it is the id a lookup with
	// no source prefers, not a chain head.
	//
	// The variable was NOT deleted, deliberately. Removing it would mean editing
	// three branches of the installer and .env.example, and would leave every
	// upgraded database holding a settings override row for a field that no
	// longer exists — a row the console cannot display and cannot delete, warned
	// about on every reload. That is the failure F1 exists to prevent, arriving
	// from the other direction. Keeping the name costs one sentence of
	// documentation.
	if cfg.WeatherProvider != "" {
		wSources.SetDefault(cfg.WeatherProvider)
	}

	log.Info("capability sources",
		"weather", wSources.IDs(), "search", sSources.IDs(), "config", cfg.ProvidersConfigPath)
	return wSources, sSources, warnings
}

func resolve(kind adapters.Kind, entries []adapters.Entry, overrides map[string]*domain.ProviderOverride) []*adapters.Source {
	out := make([]*adapters.Source, 0, len(entries))
	for _, e := range entries {
		out = append(out, adapters.Resolve(kind, e, overrides[string(kind)+"/"+e.ID], adapters.NewHealth()))
	}
	return out
}
