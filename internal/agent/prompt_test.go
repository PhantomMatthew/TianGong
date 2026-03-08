package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/PhantomMatthew/TianGong/internal/provider"
	"github.com/PhantomMatthew/TianGong/internal/session"
	"github.com/PhantomMatthew/TianGong/internal/tool"
)

// TestDefaultSystemPrompt verifies that the default system prompt is non-empty
// and mentions tool use capability.
func TestDefaultSystemPrompt(t *testing.T) {
	assert.NotEmpty(t, DefaultSystemPrompt, "DefaultSystemPrompt should not be empty")
	assert.Contains(t, DefaultSystemPrompt, "tool", "DefaultSystemPrompt should mention tools")
	assert.Contains(t, DefaultSystemPrompt, "AI assistant", "DefaultSystemPrompt should reference an AI assistant")
}

// TestFormatSystemPromptCustom verifies that a custom prompt replaces the default.
func TestFormatSystemPromptCustom(t *testing.T) {
	customPrompt := "You are a code reviewer. Check for bugs and style issues."
	result := FormatSystemPrompt(customPrompt, nil)

	assert.Equal(t, customPrompt, result, "Custom prompt should replace default")
	assert.NotContains(t, result, DefaultSystemPrompt, "Default should not appear when custom is provided")
}

// TestFormatSystemPromptDefault verifies that the default prompt is used when custom is empty.
func TestFormatSystemPromptDefault(t *testing.T) {
	result := FormatSystemPrompt("", nil)

	assert.Equal(t, DefaultSystemPrompt, result, "Default prompt should be used when custom is empty")
}

// TestFormatSystemPromptWithTools verifies that tool descriptions are appended to the prompt.
func TestFormatSystemPromptWithTools(t *testing.T) {
	// Create mock tools
	mockTools := []tool.Tool{
		&mockTool{name: "Calculator", desc: "Performs mathematical calculations"},
		&mockTool{name: "WebSearch", desc: "Searches the web for information"},
	}

	result := FormatSystemPrompt(DefaultSystemPrompt, mockTools)

	// Verify prompt contains tool descriptions
	assert.Contains(t, result, "Available tools:", "Prompt should include 'Available tools' header")
	assert.Contains(t, result, "Calculator: Performs mathematical calculations", "Prompt should include Calculator tool")
	assert.Contains(t, result, "WebSearch: Searches the web for information", "Prompt should include WebSearch tool")
}

// TestFormatSystemPromptEmptyTools verifies that empty tool list doesn't add tools section.
func TestFormatSystemPromptEmptyTools(t *testing.T) {
	result := FormatSystemPrompt(DefaultSystemPrompt, []tool.Tool{})

	assert.Equal(t, DefaultSystemPrompt, result, "Empty tools list should not modify prompt")
	assert.NotContains(t, result, "Available tools:", "Tools section should not appear with empty list")
}

// TestBuildMessages verifies that session messages are correctly converted to provider messages.
func TestBuildMessages(t *testing.T) {
	systemPrompt := "You are a test assistant."
	sessionMessages := []*session.Message{
		{
			ID:        "msg1",
			SessionID: "sess1",
			Role:      session.RoleUser,
			Content:   "Hello, what is 2+2?",
		},
		{
			ID:        "msg2",
			SessionID: "sess1",
			Role:      session.RoleAssistant,
			Content:   "2+2 equals 4",
		},
	}

	result := BuildMessages(systemPrompt, sessionMessages)

	// Verify count: system + 2 messages
	assert.Len(t, result, 3, "Result should contain system message plus 2 session messages")

	// Verify system message is first
	assert.Equal(t, provider.RoleSystem, result[0].Role, "First message should be system role")
	assert.Equal(t, systemPrompt, result[0].Content, "First message should contain system prompt")

	// Verify user message
	assert.Equal(t, provider.RoleUser, result[1].Role, "Second message should be user role")
	assert.Equal(t, "Hello, what is 2+2?", result[1].Content, "User message content should match")

	// Verify assistant message
	assert.Equal(t, provider.RoleAssistant, result[2].Role, "Third message should be assistant role")
	assert.Equal(t, "2+2 equals 4", result[2].Content, "Assistant message content should match")
}

// TestBuildMessagesEmptyHistory verifies that only system message is returned for empty history.
func TestBuildMessagesEmptyHistory(t *testing.T) {
	systemPrompt := "Test system prompt"
	result := BuildMessages(systemPrompt, nil)

	assert.Len(t, result, 1, "Empty history should result in only system message")
	assert.Equal(t, provider.RoleSystem, result[0].Role, "Single message should be system role")
	assert.Equal(t, systemPrompt, result[0].Content, "Single message should contain system prompt")
}

// TestBuildMessagesPreservesToolCalls verifies that tool calls and tool call IDs are preserved.
func TestBuildMessagesPreservesToolCalls(t *testing.T) {
	systemPrompt := "Test system prompt"
	toolCall := session.ToolCall{
		ID:        "call1",
		Name:      "calculator",
		Arguments: `{"a": 2, "b": 2}`,
	}

	sessionMessages := []*session.Message{
		{
			ID:        "msg1",
			SessionID: "sess1",
			Role:      session.RoleAssistant,
			Content:   "I'll calculate that for you",
			ToolCalls: []session.ToolCall{toolCall},
		},
		{
			ID:         "msg2",
			SessionID:  "sess1",
			Role:       session.RoleTool,
			Content:    "4",
			ToolCallID: "call1",
		},
	}

	result := BuildMessages(systemPrompt, sessionMessages)

	// Verify tool calls are preserved in assistant message (index 1)
	assert.Len(t, result[1].ToolCalls, 1, "Assistant message should have tool call")
	assert.Equal(t, "call1", result[1].ToolCalls[0].ID, "Tool call ID should be preserved")
	assert.Equal(t, "calculator", result[1].ToolCalls[0].Name, "Tool call name should be preserved")

	// Verify tool call ID is preserved in tool message (index 2)
	assert.Equal(t, "call1", result[2].ToolCallID, "Tool call ID should be preserved in tool message")
}

// TestBuildMessagesRoleMapping verifies that all session roles map to provider roles correctly.
func TestBuildMessagesRoleMapping(t *testing.T) {
	tests := []struct {
		name         string
		sessionRole  session.MessageRole
		providerRole provider.MessageRole
	}{
		{"User role", session.RoleUser, provider.RoleUser},
		{"Assistant role", session.RoleAssistant, provider.RoleAssistant},
		{"System role", session.RoleSystem, provider.RoleSystem},
		{"Tool role", session.RoleTool, provider.RoleTool},
	}

	systemPrompt := "Test system"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := []*session.Message{
				{
					ID:        "msg1",
					SessionID: "sess1",
					Role:      tt.sessionRole,
					Content:   "test content",
				},
			}

			result := BuildMessages(systemPrompt, messages)

			// Result has system message at index 0, converted message at index 1
			assert.Equal(t, tt.providerRole, result[1].Role, "Role should be correctly mapped")
		})
	}
}

// mockTool is a test helper implementing the tool.Tool interface.
type mockTool struct {
	name string
	desc string
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Description() string {
	return m.desc
}

func (m *mockTool) Parameters() map[string]any {
	return map[string]any{}
}

func (m *mockTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return "", nil
}
