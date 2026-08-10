// Package adapters holds everything about an external capability source that
// knows nothing about what the capability is.
//
// # What lives here and what does not
//
// Here: the providers.yaml shape and its loader, the manifest, the health state
// machine, and the transport clients (HTTP now, subprocess later). None of it
// mentions a forecast or a search hit.
//
// Not here: the capability interfaces themselves. Weather keeps
// weather.Register, search keeps search.Register, each typed to its own
// factory. Sharing one generic registry would save the 23 lines that registry
// actually is, and cost a type parameter in every provider subpackage's init —
// a net loss. The test is not "do these look alike", it is "are the bytes the
// same": a forecast lookup and a web search share none.
//
// What IS shared, and why it had to be: the transport rules are four documents'
// worth of detail — status-code semantics, deadline propagation, redirect
// refusal, subprocess handshake and supervision — used by four capabilities
// (storage, weather, search, channels). Copied four times, somebody fixes an
// off-by-one in one of them and the symptom is "the same adapter script works
// when mounted as storage and intermittently fails as weather", with no log
// line anywhere that says so. docs/specs/transport.md is split along exactly
// this line for exactly this reason; this package is that document in Go.
package adapters

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kind is a capability. Sources are keyed by (Kind, ID) everywhere, because ids
// are only unique within a capability — two sources may both sensibly be called
// "primary".
type Kind string

const (
	KindWeather Kind = "weather"
	KindSearch  Kind = "search"
	KindChannel Kind = "channel"
)

// Format is how a source is reached.
type Format string

const (
	// FormatBuiltin binds to an implementation compiled into this binary and
	// registered by its own package's init.
	FormatBuiltin Format = "builtin"
	// FormatHTTP is an external adapter speaking docs/specs/provider-protocol.md
	// over HTTP.
	FormatHTTP Format = "http"
)

// Entry is one source as declared in config/providers.yaml.
//
// # Boundary: every field here is BOOT-layer
//
// The process builds an HTTP client, or looks up a registered factory, out of
// these at startup — so a new value cannot change the thing already built. That
// is the same line F1 drew across the environment variables, and it is why the
// console never writes this file. The console-editable half (enabled,
// description, approved) is a different struct in a different place; see
// domain.ProviderOverride.
//
// base_url in particular is the SSRF entrance: editable from a web page, it
// lets a compromised console point the backend at a link-local metadata address
// and read the answer back out through a weather forecast the model repeats to
// the user. Requiring shell access is the whole defence, and it costs nothing
// real — changing an adapter's address was always an operations task.
type Entry struct {
	ID     string `yaml:"id"`
	Format Format `yaml:"format"`

	// Impl is the registered implementation name for format: builtin — the
	// weather provider or search engine compiled in. Separate from ID because
	// ID is the operator's name for THIS entry and Impl is ours for the code:
	// two entries can both use "qweather" with different keys and regions, and
	// collapsing them would mean one serving the other's cached answers.
	Impl string `yaml:"impl"`

	// BaseURL is where an http adapter lives.
	BaseURL string `yaml:"base_url"`
	// TokenEnv names the environment variable holding the bearer token. The
	// token itself is never written here: this file is meant to be committed,
	// and a secret in it is a secret in the repository and in every clone.
	TokenEnv string `yaml:"token_env"`

	// Enabled is the file's opinion. The override table can flip it at runtime;
	// absent here means true, because a source somebody bothered to declare is
	// one they want.
	Enabled *bool `yaml:"enabled"`

	// Description is the operator's own words, per locale, and the ONLY text
	// from here that may reach a prompt.
	//
	// ⚠️ Deliberately NOT the same field as an adapter's self-reported
	// manifest.description. They are two fields with two names so that nothing
	// can assign one to the other — an adapter that could describe itself into
	// the system prompt would make the approval gate decorative. See
	// Source.ManifestDescription.
	Description map[string]string `yaml:"description"`
}

// File is config/providers.yaml.
type File struct {
	Weather  []Entry `yaml:"weather"`
	Search   []Entry `yaml:"search"`
	Channels []Entry `yaml:"channels"`
}

// Load reads providers.yaml. A missing file is not an error — it means "use the
// defaults derived from the environment", which is what every deployment that
// predates this feature has.
func Load(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &File{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var f File
	// Strict: a misspelled key in a config file is a setting the operator
	// believes they made and the process never sees. models.yaml is lenient and
	// paid for it — `deepseek_search: true` sat in the default config declaring
	// a capability nothing implemented.
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := f.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &f, nil
}

func (f *File) validate() error {
	for kind, list := range map[Kind][]Entry{
		KindWeather: f.Weather, KindSearch: f.Search, KindChannel: f.Channels,
	} {
		seen := map[string]bool{}
		for i, e := range list {
			switch {
			case e.ID == "":
				return fmt.Errorf("%s[%d]: id is required", kind, i)
			// ':' is the separator mongostore composes into _id. An id
			// containing one could be spelled two ways, which is a row that can
			// be written twice and read once.
			case strings.ContainsAny(e.ID, ": \t"):
				return fmt.Errorf("%s[%d]: id %q may not contain ':' or whitespace", kind, i, e.ID)
			case seen[e.ID]:
				return fmt.Errorf("%s: duplicate id %q", kind, e.ID)
			}
			seen[e.ID] = true

			switch e.Format {
			case FormatBuiltin:
				if e.Impl == "" {
					return fmt.Errorf("%s/%s: format builtin needs impl", kind, e.ID)
				}
				if e.BaseURL != "" {
					return fmt.Errorf("%s/%s: format builtin cannot have base_url", kind, e.ID)
				}
			case FormatHTTP:
				if err := validateBaseURL(e.BaseURL); err != nil {
					return fmt.Errorf("%s/%s: %w", kind, e.ID, err)
				}
				if e.Impl != "" {
					return fmt.Errorf("%s/%s: format http cannot have impl", kind, e.ID)
				}
			case "":
				return fmt.Errorf("%s/%s: format is required (builtin | http)", kind, e.ID)
			default:
				return fmt.Errorf("%s/%s: unknown format %q", kind, e.ID, e.Format)
			}

			// Both locales or neither. A description present in one language
			// only would show up in a prompt for half the users and silently
			// vanish for the rest — the same rule the prompt templates are held
			// to, and for the same reason.
			if n := len(e.Description); n != 0 && n != 2 {
				return fmt.Errorf("%s/%s: description needs both zh-CN and en-US (found %d)", kind, e.ID, n)
			}
			for _, l := range []string{"zh-CN", "en-US"} {
				if len(e.Description) > 0 && strings.TrimSpace(e.Description[l]) == "" {
					return fmt.Errorf("%s/%s: description is missing %s", kind, e.ID, l)
				}
			}
			if len(e.Description[""]) > 0 {
				return fmt.Errorf("%s/%s: description has an unlabelled locale", kind, e.ID)
			}
		}
	}
	return nil
}

// Entries returns one capability's entries, sorted by id.
//
// Sorted because this ordering ends up as the enum of a tool parameter, and a
// tool band whose bytes change between rounds invalidates the prompt cache —
// tools render before system, so the ephemeral breakpoint and the whole
// four-layer system prompt behind it get rewritten. Map iteration order would
// do that on every process.
func (f *File) Entries(kind Kind) []Entry {
	var list []Entry
	switch kind {
	case KindWeather:
		list = f.Weather
	case KindSearch:
		list = f.Search
	case KindChannel:
		list = f.Channels
	}
	out := append([]Entry(nil), list...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Token reads an entry's bearer token from the environment it names.
//
// Read at use rather than stored on the Entry so that the token never sits in a
// struct that something might later marshal into an admin response or a log
// line. The value is a credential; the variable NAME is not, and the name is
// what the console shows.
func (e Entry) Token() string {
	if e.TokenEnv == "" {
		return ""
	}
	return os.Getenv(e.TokenEnv)
}
