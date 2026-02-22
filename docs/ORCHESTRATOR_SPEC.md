# Orchestrator Worker Spec - MyMCP

**Status:** Draft  
**Purpose:** Enable parallel agent execution and "agent genetics" for business workflows

---

## Overview

The orchestrator worker manages execution of multiple AI agents in parallel, with support for:
- Sequential agent chains
- Parallel agent spawning
- Fitness evaluation (agent genetics)
- Population management
- Result aggregation

---

## Data Structures

### AgentGenome

```go
type AgentGenome struct {
    ID          string            `json:"id"`           // unique identifier
    Name        string            `json:"name"`         // agent name
    Model       string            `json:"model"`        // e.g., "llama3", "qwen3:8b"
    Provider    string            `json:"provider"`     // "tgi", "lmstudio", "ollama"
    SystemPrompt string           `json:"system_prompt"`
    Tools       []string          `json:"tools"`        // MCP tool names to enable
    Temperature float64           `json:"temperature"`
    MaxTokens   int               `json:"max_tokens"`
    Metadata    map[string]any    `json:"metadata"`
}
```

### AgentRun

```go
type AgentRun struct {
    RunID       string            `json:"run_id"`
    Genome      AgentGenome       `json:"genome"`
    Input       string            `json:"input"`
    Output      string            `json:"output"`
    Status      string            `json:"status"`       // "running", "completed", "failed"
    Fitness     float64           `json:"fitness"`      // 0.0-1.0 score
    StartedAt   time.Time         `json:"started_at"`
    CompletedAt time.Time         `json:"completed_at"`
    Error       string            `json:"error,omitempty"`
}
```

### Workflow

```go
type Workflow struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Steps       []WorkflowStep    `json:"steps"`
    OnSuccess   string            `json:"on_success"`  // next step or "done"
    OnFailure   string            `json:"on_failure"`  // next step or "done"
}

type WorkflowStep struct {
    StepID      string            `json:"step_id"`
    AgentID     string            `json:"agent_id"`
    Parallel    bool              `json:"parallel"`    // run with next step
    Inputs      map[string]string `json:"inputs"`      // from previous outputs
}
```

---

## Tools (API)

| Tool | Description | Parameters |
|------|-------------|------------|
| `orchestrator_register_agent` | Register a new agent genome | `genome: AgentGenome` |
| `orchestrator_list_agents` | List all registered agents | - |
| `orchestrator_run_agent` | Run a single agent | `agent_id, input, timeout` |
| `orchestrator_run_parallel` | Run multiple agents in parallel | `agent_ids[], input, timeout` |
| `orchestrator_run_workflow` | Execute a workflow | `workflow_id, initial_input` |
| `orchestrator_evaluate` | Score agent output | `run_id, criteria` |
| `orchestrator_evolve` | Create new agent from best performers | `parent_ids[], mutations` |
| `orchestrator_get_result` | Get result of a run | `run_id` |

---

## Execution Flow

### Single Agent Run
```
1. Client calls orchestrator_run_agent
2. Lookup genome by agent_id
3. Select provider (tgi/lmstudio/ollama)
4. Execute LLM call with tools
5. Record output + timing
6. Return run_id
```

### Parallel Execution
```
1. Client calls orchestrator_run_parallel with [agent_id1, agent_id2]
2. For each agent:
   a. Lookup genome
   b. Spawn goroutine
   c. Execute independently
3. Wait for all completions (or timeout)
4. Return array of results
```

### Workflow Execution
```
1. Client calls orchestrator_run_workflow
2. Load workflow definition
3. For each step:
   a. If parallel=true, spawn all parallel steps
   b. Otherwise, execute sequentially
   c. Pass outputs to next step inputs
4. Return final result
```

---

## Agent Genetics

### Evolution Loop

```
1. Create population (initial agents)
2. For generation in max_generations:
   a. Run all agents on task
   b. Evaluate fitness for each
   c. Select top performers
   d. Apply crossover (combine 2 agents)
   e. Apply mutation (random tweaks)
   f. Replace population
3. Return best agent
```

### Mutation Operators

| Operator | Description |
|----------|-------------|
| `temperature` | Adjust temperature ±0.1 |
| `prompt` | Mutate system prompt (LLM-based) |
| `tools` | Add/remove tool access |
| `model` | Switch to different model |

### Crossover Operators

| Operator | Description |
|----------|-------------|
| `prompt_merge` | LLM merges two prompts |
| `tool_union` | Combine tool sets |

---

## Configuration

```yaml
mcp:
  workers:
    orchestrator:
      enabled: true
      max_parallel: 10
      default_timeout: 120s
      max_retries: 3
      population_size: 10
      generations: 5
      evolution:
        mutation_rate: 0.1
        crossover_rate: 0.3
        elite_count: 2
```

---

## Integration Points

### With Existing Workers

- **llm workers** (tgi, lmstudio): Execute the actual LLM calls
- **vector worker**: Store agent histories for RAG
- **sqlite worker**: Persist agent genomes and runs
- **file_io worker**: Load/save workflow definitions

### External Integrations

- **Ollama**: Default LLM provider
- **LM Studio**: MLX models
- **TGI**: HuggingFace inference

---

## Example Usage

### Register an Agent
```json
{
  "name": "researcher",
  "model": "llama3:70b",
  "provider": "lmstudio",
  "system_prompt": "You are a research assistant. Find and summarize information.",
  "tools": ["file_io_read_file", "file_io_search_file_contents"],
  "temperature": 0.7
}
```

### Run Parallel Agents
```json
{
  "agent_ids": ["researcher_v1", "coder_v1", "reviewer_v1"],
  "input": "Analyze the codebase for security vulnerabilities",
  "timeout": 300
}
```

### Evolve Agents
```json
{
  "task": "Write a Python function to parse JSON",
  "parent_ids": ["agent_001", "agent_002"],
  "mutations": ["temperature", "prompt"],
  "generations": 3
}
```

---

## TODO

- [ ] Implement AgentGenome storage (SQLite)
- [ ] Add LLM provider abstraction
- [ ] Build parallel execution engine
- [ ] Add workflow DSL/parser
- [ ] Implement fitness evaluation
- [ ] Add mutation/crossover operators
- [ ] Create REST API wrapper
- [ ] Add SwiftUI admin dashboard

---

*Draft - need to integrate with existing worker pattern in MyMCP*
