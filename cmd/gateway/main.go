package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ericksa/mymcp/internal/config"
	"github.com/ericksa/mymcp/internal/middleware"
	"github.com/ericksa/mymcp/pkg/mcp"
	"github.com/gorilla/mux"
)

var handler *mcp.Handler

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	// Create MCP handler
	handler = mcp.NewHandler(cfg)

	// Set up router
	router := mux.NewRouter()
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.AuthMiddleware(cfg))

	// MCP endpoint
	router.PathPrefix("/mcp").Handler(handler)

	// Health endpoint
	router.HandleFunc("/health", healthHandler).Methods("GET")

	// Tools endpoints
	router.HandleFunc("/tools/file_io/{tool}", fileIOToolHandler).Methods("POST")
	router.HandleFunc("/tools/sqlite/{tool}", sqliteToolHandler).Methods("POST")
	router.HandleFunc("/tools/vector/{tool}", vectorToolHandler).Methods("POST")
	router.HandleFunc("/tools/minio/{tool}", minioToolHandler).Methods("POST")

	// Configuration API
	router.PathPrefix("/configure").Handler(config.NewConfigAPI(cfg).Router())

	// Start server
	srv := &http.Server{
		Addr:         cfg.MCP.Server.Addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Starting MCP Gateway on %s", cfg.MCP.Server.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}
	log.Println("Server stopped")
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func fileIOToolHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	toolName := vars["tool"]
	executeToolHandler(w, r, "file_io", toolName)
}

func sqliteToolHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	toolName := vars["tool"]
	executeToolHandler(w, r, "sqlite", toolName)
}

func vectorToolHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	toolName := vars["tool"]
	executeToolHandler(w, r, "vector", toolName)
}

func minioToolHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	toolName := vars["tool"]
	executeToolHandler(w, r, "minio", toolName)
}

func executeToolHandler(w http.ResponseWriter, r *http.Request, workerName, toolName string) {
	if handler == nil {
		http.Error(w, "handler not initialized", http.StatusInternalServerError)
		return
	}

	var args map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	argsJSON, _ := json.Marshal(args)
	fullToolName := workerName + "_" + toolName

	result, err := handler.ExecuteTool(r.Context(), fullToolName, argsJSON)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(result)
}
