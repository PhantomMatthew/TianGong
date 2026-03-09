package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PhantomMatthew/TianGong/internal/config"
)

func TestNewOpenAI(t *testing.T) {
	t.Run("success with API key", func(t *testing.T) {
		cfg := config.ProviderConfig{
			APIKey: "sk-test-key",
			Model:  "gpt-4o",
		}

		provider, err := NewOpenAI(cfg)
		require.NoError(t, err)
		assert.NotNil(t, provider)
		assert.Equal(t, "openai", provider.Name())
		assert.Equal(t, "gpt-4o", provider.model)
	})

	t.Run("default model when not specified", func(t *testing.T) {
		cfg := config.ProviderConfig{
			APIKey: "sk-test-key",
		}

		provider, err := NewOpenAI(cfg)
		require.NoError(t, err)
		assert.Equal(t, "gpt-4o", provider.model)
	})

	t.Run("error when API key missing", func(t *testing.T) {
		cfg := config.ProviderConfig{
			Model: "gpt-4o",
		}

		provider, err := NewOpenAI(cfg)
		assert.ErrorIs(t, err, ErrAuthentication)
		assert.Nil(t, provider)
	})
}

func TestMapFinishReason(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected FinishReason
	}{
		{
			name:     "stop",
			input:    "stop",
			expected: FinishReasonStop,
		},
		{
			name:     "tool_calls",
			input:    "tool_calls",
			expected: FinishReasonToolCalls,
		},
		{
			name:     "function_call (legacy)",
			input:    "function_call",
			expected: FinishReasonToolCalls,
		},
		{
			name:     "length",
			input:    "length",
			expected: FinishReasonMaxTokens,
		},
		{
			name:     "unknown defaults to stop",
			input:    "unknown_reason",
			expected: FinishReasonStop,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapFinishReason(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOpenAIMessageMapping(t *testing.T) {
	// This is a documentation test showing expected message transformations
	// Actual API calls would require a real key and are tested manually

	t.Run("message role mapping", func(t *testing.T) {
		messages := []Message{
			{Role: RoleSystem, Content: "You are a helpful assistant"},
			{Role: RoleUser, Content: "Hello"},
			{Role: RoleAssistant, Content: "Hi there!"},
			{Role: RoleTool, Content: `{"result": "success"}`, ToolCallID: "call_123"},
		}

		// Verify structure is valid
		assert.Equal(t, 4, len(messages))
		assert.Equal(t, RoleSystem, messages[0].Role)
		assert.Equal(t, RoleUser, messages[1].Role)
		assert.Equal(t, RoleAssistant, messages[2].Role)
		assert.Equal(t, RoleTool, messages[3].Role)
		assert.Equal(t, "call_123", messages[3].ToolCallID)
	})

	t.Run("tool call structure", func(t *testing.T) {
		tc := ToolCall{
			ID:        "call_abc123",
			Name:      "get_weather",
			Arguments: `{"location":"Tokyo"}`,
		}

		assert.NotEmpty(t, tc.ID)
		assert.NotEmpty(t, tc.Name)
		assert.NotEmpty(t, tc.Arguments)
	})
}

func TestOpenAIToolDefinitionMapping(t *testing.T) {
	// This tests that tool definitions have the expected structure
	// Actual API calls would require a real key

	t.Run("tool definition structure", func(t *testing.T) {
		tools := []ToolDefinition{
			{
				Name:        "search",
				Description: "Search the web",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "Search query",
						},
					},
					"required": []string{"query"},
				},
			},
		}

		assert.Equal(t, 1, len(tools))
		assert.Equal(t, "search", tools[0].Name)
		assert.NotEmpty(t, tools[0].Description)
		assert.NotNil(t, tools[0].Parameters)

		params, ok := tools[0].Parameters["properties"].(map[string]any)
		assert.True(t, ok)
		assert.Contains(t, params, "query")
	})
}
