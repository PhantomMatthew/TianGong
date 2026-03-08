// Package tool provides tool interface and registry for agent tool calling.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Write is a tool that writes content to files with automatic directory creation.
type Write struct{}

// NewWrite creates a new Write tool.
func NewWrite() *Write {
	return &Write{}
}

// Name returns the tool name.
func (w *Write) Name() string {
	return "write"
}

// Description returns a description for the LLM.
func (w *Write) Description() string {
	return "Write content to a file, creating parent directories if needed"
}

// Parameters returns the JSON schema for tool parameters.
func (w *Write) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "File path to write to",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write to the file",
			},
		},
		"required": []string{"path", "content"},
	}
}

// writeArgs represents the parsed arguments for the write tool.
type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Execute writes content to the specified file, creating parent directories if needed.
func (w *Write) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var wa writeArgs
	if err := json.Unmarshal(args, &wa); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if wa.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	// Check if path points to an existing directory
	info, err := os.Stat(wa.Path)
	if err == nil && info.IsDir() {
		return "", fmt.Errorf("path is directory")
	}

	// Create parent directories if they don't exist
	dir := filepath.Dir(wa.Path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("permission denied: %w", err)
		}
	}

	// Write content to file with mode 0644
	if err := os.WriteFile(wa.Path, []byte(wa.Content), 0644); err != nil {
		return "", fmt.Errorf("permission denied: %w", err)
	}

	// Return confirmation message
	bytesWritten := len(wa.Content)
	return fmt.Sprintf("Wrote %d bytes to %s", bytesWritten, wa.Path), nil
}
