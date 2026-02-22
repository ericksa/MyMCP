# RAG + Contract Worker Spec - MyMCP

**Status:** Draft  
**Purpose:** Document ingestion, RAG pipeline, and contract analysis

---

## Overview

Workers for:
1. **Document ingestion** - PDF, DOCX, TXT processing
2. **RAG pipeline** - chunking, embedding, storage
3. **Contract analysis** - legal document Q&A

---

## RAG Worker

### Tools

| Tool | Description |
|------|-------------|
| `rag_ingest` | Ingest document, chunk, embed, store |
| `rag_search` | Semantic search over documents |
| `rag_ask` | RAG Q&A with context |
| `rag_list` | List indexed documents |
| `rag_delete` | Remove document from index |
| `rag_stats` | Show index statistics |

### Data Structures

```go
type Document struct {
    ID        string                 `json:"id"`
    Source    string                 `json:"source"`      // file path or URL
    Title     string                 `json:"title"`
    Type      string                 `json:"type"`        // "pdf", "docx", "txt"
    Content   string                 `json:"content"`
    Chunks    []DocumentChunk        `json:"chunks"`
    Metadata  map[string]any          `json:"metadata"`
    CreatedAt time.Time              `json:"created_at"`
}

type DocumentChunk struct {
    ChunkID   string    `json:"chunk_id"`
    DocumentID string   `json:"document_id"`
    Content   string    `json:"content"`
    Embedding []float32 `json:"embedding"`
    StartChar int       `json:"start_char"`
    EndChar   int       `json:"end_char"`
}

type RAGConfig struct {
    ChunkSize    int     `json:"chunk_size"`     // default 1000
    ChunkOverlap int     `json:"chunk_overlap"`  // default 200
    Embedder     string  `json:"embedder"`       // "tgi", "lmstudio"
    TopK         int     `json:"top_k"`         // default 5
}
```

### Ingest Flow

```
1. Load file (PDF/DOCX/TXT)
2. Extract text
3. Chunk with RecursiveCharacterTextSplitter
4. Generate embeddings (via embedder worker)
5. Store in vector DB
6. Return document_id
```

### Search Flow

```
1. Embed query
2. Vector similarity search (top_k)
3. Retrieve chunks + source metadata
4. Return ranked results
```

### Ask Flow

```
1. Embed query
2. Retrieve context chunks
3. Build prompt with context
4. Call LLM
5. Return answer + sources
```

---

## Contract Worker

### Purpose
Specialized RAG for legal contracts with:
- Clause extraction
- Risk analysis
- Key term extraction
- Q&A

### Tools

| Tool | Description |
|------|-------------|
| `contract_parse` | Extract structured data from contract |
| `contract_summarize` | Generate contract summary |
| `contract_clause_find` | Find specific clause type |
| `contract_risk_score` | Analyze contract risks |
| `contract_compare` | Compare two contracts |
| `contract_qa` | Answer questions about contract |

### Contract Schema

```go
type Contract struct {
    ID           string            `json:"id"`
    Title        string            `json:"title"`
    Parties      []string          `json:"parties"`
    EffectiveDate time.Time        `json:"effective_date"`
    ExpiryDate   *time.Time       `json:"expiry_date,omitempty"`
    Value        float64          `json:"value"`
    Currency     string            `json:"currency"`
    Clauses      []Clause          `json:"clauses"`
    Terms        []KeyTerm         `json:"terms"`
    Risks        []Risk            `json:"risks"`
}

type Clause struct {
    Type        string   `json:"type"`         // "confidentiality", "termination", etc.
    Title       string   `json:"title"`
    Content     string   `json:"content"`
    StartChar   int      `json:"start_char"`
    EndChar     int      `json:"end_char"`
    RiskLevel   string   `json:"risk_level"`   // "low", "medium", "high"
}

type KeyTerm struct {
    Term        string   `json:"term"`
    Definition  string   `json:"definition"`
    Section     string   `json:"section"`
}

type Risk struct {
    Description string   `json:"description"`
    Severity    string   `json:"severity"`
    Recommendation string `json:"recommendation"`
}
```

### Parse Flow

```
1. Load contract PDF
2. Extract text
3. Identify parties (LLM)
4. Extract key dates (regex + LLM)
5. Find clauses by type (LLM)
6. Extract key terms
7. Score risks
8. Return structured Contract
```

---

## Configuration

```yaml
mcp:
  workers:
    rag:
      enabled: true
      chunk_size: 1000
      chunk_overlap: 200
      embedder: "tgi"
      embedding_model: "bge-m3"
      vector_collection: "contracts"
      
    contract:
      enabled: true
      llm_model: "qwen3:70b"
      clause_types:
        - confidentiality
        - termination
        - payment
        - liability
        - indemnification
        - force_majeure
        - dispute_resolution
        - intellectual_property
```

---

## Integration

- Uses **vector worker** for embeddings
- Uses **tgi/lmstudio** for LLM calls
- Uses **file_io** for document loading
- Stores in **sqlite** for metadata

---

## TODO

- [ ] PDF/DOCX parser
- [ ] Chunking strategies
- [ ] Clause detection prompts
- [ ] Risk scoring prompts
- [ ] Contract comparison logic

---

*Draft - companion to ORCHESTRATOR_SPEC.md*
