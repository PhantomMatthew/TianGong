package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadFile(t *testing.T) {
	// Create a temporary file with known content
	tmpFile, err := os.CreateTemp("", "test-read-*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	content := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	// Execute read tool
	tool := NewRead()
	args, err := json.Marshal(map[string]any{
		"path":   tmpFile.Name(),
		"offset": 0,
		"limit":  2000,
	})
	require.NoError(t, err)

	result, err := tool.Execute(context.Background(), args)
	assert.NoError(t, err)

	// Verify output format: "1: line 1\n2: line 2\n..."
	assert.Contains(t, result, "1: line 1")
	assert.Contains(t, result, "2: line 2")
	assert.Contains(t, result, "3: line 3")
	assert.Contains(t, result, "4: line 4")
	assert.Contains(t, result, "5: line 5")
}

func TestReadFileWithOffset(t *testing.T) {
	// Create a temporary file with known content
	tmpFile, err := os.CreateTemp("", "test-read-offset-*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	content := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	// Execute read tool with offset=2, limit=2
	tool := NewRead()
	args, err := json.Marshal(map[string]any{
		"path":   tmpFile.Name(),
		"offset": 2,
		"limit":  2,
	})
	require.NoError(t, err)

	result, err := tool.Execute(context.Background(), args)
	assert.NoError(t, err)

	// Should return lines 3 and 4 (0-indexed offset 2 = line 3)
	assert.Contains(t, result, "3: line 3")
	assert.Contains(t, result, "4: line 4")
	assert.NotContains(t, result, "1: line 1")
	assert.NotContains(t, result, "2: line 2")
	assert.NotContains(t, result, "5: line 5")
}

func TestReadDirectory(t *testing.T) {
	// Create a temporary directory with files and subdirectories
	tmpDir, err := os.MkdirTemp("", "test-read-dir-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test files
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("test"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("test"), 0o644))

	// Create test subdirectory
	subDir := filepath.Join(tmpDir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0o755))

	// Execute read tool on directory
	tool := NewRead()
	args, err := json.Marshal(map[string]any{
		"path": tmpDir,
	})
	require.NoError(t, err)

	result, err := tool.Execute(context.Background(), args)
	assert.NoError(t, err)

	// Verify output contains files and directories
	assert.Contains(t, result, "file1.txt")
	assert.Contains(t, result, "file2.txt")
	assert.Contains(t, result, "subdir/")
}

func TestReadNotFound(t *testing.T) {
	tool := NewRead()
	args, err := json.Marshal(map[string]any{
		"path": "/nonexistent/path/to/file.txt",
	})
	require.NoError(t, err)

	result, err := tool.Execute(context.Background(), args)
	assert.Error(t, err)
	assert.Equal(t, "", result)
	assert.Contains(t, err.Error(), "file not found")
}

func TestReadMissingPath(t *testing.T) {
	tool := NewRead()
	args, err := json.Marshal(map[string]any{})
	require.NoError(t, err)

	result, err := tool.Execute(context.Background(), args)
	assert.Error(t, err)
	assert.Equal(t, "", result)
	assert.Contains(t, err.Error(), "path is required")
}

func TestReadFileLineNumbering(t *testing.T) {
	// Verify line numbers are 1-indexed in output
	tmpFile, err := os.CreateTemp("", "test-line-nums-*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	content := "first\nsecond\nthird\n"
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	tool := NewRead()
	args, err := json.Marshal(map[string]any{
		"path": tmpFile.Name(),
	})
	require.NoError(t, err)

	result, err := tool.Execute(context.Background(), args)
	assert.NoError(t, err)

	// Verify line numbers start at 1
	assert.Contains(t, result, "1: first")
	assert.Contains(t, result, "2: second")
	assert.Contains(t, result, "3: third")
}

func TestReadToolInterface(t *testing.T) {
	tool := NewRead()

	// Test Name
	assert.Equal(t, "read", tool.Name())

	// Test Description
	desc := tool.Description()
	assert.NotEmpty(t, desc)

	// Test Parameters
	params := tool.Parameters()
	assert.NotNil(t, params)
	assert.Equal(t, "object", params["type"])

	// Verify required fields
	props, ok := params["properties"].(map[string]any)
	assert.True(t, ok)
	assert.Contains(t, props, "path")
	assert.Contains(t, props, "offset")
	assert.Contains(t, props, "limit")

	// Verify required list
	required, ok := params["required"].([]string)
	assert.True(t, ok)
	assert.Contains(t, required, "path")
}

func TestReadDefaultLimit(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-default-*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Create a file with many lines
	var content string
	for i := 1; i <= 3000; i++ {
		content += "line " + string(rune(i)) + "\n"
	}
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	tool := NewRead()
	args, err := json.Marshal(map[string]any{
		"path": tmpFile.Name(),
		// offset and limit not specified
	})
	require.NoError(t, err)

	result, err := tool.Execute(context.Background(), args)
	assert.NoError(t, err)
	assert.NotEmpty(t, result)

	// Should respect default limit of 2000 lines (approximately)
	// Output size should be reasonable
	assert.Less(t, len(result), 100*1024) // Less than 100KB as sanity check
}

func TestReadOffsetBeyondFileLength(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-offset-beyond-*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	content := "line 1\nline 2\nline 3\n"
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	tool := NewRead()
	args, err := json.Marshal(map[string]any{
		"path":   tmpFile.Name(),
		"offset": 100,
		"limit":  10,
	})
	require.NoError(t, err)

	result, err := tool.Execute(context.Background(), args)
	assert.NoError(t, err)
	// Should return empty result without error
	assert.Equal(t, "", result)
}

func TestReadEmptyFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-empty-*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	tmpFile.Close()

	tool := NewRead()
	args, err := json.Marshal(map[string]any{
		"path": tmpFile.Name(),
	})
	require.NoError(t, err)

	result, err := tool.Execute(context.Background(), args)
	assert.NoError(t, err)
	// Empty file should return only the empty line at the end
	assert.Equal(t, "1: \n", result)
}
