package workers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// DBTask represents a task in the PostgreSQL database
type DBTask struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Description     string     `json:"description,omitempty"`
	Client          string     `json:"client,omitempty"`
	Project         string     `json:"project,omitempty"`
	EmailSubject    string     `json:"email_subject,omitempty"`
	EmailFrom       string     `json:"email_from,omitempty"`
	EmailID         string     `json:"email_id,omitempty"`
	DueDate         *time.Time `json:"due_date,omitempty"`
	Status          string     `json:"status"`
	Priority        int        `json:"priority"`
	Urgency         string     `json:"urgency"`
	AssignedAgent   string     `json:"assigned_agent,omitempty"`
	Source          string     `json:"source"`
	EstimatedHours  float64    `json:"estimated_hours,omitempty"`
	ActualHours     float64    `json:"actual_hours,omitempty"`
	HourlyRate      float64    `json:"hourly_rate,omitempty"`
	BillingStatus   string     `json:"billing_status"`
	Tags            []string   `json:"tags,omitempty"`
	DocumentRefs    []string   `json:"document_refs,omitempty"`
	AppleReminderID string     `json:"apple_reminder_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Task is an alias for DBTask for backwards compatibility
type Task = DBTask

// TaskWorker manages task operations with PostgreSQL
type TaskWorker struct {
	db *sql.DB
}

// NewTaskWorker creates a new TaskWorker with PostgreSQL connection
func NewTaskWorker(dbURL string) (*TaskWorker, error) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &TaskWorker{db: db}, nil
}

// NewTaskWorkerFromDB creates a TaskWorker from an existing DB connection
func NewTaskWorkerFromDB(db *sql.DB) *TaskWorker {
	return &TaskWorker{db: db}
}

// Close closes the database connection
func (w *TaskWorker) Close() error {
	return w.db.Close()
}

// GetTools returns the available task tools
func (w *TaskWorker) GetTools() []ToolDef {
	return []ToolDef{
		{Name: "task_create", Description: "Create a new task with title, description, and optional fields"},
		{Name: "task_search", Description: "Search tasks by various criteria (title, client, status, tags, date range)"},
		{Name: "task_update", Description: "Update an existing task by ID"},
		{Name: "task_delete", Description: "Delete a task by ID"},
		{Name: "task_list", Description: "List tasks with optional filtering and pagination"},
		{Name: "task_assign", Description: "Assign a task to an agent/user"},
	}
}

// Execute routes tool calls to appropriate handlers
func (w *TaskWorker) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "task_create", "task_task_create":
		return w.createTask(ctx, input)
	case "task_search", "task_task_search":
		return w.searchTasks(ctx, input)
	case "task_update", "task_task_update":
		return w.updateTask(ctx, input)
	case "task_delete", "task_task_delete":
		return w.deleteTask(ctx, input)
	case "task_list", "task_task_list":
		return w.listTasks(ctx, input)
	case "task_assign", "task_task_assign":
		return w.assignTask(ctx, input)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// CreateTaskInput defines input for creating a task
type CreateTaskInput struct {
	Title          string    `json:"title"`
	Description    string    `json:"description,omitempty"`
	Client         string    `json:"client,omitempty"`
	Project        string    `json:"project,omitempty"`
	EmailSubject   string    `json:"email_subject,omitempty"`
	EmailFrom      string    `json:"email_from,omitempty"`
	EmailID        string    `json:"email_id,omitempty"`
	DueDate        *time.Time `json:"due_date,omitempty"`
	Status         string    `json:"status,omitempty"`
	Priority       int       `json:"priority,omitempty"`
	Urgency        string    `json:"urgency,omitempty"`
	AssignedAgent  string    `json:"assigned_agent,omitempty"`
	Source         string    `json:"source,omitempty"`
	EstimatedHours float64   `json:"estimated_hours,omitempty"`
	HourlyRate     float64   `json:"hourly_rate,omitempty"`
	Tags           []string  `json:"tags,omitempty"`
	DocumentRefs   []string  `json:"document_refs,omitempty"`
}

func (w *TaskWorker) createTask(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req CreateTaskInput
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if req.Title == "" {
		return nil, fmt.Errorf("title is required")
	}

	// Set defaults
	if req.Status == "" {
		req.Status = "open"
	}
	if req.Priority == 0 {
		req.Priority = 3
	}
	if req.Urgency == "" {
		req.Urgency = "medium"
	}
	if req.Source == "" {
		req.Source = "manual"
	}

	query := `
		INSERT INTO tasks (
			title, description, client, project, email_subject, email_from, email_id,
			due_date, status, priority, urgency, assigned_agent, source,
			estimated_hours, hourly_rate, tags, document_refs
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING id, created_at, updated_at
	`

	var id string
	var createdAt, updatedAt time.Time

	err := w.db.QueryRowContext(ctx, query,
		req.Title,
		nullString(req.Description),
		nullString(req.Client),
		nullString(req.Project),
		nullString(req.EmailSubject),
		nullString(req.EmailFrom),
		nullString(req.EmailID),
		req.DueDate,
		req.Status,
		req.Priority,
		req.Urgency,
		nullString(req.AssignedAgent),
		req.Source,
		req.EstimatedHours,
		req.HourlyRate,
		arrayToString(req.Tags),
		arrayToString(req.DocumentRefs),
	).Scan(&id, &createdAt, &updatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	task := &Task{
		ID:            id,
		Title:         req.Title,
		Description:   req.Description,
		Client:        req.Client,
		Project:       req.Project,
		EmailSubject:  req.EmailSubject,
		EmailFrom:     req.EmailFrom,
		EmailID:       req.EmailID,
		DueDate:       req.DueDate,
		Status:        req.Status,
		Priority:      req.Priority,
		Urgency:       req.Urgency,
		AssignedAgent: req.AssignedAgent,
		Source:        req.Source,
		EstimatedHours: req.EstimatedHours,
		HourlyRate:    req.HourlyRate,
		Tags:          req.Tags,
		DocumentRefs:  req.DocumentRefs,
		BillingStatus: "unbilled",
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}

	return json.Marshal(task)
}

// SearchTasksInput defines search criteria
type SearchTasksInput struct {
	Query       string    `json:"query,omitempty"`
	Client      string    `json:"client,omitempty"`
	Project     string    `json:"project,omitempty"`
	Status      string    `json:"status,omitempty"`
	Urgency     string    `json:"urgency,omitempty"`
	AssignedTo  string    `json:"assigned_to,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	FromDate    *time.Time `json:"from_date,omitempty"`
	ToDate      *time.Time `json:"to_date,omitempty"`
	DueBefore   *time.Time `json:"due_before,omitempty"`
	DueAfter    *time.Time `json:"due_after,omitempty"`
	Limit       int       `json:"limit,omitempty"`
	Offset      int       `json:"offset,omitempty"`
	OrderBy     string    `json:"order_by,omitempty"`
	OrderDesc   bool      `json:"order_desc,omitempty"`
}

func (w *TaskWorker) searchTasks(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req SearchTasksInput
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if req.Limit == 0 {
		req.Limit = 50
	}
	if req.Limit > 500 {
		req.Limit = 500
	}

	// Build query
	conditions := []string{"1=1"}
	args := []interface{}{}
	argNum := 1

	if req.Query != "" {
		conditions = append(conditions, fmt.Sprintf("(title ILIKE $%d OR description ILIKE $%d)", argNum, argNum))
		args = append(args, "%"+req.Query+"%")
		argNum++
	}
	if req.Client != "" {
		conditions = append(conditions, fmt.Sprintf("client = $%d", argNum))
		args = append(args, req.Client)
		argNum++
	}
	if req.Project != "" {
		conditions = append(conditions, fmt.Sprintf("project = $%d", argNum))
		args = append(args, req.Project)
		argNum++
	}
	if req.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argNum))
		args = append(args, req.Status)
		argNum++
	}
	if req.Urgency != "" {
		conditions = append(conditions, fmt.Sprintf("urgency = $%d", argNum))
		args = append(args, req.Urgency)
		argNum++
	}
	if req.AssignedTo != "" {
		conditions = append(conditions, fmt.Sprintf("assigned_agent = $%d", argNum))
		args = append(args, req.AssignedTo)
		argNum++
	}
	if len(req.Tags) > 0 {
		conditions = append(conditions, fmt.Sprintf("tags && $%d", argNum))
		args = append(args, arrayToString(req.Tags))
		argNum++
	}
	if req.FromDate != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argNum))
		args = append(args, req.FromDate)
		argNum++
	}
	if req.ToDate != nil {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", argNum))
		args = append(args, req.ToDate)
		argNum++
	}
	if req.DueBefore != nil {
		conditions = append(conditions, fmt.Sprintf("due_date <= $%d", argNum))
		args = append(args, req.DueBefore)
		argNum++
	}
	if req.DueAfter != nil {
		conditions = append(conditions, fmt.Sprintf("due_date >= $%d", argNum))
		args = append(args, req.DueAfter)
		argNum++
	}

	// Order by
	orderCol := "created_at"
	if req.OrderBy != "" {
		validCols := map[string]bool{
			"created_at": true, "updated_at": true, "due_date": true,
			"priority": true, "title": true, "status": true,
		}
		if validCols[req.OrderBy] {
			orderCol = req.OrderBy
		}
	}
	orderDir := "ASC"
	if req.OrderDesc {
		orderDir = "DESC"
	}

	query := fmt.Sprintf(`
		SELECT id, title, description, client, project, email_subject, email_from, email_id,
			   due_date, status, priority, urgency, assigned_agent, source,
			   estimated_hours, actual_hours, hourly_rate, billing_status,
			   tags, document_refs, apple_reminder_id, created_at, updated_at
		FROM tasks
		WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, strings.Join(conditions, " AND "), orderCol, orderDir, argNum, argNum+1)

	args = append(args, req.Limit, req.Offset)

	rows, err := w.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer rows.Close()

	tasks := []*DBTask{}
	for rows.Next() {
		task, err := scanDBTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return json.Marshal(map[string]interface{}{
		"tasks": tasks,
		"count": len(tasks),
	})
}

// UpdateTaskInput defines what can be updated
type UpdateTaskInput struct {
	ID             string    `json:"id"`
	Title          string    `json:"title,omitempty"`
	Description    string    `json:"description,omitempty"`
	Client         string    `json:"client,omitempty"`
	Project        string    `json:"project,omitempty"`
	DueDate        *time.Time `json:"due_date,omitempty"`
	Status         string    `json:"status,omitempty"`
	Priority       int       `json:"priority,omitempty"`
	Urgency        string    `json:"urgency,omitempty"`
	AssignedAgent  string    `json:"assigned_agent,omitempty"`
	EstimatedHours float64   `json:"estimated_hours,omitempty"`
	ActualHours    float64   `json:"actual_hours,omitempty"`
	HourlyRate     float64   `json:"hourly_rate,omitempty"`
	BillingStatus  string    `json:"billing_status,omitempty"`
	Tags           []string  `json:"tags,omitempty"`
	DocumentRefs   []string  `json:"document_refs,omitempty"`
}

func (w *TaskWorker) updateTask(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req UpdateTaskInput
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if req.ID == "" {
		return nil, fmt.Errorf("id is required")
	}

	// Build dynamic update
	updates := []string{}
	args := []interface{}{}
	argNum := 1

	addUpdate := func(col string, val interface{}) {
		updates = append(updates, fmt.Sprintf("%s = $%d", col, argNum))
		args = append(args, val)
		argNum++
	}

	if req.Title != "" {
		addUpdate("title", req.Title)
	}
	if req.Description != "" {
		addUpdate("description", req.Description)
	}
	if req.Client != "" {
		addUpdate("client", req.Client)
	}
	if req.Project != "" {
		addUpdate("project", req.Project)
	}
	if req.DueDate != nil {
		addUpdate("due_date", req.DueDate)
	}
	if req.Status != "" {
		addUpdate("status", req.Status)
	}
	if req.Priority > 0 {
		addUpdate("priority", req.Priority)
	}
	if req.Urgency != "" {
		addUpdate("urgency", req.Urgency)
	}
	if req.AssignedAgent != "" {
		addUpdate("assigned_agent", req.AssignedAgent)
	}
	if req.EstimatedHours > 0 {
		addUpdate("estimated_hours", req.EstimatedHours)
	}
	if req.ActualHours > 0 {
		addUpdate("actual_hours", req.ActualHours)
	}
	if req.HourlyRate > 0 {
		addUpdate("hourly_rate", req.HourlyRate)
	}
	if req.BillingStatus != "" {
		addUpdate("billing_status", req.BillingStatus)
	}
	if req.Tags != nil {
		addUpdate("tags", arrayToString(req.Tags))
	}
	if req.DocumentRefs != nil {
		addUpdate("document_refs", arrayToString(req.DocumentRefs))
	}

	if len(updates) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	// Always update updated_at
	updates = append(updates, fmt.Sprintf("updated_at = $%d", argNum))
	args = append(args, time.Now())
	argNum++

	// Add ID for WHERE clause
	args = append(args, req.ID)

	query := fmt.Sprintf(`
		UPDATE tasks
		SET %s
		WHERE id = $%d
		RETURNING id, title, description, client, project, email_subject, email_from, email_id,
				  due_date, status, priority, urgency, assigned_agent, source,
				  estimated_hours, actual_hours, hourly_rate, billing_status,
				  tags, document_refs, apple_reminder_id, created_at, updated_at
	`, strings.Join(updates, ", "), argNum)

	row := w.db.QueryRowContext(ctx, query, args...)
	task, err := scanDBTask(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("task not found: %s", req.ID)
		}
		return nil, fmt.Errorf("update failed: %w", err)
	}

	return json.Marshal(task)
}

// DeleteTaskInput defines deletion input
type DeleteTaskInput struct {
	ID string `json:"id"`
}

func (w *TaskWorker) deleteTask(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req DeleteTaskInput
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if req.ID == "" {
		return nil, fmt.Errorf("id is required")
	}

	result, err := w.db.ExecContext(ctx, "DELETE FROM tasks WHERE id = $1", req.ID)
	if err != nil {
		return nil, fmt.Errorf("delete failed: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rows == 0 {
		return nil, fmt.Errorf("task not found: %s", req.ID)
	}

	return json.Marshal(map[string]interface{}{
		"success":      true,
		"deleted_id":   req.ID,
		"rows_affected": rows,
	})
}

// ListTasksInput defines list parameters
type ListTasksInput struct {
	Status     string `json:"status,omitempty"`
	Client     string `json:"client,omitempty"`
	Project    string `json:"project,omitempty"`
	AssignedTo string `json:"assigned_to,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Offset     int    `json:"offset,omitempty"`
	OrderBy    string `json:"order_by,omitempty"`
	OrderDesc  bool   `json:"order_desc,omitempty"`
}

func (w *TaskWorker) listTasks(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req ListTasksInput
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if req.Limit == 0 {
		req.Limit = 50
	}
	if req.Limit > 500 {
		req.Limit = 500
	}

	conditions := []string{}
	args := []interface{}{}
	argNum := 1

	if req.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argNum))
		args = append(args, req.Status)
		argNum++
	}
	if req.Client != "" {
		conditions = append(conditions, fmt.Sprintf("client = $%d", argNum))
		args = append(args, req.Client)
		argNum++
	}
	if req.Project != "" {
		conditions = append(conditions, fmt.Sprintf("project = $%d", argNum))
		args = append(args, req.Project)
		argNum++
	}
	if req.AssignedTo != "" {
		conditions = append(conditions, fmt.Sprintf("assigned_agent = $%d", argNum))
		args = append(args, req.AssignedTo)
		argNum++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	orderCol := "created_at"
	if req.OrderBy != "" {
		validCols := map[string]bool{
			"created_at": true, "updated_at": true, "due_date": true,
			"priority": true, "title": true, "status": true,
		}
		if validCols[req.OrderBy] {
			orderCol = req.OrderBy
		}
	}
	orderDir := "DESC"
	if !req.OrderDesc {
		orderDir = "ASC"
	}

	query := fmt.Sprintf(`
		SELECT id, title, description, client, project, email_subject, email_from, email_id,
			   due_date, status, priority, urgency, assigned_agent, source,
			   estimated_hours, actual_hours, hourly_rate, billing_status,
			   tags, document_refs, apple_reminder_id, created_at, updated_at
		FROM tasks
		%s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, whereClause, orderCol, orderDir, argNum, argNum+1)

	args = append(args, req.Limit, req.Offset)

	rows, err := w.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list failed: %w", err)
	}
	defer rows.Close()

	tasks := []*DBTask{}
	for rows.Next() {
		task, err := scanDBTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	// Get total count
	countQuery := "SELECT COUNT(*) FROM tasks"
	if whereClause != "" {
		countQuery = "SELECT COUNT(*) FROM tasks " + whereClause
	}
	var total int
	if err := w.db.QueryRowContext(ctx, countQuery, args[:argNum-1]...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count failed: %w", err)
	}

	return json.Marshal(map[string]interface{}{
		"tasks":  tasks,
		"count":  len(tasks),
		"total":  total,
		"offset": req.Offset,
		"limit":  req.Limit,
	})
}

// AssignTaskInput defines task assignment
type AssignTaskInput struct {
	ID            string `json:"id"`
	AssignedAgent string `json:"assigned_agent"`
}

func (w *TaskWorker) assignTask(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req AssignTaskInput
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	if req.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	if req.AssignedAgent == "" {
		return nil, fmt.Errorf("assigned_agent is required")
	}

	query := `
		UPDATE tasks
		SET assigned_agent = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
		RETURNING id, title, description, client, project, email_subject, email_from, email_id,
				  due_date, status, priority, urgency, assigned_agent, source,
				  estimated_hours, actual_hours, hourly_rate, billing_status,
				  tags, document_refs, apple_reminder_id, created_at, updated_at
	`

	row := w.db.QueryRowContext(ctx, query, req.AssignedAgent, req.ID)
	task, err := scanDBTask(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("task not found: %s", req.ID)
		}
		return nil, fmt.Errorf("assign failed: %w", err)
	}

	return json.Marshal(task)
}

// Helper functions

func scanDBTask(scanner interface {
	Scan(dest ...interface{}) error
}) (*Task, error) {
	task := &Task{}
	var description, client, project, emailSubject, emailFrom, emailID sql.NullString
	var dueDate sql.NullTime
	var assignedAgent, appleReminderID sql.NullString
	var estimatedHours, actualHours, hourlyRate sql.NullFloat64
	var tags, documentRefs sql.NullString

	err := scanner.Scan(
		&task.ID,
		&task.Title,
		&description,
		&client,
		&project,
		&emailSubject,
		&emailFrom,
		&emailID,
		&dueDate,
		&task.Status,
		&task.Priority,
		&task.Urgency,
		&assignedAgent,
		&task.Source,
		&estimatedHours,
		&actualHours,
		&hourlyRate,
		&task.BillingStatus,
		&tags,
		&documentRefs,
		&appleReminderID,
		&task.CreatedAt,
		&task.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	task.Description = description.String
	task.Client = client.String
	task.Project = project.String
	task.EmailSubject = emailSubject.String
	task.EmailFrom = emailFrom.String
	task.EmailID = emailID.String
	if dueDate.Valid {
		task.DueDate = &dueDate.Time
	}
	task.AssignedAgent = assignedAgent.String
	if estimatedHours.Valid {
		task.EstimatedHours = estimatedHours.Float64
	}
	if actualHours.Valid {
		task.ActualHours = actualHours.Float64
	}
	if hourlyRate.Valid {
		task.HourlyRate = hourlyRate.Float64
	}
	task.Tags = parseArray(tags.String)
	task.DocumentRefs = parseArray(documentRefs.String)
	task.AppleReminderID = appleReminderID.String

	return task, nil
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func arrayToString(arr []string) interface{} {
	if len(arr) == 0 {
		return nil
	}
	return "{" + strings.Join(arr, ",") + "}"
}

func parseArray(s string) []string {
	if s == "" || s == "{}" || s == "null" {
		return nil
	}
	// Remove braces
	s = strings.Trim(s, "{}")
	if s == "" {
		return nil
	}
	// Split by comma, handling quoted strings
	var result []string
	var current strings.Builder
	inQuotes := false
	for _, r := range s {
		switch r {
		case '"':
			inQuotes = !inQuotes
		case ',':
			if !inQuotes {
				result = append(result, strings.Trim(current.String(), `"`))
				current.Reset()
			} else {
				current.WriteRune(r)
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		result = append(result, strings.Trim(current.String(), `"`))
	}
	return result
}