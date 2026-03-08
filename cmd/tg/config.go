package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/PhantomMatthew/TianGong/internal/config"
)

// configCmd is the parent command for config-related operations.
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage TianGong configuration",
	Long: `Manage and view TianGong configuration settings.

This command provides subcommands to view and manage configuration.`,
}

// configShowCmd displays the current configuration in YAML format.
var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display current configuration",
	Long: `Display the current TianGong configuration in YAML format.

API keys are redacted for security. Configuration is loaded from:
  - ./tiangong.yaml
  - ~/.config/tiangong/tiangong.yaml
  - /etc/tiangong/tiangong.yaml
  - Environment variables (TIANGONG_*)

Example:
  tg config show`,
	RunE: runConfigShow,
}

func init() {
	configCmd.AddCommand(configShowCmd)
}

// runConfigShow loads the configuration and prints it as redacted YAML.
func runConfigShow(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create a copy of config with redacted API keys
	redactedCfg := *cfg
	if redactedCfg.Providers != nil {
		redactedProviders := make(map[string]config.ProviderConfig)
		for name, providerCfg := range redactedCfg.Providers {
			providerCfg.APIKey = "***"
			redactedProviders[name] = providerCfg
		}
		redactedCfg.Providers = redactedProviders
	}

	// Marshal to YAML
	yamlData, err := yaml.Marshal(&redactedCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	// Print to stdout
	fmt.Print(string(yamlData))

	return nil
}
