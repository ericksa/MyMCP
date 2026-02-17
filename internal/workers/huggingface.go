package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type HuggingFaceWorker struct {
	apiToken   string
	httpClient *http.Client
}

func NewHuggingFaceWorker(apiToken string) *HuggingFaceWorker {
	return &HuggingFaceWorker{
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 300 * time.Second,
		},
	}
}

func (w *HuggingFaceWorker) GetTools() []ToolDef {
	return []ToolDef{
		{Name: "list_models", Description: "List available models on HuggingFace Hub"},
		{Name: "search_models", Description: "Search models by name or task"},
		{Name: "model_info", Description: "Get information about a specific model"},
		{Name: "download_model", Description: "Get model download information"},
		{Name: "list_datasets", Description: "List available datasets"},
		{Name: "search_datasets", Description: "Search datasets by name"},
		{Name: "dataset_info", Description: "Get information about a specific dataset"},
		{Name: "inference", Description: "Run inference on a model"},
		{Name: "spaces_info", Description: "Get information about HuggingFace Spaces"},
	}
}

func (w *HuggingFaceWorker) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "list_models", "huggingface_list_models":
		return w.listModels(ctx, input)
	case "search_models", "huggingface_search_models":
		return w.searchModels(ctx, input)
	case "model_info", "huggingface_model_info":
		return w.modelInfo(ctx, input)
	case "download_model", "huggingface_download_model":
		return w.downloadModel(ctx, input)
	case "list_datasets", "huggingface_list_datasets":
		return w.listDatasets(ctx, input)
	case "search_datasets", "huggingface_search_datasets":
		return w.searchDatasets(ctx, input)
	case "dataset_info", "huggingface_dataset_info":
		return w.datasetInfo(ctx, input)
	case "inference", "huggingface_inference":
		return w.inference(ctx, input)
	case "spaces_info", "huggingface_spaces_info":
		return w.spacesInfo(ctx, input)
	default:
		return nil, nil
	}
}

type HFListModelsRequest struct {
	Filter    string `json:"filter,omitempty"`
	Sort      string `json:"sort,omitempty"`
	Direction int    `json:"direction,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

func (w *HuggingFaceWorker) listModels(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req HFListModelsRequest
	json.Unmarshal(input, &req)

	if req.Limit == 0 {
		req.Limit = 10
	}

	url := "https://huggingface.co/api/models?limit=" + fmt.Sprintf("%d", req.Limit)
	if req.Filter != "" {
		url += "&filter=" + req.Filter
	}
	if req.Sort != "" {
		url += "&sort=" + req.Sort
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	if w.apiToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+w.apiToken)
	}

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return b, nil
}

type HFSearchModelsRequest struct {
	Query string `json:"query"`
	Task  string `json:"task,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

func (w *HuggingFaceWorker) searchModels(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req HFSearchModelsRequest
	json.Unmarshal(input, &req)

	if req.Limit == 0 {
		req.Limit = 10
	}

	url := "https://huggingface.co/api/models?search=" + req.Query + "&limit=" + fmt.Sprintf("%d", req.Limit)
	if req.Task != "" {
		url += "&task=" + req.Task
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	if w.apiToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+w.apiToken)
	}

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return b, nil
}

type HFModelInfoRequest struct {
	Model string `json:"model"`
}

func (w *HuggingFaceWorker) modelInfo(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req HFModelInfoRequest
	json.Unmarshal(input, &req)

	if req.Model == "" {
		return nil, fmt.Errorf("model name is required")
	}

	url := "https://huggingface.co/api/models/" + req.Model

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	if w.apiToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+w.apiToken)
	}

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return b, nil
}

func (w *HuggingFaceWorker) downloadModel(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req HFModelInfoRequest
	json.Unmarshal(input, &req)

	if req.Model == "" {
		return nil, fmt.Errorf("model name is required")
	}

	url := "https://huggingface.co/" + req.Model + "/resolve/main/config.json"

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	if w.apiToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+w.apiToken)
	}

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return json.Marshal(map[string]interface{}{
		"model":        req.Model,
		"download_url": url,
		"status":       "Use huggingface-cli or git clone to download",
	})
}

type HFListDatasetsRequest struct {
	Filter string `json:"filter,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

func (w *HuggingFaceWorker) listDatasets(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req HFListDatasetsRequest
	json.Unmarshal(input, &req)

	if req.Limit == 0 {
		req.Limit = 10
	}

	url := "https://huggingface.co/api/datasets?limit=" + fmt.Sprintf("%d", req.Limit)
	if req.Filter != "" {
		url += "&filter=" + req.Filter
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	if w.apiToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+w.apiToken)
	}

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return b, nil
}

type HFSearchDatasetsRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

func (w *HuggingFaceWorker) searchDatasets(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req HFSearchDatasetsRequest
	json.Unmarshal(input, &req)

	if req.Limit == 0 {
		req.Limit = 10
	}

	url := "https://huggingface.co/api/datasets?search=" + req.Query + "&limit=" + fmt.Sprintf("%d", req.Limit)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	if w.apiToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+w.apiToken)
	}

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return b, nil
}

type HFDatasetInfoRequest struct {
	Dataset string `json:"dataset"`
}

func (w *HuggingFaceWorker) datasetInfo(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req HFDatasetInfoRequest
	json.Unmarshal(input, &req)

	if req.Dataset == "" {
		return nil, fmt.Errorf("dataset name is required")
	}

	url := "https://huggingface.co/api/datasets/" + req.Dataset

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	if w.apiToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+w.apiToken)
	}

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return b, nil
}

type HFInferenceRequest struct {
	Model      string                 `json:"model"`
	Inputs     interface{}            `json:"inputs"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

func (w *HuggingFaceWorker) inference(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req HFInferenceRequest
	json.Unmarshal(input, &req)

	if req.Model == "" {
		return nil, fmt.Errorf("model name is required")
	}

	// Use Inference API
	url := "https://api-inference.huggingface.co/models/" + req.Model

	body, _ := json.Marshal(map[string]interface{}{
		"inputs":  req.Inputs,
		"options": req.Parameters,
	})

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if w.apiToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+w.apiToken)
	}

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return b, nil
}

type HFSpacesInfoRequest struct {
	Space string `json:"space"`
}

func (w *HuggingFaceWorker) spacesInfo(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req HFSpacesInfoRequest
	json.Unmarshal(input, &req)

	if req.Space == "" {
		return nil, fmt.Errorf("space name is required")
	}

	url := "https://huggingface.co/api/spaces/" + req.Space

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	if w.apiToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+w.apiToken)
	}

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return b, nil
}
