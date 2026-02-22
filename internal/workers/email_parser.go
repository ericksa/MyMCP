package workers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"net/textproto"
)

type EmailParserWorker struct {
	maildirPath string
}

func NewEmailParserWorker(maildirPath string) *EmailParserWorker {
	return &EmailParserWorker{maildirPath: maildirPath}
}

func (w *EmailParserWorker) GetTools() []ToolDef {
	return []ToolDef{
		{Name: "email_parse_file", Description: "Parse an email file (.eml, .emlx, or Maildir message) and extract structured data"},
		{Name: "email_parse_raw", Description: "Parse raw email content and extract structured data"},
		{Name: "email_extract_tasks", Description: "Extract actionable tasks from email content"},
		{Name: "email_search_by_subject", Description: "Search emails by subject pattern in Maildir"},
		{Name: "email_list_recent", Description: "List recent emails in a Maildir folder"},
	}
}

func (w *EmailParserWorker) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "email_parse_file":
		return w.parseFile(ctx, input)
	case "email_parse_raw":
		return w.parseRaw(ctx, input)
	case "email_extract_tasks":
		return w.extractTasks(ctx, input)
	case "email_search_by_subject":
		return w.searchBySubject(ctx, input)
	case "email_list_recent":
		return w.listRecent(ctx, input)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

type EmailData struct {
	ID          string            `json:"id"`
	MessageID   string            `json:"message_id"`
	Subject     string            `json:"subject"`
	From        []string          `json:"from"`
	To          []string          `json:"to"`
	CC          []string          `json:"cc"`
	Date        time.Time         `json:"date"`
	BodyText    string            `json:"body_text"`
	BodyHTML    string            `json:"body_html,omitempty"`
	Attachments []Attachment      `json:"attachments,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	IsReply     bool              `json:"is_reply"`
	InReplyTo   string            `json:"in_reply_to,omitempty"`
	References  []string          `json:"references,omitempty"`
}

type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	ContentID   string `json:"content_id,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
}

type TaskExtraction struct {
	EmailSubject string     `json:"email_subject"`
	From         string     `json:"from"`
	Date         string     `json:"date"`
	Tasks        []EmailTask `json:"tasks"`
	Summary      string     `json:"summary"`
	Urgency      string     `json:"urgency"` // high, medium, low
	DueDate      string     `json:"due_date,omitempty"`
}

type EmailTask struct {
	Description string   `json:"description"`
	ActionItems []string `json:"action_items,omitempty"`
	AssignedTo  string   `json:"assigned_to,omitempty"`
	Context     string   `json:"context,omitempty"`
}

func (w *EmailParserWorker) parseFile(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	fullPath := w.resolvePath(req.Path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	email, err := w.parseEmail(string(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse email: %w", err)
	}

	return json.Marshal(email)
}

func (w *EmailParserWorker) parseRaw(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	email, err := w.parseEmail(req.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse email: %w", err)
	}

	return json.Marshal(email)
}

func (w *EmailParserWorker) parseEmail(content string) (*EmailData, error) {
	msg, err := mail.ReadMessage(strings.NewReader(content))
	if err != nil {
		// Try to parse as raw content without full headers
		return w.parseSimpleEmail(content), nil
	}

	email := &EmailData{
		Headers: make(map[string]string),
	}

	// Parse headers
	for key := range msg.Header {
		email.Headers[key] = msg.Header.Get(key)
	}

	email.MessageID = msg.Header.Get("Message-Id")
	email.Subject = msg.Header.Get("Subject")
	
	// Parse date
	if dateStr := msg.Header.Get("Date"); dateStr != "" {
		if t, err := msg.Header.Date(); err == nil {
			email.Date = t
		}
	}

	// Parse addresses
	email.From = w.parseAddressList(msg.Header.Get("From"))
	email.To = w.parseAddressList(msg.Header.Get("To"))
	email.CC = w.parseAddressList(msg.Header.Get("Cc"))

	// Check if reply
	email.InReplyTo = msg.Header.Get("In-Reply-To")
	if refs := msg.Header.Get("References"); refs != "" {
		email.References = strings.Fields(refs)
	}
	email.IsReply = email.InReplyTo != ""

	// Parse body
	contentType := msg.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/plain"
	}

	body, _ := io.ReadAll(msg.Body)
	w.parseBodyParts(contentType, string(body), email)

	// Generate ID hash
	h := sha256.New()
	h.Write([]byte(email.MessageID + email.Subject + email.Date.String()))
	email.ID = base64.URLEncoding.EncodeToString(h.Sum(nil))[:16]

	return email, nil
}

func (w *EmailParserWorker) parseSimpleEmail(content string) *EmailData {
	email := &EmailData{
		Headers: make(map[string]string),
		ID:      fmt.Sprintf("%x", sha256.Sum256([]byte(content)))[:16],
	}

	// Try to extract basic info with regex
	subjectRe := regexp.MustCompile(`\nSubject:\s*(.+?)(\n\w+:|\n\n|$)`)
	fromRe := regexp.MustCompile(`\nFrom:\s*(.+?)(\n\w+:|\n\n|$)`)
	
	if matches := subjectRe.FindStringSubmatch(content); len(matches) > 1 {
		email.Subject = strings.TrimSpace(matches[1])
	}
	if matches := fromRe.FindStringSubmatch(content); len(matches) > 1 {
		email.From = []string{strings.TrimSpace(matches[1])}
	}

	// Body is everything after first blank line
	if idx := strings.Index(content, "\n\n"); idx != -1 {
		email.BodyText = strings.TrimSpace(content[idx+2:])
	} else {
		email.BodyText = strings.TrimSpace(content)
	}

	return email
}

func (w *EmailParserWorker) parseBodyParts(contentType string, body string, email *EmailData) {
	mediaType, params, _ := mime.ParseMediaType(contentType)
	
	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			email.BodyText = body
			return
		}

		mr := multipart.NewReader(strings.NewReader(body), boundary)
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				continue
			}

			partContentType := part.Header.Get("Content-Type")
			partBody, _ := io.ReadAll(part)

			if partContentType == "" {
				partContentType = "text/plain"
			}

			w.handlePart(partContentType, part.Header, string(partBody), email)
		}
	} else {
		w.handlePart(contentType, textproto.MIMEHeader{}, body, email)
	}
}

func (w *EmailParserWorker) handlePart(contentType string, header textproto.MIMEHeader, body string, email *EmailData) {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	
	switch {
	case strings.HasPrefix(mediaType, "text/plain"):
		email.BodyText = body
	case strings.HasPrefix(mediaType, "text/html"):
		email.BodyHTML = body
		if email.BodyText == "" {
			email.BodyText = w.htmlToText(body)
		}
	case strings.HasPrefix(mediaType, "multipart/"):
		// Recursively parse
		w.parseBodyParts(contentType, body, email)
	default:
		// Attachment
		filename := ""
		if header != nil {
			if disp := header.Get("Content-Disposition"); disp != "" {
				_, params, _ := mime.ParseMediaType(disp)
				filename = params["filename"]
			}
			if filename == "" {
				_, params, _ := mime.ParseMediaType(contentType)
				filename = params["name"]
			}
		}
		
		h := sha256.New()
		h.Write([]byte(body))
		
		email.Attachments = append(email.Attachments, Attachment{
			Filename:    filename,
			ContentType: mediaType,
			Size:        int64(len(body)),
			ContentHash: base64.URLEncoding.EncodeToString(h.Sum(nil))[:16],
		})
	}
}

func (w *EmailParserWorker) parseAddressList(addrStr string) []string {
	if addrStr == "" {
		return nil
	}
	addresses := strings.Split(addrStr, ",")
	for i, addr := range addresses {
		addresses[i] = strings.TrimSpace(addr)
	}
	return addresses
}

func (w *EmailParserWorker) htmlToText(html string) string {
	// Simple HTML to text conversion
	// Remove script and style tags
	reStyle := regexp.MustCompile(`(?i)<(script|style)[^>]*>[^<]*</(script|style)>`)
	html = reStyle.ReplaceAllString(html, "")
	
	// Convert breaks and paragraphs to newlines
	html = strings.ReplaceAll(html, "<br>", "\n")
	html = strings.ReplaceAll(html, "<br/>", "\n")
	html = strings.ReplaceAll(html, "<p>", "\n\n")
	html = strings.ReplaceAll(html, "</p>", "")
	
	// Remove remaining HTML tags
	re := regexp.MustCompile(`<[^>]+>`)
	html = re.ReplaceAllString(html, "")
	
	// Decode common entities
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")
	
	return strings.TrimSpace(html)
}

func (w *EmailParserWorker) extractTasks(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Content string `json:"content"` // Raw email content
		Subject string `json:"subject,omitempty"`
		From    string `json:"from,omitempty"`
		Date    string `json:"date,omitempty"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	var bodyText string
	if req.Content != "" {
		email, _ := w.parseEmail(req.Content)
		bodyText = email.BodyText
		if req.Subject == "" {
			req.Subject = email.Subject
		}
		if req.From == "" && len(email.From) > 0 {
			req.From = email.From[0]
		}
	} else {
		bodyText = req.Content
	}

	extraction := &TaskExtraction{
		EmailSubject: req.Subject,
		From:         req.From,
		Date:         req.Date,
		Tasks:        w.extractTasksFromText(req.Subject, bodyText),
		Summary:      w.generateSummary(bodyText),
		Urgency:      w.classifyUrgency(req.Subject, bodyText),
		DueDate:      w.extractDueDate(req.Subject, bodyText),
	}

	return json.Marshal(extraction)
}

func (w *EmailParserWorker) extractTasksFromText(subject, body string) []EmailTask {
	tasks := []EmailTask{}

	// Combine subject and body for analysis
	fullText := subject + " " + body
	fullText = strings.ToLower(fullText)

	// Keyword patterns that indicate tasks
	actionKeywords := []string{
		"review", "approve", "sign", "send", "prepare", "create",
		"update", "fix", "implement", "schedule", "follow up",
		"please", "need to", "action required", "asap",
		"let me know", "get back to", "waiting for",
	}

	// Check for action indicators
	for _, keyword := range actionKeywords {
		if strings.Contains(fullText, keyword) {
			task := EmailTask{
				Description: "Action required: " + w.capitalizeFirst(keyword),
				Context:     subject,
			}
			
			// Try to extract specific action
			if keyword == "let me know" || keyword == "get back to" {
				task.ActionItems = append(task.ActionItems, "Respond to sender")
			} else if keyword == "schedule" {
				task.ActionItems = append(task.ActionItems, "Check calendar availability")
			}
			
			tasks = append(tasks, task)
			break
		}
	}

	// Extract questions as tasks
	questionRe := regexp.MustCompile(`(?m)^[^.]*\?`)
	questions := questionRe.FindAllString(body, -1)
	for _, q := range questions {
		if strings.TrimSpace(q) != "" {
			tasks = append(tasks, EmailTask{
				Description: "Answer: " + strings.TrimSpace(q),
			})
		}
	}

	return tasks
}

func (w *EmailParserWorker) generateSummary(text string) string {
	// Simple summary - first few sentences
	sentences := strings.Split(text, ".")
	if len(sentences) > 0 {
		summary := sentences[0]
		if len(sentences) > 1 {
			summary += "." + sentences[1]
		}
		return strings.TrimSpace(summary) + "..."
	}
	return text
}

func (w *EmailParserWorker) classifyUrgency(subject, body string) string {
	fullText := strings.ToLower(subject + " " + body)
	
	urgentPatterns := []string{"urgent", "asap", "immediately", "deadline", "today", "critical"}
	highPatterns := []string{"important", "please review", "action required", "needed by"}
	
	for _, pattern := range urgentPatterns {
		if strings.Contains(fullText, pattern) {
			return "high"
		}
	}
	
	for _, pattern := range highPatterns {
		if strings.Contains(fullText, pattern) {
			return "medium"
		}
	}
	
	return "low"
}

func (w *EmailParserWorker) extractDueDate(subject, body string) string {
	// Look for date patterns
	patterns := []string{
		`\b(by|before|on|due)\s+(Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Sunday)[,\s]+(\w+\s+\d{1,2})`,
		`\b(by|before|on|due)\s+(\d{1,2}[./-]\d{1,2}[./-]\d{2,4})`,
		`\b(\d{1,2}/\d{1,2}/\d{2,4})`,
		`\b(by|before)\s+(tomorrow|next week|this week)`,
		`\bdeadline[:\s]+(\w+\s+\d{1,2})`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(subject + " " + body); len(matches) > 0 {
			return matches[0]
		}
	}

	return ""
}

func (w *EmailParserWorker) searchBySubject(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Pattern string `json:"pattern"`
		Folder  string `json:"folder,omitempty"`
		Limit   int    `json:"limit,omitempty"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if req.Folder == "" {
		req.Folder = "INBOX"
	}
	if req.Limit == 0 {
		req.Limit = 50
	}

	folderPath := filepath.Join(w.maildirPath, req.Folder, "cur")
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read folder: %w", err)
	}

	re, err := regexp.Compile(req.Pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	matches := []*EmailData{}
	for _, entry := range entries {
		if len(matches) >= req.Limit {
			break
		}
		
		path := filepath.Join(folderPath, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		email, err := w.parseEmail(string(data))
		if err != nil {
			continue
		}

		if re.MatchString(email.Subject) {
			matches = append(matches, email)
		}
	}

	return json.Marshal(matches)
}

func (w *EmailParserWorker) listRecent(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Folder string `json:"folder,omitempty"`
		Limit  int    `json:"limit,omitempty"`
		Since  string `json:"since,omitempty"` // ISO8601
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if req.Folder == "" {
		req.Folder = "INBOX"
	}
	if req.Limit == 0 {
		req.Limit = 20
	}

	folderPath := filepath.Join(w.maildirPath, req.Folder, "cur")
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read folder: %w", err)
	}

	type fileWithTime struct {
		name string
		info os.FileInfo
		path string
	}

	files := []fileWithTime{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileWithTime{
			name: entry.Name(),
			info: info,
			path: filepath.Join(folderPath, entry.Name()),
		})
	}

	// Sort by modification time (newest first)
	for i := 0; i < len(files)-1; i++ {
		for j := i + 1; j < len(files); j++ {
			if files[j].info.ModTime().After(files[i].info.ModTime()) {
				files[i], files[j] = files[j], files[i]
			}
		}
	}

	emails := []*EmailData{}
	for _, f := range files {
		if len(emails) >= req.Limit {
			break
		}

		data, err := os.ReadFile(f.path)
		if err != nil {
			continue
		}

		email, err := w.parseEmail(string(data))
		if err != nil {
			continue
		}

		emails = append(emails, email)
	}

	return json.Marshal(emails)
}

func (w *EmailParserWorker) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(w.maildirPath, path)
}

func (w *EmailParserWorker) capitalizeFirst(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
