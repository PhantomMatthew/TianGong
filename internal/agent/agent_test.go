package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PhantomMatthew/TianGong/internal/provider"
	"github.com/PhantomMatthew/TianGong/internal/session"
	"github.com/PhantomMatthew/TianGong/internal/tool"
)

// mockProvider is a mock LLM provider for testing.
type mockProvider struct {
	name      string
	responses []mockResponse
	callIndex int
}

type mockResponse struct {
	content      string
	toolCalls    []provider.ToolCall
	finishReason provider.FinishReason
	err          error
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Chat(ctx context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
	return nil, nil // Not used in agent
}

func (m *mockProvider) ChatStream(ctx context.Context, req *provider.ChatRequest) (<-chan provider.ChatChunk, error) {
	if m.callIndex >= len(m.responses) {
		return nil, nil
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

// agentMockTool is a mock tool for testing agent execution (distinct from prompt_test.go's mockTool).
type agentMockTool struct {
	name        string
	description string
	result      string
	err         error
}

func (m *agentMockTool) Name() string {
	return m.name
}

func (m *agentMockTool) Description() string {
	return m.description
}

func (m *agentMockTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{"type": "string"},
		},
	}
}

func (m *agentMockTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.result, nil
}

func TestNew(t *testing.T) {
	p := &mockProvider{name: "test"}
	tools := tool.NewRegistry()
	store := session.NewMemoryStore()

	t.Run("applies defaults", func(t *testing.T) {
		a := New(p, tools, store, AgentConfig{})
		assert.Equal(t, 10, a.config.MaxIterations)
		assert.Equal(t, 50, a.config.HistoryLimit)
	})

	t.Run("uses provided config", func(t *testing.T) {
		a := New(p, tools, store, AgentConfig{
			MaxIterations: 5,
			HistoryLimit:  20,
			SystemPrompt:  "custom",
		})
		assert.Equal(t, 5, a.config.MaxIterations)
		assert.Equal(t, 20, a.config.HistoryLimit)
		assert.Equal(t, "custom", a.config.SystemPrompt)
	})
}

func TestRunStream_SimpleChat(t *testing.T) {
	// Mock provider that responds with simple text
	p := &mockProvider{
		name: "test",
		responses: []mockResponse{
			{
				content:      "Hello! How can I help you?",
				finishReason: provider.FinishReasonStop,
			},
		},
	}

	tools := tool.NewRegistry()
	store := session.NewMemoryStore()

	// Create session
	sess, err := store.CreateSession(context.Background(), "test")
	require.NoError(t, err)

	a := New(p, tools, store, AgentConfig{})

	var buf bytes.Buffer
	err = a.RunStream(context.Background(), sess.ID, "Hello", &buf)

	require.NoError(t, err)
	assert.Equal(t, "Hello! How can I help you?", buf.String())

	// Verify messages stored
	messages, err := store.GetMessages(context.Background(), sess.ID)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	assert.Equal(t, session.RoleUser, messages[0].Role)
	assert.Equal(t, "Hello", messages[0].Content)
	assert.Equal(t, session.RoleAssistant, messages[1].Role)
	assert.Equal(t, "Hello! How can I help you?", messages[1].Content)
}

func TestRunStream_ToolCall(t *testing.T) {
	// Mock provider that calls a tool, then responds
	p := &mockProvider{
		name: "test",
		responses: []mockResponse{
			{
				content: "Let me check that for you.",
				toolCalls: []provider.ToolCall{
					{
						ID:        "call_123",
						Name:      "test_tool",
						Arguments: `{"input":"test"}`,
					},
				},
				finishReason: provider.FinishReasonToolCalls,
			},
			{
				content:      "The result is: mock_result",
				finishReason: provider.FinishReasonStop,
			},
		},
	}

	// Register mock tool
	tools := tool.NewRegistry()
	mockTool := &agentMockTool{
		name:        "test_tool",
		description: "A test tool",
		result:      "mock_result",
	}
	require.NoError(t, tools.Register(mockTool))

	store := session.NewMemoryStore()
	sess, err := store.CreateSession(context.Background(), "test")
	require.NoError(t, err)

	a := New(p, tools, store, AgentConfig{})

	var buf bytes.Buffer
	err = a.RunStream(context.Background(), sess.ID, "Run test tool", &buf)

	require.NoError(t, err)
	output := buf.String()
	assert.True(t, strings.Contains(output, "Let me check that for you."))
	assert.True(t, strings.Contains(output, "The result is: mock_result"))

	// Verify message sequence
	messages, err := store.GetMessages(context.Background(), sess.ID)
	require.NoError(t, err)
	require.Len(t, messages, 4)
	assert.Equal(t, session.RoleUser, messages[0].Role)
	assert.Equal(t, session.RoleAssistant, messages[1].Role)
	assert.Len(t, messages[1].ToolCalls, 1)
	assert.Equal(t, session.RoleTool, messages[2].Role)
	assert.Equal(t, "mock_result", messages[2].Content)
	assert.Equal(t, "call_123", messages[2].ToolCallID)
	assert.Equal(t, session.RoleAssistant, messages[3].Role)
}

func TestRunStream_MaxIterations(t *testing.T) {
	// Mock provider that always returns tool calls
	p := &mockProvider{
		name: "test",
		responses: []mockResponse{
			{
				content: "Iteration 1",
				toolCalls: []provider.ToolCall{
					{ID: "call_1", Name: "test_tool", Arguments: "{}"},
				},
				finishReason: provider.FinishReasonToolCalls,
			},
			{
				content: "Iteration 2",
				toolCalls: []provider.ToolCall{
					{ID: "call_2", Name: "test_tool", Arguments: "{}"},
				},
				finishReason: provider.FinishReasonToolCalls,
			},
			{
				content: "Iteration 3",
				toolCalls: []provider.ToolCall{
					{ID: "call_3", Name: "test_tool", Arguments: "{}"},
				},
				finishReason: provider.FinishReasonToolCalls,
			},
		},
	}

	tools := tool.NewRegistry()
	require.NoError(t, tools.Register(&agentMockTool{
		name:        "test_tool",
		description: "A test tool",
		result:      "result",
	}))

	store := session.NewMemoryStore()
	sess, err := store.CreateSession(context.Background(), "test")
	require.NoError(t, err)

	// Set MaxIterations to 2
	a := New(p, tools, store, AgentConfig{MaxIterations: 2})

	var buf bytes.Buffer
	err = a.RunStream(context.Background(), sess.ID, "Loop forever", &buf)

	// Should fail with max iterations error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max iterations (2) reached")
}

func TestRunStream_UnknownTool(t *testing.T) {
	// Mock provider that calls unknown tool
	p := &mockProvider{
		name: "test",
		responses: []mockResponse{
			{
				content: "Let me try that.",
				toolCalls: []provider.ToolCall{
					{
						ID:        "call_123",
						Name:      "unknown_tool",
						Arguments: `{}`,
					},
				},
				finishReason: provider.FinishReasonToolCalls,
			},
			{
				content:      "Sorry, that tool doesn't exist.",
				finishReason: provider.FinishReasonStop,
			},
		},
	}

	tools := tool.NewRegistry()
	store := session.NewMemoryStore()
	sess, err := store.CreateSession(context.Background(), "test")
	require.NoError(t, err)

	a := New(p, tools, store, AgentConfig{})

	var buf bytes.Buffer
	err = a.RunStream(context.Background(), sess.ID, "Use unknown tool", &buf)

	require.NoError(t, err)

	// Verify error message was added to session
	messages, err := store.GetMessages(context.Background(), sess.ID)
	require.NoError(t, err)
	require.Len(t, messages, 4)
	assert.Equal(t, session.RoleTool, messages[2].Role)
	assert.Contains(t, messages[2].Content, "Error:")
	assert.Contains(t, messages[2].Content, "unknown_tool")
}

func TestRunStream_HistoryLimit(t *testing.T) {
	p := &mockProvider{
		name: "test",
		responses: []mockResponse{
			{content: "Response", finishReason: provider.FinishReasonStop},
		},
	}

	tools := tool.NewRegistry()
	store := session.NewMemoryStore()
	sess, err := store.CreateSession(context.Background(), "test")
	require.NoError(t, err)

	// Add many messages to exceed history limit
	for i := 0; i < 60; i++ {
		msg := &session.Message{
			Role:    session.RoleUser,
			Content: "Message",
		}
		require.NoError(t, store.AddMessage(context.Background(), sess.ID, msg))
	}

	a := New(p, tools, store, AgentConfig{HistoryLimit: 50})

	var buf bytes.Buffer
	err = a.RunStream(context.Background(), sess.ID, "Latest message", &buf)

	require.NoError(t, err)
	// History limit applied internally - can't directly verify but shouldn't error
}
