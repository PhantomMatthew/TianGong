package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBashEcho(t *testing.T) {
	b := NewBash()
	args, _ := json.Marshal(map[string]any{
		"command": "echo hello",
	})

	result, err := b.Execute(context.Background(), args)
	assert.NoError(t, err)
	assert.Contains(t, result, "hello")
}

func TestBashExitCode(t *testing.T) {
	b := NewBash()
	args, _ := json.Marshal(map[string]any{
		"command": "exit 1",
	})

	result, err := b.Execute(context.Background(), args)
	assert.NoError(t, err) // Non-zero exit should not return Go error
	assert.Contains(t, result, "[EXIT CODE: 1]")
}

func TestBashTimeout(t *testing.T) {
	b := NewBash()
	args, _ := json.Marshal(map[string]any{
		"command":    "sleep 10",
		"timeout_ms": 100,
	})

	result, err := b.Execute(context.Background(), args)
	assert.NoError(t, err)
	assert.Contains(t, result, "[TIMEOUT after")
}

func TestBashCombinedOutput(t *testing.T) {
	b := NewBash()
	args, _ := json.Marshal(map[string]any{
		"command": "echo stdout && echo stderr >&2",
	})

	result, err := b.Execute(context.Background(), args)
	assert.NoError(t, err)
	assert.Contains(t, result, "stdout")
	assert.Contains(t, result, "stderr")
}

func TestBashParameters(t *testing.T) {
	b := NewBash()
	params := b.Parameters()
	assert.NotNil(t, params)
	assert.Equal(t, "object", params["type"])

	props, ok := params["properties"].(map[string]any)
	assert.True(t, ok)
	assert.NotNil(t, props["command"])
	assert.NotNil(t, props["timeout_ms"])

	required, ok := params["required"].([]string)
	assert.True(t, ok)
	assert.Contains(t, required, "command")
}

func TestBashName(t *testing.T) {
	b := NewBash()
	assert.Equal(t, "bash", b.Name())
}

func TestBashDescription(t *testing.T) {
	b := NewBash()
	desc := b.Description()
	assert.NotEmpty(t, desc)
	assert.Contains(t, strings.ToLower(desc), "command")
}

func TestBashMissingCommand(t *testing.T) {
	b := NewBash()
	args, _ := json.Marshal(map[string]any{})

	result, err := b.Execute(context.Background(), args)
	assert.Error(t, err)
	assert.Empty(t, result)
	assert.Contains(t, err.Error(), "required")
}

func TestBashInvalidJSON(t *testing.T) {
	b := NewBash()
	args := json.RawMessage(`{invalid}`)

	result, err := b.Execute(context.Background(), args)
	assert.Error(t, err)
	assert.Empty(t, result)
	assert.Contains(t, err.Error(), "invalid arguments")
}

func TestBashOutputTruncation(t *testing.T) {
	b := NewBash()
	// Generate output larger than maxOutputSize
	largeOutput := strings.Repeat("x", maxOutputSize+1000)
	command := "echo '" + largeOutput + "'"
	args, _ := json.Marshal(map[string]any{
		"command": command,
	})

	result, err := b.Execute(context.Background(), args)
	assert.NoError(t, err)
	assert.LessOrEqual(t, len(result), maxOutputSize)
}

func TestBashMaxTimeout(t *testing.T) {
	b := NewBash()
	// Request timeout larger than maxTimeout - should be capped
	args, _ := json.Marshal(map[string]any{
		"command":    "echo test",
		"timeout_ms": 500000, // Larger than maxTimeout (120s)
	})

	start := time.Now()
	result, err := b.Execute(context.Background(), args)
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.Contains(t, result, "test")
	// Should complete quickly (echo), not wait full maxTimeout
	assert.Less(t, elapsed, maxTimeout-1*time.Second)
}

func TestBashInterfaceImplementation(t *testing.T) {
	b := NewBash()
	var tool Tool = b
	assert.NotNil(t, tool)
	assert.Equal(t, "bash", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.NotNil(t, tool.Parameters())
}
