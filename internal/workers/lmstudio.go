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

type LMStudioWorker struct {
	baseURL    string
	httpClient *http.Client
}

func NewLMStudioWorker(baseURL string) *LMStudioWorker {
	return &LMStudioWorker{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 180 * time.Second,
		},
	}
}

func (w *LMStudioWorker) GetTools() []ToolDef {
	return []ToolDef{
		{Name: "chat", Description: "Chat completion using LM Studio"},
		{Name: "generate", Description: "Text generation using LM Studio"},
		{Name: "embed", Description: "Generate embeddings using LM Studio"},
		{Name: "models", Description: "List available models"},
		{Name: "pull", Description: "Download a model from HuggingFace"},
		{Name: "delete", Description: "Delete a downloaded model"},
		{Name: "load", Description: "Load a model into memory"},
		{Name: "unload", Description: "Unload a model from memory"},
		{Name: "status", Description: "Get LM Studio server status"},
	}
}

func (w *LMStudioWorker) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "chat", "lmstudio_chat":
		return w.chat(ctx, input)
	case "generate", "lmstudio_generate":
		return w.generate(ctx, input)
	case "embed", "lmstudio_embed":
		return w.embed(ctx, input)
	case "models", "lmstudio_models":
		return w.listModels(ctx)
	case "pull", "lmstudio_pull":
		return w.pullModel(ctx, input)
	case "delete", "lmstudio_delete":
		return w.deleteModel(ctx, input)
	case "load", "lmstudio_load":
		return w.loadModel(ctx, input)
	case "unload", "lmstudio_unload":
		return w.unloadModel(ctx, input)
	case "status", "lmstudio_status":
		return w.status(ctx)
	default:
		return nil, nil
	}
}

type LMStudioChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LMStudioChatRequest struct {
	Model       string                `json:"model"`
	Messages    []LMStudioChatMessage `json:"messages"`
	MaxTokens   int                   `json:"max_tokens,omitempty"`
	Temperature float64               `json:"temperature,omitempty"`
	TopP        float64               `json:"top_p,omitempty"`
}

type LMStudioChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int                 `json:"index"`
		Message      LMStudioChatMessage `json:"message"`
		FinishReason string              `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (w *LMStudioWorker) chat(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req LMStudioChatRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if req.Model == "" {
		req.Model = "local-model"
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 512
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", w.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LM Studio error: %s", string(b))
	}

	var result LMStudioChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return json.Marshal(result)
}

type LMStudioGenerateRequest struct {
	Model       string  `json:"model"`
	Prompt      string  `json:"prompt"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
}

type LMStudioGenerateResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Text         string `json:"text"`
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (w *LMStudioWorker) generate(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req LMStudioGenerateRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if req.Model == "" {
		req.Model = "local-model"
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 512
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", w.baseURL+"/v1/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LM Studio error: %s", string(b))
	}

	var result LMStudioGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return json.Marshal(result)
}

type LMStudioEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type LMStudioEmbedResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func (w *LMStudioWorker) embed(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req LMStudioEmbedRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if req.Model == "" {
		req.Model = "local-embedding"
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", w.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LM Studio error: %s", string(b))
	}

	var result LMStudioEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return json.Marshal(result)
}

func (w *LMStudioWorker) listModels(ctx context.Context) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", w.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LM Studio error: %s", string(b))
	}

	b, _ := io.ReadAll(resp.Body)
	return b, nil
}

type PullModelRequest struct {
	Model string `json:"model"`
}

func (w *LMStudioWorker) pullModel(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req PullModelRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if req.Model == "" {
		return nil, fmt.Errorf("model name is required")
	}

	// LM Studio uses internal API for downloading
	httpReq, err := http.NewRequestWithContext(ctx, "POST", w.baseURL+"/api/download", bytes.NewReader([]byte(fmt.Sprintf(`{"model":"%s"}`, req.Model))))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return b, nil
}

type DeleteModelRequest struct {
	Model string `json:"model"`
}

func (w *LMStudioWorker) deleteModel(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req DeleteModelRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if req.Model == "" {
		return nil, fmt.Errorf("model name is required")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", w.baseURL+"/v1/models/"+req.Model, nil)
	if err != nil {
		return nil, err
	}

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return b, nil
}

type LoadModelRequest struct {
	Model string `json:"model"`
}

func (w *LMStudioWorker) loadModel(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req LoadModelRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if req.Model == "" {
		return nil, fmt.Errorf("model name is required")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", w.baseURL+"/api/load", bytes.NewReader([]byte(fmt.Sprintf(`{"model_name":"%s"}`, req.Model))))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return b, nil
}

type UnloadModelRequest struct {
	Model string `json:"model"`
}

func (w *LMStudioWorker) unloadModel(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req UnloadModelRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", w.baseURL+"/api/unload", nil)
	if err != nil {
		return nil, err
	}

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return b, nil
}

func (w *LMStudioWorker) status(ctx context.Context) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", w.baseURL+"/api/status", nil)
	if err != nil {
		return nil, err
	}

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return b, nil
}
