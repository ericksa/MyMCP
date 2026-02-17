package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
)

// ConfigAPI provides HTTP endpoints to view and modify configuration
// Configuration changes are applied immediately but are not persisted to disk
// Useful for runtime adjustments without restarting the server

type ConfigAPI struct {
	cfg    *Config
	mu     sync.RWMutex
	router *mux.Router
}

// NewConfigAPI creates a new ConfigAPI instance
func NewConfigAPI(cfg *Config) *ConfigAPI {
	api := &ConfigAPI{
		cfg:    cfg,
		router: mux.NewRouter(),
	}
	api.routes()
	return api
}

// Router returns the configured router for the config API
func (api *ConfigAPI) Router() *mux.Router {
	return api.router
}

// routes sets up the HTTP routes for the config API
func (api *ConfigAPI) routes() {
	api.router.HandleFunc("/configure", api.getConfig).Methods("GET")
	api.router.HandleFunc("/configure/", api.getConfig).Methods("GET")
	api.router.HandleFunc("/configure", api.updateConfig).Methods("POST")
	api.router.HandleFunc("/configure/reload", api.reloadConfig).Methods("POST")
	api.router.HandleFunc("/configure/validate", api.validateConfig).Methods("POST")
	api.router.HandleFunc("/configure/workers", api.listWorkers).Methods("GET")
	api.router.HandleFunc("/configure/workers/{worker}", api.getWorkerConfig).Methods("GET")
	api.router.HandleFunc("/configure/workers/{worker}", api.updateWorkerConfig).Methods("POST")
}

// getConfig returns the current configuration as JSON
func (api *ConfigAPI) getConfig(w http.ResponseWriter, r *http.Request) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	// Create a safe copy of the config
	safeCfg := api.safeConfigCopy()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(safeCfg); err != nil {
		http.Error(w, fmt.Sprintf("failed to encode config: %v", err), http.StatusInternalServerError)
	}
}

// updateConfig updates the runtime configuration with the provided JSON payload
func (api *ConfigAPI) updateConfig(w http.ResponseWriter, r *http.Request) {
	api.mu.Lock()
	defer api.mu.Unlock()

	var newCfg Config
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		http.Error(w, fmt.Sprintf("invalid config payload: %v", err), http.StatusBadRequest)
		return
	}

	// Validate the new configuration
	if err := newCfg.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("invalid configuration: %v", err), http.StatusBadRequest)
		return
	}

	// Apply the new configuration
	*api.cfg = newCfg

	// Return the updated configuration
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(api.safeConfigCopy()); err != nil {
		http.Error(w, fmt.Sprintf("failed to encode response: %v", err), http.StatusInternalServerError)
	}
}

// reloadConfig reloads the configuration from disk
func (api *ConfigAPI) reloadConfig(w http.ResponseWriter, r *http.Request) {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Reload from disk
	reloadedCfg, err := Load()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to reload config: %v", err), http.StatusInternalServerError)
		return
	}

	// Validate the reloaded config
	if err := reloadedCfg.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("invalid configuration after reload: %v", err), http.StatusBadRequest)
		return
	}

	// Apply the reloaded config
	*api.cfg = *reloadedCfg

	// Return the updated configuration
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(api.safeConfigCopy()); err != nil {
		http.Error(w, fmt.Sprintf("failed to encode response: %v", err), http.StatusInternalServerError)
	}
}

// validateConfig validates a configuration payload without applying it
func (api *ConfigAPI) validateConfig(w http.ResponseWriter, r *http.Request) {
	var cfg Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, fmt.Sprintf("invalid config payload: %v", err), http.StatusBadRequest)
		return
	}

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("invalid configuration: %v", err), http.StatusBadRequest)
		return
	}

	// Return success
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"valid":   true,
		"message": "configuration is valid",
	})
}

// listWorkers returns a list of enabled workers and their status
func (api *ConfigAPI) listWorkers(w http.ResponseWriter, r *http.Request) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	workers := map[string]interface{}{
		"file_io": map[string]interface{}{
			"enabled": api.cfg.MCP.Workers.Shell.Enabled, // Reusing Shell for FileIO
		},
		"minio": map[string]interface{}{
			"enabled": api.cfg.MCP.Workers.MinIO.Enabled,
		},
		"vector": map[string]interface{}{
			"enabled": api.cfg.MCP.Workers.Vector.Enabled,
			"backend": api.cfg.MCP.Workers.Vector.Backend,
		},
		"git": map[string]interface{}{
			"enabled": api.cfg.MCP.Workers.Git.Enabled,
		},
		"memory": map[string]interface{}{
			"enabled": api.cfg.MCP.Workers.Memory.StoragePath != "",
		},
		"project": map[string]interface{}{
			"enabled": api.cfg.MCP.Workers.Project.Enabled,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(workers)
}

// getWorkerConfig returns the configuration for a specific worker
func (api *ConfigAPI) getWorkerConfig(w http.ResponseWriter, r *http.Request) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	worker := mux.Vars(r)["worker"]
	var workerCfg interface{}

	// Return configuration for the requested worker
	switch worker {
	case "shell":
		workerCfg = api.cfg.MCP.Workers.Shell
	case "minio":
		// Create a safe copy without sensitive data
		minioCfg := api.cfg.MCP.Workers.MinIO
		minioCfg.AccessKey = "***"
		minioCfg.SecretKey = "***"
		workerCfg = minioCfg
	case "vector":
		workerCfg = api.cfg.MCP.Workers.Vector
	case "git":
		workerCfg = api.cfg.MCP.Workers.Git
	case "memory":
		workerCfg = api.cfg.MCP.Workers.Memory
	case "project":
		workerCfg = api.cfg.MCP.Workers.Project
	default:
		http.Error(w, fmt.Sprintf("unknown worker: %s", worker), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(workerCfg)
}

// updateWorkerConfig updates the configuration for a specific worker
func (api *ConfigAPI) updateWorkerConfig(w http.ResponseWriter, r *http.Request) {
	api.mu.Lock()
	defer api.mu.Unlock()

	worker := mux.Vars(r)["worker"]

	// Decode the new worker configuration
	var newWorkerCfg map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&newWorkerCfg); err != nil {
		http.Error(w, fmt.Sprintf("invalid worker config payload: %v", err), http.StatusBadRequest)
		return
	}

	// Apply the new worker configuration
	var validationErr error
	var updatedCfg *Config

	// Create a copy of the current config for validation
	updatedCfg = api.deepCopyConfig()

	// Update the specific worker config
	switch worker {
	case "shell":
		if err := mapToStruct(newWorkerCfg, &updatedCfg.MCP.Workers.Shell); err != nil {
			validationErr = fmt.Errorf("invalid shell config: %v", err)
		}
	case "minio":
		if err := mapToStruct(newWorkerCfg, &updatedCfg.MCP.Workers.MinIO); err != nil {
			validationErr = fmt.Errorf("invalid minio config: %v", err)
		}
	case "vector":
		if err := mapToStruct(newWorkerCfg, &updatedCfg.MCP.Workers.Vector); err != nil {
			validationErr = fmt.Errorf("invalid vector config: %v", err)
		}
	case "git":
		if err := mapToStruct(newWorkerCfg, &updatedCfg.MCP.Workers.Git); err != nil {
			validationErr = fmt.Errorf("invalid git config: %v", err)
		}
	case "memory":
		if err := mapToStruct(newWorkerCfg, &updatedCfg.MCP.Workers.Memory); err != nil {
			validationErr = fmt.Errorf("invalid memory config: %v", err)
		}
	case "project":
		if err := mapToStruct(newWorkerCfg, &updatedCfg.MCP.Workers.Project); err != nil {
			validationErr = fmt.Errorf("invalid project config: %v", err)
		}
	default:
		http.Error(w, fmt.Sprintf("unknown worker: %s", worker), http.StatusNotFound)
		return
	}

	// Validate the updated configuration
	if validationErr == nil {
		validationErr = updatedCfg.Validate()
	}

	// Return error if validation failed
	if validationErr != nil {
		http.Error(w, validationErr.Error(), http.StatusBadRequest)
		return
	}

	// Apply the updated configuration
	*api.cfg = *updatedCfg

	// Return the updated worker configuration
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newWorkerCfg)
}

// safeConfigCopy creates a copy of the config without sensitive data
func (api *ConfigAPI) safeConfigCopy() *Config {
	// Create a deep copy of the config
	safeCfg := api.deepCopyConfig()

	// Remove sensitive data
	if safeCfg.MCP.Workers.MinIO.AccessKey != "" {
		safeCfg.MCP.Workers.MinIO.AccessKey = "***"
	}
	if safeCfg.MCP.Workers.MinIO.SecretKey != "" {
		safeCfg.MCP.Workers.MinIO.SecretKey = "***"
	}
	if safeCfg.MCP.Auth.Token != "" {
		safeCfg.MCP.Auth.Token = "***"
	}

	return safeCfg
}

// deepCopyConfig creates a deep copy of the current configuration
func (api *ConfigAPI) deepCopyConfig() *Config {
	// Marshal and unmarshal to create a deep copy
	bytes, err := json.Marshal(api.cfg)
	if err != nil {
		// Fallback to manual copy if marshaling fails
		return api.manualCopyConfig()
	}

	var copyCfg Config
	if err := json.Unmarshal(bytes, &copyCfg); err != nil {
		// Fallback to manual copy if unmarshaling fails
		return api.manualCopyConfig()
	}

	return &copyCfg
}

// manualCopyConfig creates a deep copy manually (fallback)
func (api *ConfigAPI) manualCopyConfig() *Config {
	api.mu.RLock()
	defer api.mu.RUnlock()

	copyCfg := *api.cfg

	// Deep copy slices
	copyCfg.MCP.Auth.AllowedTools = make([]string, len(api.cfg.MCP.Auth.AllowedTools))
	copy(copyCfg.MCP.Auth.AllowedTools, api.cfg.MCP.Auth.AllowedTools)

	copyCfg.MCP.Workers.Shell.AllowedCommands = make([]string, len(api.cfg.MCP.Workers.Shell.AllowedCommands))
	copy(copyCfg.MCP.Workers.Shell.AllowedCommands, api.cfg.MCP.Workers.Shell.AllowedCommands)

	copyCfg.MCP.Workers.MinIO.AllowedBuckets = make([]string, len(api.cfg.MCP.Workers.MinIO.AllowedBuckets))
	copy(copyCfg.MCP.Workers.MinIO.AllowedBuckets, api.cfg.MCP.Workers.MinIO.AllowedBuckets)

	copyCfg.MCP.Workers.Git.AllowedRepos = make([]string, len(api.cfg.MCP.Workers.Git.AllowedRepos))
	copy(copyCfg.MCP.Workers.Git.AllowedRepos, api.cfg.MCP.Workers.Git.AllowedRepos)

	// Deep copy project frameworks
	if api.cfg.MCP.Workers.Project.Frameworks != nil {
		copyCfg.MCP.Workers.Project.Frameworks = make(map[string]interface{})
		for k, v := range api.cfg.MCP.Workers.Project.Frameworks {
			switch val := v.(type) {
			case string:
				copyCfg.MCP.Workers.Project.Frameworks[k] = val
			case []interface{}:
				copyCfg.MCP.Workers.Project.Frameworks[k] = append([]interface{}{}, val...)
			}
		}
	}

	return &copyCfg
}

// mapToStruct converts a map to a struct using JSON marshaling/unmarshaling
func mapToStruct(input map[string]interface{}, output interface{}) error {
	bytes, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, output)
}
