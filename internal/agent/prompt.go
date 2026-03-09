package agent

import (
	"fmt"

	"github.com/PhantomMatthew/TianGong/internal/provider"
	"github.com/PhantomMatthew/TianGong/internal/session"
	"github.com/PhantomMatthew/TianGong/internal/tool"
)

// DefaultSystemPrompt is the default system prompt for the agent.
const DefaultSystemPrompt = `You are a helpful AI assistant with access to tools to assist users.
You can invoke available tools to help answer questions and complete tasks.
Always think through your approach and use tools when appropriate.
Be clear and concise in your responses.`

// FormatSystemPrompt builds the final system prompt by combining a custom prompt
// (or default if empty) with available tool descriptions.
func FormatSystemPrompt(custom string, tools []tool.Tool) string {
	prompt := custom
	if prompt == "" {
		prompt = DefaultSystemPrompt
	}

	if len(tools) > 0 {
		prompt += "\n\nAvailable tools:\n"
		for _, t := range tools {
			prompt += fmt.Sprintf("- %s: %s\n", t.Name(), t.Description())
		}
	}

	return prompt
}

// BuildMessages converts session messages to provider messages, prepending a system message.
// The system prompt is inserted as the first message with RoleSystem.
func BuildMessages(systemPrompt string, history []*session.Message) []provider.Message {
	messages := make([]provider.Message, 0, len(history)+1)

	// Prepend system message
	messages = append(messages, provider.Message{
		Role:    provider.RoleSystem,
		Content: systemPrompt,
	})

	// Convert session messages to provider messages
	for _, msg := range history {
		// Convert tool calls from session to provider format
		providerToolCalls := make([]provider.ToolCall, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			providerToolCalls[i] = provider.ToolCall{
				ID:        tc.ID,
				Name:      tc.Name,
				Arguments: tc.Arguments,
			}
		}

		messages = append(messages, provider.Message{
			Role:       provider.MessageRole(msg.Role),
			Content:    msg.Content,
			ToolCalls:  providerToolCalls,
			ToolCallID: msg.ToolCallID,
		})
	}

	return messages
}
