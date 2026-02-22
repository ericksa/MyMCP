# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build, Test, and Run Commands

```bash
# Build all packages
go build ./...

# Run all tests
go test ./...

# Run tests with verbose output
go test ./... -v

# Run a specific test
go test ./internal/workers -run TestReadDir -v

# Run the gateway server
go run ./cmd/gateway

# Run the adapter (connects LLM to MCP tools)
go run ./cmd/adapter "your prompt here"

# Swift client (in swiftclient/ directory)
cd swiftclient && swift build
cd swiftclient && swift test
```

## Architecture Overview

MyMCP is an AI Infrastructure Platform implementing an MCP (Model Context Protocol) server with pluggable workers for AI/ML operations.

### Core Components

- **Gateway** (`cmd/gateway/`): HTTP server exposing MCP tools via REST endpoints at `/tools/{worker}/{tool}` and a configuration API at `/configure`
- **Adapter** (`cmd/adapter/`): Connects an LLM (Ollama by default) to the MCP gateway, enabling tool-calling workflows
- **MCP Handler** (`pkg/mcp/handler.go`): Core handler managing worker registration and tool execution
- **Workers** (`internal/workers/`): Pluggable tool implementations

### Worker Pattern

Each worker implements this interface:
```go
type Worker interface {
    GetTools() []ToolDef
    Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error)
}
```

Tool naming convention: `{worker}_{tool}` (e.g., `file_io_read_file`, `tgi_generate`)

Workers are registered in `pkg/mcp/handler.go` in `NewHandler()` based on configuration.

### Configuration

- Config file: `config.yaml` (loaded from `.` or `~/.mymcp/`)
- Environment variables: prefixed with `MCP_` (e.g., `MCP_LLM_MODEL`)
- Uses Viper for config management with struct mapping
- Configuration API at `/configure` allows runtime inspection and updates

### Available Workers

| Worker | Purpose | Tools |
|--------|---------|-------|
| file_io | File system operations | list_directory, read_file, write_file, delete_file, search_file_contents |
| sqlite | Database queries | sql_query |
| vector | Vector operations | upsert, search, delete, create_collection |
| tgi | Text Generation Inference | generate, chat, embed, stream_generate, health, models |
| lmstudio | LM Studio integration | chat, generate, models, pull, delete |
| huggingface | HuggingFace Hub | download_model, list_models, search_models |
| whisper | Audio transcription | transcribe |
| dataset | Dataset management | list, download, upload, process |
| minio | Object storage | put, get, delete, list |

### HTTP Endpoints

- `GET /health` - Health check
- `POST /tools/{worker}/{tool}` - Execute a tool
- `GET /configure` - Get current configuration
- `POST /configure` - Update configuration
- `POST /configure/reload` - Reload from file
- `GET /configure/workers` - List all workers
- `GET /configure/workers/{worker}` - Get worker config

### Key File Locations

- Entry points: `cmd/gateway/main.go`, `cmd/adapter/main.go`
- Worker implementations: `internal/workers/*.go`
- Config types: `internal/config/config.go`
- Config validation: `internal/config/validate.go`
- HTTP middleware: `internal/middleware/middleware.go`
- MCP protocol: `pkg/mcp/handler.go`