package workers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
)

type ToolDef struct {
	Name        string
	Description string
}

type FileIOWorker struct {
	basePath string
}

func NewFileIOWorker(basePath string) *FileIOWorker {
	return &FileIOWorker{basePath: basePath}
}

func (w *FileIOWorker) GetTools() []ToolDef {
	return []ToolDef{
		{Name: "list_directory", Description: "List files in a directory"},
		{Name: "read_file", Description: "Read contents of a file"},
		{Name: "write_file", Description: "Write content to a file"},
		{Name: "delete_file", Description: "Delete a file"},
		{Name: "search_file_contents", Description: "Search for text in files"},
	}
}

func (w *FileIOWorker) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "list_directory", "file_io_list_directory":
		return w.listDirectory(ctx, input)
	case "read_file", "file_io_read_file":
		return w.readFile(ctx, input)
	case "write_file", "file_io_write_file":
		return w.writeFile(ctx, input)
	case "delete_file", "file_io_delete_file":
		return w.deleteFile(ctx, input)
	case "search_file_contents", "file_io_search_file_contents":
		return w.searchFiles(ctx, input)
	default:
		return nil, nil
	}
}

func (w *FileIOWorker) listDirectory(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	absPath := w.resolvePath(req.Path)
	entries, err := os.ReadDir(absPath)
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
	return json.Marshal(files)
}

func (w *FileIOWorker) readFile(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(w.resolvePath(req.Path))
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"content": string(data)})
}

func (w *FileIOWorker) writeFile(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	absPath := w.resolvePath(req.Path)
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(absPath, []byte(req.Content), 0644); err != nil {
		return nil, err
	}
	return []byte(`{"success":true}`), nil
}

func (w *FileIOWorker) deleteFile(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	if err := os.Remove(w.resolvePath(req.Path)); err != nil {
		return nil, err
	}
	return []byte(`{"success":true}`), nil
}

func (w *FileIOWorker) searchFiles(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Path    string `json:"path"`
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	return []byte(`[]`), nil
}

func (w *FileIOWorker) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(w.basePath, path)
}
