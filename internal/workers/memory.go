package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type MemoryEntry struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata"`
	Timestamp time.Time              `json:"timestamp"`
	Tags      []string               `json:"tags"`
}

type MemoryWorker struct {
	basePath string
	mu       sync.RWMutex
	memories map[string][]MemoryEntry
}

func NewMemoryWorker(basePath string) *MemoryWorker {
	w := &MemoryWorker{
		basePath: basePath,
		memories: make(map[string][]MemoryEntry),
	}
	w.load()
	return w
}

func (w *MemoryWorker) GetTools() []ToolDef {
	return []ToolDef{
		{Name: "store", Description: "Store a memory"},
		{Name: "recall", Description: "Recall memories by query"},
		{Name: "list", Description: "List all memories"},
		{Name: "delete", Description: "Delete a memory"},
		{Name: "clear", Description: "Clear all memories"},
		{Name: "search", Description: "Search memories by tags"},
	}
}

func (w *MemoryWorker) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "store", "memory_store":
		return w.store(ctx, input)
	case "recall", "memory_recall":
		return w.recall(ctx, input)
	case "list", "memory_list":
		return w.list(ctx, input)
	case "delete", "memory_delete":
		return w.delete(ctx, input)
	case "clear", "memory_clear":
		return w.clear(ctx, input)
	case "search", "memory_search":
		return w.search(ctx, input)
	default:
		return nil, nil
	}
}

type StoreInput struct {
	Type     string                 `json:"type"`
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata"`
	Tags     []string               `json:"tags"`
	Session  string                 `json:"session"`
}

func (w *MemoryWorker) store(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req StoreInput
	json.Unmarshal(input, &req)

	if req.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	session := req.Session
	if session == "" {
		session = "default"
	}

	entry := MemoryEntry{
		ID:        generateID(),
		Type:      req.Type,
		Content:   req.Content,
		Metadata:  req.Metadata,
		Timestamp: time.Now(),
		Tags:      req.Tags,
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.memories[session] = append(w.memories[session], entry)
	w.save()

	return json.Marshal(map[string]interface{}{
		"status":    "stored",
		"id":        entry.ID,
		"session":   session,
		"timestamp": entry.Timestamp,
	})
}

type RecallInput struct {
	Query   string `json:"query"`
	Session string `json:"session"`
	Limit   int    `json:"limit"`
	Type    string `json:"type"`
}

func (w *MemoryWorker) recall(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req RecallInput
	json.Unmarshal(input, &req)

	session := req.Session
	if session == "" {
		session = "default"
	}

	limit := req.Limit
	if limit == 0 {
		limit = 10
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	var results []MemoryEntry
	memories := w.memories[session]

	for i := len(memories) - 1; i >= 0 && len(results) < limit; i-- {
		m := memories[i]
		if req.Query != "" {
			if memContains(m.Content, req.Query) || containsAnyStr(m.Tags, req.Query) {
				results = append(results, m)
			}
		} else if req.Type != "" {
			if m.Type == req.Type {
				results = append(results, m)
			}
		} else {
			results = append(results, m)
		}
	}

	return json.Marshal(map[string]interface{}{
		"session":  session,
		"memories": results,
		"count":    len(results),
	})
}

type ListInput struct {
	Session string `json:"session"`
	Limit   int    `json:"limit"`
}

func (w *MemoryWorker) list(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req ListInput
	json.Unmarshal(input, &req)

	session := req.Session
	if session == "" {
		session = "default"
	}

	limit := req.Limit
	if limit == 0 {
		limit = 50
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	memories := w.memories[session]
	if limit > len(memories) {
		limit = len(memories)
	}

	return json.Marshal(map[string]interface{}{
		"session":  session,
		"memories": memories[len(memories)-limit:],
		"count":    limit,
	})
}

type DeleteInput struct {
	ID      string `json:"id"`
	Session string `json:"session"`
}

func (w *MemoryWorker) delete(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req DeleteInput
	json.Unmarshal(input, &req)

	if req.ID == "" {
		return nil, fmt.Errorf("id is required")
	}

	session := req.Session
	if session == "" {
		session = "default"
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	memories := w.memories[session]
	for i, m := range memories {
		if m.ID == req.ID {
			w.memories[session] = append(memories[:i], memories[i+1:]...)
			w.save()
			return json.Marshal(map[string]string{
				"status": "deleted",
				"id":     req.ID,
			})
		}
	}

	return nil, fmt.Errorf("memory not found: %s", req.ID)
}

type ClearInput struct {
	Session string `json:"session"`
}

func (w *MemoryWorker) clear(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req ClearInput
	json.Unmarshal(input, &req)

	session := req.Session
	if session == "" {
		session = "default"
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	count := len(w.memories[session])
	w.memories[session] = []MemoryEntry{}
	w.save()

	return json.Marshal(map[string]interface{}{
		"status":  "cleared",
		"session": session,
		"count":   count,
	})
}

type MemorySearchInput struct {
	Tags    []string `json:"tags"`
	Session string   `json:"session"`
	Type    string   `json:"type"`
	Limit   int      `json:"limit"`
}

func (w *MemoryWorker) search(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req MemorySearchInput
	json.Unmarshal(input, &req)

	session := req.Session
	if session == "" {
		session = "default"
	}

	limit := req.Limit
	if limit == 0 {
		limit = 20
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	var results []MemoryEntry
	for _, m := range w.memories[session] {
		if req.Type != "" && m.Type != req.Type {
			continue
		}
		if len(req.Tags) > 0 && !containsAnyStr(m.Tags, req.Tags[0]) {
			continue
		}
		results = append(results, m)
		if len(results) >= limit {
			break
		}
	}

	return json.Marshal(map[string]interface{}{
		"session": session,
		"results": results,
		"count":   len(results),
	})
}

func (w *MemoryWorker) load() {
	path := filepath.Join(w.basePath, "memories.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	json.Unmarshal(data, &w.memories)
}

func (w *MemoryWorker) save() {
	path := filepath.Join(w.basePath, "memories.json")
	data, _ := json.Marshal(w.memories)
	os.WriteFile(path, data, 0644)
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func memContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && strings.Contains(s, substr))
}

func containsAnyStr(tags []string, query string) bool {
	for _, t := range tags {
		if strings.Contains(t, query) {
			return true
		}
	}
	return false
}
