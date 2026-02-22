package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ContractWorker handles legal document analysis
type ContractWorkerState struct {
	Tools     []ToolDef
	Contracts map[string]Contract
	RAGWorker *RAGWorkerState
	LLMCaller LLMCaller
}

type LLMCaller interface {
	Call(ctx context.Context, prompt string, systemPrompt string) (string, error)
}

type Contract struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Source        string     `json:"source"`
	Parties       []Party    `json:"parties"`
	EffectiveDate *time.Time `json:"effective_date,omitempty"`
	ExpiryDate    *time.Time `json:"expiry_date,omitempty"`
	Value         *float64   `json:"value,omitempty"`
	Currency      string     `json:"currency,omitempty"`
	Clauses       []Clause   `json:"clauses"`
	Terms         []KeyTerm  `json:"terms"`
	Risks         []Risk     `json:"risks"`
	Summary       string     `json:"summary"`
	RawText       string     `json:"raw_text"`
	AnalyzedAt    time.Time  `json:"analyzed_at"`
}

type Party struct {
	Name    string `json:"name"`
	Role    string `json:"role"`                  // "client", "vendor", "party_a", etc.
	Entity  string `json:"entity_type,omitempty"` // "individual", "corporation"
	Address string `json:"address,omitempty"`
}

type Clause struct {
	Type       string `json:"type"` // clause category
	Title      string `json:"title"`
	Content    string `json:"content"`
	StartChar  int    `json:"start_char"`
	EndChar    int    `json:"end_char"`
	RiskLevel  string `json:"risk_level"` // "low", "medium", "high"
	RiskReason string `json:"risk_reason,omitempty"`
}

type KeyTerm struct {
	Term       string `json:"term"`
	Definition string `json:"definition"`
	Section    string `json:"section"`
}

type Risk struct {
	Description    string `json:"description"`
	Severity       string `json:"severity"` // "low", "medium", "high", "critical"
	Recommendation string `json:"recommendation"`
	ClauseRef      string `json:"clause_ref,omitempty"`
}

// Known clause types to look for
var ClauseTypes = []string{
	"confidentiality",
	"non-disclosure",
	"termination",
	"payment",
	"liability",
	"indemnification",
	"indemnity",
	"force_majeure",
	"dispute_resolution",
	"arbitration",
	"intellectual_property",
	"IP",
	"non_compete",
	"non_solicitation",
	"warranty",
	"limitation_of_liability",
	"assignment",
	"amendment",
	"notice",
	"governing_law",
	"jurisdiction",
	"entire_agreement",
	"severability",
	"waiver",
	"relationship",
	"compliance",
	"data_protection",
	"privacy",
	"security",
	"insurance",
	"performance",
	"deliverables",
	"milestone",
	"support",
	"maintenance",
	"license",
	"ownership",
}

func NewContractWorkerState() *ContractWorkerState {
	return &ContractWorkerState{
		Tools: []ToolDef{
			{Name: "contract_parse", Description: "Extract structured data from contract"},
			{Name: "contract_summarize", Description: "Generate contract summary"},
			{Name: "contract_clause_find", Description: "Find specific clause type"},
			{Name: "contract_risk_score", Description: "Analyze contract risks"},
			{Name: "contract_compare", Description: "Compare two contracts"},
			{Name: "contract_qa", Description: "Answer questions about contract"},
			{Name: "contract_list", Description: "List all parsed contracts"},
			{Name: "contract_get", Description: "Get contract by ID"},
		},
		Contracts: make(map[string]Contract),
	}
}

func (w *ContractWorkerState) GetTools() []ToolDef {
	return w.Tools
}

func (w *ContractWorkerState) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "contract_contract_parse", "contract_parse":
		return w.parse(ctx, input)
	case "contract_contract_summarize", "contract_summarize":
		return w.summarize(ctx, input)
	case "contract_contract_clause_find", "contract_clause_find":
		return w.findClause(ctx, input)
	case "contract_contract_risk_score", "contract_risk_score":
		return w.riskScore(ctx, input)
	case "contract_contract_compare", "contract_compare":
		return w.compare(ctx, input)
	case "contract_contract_qa", "contract_qa":
		return w.qa(ctx, input)
	case "contract_contract_list", "contract_list":
		return w.list(ctx, input)
	case "contract_contract_get", "contract_get":
		return w.get(ctx, input)
	default:
		return nil, nil
	}
}

// SetRAGWorker connects the RAG worker for document storage
func (w *ContractWorkerState) SetRAGWorker(rag *RAGWorkerState) {
	w.RAGWorker = rag
}

// SetLLMCaller sets the LLM caller for AI analysis
func (w *ContractWorkerState) SetLLMCaller(caller LLMCaller) {
	w.LLMCaller = caller
}

// parse extracts structured data from a contract
func (w *ContractWorkerState) parse(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Source  string `json:"source"`
		Content string `json:"content"`
		Title   string `json:"title"`
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	if req.Source == "" && req.Content == "" {
		return nil, fmt.Errorf("source or content required")
	}

	// Use provided content or load from source
	content := req.Content
	if content == "" && req.Source != "" {
		// Would load from file - for now, require content
		return nil, fmt.Errorf("content required (file loading not implemented)")
	}

	contract := Contract{
		ID:         generateDocID(req.Source + req.Title + time.Now().Format(time.RFC3339)),
		Title:      req.Title,
		Source:     req.Source,
		RawText:    content,
		AnalyzedAt: time.Now(),
	}

	// Extract parties
	contract.Parties = w.extractParties(content)

	// Extract dates
	contract.EffectiveDate, contract.ExpiryDate = w.extractDates(content)

	// Extract value
	contract.Value, contract.Currency = w.extractValue(content)

	// Extract clauses
	contract.Clauses = w.extractClauses(content)

	// Extract key terms
	contract.Terms = w.extractTerms(content)

	// Assess risks
	contract.Risks = w.assessRisks(contract.Clauses)

	// Generate summary using LLM if available
	if w.LLMCaller != nil {
		summary, err := w.LLMCaller.Call(ctx,
			fmt.Sprintf("Summarize this contract in 3-5 bullet points. Focus on: parties, key obligations, duration, and any unusual terms.\n\nContract:\n%s", content[:min(len(content), 8000)]),
			"You are a legal assistant summarizing contracts.")
		if err == nil {
			contract.Summary = summary
		}
	}

	// Store contract
	w.Contracts[contract.ID] = contract

	// Also ingest into RAG if available
	if w.RAGWorker != nil {
		ragInput, _ := json.Marshal(map[string]any{
			"source":  req.Source,
			"content": content,
			"title":   req.Title,
			"metadata": map[string]any{
				"type":        "contract",
				"contract_id": contract.ID,
			},
		})
		w.RAGWorker.ingest(ctx, ragInput)
	}

	return json.Marshal(map[string]any{
		"contract_id":  contract.ID,
		"title":        contract.Title,
		"parties":      contract.Parties,
		"clause_count": len(contract.Clauses),
		"risk_count":   len(contract.Risks),
		"has_summary":  contract.Summary != "",
	})
}

// summarize returns contract summary
func (w *ContractWorkerState) summarize(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		ContractID string `json:"contract_id"`
		Detail     string `json:"detail"` // "brief", "detailed"
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	contract, ok := w.Contracts[req.ContractID]
	if !ok {
		return nil, fmt.Errorf("contract not found: %s", req.ContractID)
	}

	// Return existing summary or generate one
	if contract.Summary != "" {
		return json.Marshal(map[string]any{
			"summary": contract.Summary,
			"type":    "generated",
		})
	}

	// Generate on-the-fly
	summary := w.generateSummary(contract, req.Detail)
	return json.Marshal(map[string]any{
		"summary": summary,
		"type":    "generated",
	})
}

// findClause finds clauses of a specific type
func (w *ContractWorkerState) findClause(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		ContractID  string   `json:"contract_id"`
		ClauseTypes []string `json:"clause_types"` // e.g., ["termination", "liability"]
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	contract, ok := w.Contracts[req.ContractID]
	if !ok {
		return nil, fmt.Errorf("contract not found: %s", req.ContractID)
	}

	// Default to all clause types if not specified
	if len(req.ClauseTypes) == 0 {
		req.ClauseTypes = ClauseTypes
	}

	var results []Clause
	for _, clause := range contract.Clauses {
		for _, searchType := range req.ClauseTypes {
			if strings.Contains(strings.ToLower(clause.Type), strings.ToLower(searchType)) {
				results = append(results, clause)
				break
			}
		}
	}

	return json.Marshal(results)
}

// riskScore analyzes contract risks
func (w *ContractWorkerState) riskScore(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		ContractID string `json:"contract_id"`
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	contract, ok := w.Contracts[req.ContractID]
	if !ok {
		return nil, fmt.Errorf("contract not found: %s", req.ContractID)
	}

	// Count risks by severity
	counts := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0}
	for _, risk := range contract.Risks {
		counts[risk.Severity]++
	}

	// Calculate overall score (0-100, lower is worse)
	score := 100.0
	score -= float64(counts["critical"]) * 25
	score -= float64(counts["high"]) * 15
	score -= float64(counts["medium"]) * 5
	if score < 0 {
		score = 0
	}

	return json.Marshal(map[string]any{
		"contract_id":    req.ContractID,
		"score":          score,
		"risk_level":     w.scoreToLevel(score),
		"risk_counts":    counts,
		"risks":          contract.Risks,
		"recommendation": w.getRecommendation(score),
	})
}

// compare compares two contracts
func (w *ContractWorkerState) compare(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		ContractID1 string `json:"contract_id_1"`
		ContractID2 string `json:"contract_id_2"`
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	c1, ok1 := w.Contracts[req.ContractID1]
	c2, ok2 := w.Contracts[req.ContractID2]
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("one or both contracts not found")
	}

	// Compare clause types
	types1 := make(map[string]bool)
	types2 := make(map[string]bool)
	for _, c := range c1.Clauses {
		types1[c.Type] = true
	}
	for _, c := range c2.Clauses {
		types2[c.Type] = true
	}

	var onlyIn1, onlyIn2, inBoth []string
	for t := range types1 {
		if !types2[t] {
			onlyIn1 = append(onlyIn1, t)
		} else {
			inBoth = append(inBoth, t)
		}
	}
	for t := range types2 {
		if !types1[t] {
			onlyIn2 = append(onlyIn2, t)
		}
	}

	// Compare risk scores
	risk1 := w.calculateRiskScore(c1.Risks)
	risk2 := w.calculateRiskScore(c2.Risks)

	return json.Marshal(map[string]any{
		"contract_1": map[string]any{
			"id":    c1.ID,
			"title": c1.Title,
			"risks": risk1,
		},
		"contract_2": map[string]any{
			"id":    c2.ID,
			"title": c2.Title,
			"risks": risk2,
		},
		"comparison": map[string]any{
			"clauses_only_in_1": onlyIn1,
			"clauses_only_in_2": onlyIn2,
			"common_clauses":    inBoth,
			"risk_difference":   risk1 - risk2,
		},
	})
}

// qa answers questions about a contract
func (w *ContractWorkerState) qa(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		ContractID string `json:"contract_id"`
		Question   string `json:"question"`
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	contract, ok := w.Contracts[req.ContractID]
	if !ok {
		return nil, fmt.Errorf("contract not found: %s", req.ContractID)
	}

	// If we have an LLM, use it for Q&A
	if w.LLMCaller != nil {
		context := fmt.Sprintf(`
Contract: %s
Parties: %v
Effective Date: %v
Expiry Date: %v
Value: %v %v

Clauses:
%s

Terms:
%s

Answer the question based on this contract.
`, contract.Title, contract.Parties, contract.EffectiveDate, contract.ExpiryDate, contract.Value, contract.Currency,
			w.clausesToText(contract.Clauses),
			w.termsToText(contract.Terms))

		answer, err := w.LLMCaller.Call(ctx, req.Question, context)
		if err != nil {
			return nil, fmt.Errorf("LLM call failed: %w", err)
		}

		return json.Marshal(map[string]any{
			"question": req.Question,
			"answer":   answer,
			"sources":  contract.Clauses,
		})
	}

	// Fallback: keyword search
	answer := w.keywordAnswer(contract, req.Question)
	return json.Marshal(map[string]any{
		"question": req.Question,
		"answer":   answer,
		"sources":  contract.Clauses,
		"fallback": true,
	})
}

// list returns all contracts
func (w *ContractWorkerState) list(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Limit int `json:"limit"`
	}
	json.Unmarshal(input, &req)
	if req.Limit == 0 {
		req.Limit = 50
	}

	contracts := make([]map[string]any, 0)
	count := 0
	for _, c := range w.Contracts {
		if count >= req.Limit {
			break
		}
		contracts = append(contracts, map[string]any{
			"id":           c.ID,
			"title":        c.Title,
			"source":       c.Source,
			"party_count":  len(c.Parties),
			"clause_count": len(c.Clauses),
			"risk_count":   len(c.Risks),
			"analyzed_at":  c.AnalyzedAt,
		})
		count++
	}

	return json.Marshal(contracts)
}

// get returns a specific contract
func (w *ContractWorkerState) get(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		ContractID string `json:"contract_id"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	contract, ok := w.Contracts[req.ContractID]
	if !ok {
		return nil, fmt.Errorf("contract not found: %s", req.ContractID)
	}

	return json.Marshal(contract)
}

// --- Helper functions ---

func (w *ContractWorkerState) extractParties(content string) []Party {
	var parties []Party

	// Look for common party patterns
	partyPatterns := []string{
		`(?:between|by and between)\s+([A-Z][A-Za-z\s,\.]+?)\s+(?:and|&|with)\s+([A-Z][A-Za-z\s,\.]+?)`,
		`([A-Z][A-Za-z\s,\.]+?)\s+\("([^"]+)"\)`,
		`(?:party|parties)[:\s]+([A-Z][A-Za-z\s,\.]+)`,
	}

	for _, pattern := range partyPatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		matches := re.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			if len(m) > 1 && len(m[1]) > 2 && len(m[1]) < 100 {
				party := Party{Name: strings.TrimSpace(m[1])}
				// Determine role
				lower := strings.ToLower(party.Name)
				if strings.Contains(lower, "client") || strings.Contains(lower, "customer") {
					party.Role = "client"
				} else if strings.Contains(lower, "vendor") || strings.Contains(lower, "supplier") {
					party.Role = "vendor"
				} else if strings.Contains(lower, "party a") {
					party.Role = "party_a"
				} else if strings.Contains(lower, "party b") {
					party.Role = "party_b"
				}
				parties = append(parties, party)
			}
		}
	}

	// Deduplicate
	if len(parties) > 0 {
		seen := make(map[string]bool)
		var unique []Party
		for _, p := range parties {
			if !seen[p.Name] {
				seen[p.Name] = true
				unique = append(unique, p)
			}
		}
		return unique
	}

	return parties
}

func (w *ContractWorkerState) extractDates(content string) (*time.Time, *time.Time) {
	var effective, expiry *time.Time

	// Effective date patterns
	effectivePatterns := []string{
		`(?:effective|date)\s*(?:date)?[:\s]+(\d{1,2}[/-]\d{1,2}[/-]\d{2,4})`,
		`(?:effective|from)\s*(?:on)?[:\s]+(\w+\s+\d{1,2},?\s+\d{4})`,
		`commencing\s+(?:on|from)?[:\s]+(\d{1,2}[/-]\d{1,2}[/-]\d{2,4})`,
	}

	for _, pattern := range effectivePatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		matches := re.FindStringSubmatch(content)
		if len(matches) > 1 {
			if t, err := time.Parse("01/02/2006", matches[1]); err == nil {
				effective = &t
				break
			}
			if t, err := time.Parse("January 2, 2006", matches[1]); err == nil {
				effective = &t
				break
			}
		}
	}

	// Expiry patterns
	expiryPatterns := []string{
		`(?:expir(?:y|ation)|ends?|terminates?)\s*(?:on|date)?[:\s]+(\d{1,2}[/-]\d{1,2}[/-]\d{2,4})`,
		`(?:until|through)\s+(?:the\s+)?(?:date\s+of)?[:\s]+(\d{1,2}[/-]\d{1,2}[/-]\d{2,4})`,
		`(\d+)\s+(?:years?|months?)\s+(?:from|after)\s+(?:the\s+)?(?:effective\s+)?date`,
	}

	for _, pattern := range expiryPatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		matches := re.FindStringSubmatch(content)
		if len(matches) > 1 {
			if t, err := time.Parse("01/02/2006", matches[1]); err == nil {
				expiry = &t
				break
			}
		}
	}

	return effective, expiry
}

func (w *ContractWorkerState) extractValue(content string) (*float64, string) {
	// Currency patterns
	currencyPatterns := []struct {
		Pattern  string
		Currency string
	}{
		{`\$\s*([\d,]+(?:\.\d{2})?)`, "USD"},
		{`USD\s*([\d,]+(?:\.\d{2})?)`, "USD"},
		{`€\s*([\d,]+(?:\.\d{2})?)`, "EUR"},
		{`EUR\s*([\d,]+(?:\.\d{2})?)`, "EUR"},
		{`£\s*([\d,]+(?:\.\d{2})?)`, "GBP"},
		{`GBP\s*([\d,]+(?:\.\d{2})?)`, "GBP"},
	}

	for _, cp := range currencyPatterns {
		re := regexp.MustCompile(`(?i)` + cp.Pattern)
		matches := re.FindStringSubmatch(content)
		if len(matches) > 1 {
			var value float64
			fmt.Sscanf(matches[1], "%f", &value)
			if value > 0 {
				return &value, cp.Currency
			}
		}
	}

	return nil, ""
}

func (w *ContractWorkerState) extractClauses(content string) []Clause {
	var clauses []Clause

	for _, clauseType := range ClauseTypes {
		// Find paragraph containing the clause type
		patterns := []string{
			fmt.Sprintf(`(?i)(%s)[:\s]+([^\n]{50,500})`, clauseType),
			fmt.Sprintf(`(?i)(?:article|section|clause)\s+\d+[:\s]+(%s)[:\s]+([^\n]{50,500})`, clauseType),
		}

		for _, pattern := range patterns {
			re := regexp.MustCompile(pattern)
			matches := re.FindAllStringSubmatch(content, -1)
			for _, m := range matches {
				if len(m) > 2 {
					clause := Clause{
						Type:    clauseType,
						Title:   m[1],
						Content: strings.TrimSpace(m[2]),
					}
					clause.RiskLevel = w.assessClauseRisk(clauseType, clause.Content)
					clauses = append(clauses, clause)
				}
			}
		}
	}

	return clauses
}

func (w *ContractWorkerState) extractTerms(content string) []KeyTerm {
	var terms []KeyTerm

	// Look for defined terms
	patterns := []string{
		`"([^"]+)"\s+(?:means|includes?|refers to|shall mean|is defined as)[:\s]+([^\n.]{10,200})`,
		`(?:the\s+)?([A-Z][A-Za-z\s]+)\s+(?:means|includes?|refers to|shall mean|is defined as)[:\s]+([^\n.]{10,200})`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			if len(m) > 2 {
				term := KeyTerm{
					Term:       strings.TrimSpace(m[1]),
					Definition: strings.TrimSpace(m[2]),
				}
				// Clean up
				term.Term = strings.Trim(term.Term, `"`)
				if len(term.Term) > 3 && len(term.Definition) > 5 {
					terms = append(terms, term)
				}
			}
		}
	}

	return terms
}

func (w *ContractWorkerState) assessRisks(clauses []Clause) []Risk {
	var risks []Risk

	highRiskClauses := map[string]string{
		"liability":               "Unlimited liability exposure",
		"indemnification":         "Broad indemnification obligations",
		"indemnity":               "Indemnity obligations may be excessive",
		"limitation_of_liability": "Liability may be overly restricted",
		"non_compete":             "Restrictive non-compete terms",
		"termination":             "One-sided termination rights",
		"intellectual_property":   "IP rights may be assigned away",
		"IP":                      "IP ownership concerns",
	}

	for _, clause := range clauses {
		if reason, exists := highRiskClauses[clause.Type]; exists {
			risks = append(risks, Risk{
				Description:    reason,
				Severity:       clause.RiskLevel,
				Recommendation: w.getClauseRecommendation(clause.Type),
				ClauseRef:      clause.Type,
			})
		}
	}

	return risks
}

func (w *ContractWorkerState) assessClauseRisk(clauseType, content string) string {
	highRiskKeywords := []string{"unlimited", "sole", "exclusive", "waive", "forever", "irrevocable"}
	mediumRiskKeywords := []string{"may", "reasonable", "unless", "subject to"}

	contentLower := strings.ToLower(content)

	highCount := 0
	for _, kw := range highRiskKeywords {
		if strings.Contains(contentLower, kw) {
			highCount++
		}
	}

	if highCount >= 2 {
		return "high"
	}

	mediumCount := 0
	for _, kw := range mediumRiskKeywords {
		if strings.Contains(contentLower, kw) {
			mediumCount++
		}
	}

	if mediumCount >= 2 {
		return "medium"
	}

	return "low"
}

func (w *ContractWorkerState) scoreToLevel(score float64) string {
	switch {
	case score >= 80:
		return "low"
	case score >= 60:
		return "medium"
	case score >= 40:
		return "high"
	default:
		return "critical"
	}
}

func (w *ContractWorkerState) getRecommendation(score float64) string {
	switch w.scoreToLevel(score) {
	case "low":
		return "Standard contract terms. Proceed with standard review."
	case "medium":
		return "Some concerns identified. Recommend legal review of high-risk clauses."
	case "high":
		return "Multiple risk factors. Legal counsel review strongly recommended."
	case "critical":
		return "Significant risks identified. Do not execute without legal review."
	default:
		return "Review recommended."
	}
}

func (w *ContractWorkerState) getClauseRecommendation(clauseType string) string {
	Recommendations := map[string]string{
		"liability":               "Negotiate cap on liability, include mutual clauses",
		"indemnification":         "Limit to direct damages, add carve-outs",
		"indemnity":               "Require notice, limit to proven claims",
		"limitation_of_liability": "Ensure adequate cap, preserve certain rights",
		"non_compete":             "Narrow scope and duration, limit geography",
		"termination":             "Add termination for convenience, cure periods",
		"intellectual_property":   "Ensure license scope is appropriate, reverify IP ownership",
		"IP":                      "Clarify IP ownership and license terms",
	}
	if rec, ok := Recommendations[clauseType]; ok {
		return rec
	}
	return "Review with legal counsel"
}

func (w *ContractWorkerState) generateSummary(contract Contract, detail string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("## %s\n\n", contract.Title))

	if len(contract.Parties) > 0 {
		b.WriteString("**Parties:** ")
		for i, p := range contract.Parties {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(p.Name)
		}
		b.WriteString("\n\n")
	}

	if contract.EffectiveDate != nil {
		b.WriteString(fmt.Sprintf("**Effective:** %s\n", contract.EffectiveDate.Format("Jan 2, 2006")))
	}
	if contract.ExpiryDate != nil {
		b.WriteString(fmt.Sprintf("**Expires:** %s\n", contract.ExpiryDate.Format("Jan 2, 2006")))
	}
	if contract.Value != nil {
		b.WriteString(fmt.Sprintf("**Value:** %.2f %s\n", *contract.Value, contract.Currency))
	}

	b.WriteString(fmt.Sprintf("\n**Clauses Found:** %d\n", len(contract.Clauses)))
	b.WriteString(fmt.Sprintf("**Risks Identified:** %d\n", len(contract.Risks)))

	return b.String()
}

func (w *ContractWorkerState) clausesToText(clauses []Clause) string {
	var b strings.Builder
	for _, c := range clauses {
		b.WriteString(fmt.Sprintf("[%s] %s\n%s\n\n", c.Type, c.Title, c.Content))
	}
	return b.String()
}

func (w *ContractWorkerState) termsToText(terms []KeyTerm) string {
	var b strings.Builder
	for _, t := range terms {
		b.WriteString(fmt.Sprintf("%s: %s\n", t.Term, t.Definition))
	}
	return b.String()
}

func (w *ContractWorkerState) keywordAnswer(contract Contract, question string) string {
	questionLower := strings.ToLower(question)
	var relevantClauses []Clause

	for _, clause := range contract.Clauses {
		if strings.Contains(questionLower, clause.Type) {
			relevantClauses = append(relevantClauses, clause)
		}
	}

	if len(relevantClauses) == 0 {
		return "No specific clause found matching your question. Please review the contract directly."
	}

	var b strings.Builder
	b.WriteString("Found relevant clauses:\n\n")
	for _, c := range relevantClauses {
		b.WriteString(fmt.Sprintf("**%s**: %s\n\n", c.Type, c.Content))
	}
	return b.String()
}

func (w *ContractWorkerState) calculateRiskScore(risks []Risk) float64 {
	score := 100.0
	for _, r := range risks {
		switch r.Severity {
		case "critical":
			score -= 25
		case "high":
			score -= 15
		case "medium":
			score -= 5
		}
	}
	if score < 0 {
		score = 0
	}
	return score
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
