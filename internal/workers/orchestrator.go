package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// OrchestratorWorker manages agent execution and genetics
type OrchestratorWorkerState struct {
	Tools          []ToolDef
	Agents         map[string]AgentGenome
	Runs           map[string]AgentRun
	Workflows      map[string]Workflow
	LLMProvider    LLMProvider
	MaxParallel    int
	DefaultTimeout time.Duration
	mu             sync.RWMutex
}

type LLMProvider interface {
	Call(ctx context.Context, model, systemPrompt, userPrompt string, temperature float64, maxTokens int) (string, error)
}

// AgentGenome represents an agent configuration
type AgentGenome struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Model        string         `json:"model"`
	Provider     string         `json:"provider"` // "tgi", "lmstudio", "ollama"
	SystemPrompt string         `json:"system_prompt"`
	Tools        []string       `json:"tools"` // MCP tool names
	Temperature  float64        `json:"temperature"`
	MaxTokens    int            `json:"max_tokens"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
	Fitness      float64        `json:"fitness"` // 0.0-1.0 from evolution
	Generation   int            `json:"generation"`
	ParentIDs    []string       `json:"parent_ids"`
}

// AgentRun represents a single execution
type AgentRun struct {
	RunID       string         `json:"run_id"`
	GenomeID    string         `json:"genome_id"`
	Input       string         `json:"input"`
	Output      string         `json:"output"`
	Status      string         `json:"status"` // "running", "completed", "failed"
	Fitness     float64        `json:"fitness"`
	StartedAt   time.Time      `json:"started_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Error       string         `json:"error,omitempty"`
	Metadata    map[string]any `json:"metadata"`
}

// Workflow defines a multi-step execution
type Workflow struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Steps     []WorkflowStep `json:"steps"`
	CreatedAt time.Time      `json:"created_at"`
}

type WorkflowStep struct {
	StepID   string            `json:"step_id"`
	AgentID  string            `json:"agent_id"`
	Parallel bool              `json:"parallel"` // run with next step
	Inputs   map[string]string `json:"inputs"`   // from previous outputs
}

// EvolutionConfig for agent genetics
type EvolutionConfig struct {
	PopulationSize  int     `json:"population_size"`
	Generations     int     `json:"generations"`
	MutationRate    float64 `json:"mutation_rate"`
	CrossoverRate   float64 `json:"crossover_rate"`
	EliteCount      int     `json:"elite_count"`
	FitnessFunction string  `json:"fitness_function"`
}

func NewOrchestratorWorkerState(maxParallel int, defaultTimeout time.Duration) *OrchestratorWorkerState {
	if maxParallel == 0 {
		maxParallel = 10
	}
	if defaultTimeout == 0 {
		defaultTimeout = 120 * time.Second
	}

	return &OrchestratorWorkerState{
		Tools: []ToolDef{
			// Agent management
			{Name: "orchestrator_register_agent", Description: "Register a new agent genome"},
			{Name: "orchestrator_list_agents", Description: "List all registered agents"},
			{Name: "orchestrator_get_agent", Description: "Get agent by ID"},
			{Name: "orchestrator_delete_agent", Description: "Delete an agent"},
			// Execution
			{Name: "orchestrator_run_agent", Description: "Run a single agent"},
			{Name: "orchestrator_run_parallel", Description: "Run multiple agents in parallel"},
			{Name: "orchestrator_run_workflow", Description: "Execute a workflow"},
			// Evolution
			{Name: "orchestrator_evaluate", Description: "Score agent output"},
			{Name: "orchestrator_evolve", Description: "Create new agents via evolution"},
			{Name: "orchestrator_get_result", Description: "Get result of a run"},
			// Workflows
			{Name: "orchestrator_create_workflow", Description: "Create a workflow"},
			{Name: "orchestrator_list_workflows", Description: "List workflows"},
		},
		Agents:         make(map[string]AgentGenome),
		Runs:           make(map[string]AgentRun),
		Workflows:      make(map[string]Workflow),
		MaxParallel:    maxParallel,
		DefaultTimeout: defaultTimeout,
	}
}

func (w *OrchestratorWorkerState) GetTools() []ToolDef {
	return w.Tools
}

func (w *OrchestratorWorkerState) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	// Agent management
	case "orchestrator_orchestrator_register_agent", "orchestrator_register_agent":
		return w.registerAgent(ctx, input)
	case "orchestrator_orchestrator_list_agents", "orchestrator_list_agents":
		return w.listAgents(ctx, input)
	case "orchestrator_orchestrator_get_agent", "orchestrator_get_agent":
		return w.getAgent(ctx, input)
	case "orchestrator_orchestrator_delete_agent", "orchestrator_delete_agent":
		return w.deleteAgent(ctx, input)
	// Execution
	case "orchestrator_orchestrator_run_agent", "orchestrator_run_agent":
		return w.runAgent(ctx, input)
	case "orchestrator_orchestrator_run_parallel", "orchestrator_run_parallel":
		return w.runParallel(ctx, input)
	case "orchestrator_orchestrator_run_workflow", "orchestrator_run_workflow":
		return w.runWorkflow(ctx, input)
	// Evolution
	case "orchestrator_orchestrator_evaluate", "orchestrator_evaluate":
		return w.evaluate(ctx, input)
	case "orchestrator_orchestrator_evolve", "orchestrator_evolve":
		return w.evolve(ctx, input)
	case "orchestrator_orchestrator_get_result", "orchestrator_get_result":
		return w.getResult(ctx, input)
	// Workflows
	case "orchestrator_orchestrator_create_workflow", "orchestrator_create_workflow":
		return w.createWorkflow(ctx, input)
	case "orchestrator_orchestrator_list_workflows", "orchestrator_list_workflows":
		return w.listWorkflows(ctx, input)
	default:
		return nil, nil
	}
}

// SetLLMProvider sets the LLM provider for agent execution
func (w *OrchestratorWorkerState) SetLLMProvider(provider LLMProvider) {
	w.LLMProvider = provider
}

// --- Agent Management ---

func (w *OrchestratorWorkerState) registerAgent(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Name         string         `json:"name"`
		Model        string         `json:"model"`
		Provider     string         `json:"provider"`
		SystemPrompt string         `json:"system_prompt"`
		Tools        []string       `json:"tools"`
		Temperature  float64        `json:"temperature"`
		MaxTokens    int            `json:"max_tokens"`
		Metadata     map[string]any `json:"metadata"`
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	if req.Name == "" || req.Model == "" {
		return nil, fmt.Errorf("name and model required")
	}

	// Generate ID
	agentID := generateAgentID(req.Name)

	agent := AgentGenome{
		ID:           agentID,
		Name:         req.Name,
		Model:        req.Model,
		Provider:     req.Provider,
		SystemPrompt: req.SystemPrompt,
		Tools:        req.Tools,
		Temperature:  req.Temperature,
		MaxTokens:    req.MaxTokens,
		Metadata:     req.Metadata,
		CreatedAt:    time.Now(),
		Fitness:      0.5, // Default fitness
		Generation:   0,
	}

	w.mu.Lock()
	w.Agents[agentID] = agent
	w.mu.Unlock()

	return json.Marshal(map[string]any{
		"agent_id": agentID,
		"agent":    agent,
	})
}

func (w *OrchestratorWorkerState) listAgents(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Limit int `json:"limit"`
	}
	json.Unmarshal(input, &req)
	if req.Limit == 0 {
		req.Limit = 50
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	agents := make([]AgentGenome, 0, len(w.Agents))
	count := 0
	for _, a := range w.Agents {
		if count >= req.Limit {
			break
		}
		agents = append(agents, a)
		count++
	}

	return json.Marshal(agents)
}

func (w *OrchestratorWorkerState) getAgent(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	w.mu.RLock()
	agent, ok := w.Agents[req.AgentID]
	w.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("agent not found: %s", req.AgentID)
	}

	return json.Marshal(agent)
}

func (w *OrchestratorWorkerState) deleteAgent(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if _, ok := w.Agents[req.AgentID]; !ok {
		return nil, fmt.Errorf("agent not found: %s", req.AgentID)
	}

	delete(w.Agents, req.AgentID)
	return json.Marshal(map[string]any{"deleted": true, "agent_id": req.AgentID})
}

// --- Execution ---

func (w *OrchestratorWorkerState) runAgent(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		AgentID string        `json:"agent_id"`
		Input   string        `json:"input"`
		Timeout time.Duration `json:"timeout"`
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	if req.AgentID == "" || req.Input == "" {
		return nil, fmt.Errorf("agent_id and input required")
	}

	timeout := req.Timeout
	if timeout == 0 {
		timeout = w.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Get agent
	w.mu.RLock()
	agent, ok := w.Agents[req.AgentID]
	w.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("agent not found: %s", req.AgentID)
	}

	// Create run
	runID := generateRunID()
	run := AgentRun{
		RunID:     runID,
		GenomeID:  req.AgentID,
		Input:     req.Input,
		Status:    "running",
		StartedAt: time.Now(),
	}

	w.mu.Lock()
	w.Runs[runID] = run
	w.mu.Unlock()

	// Execute
	var output string
	var execErr error

	if w.LLMProvider != nil {
		temp := agent.Temperature
		if temp == 0 {
			temp = 0.7
		}
		maxTokens := agent.MaxTokens
		if maxTokens == 0 {
			maxTokens = 2048
		}
		output, execErr = w.LLMProvider.Call(ctx, agent.Model, agent.SystemPrompt, req.Input, temp, maxTokens)
	} else {
		// Fallback: simulate execution
		output = fmt.Sprintf("[Simulated] Agent '%s' would process: %s", agent.Name, req.Input)
	}

	now := time.Now()
	w.mu.Lock()
	existingRun := w.Runs[runID]
	if execErr != nil {
		existingRun.Status = "failed"
		existingRun.Error = execErr.Error()
	} else {
		existingRun.Status = "completed"
		existingRun.Output = output
	}
	existingRun.CompletedAt = &now
	w.Runs[runID] = existingRun
	w.mu.Unlock()

	if execErr != nil {
		return json.Marshal(map[string]any{
			"run_id": runID,
			"status": "failed",
			"error":  execErr.Error(),
		})
	}

	return json.Marshal(map[string]any{
		"run_id": runID,
		"status": "completed",
		"output": output,
	})
}

func (w *OrchestratorWorkerState) runParallel(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		AgentIDs []string      `json:"agent_ids"`
		Input    string        `json:"input"`
		Timeout  time.Duration `json:"timeout"`
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	if len(req.AgentIDs) == 0 || req.Input == "" {
		return nil, fmt.Errorf("agent_ids and input required")
	}

	// Limit parallelism
	if len(req.AgentIDs) > w.MaxParallel {
		return nil, fmt.Errorf("too many agents (max %d)", w.MaxParallel)
	}

	timeout := req.Timeout
	if timeout == 0 {
		timeout = w.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type result struct {
		AgentID string `json:"agent_id"`
		RunID   string `json:"run_id"`
		Output  string `json:"output,omitempty"`
		Status  string `json:"status"`
		Error   string `json:"error,omitempty"`
	}

	results := make([]result, len(req.AgentIDs))
	var wg sync.WaitGroup

	for i, agentID := range req.AgentIDs {
		wg.Add(1)
		go func(idx int, agentID string) {
			defer wg.Done()
			runInput, _ := json.Marshal(map[string]any{
				"agent_id": agentID,
				"input":    req.Input,
				"timeout":  timeout,
			})
			runOutput, err := w.runAgent(ctx, runInput)

			var r result
			r.AgentID = agentID
			r.Status = "failed"

			if err != nil {
				r.Error = err.Error()
			} else {
				var runResult map[string]any
				json.Unmarshal(runOutput, &runResult)
				r.RunID, _ = runResult["run_id"].(string)
				r.Output, _ = runResult["output"].(string)
				r.Status, _ = runResult["status"].(string)
			}

			results[idx] = r
		}(i, agentID)
	}

	wg.Wait()

	return json.Marshal(map[string]any{
		"results": results,
		"count":   len(results),
	})
}

func (w *OrchestratorWorkerState) runWorkflow(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		WorkflowID   string        `json:"workflow_id"`
		InitialInput string        `json:"initial_input"`
		Timeout      time.Duration `json:"timeout"`
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	if req.WorkflowID == "" || req.InitialInput == "" {
		return nil, fmt.Errorf("workflow_id and initial_input required")
	}

	// Get workflow
	w.mu.RLock()
	workflow, ok := w.Workflows[req.WorkflowID]
	w.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("workflow not found: %s", req.WorkflowID)
	}

	timeout := req.Timeout
	if timeout == 0 {
		timeout = w.DefaultTimeout * time.Duration(len(workflow.Steps)+1)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute steps
	stepResults := make(map[string]string)
	stepResults["_initial"] = req.InitialInput

	var lastOutput string
	lastOutput = req.InitialInput

	for _, step := range workflow.Steps {
		// Get input from previous step or initial
		input := stepResults[step.StepID]
		if input == "" {
			input = lastOutput
		}

		// Override with explicit inputs
		for key, fromStep := range step.Inputs {
			if val, ok := stepResults[fromStep]; ok {
				input = strings.ReplaceAll(input, "${"+key+"}", val)
			}
		}

		// Run agent
		runInput, _ := json.Marshal(map[string]any{
			"agent_id": step.AgentID,
			"input":    input,
		})
		runOutput, err := w.runAgent(ctx, runInput)

		if err != nil {
			return json.Marshal(map[string]any{
				"status":  "failed",
				"step":    step.StepID,
				"error":   err.Error(),
				"results": stepResults,
			})
		}

		var runResult map[string]any
		json.Unmarshal(runOutput, &runResult)

		output, _ := runResult["output"].(string)
		runID, _ := runResult["run_id"].(string)

		stepResults[step.StepID] = output
		lastOutput = output

		// If parallel with next, continue without waiting (already handled)
		_ = runID
	}

	return json.Marshal(map[string]any{
		"status":   "completed",
		"workflow": workflow.Name,
		"output":   lastOutput,
		"results":  stepResults,
	})
}

// --- Evolution ---

func (w *OrchestratorWorkerState) evaluate(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		RunID       string  `json:"run_id"`
		Fitness     float64 `json:"fitness"` // 0.0-1.0
		Feedback    string  `json:"feedback"`
		Correctness bool    `json:"correctness"`
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	run, ok := w.Runs[req.RunID]
	if !ok {
		return nil, fmt.Errorf("run not found: %s", req.RunID)
	}

	// Use provided fitness or calculate from correctness
	fitness := req.Fitness
	if fitness == 0 && req.Correctness {
		fitness = 1.0
	} else if fitness == 0 {
		fitness = 0.5
	}

	run.Fitness = fitness
	w.Runs[req.RunID] = run

	// Update agent fitness
	if agent, ok := w.Agents[run.GenomeID]; ok {
		agent.Fitness = fitness
		w.Agents[run.GenomeID] = agent
	}

	return json.Marshal(map[string]any{
		"run_id":  req.RunID,
		"fitness": fitness,
		"updated": true,
	})
}

func (w *OrchestratorWorkerState) evolve(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Task           string   `json:"task"`
		ParentIDs      []string `json:"parent_ids"`
		PopulationSize int      `json:"population_size"`
		Generations    int      `json:"generations"`
		MutationRate   float64  `json:"mutation_rate"`
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	if req.PopulationSize == 0 {
		req.PopulationSize = 10
	}
	if req.Generations == 0 {
		req.Generations = 5
	}
	if req.MutationRate == 0 {
		req.MutationRate = 0.1
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Get parent agents
	var parents []AgentGenome
	for _, id := range req.ParentIDs {
		if a, ok := w.Agents[id]; ok {
			parents = append(parents, a)
		}
	}

	if len(parents) == 0 {
		return nil, fmt.Errorf("no valid parent agents found")
	}

	// Evolution loop
	type scoredAgent struct {
		genome AgentGenome
		score  float64
	}

	population := make([]scoredAgent, 0, req.PopulationSize)

	// Initialize with parents + mutations
	for i := 0; i < req.PopulationSize; i++ {
		var genome AgentGenome
		if i < len(parents) {
			genome = w.mutate(parents[i], req.MutationRate)
		} else {
			// Random mutation of random parent
			genome = w.mutate(parents[rand.Intn(len(parents))], req.MutationRate)
		}
		genome.ID = generateAgentID(genome.Name)
		genome.Generation = 1
		genome.ParentIDs = req.ParentIDs

		population = append(population, scoredAgent{genome: genome, score: genome.Fitness})
	}

	// Run evolution generations
	for gen := 0; gen < req.Generations; gen++ {
		// Evaluate (simulated - in real impl would run agents on task)
		for i := range population {
			// Simulated fitness based on diversity
			population[i].score = 0.3 + rand.Float64()*0.7
			population[i].genome.Fitness = population[i].score
		}

		// Sort by fitness
		for i := 0; i < len(population)-1; i++ {
			for j := i + 1; j < len(population); j++ {
				if population[j].score > population[i].score {
					population[i], population[j] = population[j], population[i]
				}
			}
		}

		// Elitism: keep top performers
		eliteCount := min(2, len(population)/2)

		// Create next generation
		newPopulation := make([]scoredAgent, 0, req.PopulationSize)

		// Keep elites
		for i := 0; i < eliteCount; i++ {
			newPopulation = append(newPopulation, population[i])
		}

		// Fill rest with crossover + mutation
		for i := eliteCount; i < req.PopulationSize; i++ {
			parent1 := population[rand.Intn(eliteCount)].genome
			parent2 := population[rand.Intn(eliteCount)].genome

			var child AgentGenome
			if rand.Float64() < 0.3 {
				child = w.crossover(parent1, parent2)
			} else {
				child = w.mutate(parent1, req.MutationRate)
			}

			child.ID = generateAgentID(child.Name)
			child.Generation = gen + 1
			child.ParentIDs = []string{parent1.ID, parent2.ID}

			newPopulation = append(newPopulation, scoredAgent{genome: child, score: child.Fitness})
		}

		population = newPopulation
	}

	// Save best agents
	bestAgents := make([]AgentGenome, 0)
	for i := 0; i < min(3, len(population)); i++ {
		agent := population[i].genome
		w.Agents[agent.ID] = agent
		bestAgents = append(bestAgents, agent)
	}

	return json.Marshal(map[string]any{
		"evolved":      true,
		"generations":  req.Generations,
		"best_agents":  bestAgents,
		"best_fitness": population[0].score,
	})
}

func (w *OrchestratorWorkerState) mutate(agent AgentGenome, rate float64) AgentGenome {
	mutated := agent
	mutated.ID = "" // Will be regenerated

	r := rand.Float64()
	if r < rate {
		// Mutate temperature
		delta := (rand.Float64() - 0.5) * 0.2
		mutated.Temperature = math.Max(0, math.Min(2, agent.Temperature+delta))
	}

	r = rand.Float64()
	if r < rate {
		// Mutate system prompt (simple truncation/extension)
		if len(agent.SystemPrompt) > 50 {
			start := rand.Intn(len(agent.SystemPrompt) - 50)
			mutated.SystemPrompt = agent.SystemPrompt[start : start+50]
		}
	}

	r = rand.Float64()
	if r < rate {
		// Add/remove a tool
		if len(agent.Tools) > 0 && rand.Float64() < 0.5 {
			idx := rand.Intn(len(agent.Tools))
			mutated.Tools = append(agent.Tools[:idx], agent.Tools[idx+1:]...)
		} else {
			mutated.Tools = append(mutated.Tools, "tool_"+fmt.Sprintf("%d", rand.Intn(100)))
		}
	}

	return mutated
}

func (w *OrchestratorWorkerState) crossover(parent1, parent2 AgentGenome) AgentGenome {
	child := parent1

	// Crossover: mix prompts
	if rand.Float64() < 0.5 && len(parent1.SystemPrompt) > 0 && len(parent2.SystemPrompt) > 0 {
		mid1 := len(parent1.SystemPrompt) / 2
		mid2 := len(parent2.SystemPrompt) / 2
		child.SystemPrompt = parent1.SystemPrompt[:mid1] + parent2.SystemPrompt[mid2:]
	}

	// Mix tools
	toolSet := make(map[string]bool)
	for _, t := range parent1.Tools {
		toolSet[t] = true
	}
	for _, t := range parent2.Tools {
		if rand.Float64() < 0.5 {
			toolSet[t] = true
		}
	}
	child.Tools = make([]string, 0, len(toolSet))
	for t := range toolSet {
		child.Tools = append(child.Tools, t)
	}

	// Average temperature
	child.Temperature = (parent1.Temperature + parent2.Temperature) / 2

	child.ID = ""
	child.Fitness = 0.5

	return child
}

func (w *OrchestratorWorkerState) getResult(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	w.mu.RLock()
	run, ok := w.Runs[req.RunID]
	w.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("run not found: %s", req.RunID)
	}

	return json.Marshal(run)
}

// --- Workflows ---

func (w *OrchestratorWorkerState) createWorkflow(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Name  string         `json:"name"`
		Steps []WorkflowStep `json:"steps"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	if req.Name == "" || len(req.Steps) == 0 {
		return nil, fmt.Errorf("name and steps required")
	}

	workflowID := generateWorkflowID(req.Name)
	workflow := Workflow{
		ID:        workflowID,
		Name:      req.Name,
		Steps:     req.Steps,
		CreatedAt: time.Now(),
	}

	w.mu.Lock()
	w.Workflows[workflowID] = workflow
	w.mu.Unlock()

	return json.Marshal(map[string]any{
		"workflow_id": workflowID,
		"workflow":    workflow,
	})
}

func (w *OrchestratorWorkerState) listWorkflows(ctx context.Context, input json.RawMessage) ([]byte, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	workflows := make([]Workflow, 0, len(w.Workflows))
	for _, wf := range w.Workflows {
		workflows = append(workflows, wf)
	}

	return json.Marshal(workflows)
}

// --- Helpers ---

func generateAgentID(name string) string {
	return fmt.Sprintf("agent_%s_%d", strings.ReplaceAll(name, " ", "_"), time.Now().UnixNano()%10000)
}

func generateRunID() string {
	return fmt.Sprintf("run_%d", time.Now().UnixNano()%100000)
}

func generateWorkflowID(name string) string {
	return fmt.Sprintf("wf_%s_%d", strings.ReplaceAll(name, " ", "_"), time.Now().UnixNano()%10000)
}
