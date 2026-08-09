// Package weather resolves the configured weather sources and answers lookups
// against a named one.
//
// Concrete providers live in subpackages (openmeteo / qweather /
// openweathermap / wttrin) and self-register via init(); an external adapter is
// reached over the protocol in docs/specs/provider-protocol.md. Both satisfy
// domain.WeatherProvider, which still means exactly one source — the set lives
// in Sources.
package weather

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"daycore/internal/adapters"
	"daycore/internal/domain"
)

// Config is passed to a provider factory.
type Config struct {
	APIKey string
	HTTP   *http.Client
}

// Factory builds a WeatherProvider from config.
type Factory func(Config) domain.WeatherProvider

var (
	mu        sync.RWMutex
	factories = map[string]Factory{}
)

// Register adds a provider factory under a name (called from a provider init()).
//
// # Why this stayed its own registry rather than becoming a generic one
//
// A shared registry across weather, search and channels would save the twenty
// lines below and cost a type parameter in every provider subpackage's init.
// The test is not "do these look alike" but "are the bytes the same": a
// forecast lookup and a web search share none. What IS shared — the transport,
// the health machine, the config shape — lives in internal/adapters, which is
// four capabilities' worth of identical detail and therefore worth abstracting.
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	factories[name] = f
}

// Providers lists the registered implementation names (sorted), for diagnostics
// and for validating an entry's `impl` before the process starts serving.
func Providers() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(factories))
	for n := range factories {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func build(name string, cfg Config) domain.WeatherProvider {
	mu.RLock()
	f, ok := factories[name]
	mu.RUnlock()
	if !ok {
		return nil
	}
	return f(cfg)
}

// Options carries what a builtin provider needs that the entry does not say.
//
// The two keys are here rather than in providers.yaml for the reason every
// credential is: that file is meant to be committed, and a key in it is a key
// in the repository and in every clone. An http adapter names an environment
// variable via token_env; a builtin one reads the variable the deployment has
// always used.
type Options struct {
	QWeatherKey       string
	OpenWeatherMapKey string
	// Timeout bounds one upstream call. Eight seconds is what this package has
	// always used; it is short enough that a dead source does not hold up a
	// conversation and long enough for a slow forecast API.
	Timeout time.Duration
}

// buildProvider turns one resolved source into something that can answer.
func buildProvider(src *adapters.Source, o Options) (domain.WeatherProvider, error) {
	timeout := o.Timeout
	if timeout == 0 {
		timeout = 8 * time.Second
	}
	switch src.Entry.Format {
	case adapters.FormatBuiltin:
		p := build(src.Entry.Impl, Config{
			APIKey: keyFor(src.Entry.Impl, o),
			HTTP:   &http.Client{Timeout: timeout},
		})
		if p == nil {
			// Named an implementation nothing registered. Refusing beats
			// falling back to a free default: the old code did fall back, so a
			// typo in WEATHER_PROVIDER produced working forecasts from a source
			// nobody chose, and nothing said so.
			return nil, fmt.Errorf("no built-in provider %q (registered: %v)", src.Entry.Impl, Providers())
		}
		if needsKey(src.Entry.Impl) && keyFor(src.Entry.Impl, o) == "" {
			return nil, fmt.Errorf("%q needs an API key and none is configured", src.Entry.Impl)
		}
		return p, nil
	case adapters.FormatHTTP:
		return &httpProvider{
			client: adapters.NewClient(src.Entry.ID, src.Entry.BaseURL, src.Entry.Token(), timeout),
			id:     src.Entry.ID,
		}, nil
	}
	return nil, fmt.Errorf("unsupported format %q", src.Entry.Format)
}

func needsKey(impl string) bool { return impl == "qweather" || impl == "openweathermap" }

func keyFor(name string, o Options) string {
	switch name {
	case "qweather":
		return o.QWeatherKey
	case "openweathermap":
		return o.OpenWeatherMapKey
	}
	return ""
}

// DefaultEntries is what a deployment with no providers.yaml gets.
//
// # Why there is a default at all
//
// Every deployment that predates providers.yaml has WEATHER_PROVIDER and maybe
// a key, and their behaviour must not change on upgrade. So the absence of the
// file synthesises the same set the old code had access to: the free source,
// the free fallback, and whichever keyed source has a key.
//
// ⚠️ This is NOT the old chain. The old code tried the primary and silently
// answered from wttr.in when it failed. These are simply two sources; the model
// picks, and a failure is reported as a failure.
func DefaultEntries(o Options) []adapters.Entry {
	out := []adapters.Entry{
		{ID: "open-meteo", Format: adapters.FormatBuiltin, Impl: "open-meteo"},
		{ID: "wttr", Format: adapters.FormatBuiltin, Impl: "wttr"},
	}
	if o.QWeatherKey != "" {
		out = append(out, adapters.Entry{ID: "qweather", Format: adapters.FormatBuiltin, Impl: "qweather"})
	}
	if o.OpenWeatherMapKey != "" {
		out = append(out, adapters.Entry{ID: "openweathermap", Format: adapters.FormatBuiltin, Impl: "openweathermap"})
	}
	return out
}

// OptionsFromEnv reads the two keys a builtin provider may need.
func OptionsFromEnv() Options {
	return Options{
		QWeatherKey:       os.Getenv("QWEATHER_API_KEY"),
		OpenWeatherMapKey: os.Getenv("OPENWEATHERMAP_API_KEY"),
	}
}
