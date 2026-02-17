package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ericksa/mymcp/internal/config"
	"github.com/spf13/viper"
)

type LLMAdapter struct {
	cfg    *config.Config
	client *http.Client
	mcpURL string
}

type ChatRequest struct {
	Model    string          `json:"model"`
	Messages []Message       `json:"messages"`
	Tools    json.RawMessage `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
}

type Message struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []ToolResponse `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

type Function struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ChatResponse struct {
	Choices []Choice `json:"choices"`
	Message Message  `json:"message"`
}

type Choice struct {
	Message Message `json:"message"`
}

type ToolResponse struct {
	Index    int      `json:"index"`
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function ToolFunc `json:"function"`
}

type ToolFunc struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

func NewLLMAdapter(cfg *config.Config, mcpURL string) *LLMAdapter {
	return &LLMAdapter{
		cfg:    cfg,
		client: &http.Client{Timeout: 120 * time.Second},
		mcpURL: mcpURL,
	}
}

func (a *LLMAdapter) Chat(ctx context.Context, messages []Message, tools json.RawMessage) (*ChatResponse, error) {
	req := ChatRequest{
		Model:    a.cfg.LLM.Model,
		Messages: messages,
		Stream:   false,
	}

	if len(tools) > 0 {
		req.Tools = tools
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.cfg.LLM.Endpoint+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if a.cfg.LLM.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.cfg.LLM.APIKey)
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LLM API error: %s", string(b))
	}

	b, _ := io.ReadAll(resp.Body)

	var result ChatResponse
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (a *LLMAdapter) CallMCPTool(ctx context.Context, toolCallID, toolName string, args json.RawMessage) (string, error) {
	var workerName, toolShortName string

	if strings.HasPrefix(toolName, "file_io_") {
		workerName = "file_io"
		toolShortName = strings.TrimPrefix(toolName, "file_io_")
	} else if strings.HasPrefix(toolName, "sqlite_") {
		workerName = "sqlite"
		toolShortName = strings.TrimPrefix(toolName, "sqlite_")
	} else if strings.HasPrefix(toolName, "vector_") {
		workerName = "vector"
		toolShortName = strings.TrimPrefix(toolName, "vector_")
	} else {
		return "", fmt.Errorf("unknown tool prefix: %s", toolName)
	}

	url := fmt.Sprintf("%s/tools/%s/%s", a.mcpURL, workerName, toolShortName)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(args))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tool call failed: %s", string(b))
	}
	return string(b), nil
}

func (a *LLMAdapter) Run(ctx context.Context, systemPrompt string, userPrompt string, tools json.RawMessage) (string, error) {
	messages := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	for {
		resp, err := a.Chat(ctx, messages, tools)
		if err != nil {
			return "", err
		}

		var msg Message
		if len(resp.Choices) > 0 {
			msg = resp.Choices[0].Message
		} else if resp.Message.Role != "" {
			msg = resp.Message
		} else {
			return "", fmt.Errorf("no response from LLM")
		}

		messages = append(messages, msg)

		if len(msg.ToolCalls) == 0 {
			return msg.Content, nil
		}

		for _, tc := range msg.ToolCalls {
			args := tc.Function.Arguments
			result, err := a.CallMCPTool(ctx, tc.ID, tc.Function.Name, args)
			if err != nil {
				result = fmt.Sprintf("error: %v", err)
			}
			messages = append(messages, Message{
				Role:    "tool",
				Content: result,
			})
		}
	}
}

func loadToolsSchema() (json.RawMessage, error) {
	tools := []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "file_io_list_directory",
				"description": "List files in a directory",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "The directory path to list",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "file_io_read_file",
				"description": "Read contents of a file",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "The file path to read",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "sqlite_sql_query",
				"description": "Execute a SQL query",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "The SQL query to execute",
						},
					},
					"required": []string{"query"},
				},
			},
		},
	}
	return json.Marshal(tools)
}

func main() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME/.mymcp")
	viper.AutomaticEnv()

	viper.SetDefault("server.addr", "localhost:8080")
	viper.SetDefault("llm.provider", "ollama")
	viper.SetDefault("llm.endpoint", "http://localhost:11434")
	viper.SetDefault("llm.model", "llama3:8b")

	viper.ReadInConfig()

	cfg := &config.Config{
		ServerAddr: viper.GetString("server.addr"),
		LLM: config.LLMConfig{
			Provider: viper.GetString("llm.provider"),
			Endpoint: viper.GetString("llm.endpoint"),
			Model:    viper.GetString("llm.model"),
			APIKey:   viper.GetString("llm.api_key"),
		},
	}

	mcpURL := "http://localhost:8080"

	adapter := NewLLMAdapter(cfg, mcpURL)

	tools, err := loadToolsSchema()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load tools: %v\n", err)
		os.Exit(1)
	}

	systemPrompt := "You are a helpful assistant with access to file and database tools. Use the tools when needed."
	userPrompt := "List the files in the current directory."

	if len(os.Args) > 1 {
		userPrompt = strings.Join(os.Args[1:], " ")
	}

	result, err := adapter.Run(context.Background(), systemPrompt, userPrompt, tools)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(result)
}
