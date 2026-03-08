package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PhantomMatthew/TianGong/internal/agent"
	"github.com/PhantomMatthew/TianGong/internal/provider"
	"github.com/PhantomMatthew/TianGong/internal/session"
	"github.com/PhantomMatthew/TianGong/internal/tool"
)

// mockIntegrationProvider is a mock LLM provider for integration testing.
type mockIntegrationProvider struct {
	name      string
	responses []mockIntegrationResponse
	callIndex int
}

type mockIntegrationResponse struct {
	content      string
	toolCalls    []provider.ToolCall
	finishReason provider.FinishReason
	err          error
}

func (m *mockIntegrationProvider) Name() string {
	return m.name
}

func (m *mockIntegrationProvider) Chat(ctx context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
	return nil, fmt.Errorf("Chat not implemented in mock")
}

func (m *mockIntegrationProvider) ChatStream(ctx context.Context, req *provider.ChatRequest) (<-chan provider.ChatChunk, error) {
	if m.callIndex >= len(m.responses) {
		// No more responses configured
		ch := make(chan provider.ChatChunk, 1)
		ch <- provider.ChatChunk{
			Done:         true,
			FinishReason: provider.FinishReasonStop,
		}
		close(ch)
		return ch, nil
	}

	resp := m.responses[m.callIndex]
	m.callIndex++

	if resp.err != nil {
		return nil, resp.err
	}

	ch := make(chan provider.ChatChunk, 2)
	go func() {
		defer close(ch)

		// Send content delta if present
		if resp.content != "" {
			ch <- provider.ChatChunk{
				Delta: resp.content,
				Done:  false,
			}
		}

		// Send final chunk with finish reason and tool calls
		ch <- provider.ChatChunk{
			Done:         true,
			ToolCalls:    resp.toolCalls,
			FinishReason: resp.finishReason,
		}
	}()

	return ch, nil
}

// mockIntegrationTool is a mock tool for integration testing.
type mockIntegrationTool struct {
	name        string
	description string
	executions  []mockIntegrationExecution
	callIndex   int
}

type mockIntegrationExecution struct {
	result string
	err    error
}

func (m *mockIntegrationTool) Name() string {
	return m.name
}

func (m *mockIntegrationTool) Description() string {
	return m.description
}

func (m *mockIntegrationTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{"type": "string"},
		},
		"required": []string{"input"},
	}
}

func (m *mockIntegrationTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	if m.callIndex >= len(m.executions) {
		return "", fmt.Errorf("no execution configured for call %d", m.callIndex)
	}

	exec := m.executions[m.callIndex]
	m.callIndex++

	if exec.err != nil {
		return "", exec.err
	}
	return exec.result, nil
}

// TestFullPipeline tests the complete ReAct loop with tool execution.
func TestFullPipeline(t *testing.T) {
	// Setup: Mock provider that requests a tool, then gives final answer
	mockProv := &mockIntegrationProvider{
		name: "test-provider",
		responses: []mockIntegrationResponse{
			{
				// First response: request tool call
				content:      "",
				finishReason: provider.FinishReasonToolCalls,
				toolCalls: []provider.ToolCall{
					{
						ID:   "call_1",
						Name: "echo",
						Arguments: `{
							"input": "test message"
						}`,
					},
				},
			},
			{
				// Second response: final answer after tool execution
				content:      "The tool returned: 'echo: test message'",
				finishReason: provider.FinishReasonStop,
			},
		},
	}

	// Setup: Mock tool that echoes input
	mockTool := &mockIntegrationTool{
		name:        "echo",
		description: "Echo the input message",
		executions: []mockIntegrationExecution{
			{result: "echo: test message", err: nil},
		},
	}

	// Setup: Tool registry
	toolRegistry := tool.NewRegistry()
	toolRegistry.Register(mockTool)

	// Setup: In-memory session store
	store := session.NewMemoryStore()
	sess, err := store.CreateSession(context.Background(), "integration-test")
	require.NoError(t, err)

	// Setup: Agent
	a := agent.New(mockProv, toolRegistry, store, agent.AgentConfig{
		MaxIterations: 10,
	})

	// Execute: Run agent with user message
	var output bytes.Buffer
	err = a.RunStream(context.Background(), sess.ID, "Please use the echo tool with 'test message'", &output)
	require.NoError(t, err)

	// Verify: Output contains final answer
	outputStr := output.String()
	assert.Contains(t, outputStr, "The tool returned: 'echo: test message'")

	// Verify: Session has correct message sequence
	messages, err := store.GetMessages(context.Background(), sess.ID)
	require.NoError(t, err)

	// Expected message sequence:
	// 1. User message
	// 2. Assistant message with tool call
	// 3. Tool result message
	// 4. Assistant final answer
	require.GreaterOrEqual(t, len(messages), 4, "expected at least 4 messages in session")

	// Check user message
	assert.Equal(t, session.RoleUser, messages[0].Role)
	assert.Contains(t, messages[0].Content, "Please use the echo tool")

	// Check assistant tool call message
	assert.Equal(t, session.RoleAssistant, messages[1].Role)

	// Check tool result message
	assert.Equal(t, session.RoleTool, messages[2].Role)
	assert.Contains(t, messages[2].Content, "echo: test message")

	// Check final assistant message
	assert.Equal(t, session.RoleAssistant, messages[3].Role)
	assert.Contains(t, messages[3].Content, "The tool returned")

	// Verify: Tool was called once
	assert.Equal(t, 1, mockTool.callIndex, "tool should be called once")

	// Verify: Provider was called twice (tool call + final answer)
	assert.Equal(t, 2, mockProv.callIndex, "provider should be called twice")
}

// TestEmptyUserInput tests that empty input is handled gracefully.
func TestEmptyUserInput(t *testing.T) {
	// Setup: Provider that returns empty response
	mockProv := &mockIntegrationProvider{
		name: "test-provider",
		responses: []mockIntegrationResponse{
			{
				content:      "",
				finishReason: provider.FinishReasonStop,
			},
		},
	}
	toolRegistry := tool.NewRegistry()
	store := session.NewMemoryStore()

	sess, err := store.CreateSession(context.Background(), "empty-input-test")
	require.NoError(t, err)

	a := agent.New(mockProv, toolRegistry, store, agent.AgentConfig{})

	// Execute: Empty input should be handled gracefully (no crash)
	var output bytes.Buffer
	err = a.RunStream(context.Background(), sess.ID, "", &output)
	assert.NoError(t, err, "empty input should be handled gracefully")

	// Verify: Session has user message with empty content
	messages, err := store.GetMessages(context.Background(), sess.ID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(messages), 2, "should have user + assistant messages")

	// Check user message is stored even if empty
	assert.Equal(t, session.RoleUser, messages[0].Role)
	assert.Equal(t, "", messages[0].Content)

}

// TestProviderError tests that provider errors are propagated correctly.
func TestProviderError(t *testing.T) {
	mockProv := &mockIntegrationProvider{
		name: "test-provider",
		responses: []mockIntegrationResponse{
			{
				err: fmt.Errorf("provider error: rate limit exceeded"),
			},
		},
	}

	toolRegistry := tool.NewRegistry()
	store := session.NewMemoryStore()

	sess, err := store.CreateSession(context.Background(), "provider-error-test")
	require.NoError(t, err)

	a := agent.New(mockProv, toolRegistry, store, agent.AgentConfig{})

	var output bytes.Buffer
	err = a.RunStream(context.Background(), sess.ID, "test message", &output)
	assert.Error(t, err, "provider error should be propagated")
	assert.Contains(t, err.Error(), "rate limit exceeded", "error should contain provider error message")
}

// TestToolError tests that tool execution errors are fed back as tool result messages.
func TestToolError(t *testing.T) {
	// Setup: Mock provider that requests tool, then sees error result
	mockProv := &mockIntegrationProvider{
		name: "test-provider",
		responses: []mockIntegrationResponse{
			{
				// First response: request tool call
				content:      "",
				finishReason: provider.FinishReasonToolCalls,
				toolCalls: []provider.ToolCall{
					{
						ID:        "call_1",
						Name:      "failing-tool",
						Arguments: `{"input": "test"}`,
					},
				},
			},
			{
				// Second response: acknowledge error and give final answer
				content:      "I encountered an error with the tool.",
				finishReason: provider.FinishReasonStop,
			},
		},
	}

	// Setup: Mock tool that always fails
	mockTool := &mockIntegrationTool{
		name:        "failing-tool",
		description: "A tool that always fails",
		executions: []mockIntegrationExecution{
			{err: fmt.Errorf("tool execution failed: invalid input")},
		},
	}

	toolRegistry := tool.NewRegistry()
	toolRegistry.Register(mockTool)

	store := session.NewMemoryStore()
	sess, err := store.CreateSession(context.Background(), "tool-error-test")
	require.NoError(t, err)

	a := agent.New(mockProv, toolRegistry, store, agent.AgentConfig{})

	var output bytes.Buffer
	err = a.RunStream(context.Background(), sess.ID, "Please use the failing-tool", &output)
	require.NoError(t, err, "agent should handle tool error gracefully")

	// Verify: Session has tool error message
	messages, err := store.GetMessages(context.Background(), sess.ID)
	require.NoError(t, err)

	// Find tool result message
	var toolResultMsg *session.Message
	for _, msg := range messages {
		if msg.Role == session.RoleTool {
			toolResultMsg = msg
			break
		}
	}

	require.NotNil(t, toolResultMsg, "should have tool result message")
	assert.Contains(t, toolResultMsg.Content, "Error:", "tool result should contain error message")
	assert.Contains(t, toolResultMsg.Content, "tool execution failed", "tool result should contain error details")

	// Verify: Provider received the error message in next call
	assert.Equal(t, 2, mockProv.callIndex, "provider should be called twice")
}

// TestMaxIterations tests that exceeding max iterations returns an error.
func TestMaxIterations(t *testing.T) {
	// Setup: Mock provider that always requests tools (infinite loop)
	mockProv := &mockIntegrationProvider{
		name: "test-provider",
		responses: []mockIntegrationResponse{
			// Repeat tool call response multiple times
			{
				content:      "",
				finishReason: provider.FinishReasonToolCalls,
				toolCalls: []provider.ToolCall{
					{
						ID:        "call_1",
						Name:      "loop-tool",
						Arguments: `{"input": "test"}`,
					},
				},
			},
			{
				content:      "",
				finishReason: provider.FinishReasonToolCalls,
				toolCalls: []provider.ToolCall{
					{
						ID:        "call_2",
						Name:      "loop-tool",
						Arguments: `{"input": "test"}`,
					},
				},
			},
			{
				content:      "",
				finishReason: provider.FinishReasonToolCalls,
				toolCalls: []provider.ToolCall{
					{
						ID:        "call_3",
						Name:      "loop-tool",
						Arguments: `{"input": "test"}`,
					},
				},
			},
			{
				content:      "",
				finishReason: provider.FinishReasonToolCalls,
				toolCalls: []provider.ToolCall{
					{
						ID:        "call_4",
						Name:      "loop-tool",
						Arguments: `{"input": "test"}`,
					},
				},
			},
		},
	}

	// Setup: Mock tool that always succeeds
	mockTool := &mockIntegrationTool{
		name:        "loop-tool",
		description: "A tool that causes loops",
		executions: []mockIntegrationExecution{
			{result: "ok", err: nil},
			{result: "ok", err: nil},
			{result: "ok", err: nil},
			{result: "ok", err: nil},
		},
	}

	toolRegistry := tool.NewRegistry()
	toolRegistry.Register(mockTool)

	store := session.NewMemoryStore()
	sess, err := store.CreateSession(context.Background(), "max-iterations-test")
	require.NoError(t, err)

	// Set max iterations to 3
	a := agent.New(mockProv, toolRegistry, store, agent.AgentConfig{
		MaxIterations: 3,
	})

	var output bytes.Buffer
	err = a.RunStream(context.Background(), sess.ID, "Start the loop", &output)
	assert.Error(t, err, "should return error when max iterations exceeded")
	assert.Contains(t, err.Error(), "max iterations", "error should mention max iterations")
}

// TestSessionNotFound tests that agent handles missing session gracefully.
func TestSessionNotFound(t *testing.T) {
	mockProv := &mockIntegrationProvider{name: "test-provider"}
	toolRegistry := tool.NewRegistry()
	store := session.NewMemoryStore()

	a := agent.New(mockProv, toolRegistry, store, agent.AgentConfig{})

	// Use non-existent session ID
	var output bytes.Buffer
	err := a.RunStream(context.Background(), "non-existent-session", "test message", &output)
	assert.Error(t, err, "should return error for non-existent session")
}
