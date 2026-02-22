package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

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

	// File I/O worker
	h.workers["file_io"] = workers.NewFileIOWorker(cfg.MCP.Workers.BasePath)

	// SQLite worker
	h.workers["sqlite"] = workers.NewSQLiteWorkerState()

	// Vector worker
	if cfg.MCP.Workers.Vector.Enabled {
		h.workers["vector"] = workers.NewVectorWorkerState()
	}

	// TGI worker for LLM inference
	if cfg.MCP.Workers.TGI.Enabled {
		h.workers["tgi"] = workers.NewTGIWorker(cfg.MCP.Workers.TGI.Endpoint)
	}

	// LM Studio worker
	if cfg.MCP.Workers.LMStudio.Enabled {
		h.workers["lmstudio"] = workers.NewLMStudioWorker(cfg.MCP.Workers.LMStudio.Endpoint)
	}

	// HuggingFace worker
	if cfg.MCP.Workers.HuggingFace.Enabled {
		h.workers["huggingface"] = workers.NewHuggingFaceWorker(cfg.MCP.Workers.HuggingFace.APIToken)
	}

	// Whisper worker
	if cfg.MCP.Workers.Whisper.Enabled {
		h.workers["whisper"] = workers.NewWhisperWorker(cfg.MCP.Workers.Whisper.Endpoint, cfg.MCP.Workers.Whisper.APIKey)
	}

	// Dataset worker
	if cfg.MCP.Workers.Dataset.Enabled {
		h.workers["dataset"] = workers.NewDatasetWorker(cfg.MCP.Workers.Dataset.BasePath)
	}

	// RAG worker
	if cfg.MCP.Workers.RAG.Enabled {
		ragWorker := workers.NewRAGWorkerState(workers.RAGConfig{
			ChunkSize:    cfg.MCP.Workers.RAG.ChunkSize,
			ChunkOverlap: cfg.MCP.Workers.RAG.ChunkOverlap,
			Collection:   "rag",
		})
		h.workers["rag"] = ragWorker
	}

	// Contract worker (always enabled)
	contractWorker := workers.NewContractWorkerState()
	// Connect to RAG if available
	if ragWorker, ok := h.workers["rag"].(*workers.RAGWorkerState); ok {
		contractWorker.SetRAGWorker(ragWorker)
	}
	h.workers["contract"] = contractWorker

	// Orchestrator worker
	h.workers["orchestrator"] = workers.NewOrchestratorWorkerState(10, 120*time.Second)

	// Email parser worker for local mail access
	h.workers["email_parser"] = workers.NewEmailParserWorker(cfg.MCP.Workers.EmailParser.MaildirPath)

	// MinIO worker for S3-compatible storage
	if cfg.MCP.Workers.MinIO.Enabled {
		minioWorker, err := workers.NewMinIOWorker(workers.MinIOConfig{
			Endpoint:  cfg.MCP.Workers.MinIO.Endpoint,
			AccessKey: cfg.MCP.Workers.MinIO.AccessKey,
			SecretKey: cfg.MCP.Workers.MinIO.SecretKey,
			Bucket:    cfg.MCP.Workers.MinIO.DefaultBucket,
			UseSSL:    cfg.MCP.Workers.MinIO.UseSSL,
		})
		if err != nil {
			fmt.Printf("Warning: failed to initialize MinIO worker: %v\n", err)
		} else {
			h.workers["minio"] = minioWorker
		}
	}

	// Task worker for task management
	if cfg.MCP.Workers.Task.Enabled {
		taskWorker, err := workers.NewTaskWorker(cfg.MCP.Workers.Task.DBURL)
		if err != nil {
			// Log error but don't fail - task worker is optional
			fmt.Printf("Warning: failed to initialize task worker: %v\n", err)
		} else {
			h.workers["task"] = taskWorker
		}
	}

	// Reminders sync worker for Apple Reminders <-> PostgreSQL sync
	if cfg.MCP.Workers.RemindersSync.Enabled {
		remindersWorker, err := workers.NewRemindersSyncWorker(workers.RemindersConfig{
			Enabled:       cfg.MCP.Workers.RemindersSync.Enabled,
			PostgresURL:   cfg.MCP.Workers.RemindersSync.PostgresURL,
			RemindctlPath: cfg.MCP.Workers.RemindersSync.RemindctlPath,
			SyncInterval:  cfg.MCP.Workers.RemindersSync.SyncInterval,
		})
		if err != nil {
			// Log error but don't fail - reminders sync is optional
			fmt.Printf("Warning: failed to initialize reminders sync worker: %v\n", err)
		} else {
			h.workers["reminders_sync"] = remindersWorker
		}
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

func (h *Handler) ExecuteTool(ctx context.Context, toolName string, args json.RawMessage) ([]byte, error) {
	for name, worker := range h.workers {
		fullPrefix := name + "_"
		if len(toolName) > len(fullPrefix) && toolName[:len(fullPrefix)] == fullPrefix {
			shortName := toolName[len(fullPrefix):]
			return worker.Execute(ctx, shortName, args)
		}
	}
	return nil, fmt.Errorf("tool not found: %s", toolName)
}
