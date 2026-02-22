package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// RAG Worker State
type RAGWorkerState struct {
	Tools        []ToolDef
	Documents    map[string]Document
	ChunkSize    int
	ChunkOverlap int
	VectorStore  VectorStore
	Embedder     Embedder
}

type VectorStore interface {
	Upsert(collection string, id string, vector []float32, metadata map[string]any) error
	Search(collection string, queryVector []float32, topK int) ([]SearchResult, error)
	Delete(collection string, id string) error
}

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type SearchResult struct {
	ID       string         `json:"id"`
	Score    float32        `json:"score"`
	Metadata map[string]any `json:"metadata"`
}

type Document struct {
	ID        string          `json:"id"`
	Source    string          `json:"source"`
	Title     string          `json:"title"`
	Type      string          `json:"type"`
	Content   string          `json:"content"`
	Chunks    []DocumentChunk `json:"chunks"`
	Metadata  map[string]any  `json:"metadata"`
	IndexedAt time.Time       `json:"indexed_at"`
}

type DocumentChunk struct {
	ChunkID    string `json:"chunk_id"`
	DocumentID string `json:"document_id"`
	Content    string `json:"content"`
	StartChar  int    `json:"start_char"`
	EndChar    int    `json:"end_char"`
	Index      int    `json:"index"`
}

type RAGConfig struct {
	ChunkSize    int    `json:"chunk_size"`
	ChunkOverlap int    `json:"chunk_overlap"`
	Collection   string `json:"collection"`
}

func NewRAGWorkerState(cfg RAGConfig) *RAGWorkerState {
	if cfg.ChunkSize == 0 {
		cfg.ChunkSize = 1000
	}
	if cfg.ChunkOverlap == 0 {
		cfg.ChunkOverlap = 200
	}
	if cfg.Collection == "" {
		cfg.Collection = "default"
	}

	return &RAGWorkerState{
		Tools: []ToolDef{
			{Name: "rag_ingest", Description: "Ingest document, chunk, embed, and store"},
			{Name: "rag_search", Description: "Semantic search over indexed documents"},
			{Name: "rag_ask", Description: "RAG Q&A with context retrieval"},
			{Name: "rag_list", Description: "List all indexed documents"},
			{Name: "rag_delete", Description: "Remove document from index"},
			{Name: "rag_stats", Description: "Show index statistics"},
		},
		Documents:    make(map[string]Document),
		ChunkSize:    cfg.ChunkSize,
		ChunkOverlap: cfg.ChunkOverlap,
	}
}

func (w *RAGWorkerState) GetTools() []ToolDef {
	return w.Tools
}

func (w *RAGWorkerState) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "rag_rag_ingest", "rag_ingest":
		return w.ingest(ctx, input)
	case "rag_rag_search", "rag_search":
		return w.search(ctx, input)
	case "rag_rag_ask", "rag_ask":
		return w.ask(ctx, input)
	case "rag_rag_list", "rag_list":
		return w.list(ctx, input)
	case "rag_rag_delete", "rag_delete":
		return w.delete(ctx, input)
	case "rag_rag_stats", "rag_stats":
		return w.stats(ctx, input)
	default:
		return nil, nil
	}
}

// SetEmbedder sets the embedder for the RAG worker
func (w *RAGWorkerState) SetEmbedder(e Embedder) {
	w.Embedder = e
}

// SetVectorStore sets the vector store for the RAG worker
func (w *RAGWorkerState) SetVectorStore(v VectorStore) {
	w.VectorStore = v
}

// ingest handles document ingestion
func (w *RAGWorkerState) ingest(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Source   string         `json:"source"`
		Content  string         `json:"content"`
		Title    string         `json:"title"`
		Type     string         `json:"type"`
		Metadata map[string]any `json:"metadata"`
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	if req.Source == "" && req.Content == "" {
		return nil, fmt.Errorf("either source or content required")
	}

	// Determine document type
	docType := req.Type
	if docType == "" && req.Source != "" {
		docType = detectDocType(req.Source)
	}

	// Generate document ID
	docID := generateDocID(req.Source + req.Title + time.Now().Format(time.RFC3339))

	// Chunk the content
	chunks := w.chunkText(req.Content)

	// Create document
	doc := Document{
		ID:        docID,
		Source:    req.Source,
		Title:     req.Title,
		Type:      docType,
		Content:   req.Content,
		Chunks:    chunks,
		Metadata:  req.Metadata,
		IndexedAt: time.Now(),
	}

	// Update chunk document IDs
	for i := range chunks {
		chunks[i].DocumentID = docID
	}
	doc.Chunks = chunks

	// Store document
	w.Documents[docID] = doc

	// Generate embeddings and store in vector DB if available
	if w.Embedder != nil && w.VectorStore != nil {
		texts := make([]string, len(chunks))
		for i, chunk := range chunks {
			texts[i] = chunk.Content
		}

		embeddings, err := w.Embedder.Embed(ctx, texts)
		if err != nil {
			// Log but don't fail - document is still stored
			fmt.Printf("Warning: failed to generate embeddings: %v\n", err)
		} else {
			for i, chunk := range chunks {
				metadata := map[string]any{
					"document_id": docID,
					"chunk_index": i,
					"content":     chunk.Content,
					"title":       doc.Title,
					"source":      doc.Source,
				}
				if err := w.VectorStore.Upsert("rag", chunk.ChunkID, embeddings[i], metadata); err != nil {
					fmt.Printf("Warning: failed to store vector: %v\n", err)
				}
			}
		}
	}

	return json.Marshal(map[string]any{
		"document_id": docID,
		"chunk_count": len(chunks),
		"indexed":     w.VectorStore != nil,
	})
}

// search performs semantic search
func (w *RAGWorkerState) search(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	if req.Query == "" {
		return nil, fmt.Errorf("query required")
	}

	if req.TopK == 0 {
		req.TopK = 5
	}

	// If no vector store, fall back to keyword search
	if w.VectorStore == nil || w.Embedder == nil {
		return w.keywordSearch(req.Query, req.TopK)
	}

	// Generate embedding for query
	embeddings, err := w.Embedder.Embed(ctx, []string{req.Query})
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	// Search vector store
	results, err := w.VectorStore.Search("rag", embeddings[0], req.TopK)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Format results
	type SearchResult struct {
		ChunkID    string  `json:"chunk_id"`
		DocumentID string  `json:"document_id"`
		Content    string  `json:"content"`
		Score      float32 `json:"score"`
		Title      string  `json:"title"`
		Source     string  `json:"source"`
	}

	var formattedResults []SearchResult
	for _, r := range results {
		docID, _ := r.Metadata["document_id"].(string)
		content, _ := r.Metadata["content"].(string)
		title, _ := r.Metadata["title"].(string)
		source, _ := r.Metadata["source"].(string)

		formattedResults = append(formattedResults, SearchResult{
			ChunkID:    r.ID,
			DocumentID: docID,
			Content:    content,
			Score:      r.Score,
			Title:      title,
			Source:     source,
		})
	}

	return json.Marshal(formattedResults)
}

// ask performs RAG Q&A
func (w *RAGWorkerState) ask(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Query  string `json:"query"`
		TopK   int    `json:"top_k"`
		Prompt string `json:"prompt"`
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	if req.Query == "" {
		return nil, fmt.Errorf("query required")
	}

	if req.TopK == 0 {
		req.TopK = 5
	}

	// Default prompt template
	if req.Prompt == "" {
		req.Prompt = "Based on the following context, answer the question.\n\nContext:\n%s\n\nQuestion: %s\n\nAnswer:"
	}

	// Search for relevant context
	searchInput, _ := json.Marshal(map[string]any{
		"query": req.Query,
		"top_k": req.TopK,
	})
	searchResults, err := w.search(ctx, searchInput)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	type SearchResult struct {
		Content string `json:"content"`
		Title   string `json:"title"`
	}
	var results []SearchResult
	json.Unmarshal(searchResults, &results)

	// Build context from results
	var contextBuilder strings.Builder
	for i, r := range results {
		if i > 0 {
			contextBuilder.WriteString("\n---\n")
		}
		contextBuilder.WriteString(fmt.Sprintf("[%s]\n%s", r.Title, r.Content))
	}

	// For now, return the context - actual LLM call would happen in orchestrator
	// This allows the RAG worker to be used with any LLM provider
	response := map[string]any{
		"answer":    "", // Would be filled by LLM
		"context":   contextBuilder.String(),
		"sources":   results,
		"processed": true,
	}

	return json.Marshal(response)
}

// list lists all indexed documents
func (w *RAGWorkerState) list(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}
	json.Unmarshal(input, &req)

	if req.Limit == 0 {
		req.Limit = 50
	}

	docs := make([]map[string]any, 0)
	i := 0
	skipped := 0
	for _, doc := range w.Documents {
		if skipped < req.Offset {
			skipped++
			continue
		}
		if i >= req.Limit {
			break
		}
		docs = append(docs, map[string]any{
			"id":          doc.ID,
			"title":       doc.Title,
			"source":      doc.Source,
			"type":        doc.Type,
			"chunk_count": len(doc.Chunks),
			"indexed_at":  doc.IndexedAt,
		})
		i++
	}

	return json.Marshal(docs)
}

// delete removes a document from the index
func (w *RAGWorkerState) delete(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		DocumentID string `json:"document_id"`
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	if req.DocumentID == "" {
		return nil, fmt.Errorf("document_id required")
	}

	doc, exists := w.Documents[req.DocumentID]
	if !exists {
		return nil, fmt.Errorf("document not found: %s", req.DocumentID)
	}

	// Delete from vector store
	if w.VectorStore != nil {
		for _, chunk := range doc.Chunks {
			if err := w.VectorStore.Delete("rag", chunk.ChunkID); err != nil {
				fmt.Printf("Warning: failed to delete vector: %v\n", err)
			}
		}
	}

	// Delete from documents
	delete(w.Documents, req.DocumentID)

	return json.Marshal(map[string]any{
		"deleted":     true,
		"document_id": req.DocumentID,
	})
}

// stats returns index statistics
func (w *RAGWorkerState) stats(ctx context.Context, input json.RawMessage) ([]byte, error) {
	totalDocs := len(w.Documents)
	totalChunks := 0
	for _, doc := range w.Documents {
		totalChunks += len(doc.Chunks)
	}

	byType := make(map[string]int)
	for _, doc := range w.Documents {
		byType[doc.Type]++
	}

	return json.Marshal(map[string]any{
		"documents":      totalDocs,
		"chunks":         totalChunks,
		"by_type":        byType,
		"chunk_size":     w.ChunkSize,
		"chunk_overlap":  w.ChunkOverlap,
		"vector_enabled": w.VectorStore != nil,
	})
}

// chunkText splits content into chunks using recursive character splitting
func (w *RAGWorkerState) chunkText(content string) []DocumentChunk {
	if content == "" {
		return nil
	}

	var chunks []DocumentChunk
	contentLength := len(content)

	// Split by paragraphs first (preserves logical units)
	paragraphs := strings.Split(content, "\n\n")

	var currentChunk strings.Builder
	currentStart := 0
	chunkIndex := 0

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// If adding this paragraph exceeds chunk size, save current chunk
		if currentChunk.Len()+len(para)+2 > w.ChunkSize && currentChunk.Len() > 0 {
			chunkContent := currentChunk.String()
			chunks = append(chunks, DocumentChunk{
				ChunkID:   generateDocID(chunkContent),
				Content:   chunkContent,
				StartChar: currentStart,
				EndChar:   currentStart + len(chunkContent),
				Index:     chunkIndex,
			})
			chunkIndex++

			// Handle overlap
			overlapStart := len(chunkContent) - w.ChunkOverlap
			if overlapStart > 0 {
				currentChunk.Reset()
				currentChunk.WriteString(chunkContent[overlapStart:])
				currentStart = currentStart + overlapStart
			} else {
				currentChunk.Reset()
				currentStart = 0
			}
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n\n")
		}
		currentChunk.WriteString(para)
	}

	// Add final chunk
	if currentChunk.Len() > 0 {
		chunkContent := currentChunk.String()
		chunks = append(chunks, DocumentChunk{
			ChunkID:   generateDocID(chunkContent),
			Content:   chunkContent,
			StartChar: currentStart,
			EndChar:   currentStart + len(chunkContent),
			Index:     chunkIndex,
		})
	}

	// Fallback: if no chunks, create single chunk
	if len(chunks) == 0 && contentLength > 0 {
		maxChunk := w.ChunkSize
		if contentLength < maxChunk {
			maxChunk = contentLength
		}
		chunks = append(chunks, DocumentChunk{
			ChunkID:   generateDocID(content[:maxChunk]),
			Content:   content[:maxChunk],
			StartChar: 0,
			EndChar:   maxChunk,
			Index:     0,
		})
	}

	return chunks
}

// keywordSearch fallback when vector store unavailable
func (w *RAGWorkerState) keywordSearch(query string, topK int) ([]byte, error) {
	queryLower := strings.ToLower(query)
	words := strings.Fields(queryLower)

	type scoredDoc struct {
		doc   Document
		score int
	}

	var scored []scoredDoc

	for _, doc := range w.Documents {
		contentLower := strings.ToLower(doc.Content)
		score := 0

		for _, word := range words {
			score += strings.Count(contentLower, word)
		}

		if score > 0 {
			scored = append(scored, scoredDoc{doc: doc, score: score})
		}
	}

	// Sort by score descending
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	// Limit results
	if len(scored) > topK {
		scored = scored[:topK]
	}

	type Result struct {
		DocumentID string `json:"document_id"`
		Title      string `json:"title"`
		Content    string `json:"content"`
		Score      int    `json:"score"`
	}

	var results []Result
	for _, s := range scored {
		// Return first chunk as preview
		preview := ""
		if len(s.doc.Chunks) > 0 {
			preview = s.doc.Chunks[0].Content
		}
		results = append(results, Result{
			DocumentID: s.doc.ID,
			Title:      s.doc.Title,
			Content:    preview,
			Score:      s.score,
		})
	}

	return json.Marshal(results)
}

// detectDocType from file extension
func detectDocType(source string) string {
	ext := strings.ToLower(source)
	switch {
	case strings.HasSuffix(ext, ".pdf"):
		return "pdf"
	case strings.HasSuffix(ext, ".docx") || strings.HasSuffix(ext, ".doc"):
		return "docx"
	case strings.HasSuffix(ext, ".txt"):
		return "txt"
	case strings.HasSuffix(ext, ".md"):
		return "markdown"
	case strings.HasSuffix(ext, ".html") || strings.HasSuffix(ext, ".htm"):
		return "html"
	default:
		return "text"
	}
}

// Simple ID generator
var docIDCounter int

func generateDocID(input string) string {
	docIDCounter++
	// Simple hash-like ID
	re := regexp.MustCompile(`[^a-zA-Z0-9]`)
	cleaned := re.ReplaceAllString(input, "")
	if len(cleaned) > 20 {
		cleaned = cleaned[:20]
	}
	return fmt.Sprintf("doc_%s_%d", cleaned, docIDCounter)
}
