package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PhantomMatthew/TianGong/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromYAML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "tiangong.yaml")

	yaml := `
server:
  port: 9090
agent:
  max_iterations: 5
`
	require.NoError(t, os.WriteFile(cfgFile, []byte(yaml), 0644))

	cfg, err := config.Load(cfgFile)
	require.NoError(t, err)
	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, 5, cfg.Agent.MaxIterations)
}

func TestLoadFromEnv(t *testing.T) {
	os.Setenv("TIANGONG_SERVER_PORT", "7070")
	defer os.Unsetenv("TIANGONG_SERVER_PORT")

	cfg, err := config.Load("")
	require.NoError(t, err)
	assert.Equal(t, 7070, cfg.Server.Port)
}

func TestDefaults(t *testing.T) {
	cfg, err := config.Load("")
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 10, cfg.Agent.MaxIterations)
	assert.Equal(t, 50, cfg.Agent.HistoryLimit)
}

func TestValidation(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "tiangong.yaml")

	yaml := `
server:
  port: 99999
`
	require.NoError(t, os.WriteFile(cfgFile, []byte(yaml), 0644))

	_, err := config.Load(cfgFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation")
}

func TestNestedEnvBinding(t *testing.T) {
	os.Setenv("TIANGONG_PROVIDERS_OPENAI_API_KEY", "sk-test123")
	os.Setenv("TIANGONG_PROVIDERS_OPENAI_MODEL", "gpt-4")
	defer func() {
		os.Unsetenv("TIANGONG_PROVIDERS_OPENAI_API_KEY")
		os.Unsetenv("TIANGONG_PROVIDERS_OPENAI_MODEL")
	}()

	cfg, err := config.Load("")
	require.NoError(t, err)
	require.Contains(t, cfg.Providers, "openai")
	assert.Equal(t, "sk-test123", cfg.Providers["openai"].APIKey)
	assert.Equal(t, "gpt-4", cfg.Providers["openai"].Model)
}
