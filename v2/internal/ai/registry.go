package ai

import (
	"fmt"
	"sort"
	"sync"
)

// ModelConfig is the fully-resolved description of one model the app can use.
// It is produced from the catalog config (api key already resolved from env).
type ModelConfig struct {
	ID        string         // catalog id used everywhere in the app (e.g. "deepseek-chat")
	Format    string         // which registered wire format builds it ("openai"/"anthropic"/...)
	BaseURL   string         // API base URL
	APIKey    string         // resolved secret
	Model     string         // upstream model name (defaults to ID)
	MaxTokens int            // default max_tokens when the request doesn't set one (0 = provider default)
	ExtraBody map[string]any // extra top-level request keys, never overriding built ones (openai format)
	Caps      Capabilities   // declared capabilities
}

// Factory builds an AIProvider for a resolved model config.
type Factory func(ModelConfig) (AIProvider, error)

var (
	formatsMu sync.RWMutex
	formats   = map[string]Factory{}
)

// RegisterFormat registers a wire-format factory under a name. Called from each
// format package's init(). Panics on duplicate/empty registration (programmer error).
func RegisterFormat(name string, f Factory) {
	formatsMu.Lock()
	defer formatsMu.Unlock()
	if name == "" || f == nil {
		panic("ai: invalid RegisterFormat arguments")
	}
	if _, dup := formats[name]; dup {
		panic("ai: format already registered: " + name)
	}
	formats[name] = f
}

// Formats lists the registered wire-format names (sorted), for diagnostics.
func Formats() []string {
	formatsMu.RLock()
	defer formatsMu.RUnlock()
	out := make([]string, 0, len(formats))
	for n := range formats {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// build constructs a provider for a model config via its registered format.
func build(cfg ModelConfig) (AIProvider, error) {
	formatsMu.RLock()
	f, ok := formats[cfg.Format]
	formatsMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("ai: unknown format %q for model %q (registered: %v)", cfg.Format, cfg.ID, Formats())
	}
	if cfg.Model == "" {
		cfg.Model = cfg.ID
	}
	return f(cfg)
}
