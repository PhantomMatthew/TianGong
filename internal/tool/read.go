package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Read implements the Tool interface for reading files with offset/limit support.
type Read struct{}

// NewRead creates a new read tool.
func NewRead() *Read {
	return &Read{}
}

// Name returns the unique tool name.
func (r *Read) Name() string {
	return "read"
}

// Description returns a human-readable description for the LLM.
func (r *Read) Description() string {
	return "Read file or directory contents. For files, returns content with line numbers (format: '1: line content'). Supports offset and limit parameters. For directories, returns a listing of entries."
}

// Parameters returns the JSON schema for tool parameters.
func (r *Read) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file or directory to read",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Starting line number (0-indexed, default 0)",
				"default":     0,
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of lines to return (default 2000)",
				"default":     2000,
			},
		},
		"required": []string{"path"},
	}
}

// readArgs represents the parsed arguments for the read tool.
type readArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

// Execute reads a file or directory with the given parameters.
func (r *Read) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var readArgs readArgs
	if err := json.Unmarshal(args, &readArgs); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	// Validate path is provided
	if readArgs.Path == "" {
		return "", fmt.Errorf("path is required")
	}

	// Set defaults
	if readArgs.Limit == 0 {
		readArgs.Limit = 2000
	}
	if readArgs.Offset < 0 {
		readArgs.Offset = 0
	}

	// Check if path exists
	fi, err := os.Stat(readArgs.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", readArgs.Path)
		}
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied: %s", readArgs.Path)
		}
		return "", fmt.Errorf("cannot stat path: %w", err)
	}

	// Handle directories
	if fi.IsDir() {
		return r.readDirectory(readArgs.Path)
	}

	// Handle regular files
	return r.readFile(readArgs.Path, readArgs.Offset, readArgs.Limit)
}

// readFile reads a regular file and applies offset/limit.
func (r *Read) readFile(path string, offset, limit int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied: %s", path)
		}
		return "", fmt.Errorf("cannot read file: %w", err)
	}

	// Split into lines
	lines := strings.Split(string(data), "\n")

	// Apply offset and limit
	start := offset
	if start >= len(lines) {
		start = len(lines)
	}

	end := start + limit
	if end > len(lines) {
		end = len(lines)
	}

	// Extract slice
	selectedLines := lines[start:end]

	// Add line numbers (1-indexed for display)
	var result strings.Builder
	for i, line := range selectedLines {
		lineNum := start + i + 1
		result.WriteString(fmt.Sprintf("%d: %s\n", lineNum, line))
	}

	output := result.String()

	// Truncate to 64KB if necessary
	const maxSize = 64 * 1024
	if len(output) > maxSize {
		output = output[:maxSize]
	}

	return output, nil
}

// readDirectory reads a directory and returns its entries.
func (r *Read) readDirectory(path string) (string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied: %s", path)
		}
		return "", fmt.Errorf("cannot read directory: %w", err)
	}

	var result strings.Builder
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			result.WriteString(name + "/\n")
		} else {
			result.WriteString(name + "\n")
		}
	}

	return result.String(), nil
}
