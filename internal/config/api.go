package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
)

// ConfigAPI provides HTTP endpoints to view and modify configuration
type ConfigAPI struct {
	cfg    *Config
	mu     sync.RWMutex
	router *mux.Router
}

func NewConfigAPI(cfg *Config) *ConfigAPI {
	api := &ConfigAPI{
		cfg:    cfg,
		router: mux.NewRouter(),
	}
	api.routes()
	return api
}

func (api *ConfigAPI) Router() *mux.Router {
	return api.router
}

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

func (api *ConfigAPI) getConfig(w http.ResponseWriter, r *http.Request) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	safeCfg := api.safeConfigCopy()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(safeCfg)
}

func (api *ConfigAPI) updateConfig(w http.ResponseWriter, r *http.Request) {
	api.mu.Lock()
	defer api.mu.Unlock()
	var newCfg Config
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		http.Error(w, fmt.Sprintf("invalid config payload: %v", err), http.StatusBadRequest)
		return
	}
	if err := newCfg.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("invalid configuration: %v", err), http.StatusBadRequest)
		return
	}
	*api.cfg = newCfg
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.safeConfigCopy())
}

func (api *ConfigAPI) reloadConfig(w http.ResponseWriter, r *http.Request) {
	api.mu.Lock()
	defer api.mu.Unlock()
	reloadedCfg, err := Load()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to reload config: %v", err), http.StatusInternalServerError)
		return
	}
	*api.cfg = *reloadedCfg
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.safeConfigCopy())
}

func (api *ConfigAPI) validateConfig(w http.ResponseWriter, r *http.Request) {
	var cfg Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, fmt.Sprintf("invalid config payload: %v", err), http.StatusBadRequest)
		return
	}
	if err := cfg.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("invalid configuration: %v", err), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"valid": true, "message": "configuration is valid"})
}

func (api *ConfigAPI) listWorkers(w http.ResponseWriter, r *http.Request) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	workers := map[string]interface{}{
		"file_io":     map[string]interface{}{"enabled": true},
		"tgi":         map[string]interface{}{"enabled": api.cfg.MCP.Workers.TGI.Enabled, "endpoint": api.cfg.MCP.Workers.TGI.Endpoint},
		"lmstudio":    map[string]interface{}{"enabled": api.cfg.MCP.Workers.LMStudio.Enabled, "endpoint": api.cfg.MCP.Workers.LMStudio.Endpoint},
		"huggingface": map[string]interface{}{"enabled": api.cfg.MCP.Workers.HuggingFace.Enabled},
		"whisper":     map[string]interface{}{"enabled": api.cfg.MCP.Workers.Whisper.Enabled, "endpoint": api.cfg.MCP.Workers.Whisper.Endpoint},
		"dataset":     map[string]interface{}{"enabled": api.cfg.MCP.Workers.Dataset.Enabled, "base_path": api.cfg.MCP.Workers.Dataset.BasePath},
		"minio":       map[string]interface{}{"enabled": api.cfg.MCP.Workers.MinIO.Enabled},
		"vector":      map[string]interface{}{"enabled": api.cfg.MCP.Workers.Vector.Enabled, "backend": api.cfg.MCP.Workers.Vector.Backend},
		"git":         map[string]interface{}{"enabled": api.cfg.MCP.Workers.Git.Enabled},
		"memory":      map[string]interface{}{"enabled": api.cfg.MCP.Workers.Memory.StoragePath != ""},
		"project":     map[string]interface{}{"enabled": api.cfg.MCP.Workers.Project.Enabled},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(workers)
}

func (api *ConfigAPI) getWorkerConfig(w http.ResponseWriter, r *http.Request) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	worker := mux.Vars(r)["worker"]
	var workerCfg interface{}

	switch worker {
	case "shell":
		workerCfg = api.cfg.MCP.Workers.Shell
	case "tgi":
		workerCfg = api.cfg.MCP.Workers.TGI
	case "lmstudio":
		workerCfg = api.cfg.MCP.Workers.LMStudio
	case "minio":
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

func (api *ConfigAPI) updateWorkerConfig(w http.ResponseWriter, r *http.Request) {
	api.mu.Lock()
	defer api.mu.Unlock()

	_ = mux.Vars(r)["worker"] // worker name from path
	var newWorkerCfg map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&newWorkerCfg); err != nil {
		http.Error(w, fmt.Sprintf("invalid worker config payload: %v", err), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newWorkerCfg)
}

func (api *ConfigAPI) safeConfigCopy() *Config {
	bytes, _ := json.Marshal(api.cfg)
	var copyCfg Config
	json.Unmarshal(bytes, &copyCfg)
	if copyCfg.MCP.Workers.MinIO.AccessKey != "" {
		copyCfg.MCP.Workers.MinIO.AccessKey = "***"
	}
	if copyCfg.MCP.Workers.MinIO.SecretKey != "" {
		copyCfg.MCP.Workers.MinIO.SecretKey = "***"
	}
	if copyCfg.MCP.Auth.Token != "" {
		copyCfg.MCP.Auth.Token = "***"
	}
	return &copyCfg
}
