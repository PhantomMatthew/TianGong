// Package config provides configuration loading and validation for TianGong.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

// Config is the root configuration for TianGong.
type Config struct {
	Providers map[string]ProviderConfig `mapstructure:"providers" validate:"dive"`
	Server    ServerConfig              `mapstructure:"server"`
	Agent     AgentConfig               `mapstructure:"agent"`
	Database  DatabaseConfig            `mapstructure:"database"`
}

// ProviderConfig configures an LLM provider.
type ProviderConfig struct {
	Name     string        `mapstructure:"name"`
	APIKey   string        `mapstructure:"api_key" validate:"required"`
	Model    string        `mapstructure:"model" validate:"required"`
	Endpoint string        `mapstructure:"endpoint"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

// ServerConfig configures the HTTP server.
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port" validate:"gte=1,lte=65535"`
}

// AgentConfig configures agent behavior.
type AgentConfig struct {
	SystemPrompt  string        `mapstructure:"system_prompt"`
	MaxIterations int           `mapstructure:"max_iterations" validate:"gte=1,lte=50"`
	HistoryLimit  int           `mapstructure:"history_limit" validate:"gte=1"`
	Timeout       time.Duration `mapstructure:"timeout"`
}

// DatabaseConfig configures database access.
type DatabaseConfig struct {
	URL string `mapstructure:"url"`
}

// Load loads configuration from YAML files and environment variables.
func Load(cfgPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigName("tiangong")
	v.SetConfigType("yaml")

	// Search paths
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("$HOME/.config/tiangong")
	v.AddConfigPath("/etc/tiangong")

	// Override with explicit path if provided
	if cfgPath != "" {
		v.SetConfigFile(cfgPath)
	}

	// Environment variables
	v.SetEnvPrefix("TIANGONG")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Read config file (ignore if not found)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
		slog.Info("no config file found, using defaults and env vars")
	} else {
		slog.Info("loaded config", "file", v.ConfigFileUsed())
	}

	// Apply defaults
	ApplyDefaults(v)

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// WORKAROUND: Viper doesn't auto-bind nested map env vars
	// Manually populate providers from env vars
	cfg.populateProvidersFromEnv()

	// Validate
	validate := validator.New()
	if err := validate.Struct(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// populateProvidersFromEnv manually populates provider configs from env vars.
// This works around Viper's limitation with nested map env var binding.
func (c *Config) populateProvidersFromEnv() {
	if c.Providers == nil {
		c.Providers = make(map[string]ProviderConfig)
	}

	// Check for TIANGONG_PROVIDERS_<NAME>_API_KEY
	for _, envVar := range os.Environ() {
		if strings.HasPrefix(envVar, "TIANGONG_PROVIDERS_") {
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := parts[0]
			value := parts[1]

			// Parse: TIANGONG_PROVIDERS_OPENAI_API_KEY
			segments := strings.Split(key, "_")
			if len(segments) < 4 {
				continue
			}

			providerName := strings.ToLower(segments[2])
			field := strings.Join(segments[3:], "_")
			field = strings.ToLower(field)

			provider, exists := c.Providers[providerName]
			if !exists {
				provider = ProviderConfig{Name: providerName}
			}

			switch field {
			case "api_key", "apikey":
				provider.APIKey = value
			case "model":
				provider.Model = value
			case "endpoint":
				provider.Endpoint = value
			}

			c.Providers[providerName] = provider
		}
	}
}
