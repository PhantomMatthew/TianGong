// Package tool provides tool interface and registry for agent tool calling.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Tool defines the interface for tools that can be invoked by the agent.
type Tool interface {
	// Name returns the unique tool name.
	Name() string
	// Description returns a human-readable description for the LLM.
	Description() string
	// Parameters returns the JSON schema for tool parameters.
	Parameters() map[string]any
	// Execute runs the tool with the given JSON arguments and returns the result.
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

// Registry is a concurrent-safe registry of tools.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates a new empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry. It returns an error if a tool
// with the same name is already registered.
func (r *Registry) Register(t Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := t.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = t
	return nil
}

// Get retrieves a tool by name. The second return value indicates
// whether the tool was found.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tools.
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// ListDefinitions returns tool definitions in the format expected by LLM providers.
// Each definition follows the OpenAI function calling schema.
func (r *Registry) ListDefinitions() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]map[string]any, 0, len(r.tools))
	for _, t := range r.tools {
		def := map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  t.Parameters(),
			},
		}
		defs = append(defs, def)
	}
	return defs
}

// NewDefaultRegistry creates a registry with the default set of tools.
// TODO: register bash, read, write tools after Tasks 9-11
func NewDefaultRegistry() *Registry {
	return NewRegistry()
}
