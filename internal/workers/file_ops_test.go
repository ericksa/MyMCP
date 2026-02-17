package workers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadDir(t *testing.T) {
	tmpDir := t.TempDir()
	err := os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)
	require.NoError(t, err)
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello"), 0644)

	files, err := readDir(tmpDir)
	require.NoError(t, err)
	assert.Len(t, files, 2)
}

func TestReadFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("hello world"), 0644)
	require.NoError(t, err)

	data, err := readFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data))
}

func TestWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "subdir", "test.txt")

	err := writeFile(testFile, []byte("test content"))
	require.NoError(t, err)

	data, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(data))
}

func TestDeleteFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("hello"), 0644)

	err := deleteFile(testFile)
	require.NoError(t, err)

	_, err = os.ReadFile(testFile)
	assert.Error(t, err)
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"hello\nworld", []string{"hello", "world"}},
		{"hello", []string{"hello"}},
		{"", []string{}},
		{"a\nb\nc", []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		result := splitLines(tt.input)
		assert.Equal(t, tt.expected, result)
	}
}

func TestContains(t *testing.T) {
	assert.True(t, contains("hello world", "hello"))
	assert.True(t, contains("hello world", "world"))
	assert.False(t, contains("hello", "xyz"))
	assert.True(t, contains("a", "a"))
}
