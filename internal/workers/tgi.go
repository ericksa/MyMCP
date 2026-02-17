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

type TGIWorker struct {
	baseURL    string
	httpClient *http.Client
}

func NewTGIWorker(baseURL string) *TGIWorker {
	return &TGIWorker{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (w *TGIWorker) GetTools() []ToolDef {
	return []ToolDef{
		{Name: "generate", Description: "Generate text using TGI inference server"},
		{Name: "stream_generate", Description: "Stream text generation from TGI"},
		{Name: "chat", Description: "Chat completion using TGI"},
		{Name: "embed", Description: "Generate embeddings using TEI"},
		{Name: "health", Description: "Check TGI server health"},
		{Name: "models", Description: "List available models"},
	}
}

func (w *TGIWorker) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "generate", "tgi_generate":
		return w.generate(ctx, input)
	case "stream_generate", "tgi_stream_generate":
		return w.streamGenerate(ctx, input)
	case "chat", "tgi_chat":
		return w.chat(ctx, input)
	case "embed", "tgi_embed":
		return w.embed(ctx, input)
	case "health", "tgi_health":
		return w.health(ctx)
	case "models", "tgi_models":
		return w.listModels(ctx)
	default:
		return nil, nil
	}
}

type GenerateRequest struct {
	Model       string  `json:"model"`
	Prompt      string  `json:"prompt"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	Stream      bool    `json:"stream,omitempty"`
}

type GenerateResponse struct {
	GeneratedText string  `json:"generated_text,omitempty"`
	TokenCount    int     `json:"token_count,omitempty"`
	TokensPerSec  float64 `json:"tokens_per_second,omitempty"`
	FinishReason  string  `json:"finish_reason,omitempty"`
}

func (w *TGIWorker) generate(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req GenerateRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if req.MaxTokens == 0 {
		req.MaxTokens = 512
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", w.baseURL+"/generate", bytes.NewReader(body))
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
		return nil, fmt.Errorf("TGI error: %s", string(b))
	}

	var result GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return json.Marshal(result)
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

type ChatResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (w *TGIWorker) chat(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req ChatRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
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
		return nil, fmt.Errorf("TGI error: %s", string(b))
	}

	var result ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return json.Marshal(result)
}

type EmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type EmbedResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func (w *TGIWorker) embed(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req EmbedRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if len(req.Input) == 0 {
		return nil, fmt.Errorf("no input provided")
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", w.baseURL+"/embed", bytes.NewReader(body))
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
		return nil, fmt.Errorf("TEI error: %s", string(b))
	}

	var result EmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return json.Marshal(result)
}

func (w *TGIWorker) health(ctx context.Context) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", w.baseURL+"/health", nil)
	if err != nil {
		return nil, err
	}

	resp, err := w.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return json.Marshal(map[string]interface{}{
		"status":      "ok",
		"base_url":    w.baseURL,
		"status_code": resp.StatusCode,
	})
}

func (w *TGIWorker) listModels(ctx context.Context) ([]byte, error) {
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
		return nil, fmt.Errorf("TGI error: %s", string(b))
	}

	b, _ := io.ReadAll(resp.Body)
	return b, nil
}

func (w *TGIWorker) streamGenerate(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req GenerateRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	req.Stream = true
	if req.MaxTokens == 0 {
		req.MaxTokens = 512
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", w.baseURL+"/generate", bytes.NewReader(body))
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
		return nil, fmt.Errorf("TGI error: %s", string(b))
	}

	// Read streaming response
	var fullText string
	decoder := json.NewDecoder(resp.Body)
	for decoder.More() {
		var token struct {
			GeneratedText string `json:"generated_text"`
		}
		if err := decoder.Decode(&token); err != nil {
			break
		}
		fullText += token.GeneratedText
	}

	return json.Marshal(map[string]string{
		"generated_text": fullText,
	})
}
