// Package provider provides LLM provider abstractions.
package provider

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"

	"github.com/PhantomMatthew/TianGong/internal/config"
)

// OpenAIProvider implements the Provider interface for OpenAI.
type OpenAIProvider struct {
	client openai.Client
	model  string
}

// NewOpenAI creates a new OpenAI provider from configuration.
// Returns an error if the API key is missing.
func NewOpenAI(cfg config.ProviderConfig) (*OpenAIProvider, error) {
	if cfg.APIKey == "" {
		return nil, ErrAuthentication
	}

	client := openai.NewClient(
		option.WithAPIKey(cfg.APIKey),
	)

	model := cfg.Model
	if model == "" {
		model = "gpt-4o"
	}

	return &OpenAIProvider{
		client: client,
		model:  model,
	}, nil
}

// Name returns the provider identifier.
func (p *OpenAIProvider) Name() string {
	return "openai"
}

// Chat sends a chat completion request and returns the full response.
func (p *OpenAIProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages))
	for _, msg := range req.Messages {
		switch msg.Role {
		case RoleSystem:
			messages = append(messages, openai.SystemMessage(msg.Content))
		case RoleUser:
			messages = append(messages, openai.UserMessage(msg.Content))
		case RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				// Assistant message with tool calls
				toolCalls := make([]openai.ChatCompletionMessageToolCallUnionParam, len(msg.ToolCalls))
				for i, tc := range msg.ToolCalls {
					toolCalls[i] = openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: tc.ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Name,
								Arguments: tc.Arguments,
							},
						},
					}
				}
				messages = append(messages, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						Content: openai.ChatCompletionAssistantMessageParamContentUnion{
							OfString: param.NewOpt(msg.Content),
						},
						ToolCalls: toolCalls,
					},
				})
			} else {
				messages = append(messages, openai.AssistantMessage(msg.Content))
			}
		case RoleTool:
			messages = append(messages, openai.ToolMessage(msg.ToolCallID, msg.Content))
		}
	}

	// Map tools
	var tools []openai.ChatCompletionToolUnionParam
	if len(req.Tools) > 0 {
		tools = make([]openai.ChatCompletionToolUnionParam, len(req.Tools))
		for i, t := range req.Tools {
			paramsJSON, err := json.Marshal(t.Parameters)
			if err != nil {
				continue
			}
			var parameters shared.FunctionParameters
			if err := json.Unmarshal(paramsJSON, &parameters); err != nil {
				continue
			}
			tools[i] = openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
				Name:        t.Name,
				Description: param.NewOpt(t.Description),
				Parameters:  parameters,
			})
		}
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(model),
		Messages: messages,
	}

	if len(tools) > 0 {
		params.Tools = tools
	}

	if req.MaxTokens > 0 {
		params.MaxTokens = param.NewOpt(int64(req.MaxTokens))
	}

	if req.Temperature != nil {
		params.Temperature = param.NewOpt(*req.Temperature)
	}

	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, p.mapError(err)
	}

	if len(resp.Choices) == 0 {
		return nil, &ProviderError{
			Provider: "openai",
			Message:  "no choices in response",
			Err:      ErrInvalidRequest,
		}
	}

	choice := resp.Choices[0]
	chatResp := &ChatResponse{
		ID:           resp.ID,
		Content:      choice.Message.Content,
		FinishReason: mapFinishReason(string(choice.FinishReason)),
		Usage: Usage{
			PromptTokens:     int(resp.Usage.PromptTokens),
			CompletionTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:      int(resp.Usage.TotalTokens),
		},
	}

	// Map tool calls
	if len(choice.Message.ToolCalls) > 0 {
		chatResp.ToolCalls = make([]ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			chatResp.ToolCalls[i] = ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			}
		}
	}

	return chatResp, nil
}

// ChatStream sends a streaming chat completion request.
func (p *OpenAIProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatChunk, error) {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages))
	for _, msg := range req.Messages {
		switch msg.Role {
		case RoleSystem:
			messages = append(messages, openai.SystemMessage(msg.Content))
		case RoleUser:
			messages = append(messages, openai.UserMessage(msg.Content))
		case RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				// Assistant message with tool calls
				toolCalls := make([]openai.ChatCompletionMessageToolCallUnionParam, len(msg.ToolCalls))
				for i, tc := range msg.ToolCalls {
					toolCalls[i] = openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: tc.ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Name,
								Arguments: tc.Arguments,
							},
						},
					}
				}
				messages = append(messages, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						Content: openai.ChatCompletionAssistantMessageParamContentUnion{
							OfString: param.NewOpt(msg.Content),
						},
						ToolCalls: toolCalls,
					},
				})
			} else {
				messages = append(messages, openai.AssistantMessage(msg.Content))
			}
		case RoleTool:
			messages = append(messages, openai.ToolMessage(msg.ToolCallID, msg.Content))
		}
	}

	var tools []openai.ChatCompletionToolUnionParam
	if len(req.Tools) > 0 {
		tools = make([]openai.ChatCompletionToolUnionParam, len(req.Tools))
		for i, t := range req.Tools {
			paramsJSON, err := json.Marshal(t.Parameters)
			if err != nil {
				continue
			}
			var parameters shared.FunctionParameters
			if err := json.Unmarshal(paramsJSON, &parameters); err != nil {
				continue
			}
			tools[i] = openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
				Name:        t.Name,
				Description: param.NewOpt(t.Description),
				Parameters:  parameters,
			})
		}
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(model),
		Messages: messages,
	}

	if len(tools) > 0 {
		params.Tools = tools
	}

	if req.MaxTokens > 0 {
		params.MaxTokens = param.NewOpt(int64(req.MaxTokens))
	}

	if req.Temperature != nil {
		params.Temperature = param.NewOpt(*req.Temperature)
	}

	stream := p.client.Chat.Completions.NewStreaming(ctx, params)

	ch := make(chan ChatChunk, 16)

	go func() {
		defer close(ch)

		// Accumulate tool calls across chunks
		toolCallAccumulator := make(map[int]*ToolCall)

		for stream.Next() {
			chunk := stream.Current()

			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]

			// Content delta
			if choice.Delta.Content != "" {
				ch <- ChatChunk{
					Delta: choice.Delta.Content,
					Done:  false,
				}
			}

			// Tool call deltas
			if len(choice.Delta.ToolCalls) > 0 {
				for _, tc := range choice.Delta.ToolCalls {
					idx := int(tc.Index)
					if _, exists := toolCallAccumulator[idx]; !exists {
						toolCallAccumulator[idx] = &ToolCall{
							ID:   tc.ID,
							Name: tc.Function.Name,
						}
					}
					// Accumulate arguments
					toolCallAccumulator[idx].Arguments += tc.Function.Arguments
				}
			}

			// Finish reason
			if choice.FinishReason != "" {
				// Send final chunk with accumulated tool calls
				finalToolCalls := make([]ToolCall, 0, len(toolCallAccumulator))
				for i := 0; i < len(toolCallAccumulator); i++ {
					if tc, exists := toolCallAccumulator[i]; exists {
						finalToolCalls = append(finalToolCalls, *tc)
					}
				}

				ch <- ChatChunk{
					Done:         true,
					ToolCalls:    finalToolCalls,
					FinishReason: mapFinishReason(string(choice.FinishReason)),
				}
				return
			}
		}

		// Check for stream error
		if err := stream.Err(); err != nil {
			ch <- ChatChunk{
				Error: p.mapError(err),
				Done:  true,
			}
			return
		}

		// No explicit finish reason - send done chunk
		ch <- ChatChunk{Done: true}
	}()

	return ch, nil
}

// mapError maps OpenAI errors to provider errors.
func (p *OpenAIProvider) mapError(err error) error {
	if err == nil {
		return nil
	}

	// Check for specific OpenAI error types
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 401:
			return &ProviderError{
				Provider: "openai",
				Code:     apiErr.Code,
				Message:  "authentication failed",
				Err:      ErrAuthentication,
			}
		case 429:
			return &ProviderError{
				Provider: "openai",
				Code:     apiErr.Code,
				Message:  "rate limit exceeded",
				Err:      ErrRateLimit,
			}
		case 400:
			// Check if it's a context length error
			if apiErr.Code == "context_length_exceeded" {
				return &ProviderError{
					Provider: "openai",
					Code:     apiErr.Code,
					Message:  "context length exceeded",
					Err:      ErrContextLength,
				}
			}
			return &ProviderError{
				Provider: "openai",
				Code:     apiErr.Code,
				Message:  apiErr.Message,
				Err:      ErrInvalidRequest,
			}
		default:
			return &ProviderError{
				Provider: "openai",
				Code:     apiErr.Code,
				Message:  apiErr.Message,
				Err:      err,
			}
		}
	}

	return &ProviderError{
		Provider: "openai",
		Message:  err.Error(),
		Err:      err,
	}
}

// mapFinishReason maps OpenAI finish reasons to our FinishReason type.
func mapFinishReason(reason string) FinishReason {
	switch reason {
	case "stop":
		return FinishReasonStop
	case "tool_calls", "function_call":
		return FinishReasonToolCalls
	case "length":
		return FinishReasonMaxTokens
	default:
		return FinishReasonStop
	}
}
