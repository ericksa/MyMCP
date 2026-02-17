package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ericksa/mymcp/internal/audit"
	"github.com/ericksa/mymcp/internal/config"
	"github.com/ericksa/mymcp/internal/workers"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Worker interface {
	GetTools() []workers.ToolDef
	Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error)
}

type Handler struct {
	config  *config.Config
	audit   *audit.Auditor
	workers map[string]Worker
	server  *mcp.Server
}

func NewHandler(cfg *config.Config) *Handler {
	h := &Handler{
		config:  cfg,
		audit:   audit.NewAuditor(),
		workers: make(map[string]Worker),
	}

	if cfg.Workers.EnableFileIO {
		h.workers["file_io"] = workers.NewFileIOWorker(cfg.Workers.BasePath)
	}
	if cfg.Workers.EnableSQLite {
		h.workers["sqlite"] = workers.NewSQLiteWorkerState()
	}
	if cfg.Workers.EnableVector {
		h.workers["vector"] = workers.NewVectorWorkerState()
	}

	h.initMCPServer()
	return h
}

func (h *Handler) initMCPServer() {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "MyMCP Gateway",
		Version: "1.0.0",
	}, nil)

	for name, worker := range h.workers {
		for _, tool := range worker.GetTools() {
			toolName := fmt.Sprintf("%s_%s", name, tool.Name)
			toolDesc := tool.Description
			w := worker
			mcp.AddTool(server, &mcp.Tool{
				Name:        toolName,
				Description: toolDesc,
			}, h.wrapTool(w, toolName))
		}
	}

	h.server = server
}

func (h *Handler) wrapTool(w Worker, toolName string) func(ctx context.Context, req *mcp.CallToolRequest, input any) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input any) (*mcp.CallToolResult, any, error) {
		inputBytes, _ := json.Marshal(input)
		result, err := w.Execute(ctx, toolName, inputBytes)
		h.audit.Log(toolName, inputBytes, result, err)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
			}, nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(result)},
			},
		}, nil, nil
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.server == nil {
		http.Error(w, "MCP server not initialized", http.StatusInternalServerError)
		return
	}
	h.server.Run(r.Context(), &mcp.StdioTransport{})
}
