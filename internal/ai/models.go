package ai

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// modelEntry is one model row in config/models.yaml. The set of models is fully
// data-driven — add/remove rows to change which models exist, no code changes.
type modelEntry struct {
	ID            string         `yaml:"id"`
	Format        string         `yaml:"format"` // must match a RegisterFormat name
	BaseURL       string         `yaml:"base_url"`
	Model         string         `yaml:"model"`       // upstream name (defaults to id)
	APIKeyEnv     string         `yaml:"api_key_env"` // env var holding the key
	APIKey        string         `yaml:"api_key"`     // inline key (discouraged; env preferred)
	Vision        bool           `yaml:"vision"`
	Tools         bool           `yaml:"tools"`
	Stream        bool           `yaml:"stream"`
	Thinking      bool           `yaml:"thinking"`
	ContextWindow int            `yaml:"context_window"`
	MaxTokens     int            `yaml:"max_tokens"`
	ExtraBody     map[string]any `yaml:"extra_body"`
}

type catalogFile struct {
	Models []modelEntry `yaml:"models"`
}

// ModelInfo is a read-only catalog summary entry.
type ModelInfo struct {
	ID   string       `json:"id"`
	Caps Capabilities `json:"caps"`
}

// Catalog holds the constructed providers keyed by model id, plus the chosen
// default chat and vision models.
type Catalog struct {
	providers     map[string]AIProvider
	order         []string
	defaultChatID string
	visionID      string // "" when no vision model is available
	plannerID     string // "" → planner falls back to the default chat model
}

// LoadCatalog reads the YAML catalog, resolves API keys from env, builds a
// provider per model via the format registry, and selects defaults. defaultChat
// must exist. defaultVision may be "" → the first vision-capable model is used.
// defaultPlanner may be "" or unknown → Planner() falls back to DefaultChat().
func LoadCatalog(path, defaultChat, defaultVision, defaultPlanner string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read models config %s: %w", path, err)
	}
	var f catalogFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse models config: %w", err)
	}
	if len(f.Models) == 0 {
		return nil, fmt.Errorf("no models configured in %s", path)
	}

	c := &Catalog{providers: map[string]AIProvider{}}
	for _, m := range f.Models {
		if m.ID == "" || m.Format == "" {
			return nil, fmt.Errorf("model entry needs both id and format")
		}
		if _, dup := c.providers[m.ID]; dup {
			return nil, fmt.Errorf("duplicate model id %q", m.ID)
		}
		key := m.APIKey
		if key == "" && m.APIKeyEnv != "" {
			key = os.Getenv(m.APIKeyEnv)
		}
		p, err := build(ModelConfig{
			ID:        m.ID,
			Format:    m.Format,
			BaseURL:   m.BaseURL,
			APIKey:    key,
			Model:     m.Model,
			MaxTokens: m.MaxTokens,
			ExtraBody: m.ExtraBody,
			Caps:      Capabilities{Vision: m.Vision, Tools: m.Tools, Stream: m.Stream, Thinking: m.Thinking, ContextWindow: m.ContextWindow},
		})
		if err != nil {
			return nil, fmt.Errorf("build model %q: %w", m.ID, err)
		}
		c.providers[m.ID] = p
		c.order = append(c.order, m.ID)
	}

	if _, ok := c.providers[defaultChat]; !ok {
		return nil, fmt.Errorf("DEFAULT_CHAT_MODEL %q is not in the catalog", defaultChat)
	}
	c.defaultChatID = defaultChat

	if _, ok := c.providers[defaultPlanner]; ok {
		c.plannerID = defaultPlanner
	}

	switch {
	case defaultVision != "":
		p, ok := c.providers[defaultVision]
		if !ok {
			return nil, fmt.Errorf("DEFAULT_VISION_MODEL %q is not in the catalog", defaultVision)
		}
		if !p.Capabilities().Vision {
			return nil, fmt.Errorf("DEFAULT_VISION_MODEL %q is not vision-capable", defaultVision)
		}
		c.visionID = defaultVision
	default:
		for _, id := range c.order { // auto-pick first vision-capable
			if c.providers[id].Capabilities().Vision {
				c.visionID = id
				break
			}
		}
	}
	return c, nil
}

// Provider returns the provider for a model id.
func (c *Catalog) Provider(id string) (AIProvider, bool) {
	p, ok := c.providers[id]
	return p, ok
}

// DefaultChat returns the default text-chat provider.
func (c *Catalog) DefaultChat() AIProvider { return c.providers[c.defaultChatID] }

// Planner returns the provider used for autonomous planning: the configured
// DEFAULT_PLANNER_MODEL when it exists in the catalog, else the default chat model.
func (c *Catalog) Planner() AIProvider {
	if c.plannerID == "" {
		return c.DefaultChat()
	}
	return c.providers[c.plannerID]
}

// Vision returns the default vision provider, or (nil,false) if none configured.
func (c *Catalog) Vision() (AIProvider, bool) {
	if c.visionID == "" {
		return nil, false
	}
	return c.providers[c.visionID], true
}

// HasVision reports whether any vision model is available.
func (c *Catalog) HasVision() bool { return c.visionID != "" }

// firstCap walks the catalog in order and returns the first provider the
// assertion accepts. The four capability selectors below are the discovery
// layer for the optional interfaces in generators.go: an endpoint that wants
// to transcribe asks the catalog, not a hardcoded model id, so a deployment
// gains the capability by adding a models.yaml row — the same zero-code deal
// the chat models get.
func (c *Catalog) firstCap(match func(AIProvider) bool) (AIProvider, bool) {
	for _, id := range c.order {
		if p := c.providers[id]; match(p) {
			return p, true
		}
	}
	return nil, false
}

// Transcriber returns the first catalog provider that can turn audio into
// text, or (nil,false) when no configured model implements it.
func (c *Catalog) Transcriber() (Transcriber, bool) {
	p, ok := c.firstCap(func(p AIProvider) bool { _, ok := AsTranscriber(p); return ok })
	if !ok {
		return nil, false
	}
	t, _ := AsTranscriber(p)
	return t, true
}

// SpeechSynthesizer returns the first catalog provider that can speak text.
func (c *Catalog) SpeechSynthesizer() (SpeechSynthesizer, bool) {
	p, ok := c.firstCap(func(p AIProvider) bool { _, ok := AsSpeechSynthesizer(p); return ok })
	if !ok {
		return nil, false
	}
	s, _ := AsSpeechSynthesizer(p)
	return s, true
}

// ImageGenerator returns the first catalog provider that can draw.
func (c *Catalog) ImageGenerator() (ImageGenerator, bool) {
	p, ok := c.firstCap(func(p AIProvider) bool { _, ok := AsImageGenerator(p); return ok })
	if !ok {
		return nil, false
	}
	g, _ := AsImageGenerator(p)
	return g, true
}

// Embedder returns the first catalog provider that can vectorise text.
func (c *Catalog) Embedder() (Embedder, bool) {
	p, ok := c.firstCap(func(p AIProvider) bool { _, ok := AsEmbedder(p); return ok })
	if !ok {
		return nil, false
	}
	e, _ := AsEmbedder(p)
	return e, true
}

// List returns a summary of every model in catalog order.
func (c *Catalog) List() []ModelInfo {
	out := make([]ModelInfo, 0, len(c.order))
	for _, id := range c.order {
		out = append(out, ModelInfo{ID: id, Caps: c.providers[id].Capabilities()})
	}
	return out
}
