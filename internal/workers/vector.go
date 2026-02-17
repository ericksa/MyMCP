package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type VectorWorkerState struct {
	Tools     []ToolDef
	documents map[string][]float32
	ids       []string
}

func NewVectorWorkerState() *VectorWorkerState {
	return &VectorWorkerState{
		Tools: []ToolDef{
			{Name: "vector_embed_text", Description: "Embed text using a local embedding model"},
			{Name: "vector_store", Description: "Store embedded text with metadata"},
			{Name: "vector_search", Description: "Search for similar documents using vector similarity"},
			{Name: "vector_get", Description: "Retrieve stored document by ID"},
			{Name: "vector_list", Description: "List all stored document IDs"},
			{Name: "vector_delete", Description: "Delete a document by ID"},
		},
		documents: make(map[string][]float32),
		ids:       []string{},
	}
}

func (w *VectorWorkerState) GetTools() []ToolDef {
	return w.Tools
}

func (w *VectorWorkerState) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "vector_vector_embed_text", "vector_embed_text":
		return w.embedText(ctx, input)
	case "vector_vector_store", "vector_store":
		return w.store(ctx, input)
	case "vector_vector_search", "vector_search":
		return w.search(ctx, input)
	case "vector_vector_get", "vector_get":
		return w.get(ctx, input)
	case "vector_vector_list", "vector_list":
		return w.list(ctx, input)
	case "vector_vector_delete", "vector_delete":
		return w.delete(ctx, input)
	default:
		return nil, nil
	}
}

func (w *VectorWorkerState) embedText(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	embedding := simpleEmbed(req.Text)
	return json.Marshal(map[string]interface{}{
		"embedding": embedding,
		"dimension": len(embedding),
	})
}

func (w *VectorWorkerState) store(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		ID        string                 `json:"id"`
		Text      string                 `json:"text"`
		Embedding []float32              `json:"embedding,omitempty"`
		Metadata  map[string]interface{} `json:"metadata,omitempty"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if req.Embedding == nil {
		req.Embedding = simpleEmbed(req.Text)
	}

	w.documents[req.ID] = req.Embedding
	w.ids = append(w.ids, req.ID)

	return json.Marshal(map[string]interface{}{
		"success": true,
		"id":      req.ID,
		"count":   len(w.ids),
	})
}

func (w *VectorWorkerState) search(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Query     string    `json:"query"`
		Embedding []float32 `json:"embedding,omitempty"`
		TopK      int       `json:"top_k"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if req.TopK == 0 {
		req.TopK = 5
	}

	if req.Embedding == nil {
		req.Embedding = simpleEmbed(req.Query)
	}

	type result struct {
		ID    string  `json:"id"`
		Score float32 `json:"score"`
	}

	var results []result
	for _, id := range w.ids {
		docEmbedding := w.documents[id]
		if docEmbedding == nil {
			continue
		}
		score := cosineSimilarity(req.Embedding, docEmbedding)
		results = append(results, result{ID: id, Score: score})
	}

	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if len(results) > req.TopK {
		results = results[:req.TopK]
	}

	return json.Marshal(results)
}

func (w *VectorWorkerState) get(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	embedding, ok := w.documents[req.ID]
	if !ok {
		return nil, fmt.Errorf("document not found: %s", req.ID)
	}

	return json.Marshal(map[string]interface{}{
		"id":        req.ID,
		"embedding": embedding,
		"dimension": len(embedding),
	})
}

func (w *VectorWorkerState) list(ctx context.Context, input json.RawMessage) ([]byte, error) {
	return json.Marshal(w.ids)
}

func (w *VectorWorkerState) delete(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if _, ok := w.documents[req.ID]; !ok {
		return nil, fmt.Errorf("document not found: %s", req.ID)
	}

	delete(w.documents, req.ID)
	for i, id := range w.ids {
		if id == req.ID {
			w.ids = append(w.ids[:i], w.ids[i+1:]...)
			break
		}
	}

	return json.Marshal(map[string]interface{}{"success": true})
}

func simpleEmbed(text string) []float32 {
	text = strings.ToLower(text)
	words := strings.Fields(text)
	dim := 384
	embedding := make([]float32, dim)

	hash := 0
	for _, c := range text {
		hash = hash*31 + int(c)
	}

	for i := 0; i < dim; i++ {
		embedding[i] = float32(hash+i*17%100) / 100.0
	}

	for _, word := range words {
		wordHash := 0
		for _, c := range word {
			wordHash = wordHash*31 + int(c)
		}
		idx := wordHash % dim
		embedding[idx] += 1.0
	}

	mag := float32(0)
	for _, v := range embedding {
		mag += v * v
	}
	mag = sqrt(mag)
	if mag > 0 {
		for i := range embedding {
			embedding[i] = embedding[i] / mag
		}
	}

	return embedding
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	denom := sqrt(normA) * sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

func sqrt(x float32) float32 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 20; i++ {
		z = (z + x/z) / 2
	}
	return z
}
