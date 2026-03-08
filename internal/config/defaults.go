package config

import (
	"time"

	"github.com/spf13/viper"
)

// ApplyDefaults sets default values in the Viper instance.
func ApplyDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "127.0.0.1")
	v.SetDefault("server.port", 8080)
	v.SetDefault("agent.max_iterations", 10)
	v.SetDefault("agent.history_limit", 50)
	v.SetDefault("agent.timeout", 30*time.Second)
	v.SetDefault("agent.system_prompt", DefaultSystemPrompt)
}

// DefaultSystemPrompt is the default system prompt for the agent.
const DefaultSystemPrompt = `You are a helpful AI assistant with access to tools.
Use tools when needed to accomplish tasks. Be concise and accurate.`
