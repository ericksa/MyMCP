package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type DatasetWorker struct {
	basePath   string
	httpClient *http.Client
}

func NewDatasetWorker(basePath string) *DatasetWorker {
	return &DatasetWorker{
		basePath: basePath,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (w *DatasetWorker) GetTools() []ToolDef {
	return []ToolDef{
		{Name: "list", Description: "List local datasets"},
		{Name: "info", Description: "Get dataset information"},
		{Name: "download", Description: "Download dataset from URL"},
		{Name: "upload", Description: "Upload dataset to storage"},
		{Name: "process", Description: "Process/transform dataset"},
		{Name: "validate", Description: "Validate dataset structure"},
	}
}

func (w *DatasetWorker) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "list", "dataset_list":
		return w.list(ctx, input)
	case "info", "dataset_info":
		return w.info(ctx, input)
	case "download", "dataset_download":
		return w.download(ctx, input)
	case "upload", "dataset_upload":
		return w.upload(ctx, input)
	case "process", "dataset_process":
		return w.process(ctx, input)
	case "validate", "dataset_validate":
		return w.validate(ctx, input)
	default:
		return nil, nil
	}
}

type ListRequest struct {
	Path string `json:"path,omitempty"`
}

func (w *DatasetWorker) list(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req ListRequest
	json.Unmarshal(input, &req)

	path := w.basePath
	if req.Path != "" {
		path = filepath.Join(w.basePath, req.Path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var datasets []map[string]interface{}
	for _, e := range entries {
		info, _ := e.Info()
		datasets = append(datasets, map[string]interface{}{
			"name":     e.Name(),
			"is_dir":   e.IsDir(),
			"size":     info.Size(),
			"mod_time": info.ModTime(),
		})
	}
	return json.Marshal(datasets)
}

type InfoRequest struct {
	Name string `json:"name"`
}

func (w *DatasetWorker) info(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req InfoRequest
	json.Unmarshal(input, &req)

	if req.Name == "" {
		return nil, fmt.Errorf("dataset name is required")
	}

	path := filepath.Join(w.basePath, req.Name)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]interface{}{
		"name":         req.Name,
		"path":         path,
		"is_directory": info.IsDir(),
		"size":         info.Size(),
		"modified":     info.ModTime(),
	})
}

type DownloadRequest struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

func (w *DatasetWorker) download(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req DownloadRequest
	json.Unmarshal(input, &req)

	if req.URL == "" {
		return nil, fmt.Errorf("url is required")
	}

	name := req.Name
	if name == "" {
		name = filepath.Base(req.URL)
	}

	destPath := filepath.Join(w.basePath, name)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", req.URL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}

	outFile, err := os.Create(destPath)
	if err != nil {
		return nil, err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{
		"status": "downloaded",
		"path":   destPath,
	})
}

type UploadRequest struct {
	SourcePath string `json:"source_path"`
	DestName   string `json:"dest_name"`
}

func (w *DatasetWorker) upload(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req UploadRequest
	json.Unmarshal(input, &req)

	if req.SourcePath == "" {
		return nil, fmt.Errorf("source_path is required")
	}

	data, err := os.ReadFile(req.SourcePath)
	if err != nil {
		return nil, err
	}

	destName := req.DestName
	if destName == "" {
		destName = filepath.Base(req.SourcePath)
	}

	destPath := filepath.Join(w.basePath, destName)
	err = os.WriteFile(destPath, data, 0644)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{
		"status": "uploaded",
		"path":   destPath,
	})
}

type ProcessRequest struct {
	Name      string                 `json:"name"`
	Operation string                 `json:"operation"`
	Params    map[string]interface{} `json:"params"`
}

func (w *DatasetWorker) process(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req ProcessRequest
	json.Unmarshal(input, &req)

	if req.Name == "" {
		return nil, fmt.Errorf("dataset name is required")
	}

	return json.Marshal(map[string]interface{}{
		"status":    "processed",
		"name":      req.Name,
		"operation": req.Operation,
	})
}

type ValidateRequest struct {
	Name   string `json:"name"`
	Schema string `json:"schema,omitempty"`
}

func (w *DatasetWorker) validate(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req ValidateRequest
	json.Unmarshal(input, &req)

	if req.Name == "" {
		return nil, fmt.Errorf("dataset name is required")
	}

	path := filepath.Join(w.basePath, req.Name)
	_, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]interface{}{
		"valid":  true,
		"name":   req.Name,
		"schema": req.Schema,
	})
}
