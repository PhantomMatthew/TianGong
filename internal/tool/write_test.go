// Package tool provides tool interface and registry for agent tool calling.
package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteFile(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")

	w := NewWrite()
	content := "Hello, World!"

	// Create arguments
	args := map[string]any{
		"path":    filePath,
		"content": content,
	}
	argsJSON, _ := json.Marshal(args)

	// Execute the write tool
	result, err := w.Execute(context.Background(), argsJSON)

	// Verify no error
	assert.NoError(t, err)
	assert.Equal(t, "Wrote 13 bytes to "+filePath, result)

	// Verify file was created and content matches
	fileContent, _ := os.ReadFile(filePath)
	assert.Equal(t, content, string(fileContent))
}

func TestWriteCreatesDirectories(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "a", "b", "c", "test.txt")

	w := NewWrite()
	content := "nested directory test"

	// Create arguments
	args := map[string]any{
		"path":    nestedPath,
		"content": content,
	}
	argsJSON, _ := json.Marshal(args)

	// Execute the write tool
	result, err := w.Execute(context.Background(), argsJSON)

	// Verify no error
	assert.NoError(t, err)
	assert.Equal(t, "Wrote 21 bytes to "+nestedPath, result)

	// Verify nested directories were created
	fileContent, _ := os.ReadFile(nestedPath)
	assert.Equal(t, content, string(fileContent))

	// Verify intermediate directories exist
	assert.DirExists(t, filepath.Join(tmpDir, "a"))
	assert.DirExists(t, filepath.Join(tmpDir, "a", "b"))
	assert.DirExists(t, filepath.Join(tmpDir, "a", "b", "c"))
}

func TestWriteOverwrite(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "overwrite.txt")

	w := NewWrite()

	// First write
	firstContent := "first content"
	args1 := map[string]any{
		"path":    filePath,
		"content": firstContent,
	}
	argsJSON1, _ := json.Marshal(args1)
	_, err := w.Execute(context.Background(), argsJSON1)
	assert.NoError(t, err)

	// Verify first write
	fileContent, _ := os.ReadFile(filePath)
	assert.Equal(t, firstContent, string(fileContent))

	// Second write (overwrite)
	secondContent := "second content"
	args2 := map[string]any{
		"path":    filePath,
		"content": secondContent,
	}
	argsJSON2, _ := json.Marshal(args2)
	result, err := w.Execute(context.Background(), argsJSON2)
	assert.NoError(t, err)
	assert.Equal(t, "Wrote 14 bytes to "+filePath, result)

	// Verify second write overwrote the first
	fileContent, _ = os.ReadFile(filePath)
	assert.Equal(t, secondContent, string(fileContent))
}

func TestWritePermissionDenied(t *testing.T) {
	// Try to write to an invalid path
	w := NewWrite()

	// Use a path that is a directory (should fail)
	tmpDir := t.TempDir()

	args := map[string]any{
		"path":    tmpDir, // tmpDir is a directory
		"content": "test",
	}
	argsJSON, _ := json.Marshal(args)

	// Execute should return an error
	_, err := w.Execute(context.Background(), argsJSON)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path is directory")
}

func TestWriteEmptyPath(t *testing.T) {
	w := NewWrite()

	// Create arguments with empty path
	args := map[string]any{
		"path":    "",
		"content": "test",
	}
	argsJSON, _ := json.Marshal(args)

	// Execute should return an error
	_, err := w.Execute(context.Background(), argsJSON)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path is required")
}

func TestWriteInvalidJSON(t *testing.T) {
	w := NewWrite()

	// Create invalid JSON
	invalidJSON := []byte("{invalid json}")

	// Execute should return an error
	_, err := w.Execute(context.Background(), invalidJSON)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid arguments")
}

func TestWriteName(t *testing.T) {
	w := NewWrite()
	assert.Equal(t, "write", w.Name())
}

func TestWriteDescription(t *testing.T) {
	w := NewWrite()
	assert.Equal(t, "Write content to a file, creating parent directories if needed", w.Description())
}

func TestWriteParameters(t *testing.T) {
	w := NewWrite()
	params := w.Parameters()

	// Verify structure
	assert.Equal(t, "object", params["type"])
	assert.NotNil(t, params["properties"])
	assert.NotNil(t, params["required"])

	// Verify required fields
	required := params["required"].([]string)
	assert.Contains(t, required, "path")
	assert.Contains(t, required, "content")
}
