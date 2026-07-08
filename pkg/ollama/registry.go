package ollama

import (
	"fmt"
	"sync"
)

// ModelTier represents the capability tier of a model.
type ModelTier int

const (
	TierSmall ModelTier = iota // 7B range
	TierMedium                 // 8B-13B
	TierLarge                  // 30B+
)

// ModelConfig describes a registered model and its capabilities.
type ModelConfig struct {
	Name        string
	DisplayName string
	Tier        ModelTier
	ToolCalling bool
	MaxContext  int
	DefaultTemp float64
	Embedding   bool
}

// Registry manages available LLM models.
type Registry struct {
	mu           sync.RWMutex
	models       map[string]*ModelConfig
	defaultModel string
}

// NewRegistry creates a model registry with defaults.
func NewRegistry() *Registry {
	r := &Registry{
		models: make(map[string]*ModelConfig),
	}

	r.Register(&ModelConfig{
		Name:        "llama3.1:8b",
		DisplayName: "LLaMA 3.1 8B",
		Tier:        TierMedium,
		ToolCalling: true,
		MaxContext:  128000,
		DefaultTemp: 0.7,
		Embedding:   false,
	})

	r.Register(&ModelConfig{
		Name:        "qwen2.5:7b",
		DisplayName: "Qwen 2.5 7B",
		Tier:        TierSmall,
		ToolCalling: true,
		MaxContext:  131072,
		DefaultTemp: 0.7,
		Embedding:   false,
	})

	r.Register(&ModelConfig{
		Name:        "mistral",
		DisplayName: "Mistral",
		Tier:        TierMedium,
		ToolCalling: true,
		MaxContext:  32768,
		DefaultTemp: 0.7,
		Embedding:   false,
	})

	r.Register(&ModelConfig{
		Name:        "nomic-embed-text",
		DisplayName: "Nomic Embed Text",
		Tier:        TierSmall,
		ToolCalling: false,
		MaxContext:  8192,
		DefaultTemp: 0.0,
		Embedding:   true,
	})

	r.defaultModel = "llama3.1:8b"
	return r
}

// Register adds a model to the registry.
func (r *Registry) Register(cfg *ModelConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models[cfg.Name] = cfg
}

// Get returns a model config by name.
func (r *Registry) Get(name string) (*ModelConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cfg, ok := r.models[name]
	if !ok {
		return nil, fmt.Errorf("model %q not registered", name)
	}
	return cfg, nil
}

// Default returns the default model config.
func (r *Registry) Default() *ModelConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.models[r.defaultModel]
}

// SetDefault changes the default model.
func (r *Registry) SetDefault(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.models[name]; !ok {
		return fmt.Errorf("model %q not registered", name)
	}
	r.defaultModel = name
	return nil
}

// List returns all registered models.
func (r *Registry) List() []*ModelConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*ModelConfig, 0, len(r.models))
	for _, cfg := range r.models {
		out = append(out, cfg)
	}
	return out
}

// ToolCapable returns all models that support tool calling.
func (r *Registry) ToolCapable() []*ModelConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*ModelConfig, 0)
	for _, cfg := range r.models {
		if cfg.ToolCalling {
			out = append(out, cfg)
		}
	}
	return out
}

// EmbeddingModel returns the first embedding-capable model.
func (r *Registry) EmbeddingModel() *ModelConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, cfg := range r.models {
		if cfg.Embedding {
			return cfg
		}
	}
	return nil
}
