// Package agent provides AI agent orchestration with tool calling.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/PhantomMatthew/TianGong/internal/provider"
	"github.com/PhantomMatthew/TianGong/internal/session"
	"github.com/PhantomMatthew/TianGong/internal/tool"
)

// AgentConfig holds agent configuration.
type AgentConfig struct {
	// MaxIterations is the maximum number of ReAct loop iterations (default: 10).
	MaxIterations int
	// HistoryLimit is the maximum number of messages to keep in context (default: 50).
	HistoryLimit int
	// SystemPrompt is an optional custom system prompt (uses DefaultSystemPrompt if empty).
	SystemPrompt string
}

// Agent orchestrates LLM interactions with tool calling using a ReAct loop.
type Agent struct {
	provider provider.Provider
	tools    *tool.Registry
	store    session.SessionStore
	config   AgentConfig
}

// New creates a new Agent with the given provider, tools, store, and configuration.
// If config values are zero, defaults are applied: MaxIterations=10, HistoryLimit=50.
func New(p provider.Provider, tools *tool.Registry, store session.SessionStore, cfg AgentConfig) *Agent {
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 10
	}
	if cfg.HistoryLimit <= 0 {
		cfg.HistoryLimit = 50
	}

	return &Agent{
		provider: p,
		tools:    tools,
		store:    store,
		config:   cfg,
	}
}

// RunStream processes a user message and streams the response to the writer.
// It implements a ReAct loop: Reason -> Act -> Observe until completion or max iterations.
// The loop handles tool calls sequentially and adds results back to the conversation.
func (a *Agent) RunStream(ctx context.Context, sessionID, userMessage string, w io.Writer) error {
	// Add user message to session
	userMsg := &session.Message{
		Role:    session.RoleUser,
		Content: userMessage,
	}
	if err := a.store.AddMessage(ctx, sessionID, userMsg); err != nil {
		return fmt.Errorf("failed to add user message: %w", err)
	}

	// ReAct loop with iteration guard
	for iteration := 0; iteration < a.config.MaxIterations; iteration++ {
		slog.Info("agent react iteration", "iteration", iteration, "session_id", sessionID)

		// Get conversation history
		history, err := a.store.GetMessages(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("failed to get history: %w", err)
		}

		// Apply history limit (keep last N messages)
		if len(history) > a.config.HistoryLimit {
			history = history[len(history)-a.config.HistoryLimit:]
		}

		// Build system prompt with tools
		toolList := a.tools.List()
		systemPrompt := FormatSystemPrompt(a.config.SystemPrompt, toolList)

		// Build provider messages
		messages := BuildMessages(systemPrompt, history)

		// Build tool definitions
		var toolDefs []provider.ToolDefinition
		if len(toolList) > 0 {
			toolDefs = make([]provider.ToolDefinition, len(toolList))
			for i, t := range toolList {
				toolDefs[i] = provider.ToolDefinition{
					Name:        t.Name(),
					Description: t.Description(),
					Parameters:  t.Parameters(),
				}
			}
		}

		// Call provider streaming
		req := &provider.ChatRequest{
			Messages: messages,
			Tools:    toolDefs,
		}

		stream, err := a.provider.ChatStream(ctx, req)
		if err != nil {
			return fmt.Errorf("provider stream error: %w", err)
		}

		// Consume stream and collect response
		var assistantContent string
		var toolCalls []provider.ToolCall
		var finishReason provider.FinishReason

		for chunk := range stream {
			if chunk.Error != nil {
				return fmt.Errorf("stream chunk error: %w", chunk.Error)
			}

			// Stream text deltas to writer
			if chunk.Delta != "" {
				assistantContent += chunk.Delta
				if _, err := w.Write([]byte(chunk.Delta)); err != nil {
					return fmt.Errorf("failed to write delta: %w", err)
				}
			}

			// Accumulate tool calls
			if len(chunk.ToolCalls) > 0 {
				toolCalls = chunk.ToolCalls
			}

			// Check finish reason
			if chunk.Done {
				finishReason = chunk.FinishReason
				break
			}
		}

		slog.Info("stream finished", "finish_reason", finishReason, "tool_calls", len(toolCalls))

		// Add assistant message to session
		assistantMsg := &session.Message{
			Role:    session.RoleAssistant,
			Content: assistantContent,
		}

		// Convert provider tool calls to session tool calls
		if len(toolCalls) > 0 {
			assistantMsg.ToolCalls = make([]session.ToolCall, len(toolCalls))
			for i, tc := range toolCalls {
				assistantMsg.ToolCalls[i] = session.ToolCall{
					ID:        tc.ID,
					Name:      tc.Name,
					Arguments: tc.Arguments,
				}
			}
		}

		if err := a.store.AddMessage(ctx, sessionID, assistantMsg); err != nil {
			return fmt.Errorf("failed to add assistant message: %w", err)
		}

		// Handle finish reasons
		switch finishReason {
		case provider.FinishReasonStop:
			// Normal completion - done
			slog.Info("agent completed successfully", "iterations", iteration+1)
			return nil

		case provider.FinishReasonToolCalls:
			// Execute tools sequentially
			slog.Info("executing tool calls", "count", len(toolCalls))

			for _, tc := range toolCalls {
				result, err := a.executeTool(ctx, tc)
				if err != nil {
					// Add error as tool result - let LLM handle it
					slog.Warn("tool execution failed", "tool", tc.Name, "error", err)
					result = fmt.Sprintf("Error: %s", err.Error())
				}

				// Add tool result message
				toolMsg := &session.Message{
					Role:       session.RoleTool,
					Content:    result,
					ToolCallID: tc.ID,
				}
				if err := a.store.AddMessage(ctx, sessionID, toolMsg); err != nil {
					return fmt.Errorf("failed to add tool result: %w", err)
				}

				slog.Info("tool executed", "tool", tc.Name, "result_length", len(result))
			}

			// Continue loop to get next response

		case provider.FinishReasonMaxTokens:
			// Token limit reached - treat as completion
			slog.Warn("response truncated due to max tokens")
			return nil

		default:
			// Unknown finish reason - treat as completion
			slog.Warn("unknown finish reason", "reason", finishReason)
			return nil
		}
	}

	// Max iterations reached
	return fmt.Errorf("max iterations (%d) reached without completion", a.config.MaxIterations)
}

// executeTool executes a single tool call and returns the result.
// If the tool is not found, returns an error message.
func (a *Agent) executeTool(ctx context.Context, tc provider.ToolCall) (string, error) {
	t, ok := a.tools.Get(tc.Name)
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", tc.Name)
	}

	// Parse arguments as JSON
	var args json.RawMessage
	if tc.Arguments != "" {
		args = json.RawMessage(tc.Arguments)
	}

	result, err := t.Execute(ctx, args)
	if err != nil {
		return "", fmt.Errorf("tool execution error: %w", err)
	}

	return result, nil
}
