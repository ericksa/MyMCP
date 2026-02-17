package workers

import (
	"io/fs"
	"os"
	"path/filepath"
)

type FileInfo struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
	Mode  string `json:"mode"`
}

func readDir(path string) ([]FileInfo, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var files []FileInfo
	for _, e := range entries {
		info, _ := e.Info()
		files = append(files, FileInfo{
			Name:  e.Name(),
			Size:  info.Size(),
			IsDir: e.IsDir(),
			Mode:  info.Mode().String(),
		})
	}
	return files, nil
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func writeFile(path string, data []byte) error {
	absPath := path
	if !filepath.IsAbs(path) {
		absPath = filepath.Join("$HOME", path)
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(absPath, data, 0644)
}

func deleteFile(path string) error {
	return os.Remove(path)
}

func searchInDir(root, pattern string) ([]map[string]string, error) {
	var results []map[string]string
	err := filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		for i, line := range splitLines(content) {
			if len(line) > 0 && contains(line, pattern) {
				results = append(results, map[string]string{
					"file":  path,
					"line":  string(rune(i + 1)),
					"match": line,
				})
			}
		}
		return nil
	})
	return results, err
}

func splitLines(s string) []string {
	if len(s) == 0 {
		return []string{}
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}

func contains(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
