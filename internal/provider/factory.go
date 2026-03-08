// Package provider provides LLM provider abstractions.
package provider

import (
	"fmt"

	"github.com/PhantomMatthew/TianGong/internal/config"
)

// NewProvider creates a provider instance from configuration.
// Returns an error if the provider is unknown or configuration is invalid.
func NewProvider(name string, cfg config.ProviderConfig) (Provider, error) {
	switch name {
	case "openai":
		return NewOpenAI(cfg)
	case "anthropic":
		return nil, fmt.Errorf("anthropic provider not yet implemented")
	case "google":
		return nil, fmt.Errorf("google provider not yet implemented")
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}
