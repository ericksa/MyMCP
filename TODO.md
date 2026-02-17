# TODO – Integrate Enchanted UI with MyMCP (MCP + PAT)

## Overview
We want the Enchanted SwiftUI client to talk to the new Go‑based MyMCP server instead of calling Ollama directly. 
The goal is a **single source of truth for models**, a **unified tool‑calling API**, and **full access to PAT Core services** (calendar, tasks, email, etc.) through the same MCP gateway.

## Phase 0 – Prep
- [ ] Ensure MyMCP server (port 8080) is running with the latest code (workers, config, PATWorker).  
- [ ] Verify Docker compose for PAT services (`ingest-service`, `agent-service`, `pat‑core`, etc.) is up.  
- [ ] Commit current changes (`git push`) so the repo is clean before we add new files.

---

## Phase 1 – MCP Bridge for Swift (MCPClient)

| Task | Details | Owner | Status |
|------|---------|-------|--------|
| **1.1** | Create `MCPClient.swift` in `Enchanted/Services` | – | ☐ |
|  | • Wrap HTTP calls to `http://localhost:8080/mcp` (`/execute`, `/configure`, `/tools`). | | |
|  | • Provide methods: `chat(message:)`, `executeTool(name:args:)`, `fetchModelList()`, `setModel(_:)`. | | |
| **1.2** | Add a bearer‑token helper that reads a token from the keychain and injects `Authorization: Bearer …`. | – | ☐ |
| **1.3** | Write a lightweight `ToolCallResult` struct to decode MCP tool‑call JSON. | – | ☐ |
| **1.4** | Unit‑test `MCPClient` with a local mock server (e.g. `httptest`). | – | ☐ |
| **1.5** | Update `EnchantedApp.swift` to inject `MCPClient.shared` into the environment. | – | ☐ |

---

## Phase 2 – Swap OllamaService → MCPClient in UI

| Task | Details | Owner | Status |
|------|---------|-------|--------|
| **2.1** | Search the codebase for `OllamaService.shared` usage (chat, completions, model list). | – | ☐ |
| **2.2** | Replace each call with the equivalent `MCPClient` method (`chat`, `fetchModelList`, etc.). | – | ☐ |
| **2.3** | Adjust any `ChatRequest` construction to include the model name from `LanguageModelStore`. | – | ☐ |
| **2.4** | Update UI bindings (e.g., `ModelSelectorView`) to read from `MCPClient.fetchModelList()` instead of Ollama. | – | ☐ |
| **2.5** | Verify that the UI still compiles and runs against a local MyMCP instance. | – | ☐ |
| **2.6** | Write UI integration tests (Xcode UI test target) that fire a simple message and assert a response. | – | ☐ |

---

## Phase 3 – Register PATWorker in MyMCP

| Task | Details | Owner | Status |
|------|---------|-------|--------|
| **3.1** | Add `NewPATWorker` registration in `pkg/mcp/handler.go` (e.g., `h.workers["pat"] = workers.NewPATWorker(cfg.MCP.Workers.PATCoreURL)`). | – | ☐ |
| **3.2** | Add a `PATCoreURL` field to `WorkersConfig` (already present as env var `PAT_CORE_URL`). | – | ☐ |
| **3.3** | Implement tool mapping in `internal/workers/pat.go` – expose tools with `pat_` prefix: `pat_calendar_list`, `pat_task_create`, `pat_email_send`, etc. | – | ☐ |
| **3.4** | Ensure each tool forwards the request to `PAT_CORE_URL` (host.docker.internal:8010) via `net/http`. | – | ☐ |
| **3.5** | Run `go test ./...` / `go build ./...` to confirm the worker compiles and registers. | – | ☐ |
| **3.6** | Add unit tests for a couple of PAT tools (e.g., mock `PAT_CORE_URL` and assert JSON round‑trip). | – | ☐ |

---

## Phase 4 – UI Hooks for PAT Core Tools

| Task | Details | Owner | Status |
|------|---------|-------|--------|
| **4.1** | In Enchanted UI, add new actions/buttons (e.g., “Create Calendar Event”, “Add Task”, “Send Email”). | – | ☐ |
| **4.2** | Wire each action to `MCPClient.executeTool(name:args:)` using the `pat_` tool names. | – | ☐ |
| **4.3** | Build Swift structs for the input payloads (matching the JSON schemas in `tools/pat.go`). | – | ☐ |
| **4.4** | Show success/error feedback in the UI (toast or alert). | – | ☐ |
| **4.5** | Add unit tests for the UI‑to‑MCP bridge (can use mock `MCPClient`). | – | ☐ |

---

## Phase 5 – Model‑Selection & Configuration UI

| Task | Details | Owner | Status |
|------|---------|-------|--------|
| **5.1** | Extend `ModelSelectorView` to fetch the model list via `MCPClient.fetchModelList()`. | – | ☐ |
| **5.2** | When a model is chosen, call `MCPClient.setModel(_:)` which sends a POST to `/configure` (or a custom `set_model` tool). | – | ☐ |
| **5.3** | Persist the chosen model in `LanguageModelStore` for UI state. | – | ☐ |
| **5.4** | Update `LLMConfig` defaults (already set to `mistral-large-3:675b-cloud`). | – | ☐ |
| **5.5** | Add a Settings tab that lists all available MCP tools (`GET /tools`) for power‑users. | – | ☐ |
| **5.6** | Write integration tests that switch models and verify the chat endpoint uses the new model. | – | ☐ |

---

## Phase 6 – Persistence & Memory Sync (optional)

| Task | Details | Owner | Status |
|------|---------|-------|--------|
| **6.1** | Decide whether to keep `SwiftData` as a local cache (fast UI reload) **and** push every new message to the MCP `memory_store` tool. | – | ☐ |
| **6.2** | If keeping both, add a background sync that reads from `MemoryWorker` on app launch and hydrates `SwiftData`. | – | ☐ |
| **6.3** | Implement a “Clear Conversation” action that calls `memory_clear` in MCP and clears the CoreData store. | – | ☐ |
| **6.4** | Write end‑to‑end tests that simulate a full chat, then kill/relaunch the app and verify state is restored. | – | ☐ |

---

## Phase 7 – Security Hardening

| Task | Details | Owner | Status |
|------|---------|-------|--------|
| **7.1** | Store the MCP bearer token securely in the macOS/iOS Keychain. | – | ☐ |
| **7.2** | Ensure every request from `MCPClient` includes the token header. | – | ☐ |
| **7.3** | Add a simple “login” screen (optional) that writes the token to the keychain. | – | ☐ |
| **7.4** | Update the Go server to reject requests missing a valid token (already enforced by `middleware.AuthMiddleware`). | – | ☐ |

---

## Phase 8 – CI / Automation

| Task | Details | Owner | Status |
|------|---------|-------|--------|
| **8.1** | Add a GitHub Actions workflow that spins up the full Docker Compose stack (MyMCP + PAT services). | – | ☐ |
| **8.2** | Run UI unit tests + integration tests against the live MCP endpoint. | – | ☐ |
| **8.3** | Lint and test the new Go `PATWorker` (go vet, go test). | – | ☐ |
| **8.4** | Publish a build artifact of the Enchanted app that includes the MCP bridge. | – | ☐ |

---

## Quick Reference – File Locations

| Component | Path |
|-----------|------|
| **MCP Bridge (Swift)** | `Enchanted/Services/MCPClient.swift` |
| **PAT Worker (Go)** | `MyMCP/internal/workers/pat.go` |
| **MCP Registration (Go)** | `MyMCP/pkg/mcp/handler.go` |
| **Model‑Selector UI** | `Enchanted/UI/Shared/Chat/Components/ModelSelectorView.swift` |
| **New UI actions** | `Enchanted/UI/Shared/Chat/Components/...` (e.g., `CreateEventView.swift`, `AddTaskView.swift`) |
| **Configuration defaults** | `MyMCP/internal/config/config.go` (LLM model default is now `mistral-large-3:675b-cloud`). |

---

### What to do before you restart
1. **Save** this file as `TODO.md` at the repository root.  
2. **Commit** the file (`git add TODO.md && git commit -m "Add integration TODO list"`).  
3. **Push** (`git push`) so you have a remote record of the plan.  

---

When you’re back online, follow the phases top‑to‑bottom (or jump to the highest‑priority phase) and tick off each item. Feel free to split any task further into subtasks as you see fit. Happy coding!