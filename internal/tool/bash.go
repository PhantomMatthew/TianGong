// Package tool provides tool interface and registry for agent tool calling.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	defaultTimeout = 30 * time.Second
	maxTimeout     = 120 * time.Second
	maxOutputSize  = 32 * 1024 // 32KB
)

// Bash is a tool that executes shell commands.
type Bash struct{}

// NewBash creates a new Bash tool.
func NewBash() *Bash {
	return &Bash{}
}

// Name returns the tool name.
func (b *Bash) Name() string {
	return "bash"
}

// Description returns a description for the LLM.
func (b *Bash) Description() string {
	return "Execute shell commands and capture output"
}

// Parameters returns the JSON schema for tool parameters.
func (b *Bash) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Shell command to execute",
			},
			"timeout_ms": map[string]any{
				"type":        "integer",
				"description": "Timeout in milliseconds (default 30000, max 120000)",
			},
		},
		"required": []string{"command"},
	}
}

// bashArgs represents the parsed arguments for the bash tool.
type bashArgs struct {
	Command   string `json:"command"`
	TimeoutMS int    `json:"timeout_ms"`
}

// Execute runs the shell command and returns the output.
func (b *Bash) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var ba bashArgs
	if err := json.Unmarshal(args, &ba); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if ba.Command == "" {
		return "", fmt.Errorf("command is required")
	}

	// Determine timeout
	timeout := defaultTimeout
	if ba.TimeoutMS > 0 {
		timeout = time.Duration(ba.TimeoutMS) * time.Millisecond
		if timeout > maxTimeout {
			timeout = maxTimeout
		}
	}

	// Create context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create and execute command
	cmd := exec.CommandContext(cmdCtx, "sh", "-c", ba.Command)
	output, err := cmd.CombinedOutput()

	// Check if context was exceeded (timeout)
	if cmdCtx.Err() == context.DeadlineExceeded {
		result := string(output)
		if len(result) > maxOutputSize {
			result = result[:maxOutputSize]
		}
		return result + fmt.Sprintf("\n[TIMEOUT after %v]", timeout), nil
	}

	// Convert output to string
	result := string(output)

	// Handle non-zero exit codes - extract exit code and include in output
	if err != nil {
		var exitCode int
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			}
		}
		// Return output with exit code (not as Go error)
		if exitCode > 0 {
			if len(result) > maxOutputSize {
				result = result[:maxOutputSize]
			}
			return result + fmt.Sprintf("\n[EXIT CODE: %d]", exitCode), nil
		}
	}

	// Truncate output if too large
	if len(result) > maxOutputSize {
		result = result[:maxOutputSize]
	}

	return strings.TrimRight(result, "\n"), nil
}
