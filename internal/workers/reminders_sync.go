package workers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// RemindersSyncWorker syncs Apple Reminders with PostgreSQL tasks table
type RemindersSyncWorkerState struct {
	Tools    []ToolDef
	DB       *sql.DB
	remindctlPath string
}

// RemindersTask represents a task in the reminders database
type RemindersTask struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Notes       string     `json:"notes,omitempty"`
	ListName    string     `json:"list_name"`
	Priority    string     `json:"priority"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	Completed   bool       `json:"completed"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ExternalID  string     `json:"external_id"` // Apple Reminders ID
	Source      string     `json:"source"`      // "apple" or "mymcp"
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// AppleReminder represents a reminder from remindctl
type AppleReminder struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Notes       string     `json:"notes,omitempty"`
	List        string     `json:"list"`
	Priority    string     `json:"priority"`
	DueDate     *time.Time `json:"dueDate,omitempty"`
	Completed   bool       `json:"completed"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	ModifiedAt  time.Time  `json:"modifiedAt"`
}

// RemindersConfig contains configuration for the reminders sync worker
type RemindersConfig struct {
	Enabled       bool   `json:"enabled" mapstructure:"enabled"`
	PostgresURL   string `json:"postgres_url" mapstructure:"postgres_url"`
	RemindctlPath string `json:"remindctl_path" mapstructure:"remindctl_path"`
	SyncInterval  int    `json:"sync_interval" mapstructure:"sync_interval"` // seconds
}

// NewRemindersSyncWorker creates a new reminders sync worker
func NewRemindersSyncWorker(cfg RemindersConfig) (*RemindersSyncWorkerState, error) {
	// Find remindctl
	remindctlPath := cfg.RemindctlPath
	if remindctlPath == "" {
		if path, err := exec.LookPath("remindctl"); err == nil {
			remindctlPath = path
		} else {
			remindctlPath = "/usr/local/bin/remindctl"
		}
	}

	// Connect to PostgreSQL if URL provided
	var db *sql.DB
	var err error
	if cfg.PostgresURL != "" {
		db, err = sql.Open("postgres", cfg.PostgresURL)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}
		if err := db.Ping(); err != nil {
			return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
		}

		// Don't create table - it should already exist from SUBCONTRACTING_TASK_SYSTEM.md
		// The table has: id, title, description, client, project, email_subject, 
		// email_from, email_id, due_date, status, priority, urgency, assigned_agent,
		// source, estimated_hours, actual_hours, hourly_rate, billing_status, tags,
		// document_refs, apple_reminder_id, vector_embedding, created_at, updated_at

		// Just verify connection works
		if err := db.Ping(); err != nil {
			return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
		}
	}

	return &RemindersSyncWorkerState{
		DB:            db,
		remindctlPath: remindctlPath,
		Tools: []ToolDef{
			{Name: "reminders_sync_to_db", Description: "Sync Apple Reminders to PostgreSQL database"},
			{Name: "reminders_sync_from_db", Description: "Sync PostgreSQL tasks to Apple Reminders"},
			{Name: "reminders_create", Description: "Create a new reminder in both Apple and database"},
			{Name: "reminders_complete", Description: "Mark a reminder as complete"},
			{Name: "reminders_list", Description: "List reminders from database"},
			{Name: "reminders_show", Description: "Show reminders from Apple Reminders"},
			{Name: "reminders_sync_status", Description: "Check sync status and counts"},
		},
	}, nil
}

// createTasksTable creates the tasks table in PostgreSQL
func createTasksTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS tasks (
		id SERIAL PRIMARY KEY,
		title TEXT NOT NULL,
		notes TEXT,
		list_name TEXT DEFAULT 'Default',
		priority TEXT DEFAULT 'none',
		due_date TIMESTAMP,
		completed BOOLEAN DEFAULT FALSE,
		completed_at TIMESTAMP,
		external_id TEXT UNIQUE,
		source TEXT DEFAULT 'mymcp',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		synced_at TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_tasks_external_id ON tasks(external_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_completed ON tasks(completed);
	CREATE INDEX IF NOT EXISTS idx_tasks_due_date ON tasks(due_date);
	`
	_, err := db.Exec(query)
	return err
}

// GetTools returns the available tools
func (w *RemindersSyncWorkerState) GetTools() []ToolDef {
	return w.Tools
}

// Execute handles tool execution
func (w *RemindersSyncWorkerState) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "reminders_reminders_sync_to_db", "reminders_sync_to_db":
		return w.syncToDB(ctx, input)
	case "reminders_reminders_sync_from_db", "reminders_sync_from_db":
		return w.syncFromDB(ctx, input)
	case "reminders_reminders_create", "reminders_create":
		return w.createReminder(ctx, input)
	case "reminders_reminders_complete", "reminders_complete":
		return w.completeReminder(ctx, input)
	case "reminders_reminders_list", "reminders_list":
		return w.listReminders(ctx, input)
	case "reminders_reminders_show", "reminders_show":
		return w.showReminders(ctx, input)
	case "reminders_reminders_sync_status", "reminders_sync_status":
		return w.syncStatus(ctx, input)
	default:
		return nil, nil
	}
}

// syncToDB syncs Apple Reminders to PostgreSQL
func (w *RemindersSyncWorkerState) syncToDB(ctx context.Context, input json.RawMessage) ([]byte, error) {
	if w.DB == nil {
		return nil, fmt.Errorf("database not configured")
	}

	var req struct {
		List string `json:"list"` // optional: specific list to sync
	}
	json.Unmarshal(input, &req)

	// Fetch all reminders from Apple Reminders
	reminders, err := w.fetchAppleReminders(ctx, req.List)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Apple Reminders: %w", err)
	}

	synced := 0
	updated := 0
	duplicates := 0

	for _, reminder := range reminders {
		// Check if already exists by external_id
		var existingTask RemindersTask
		err := w.DB.QueryRowContext(ctx,
			"SELECT id, title, notes, completed, updated_at FROM tasks WHERE external_id = $1",
			reminder.ID,
		).Scan(&existingTask.ID, &existingTask.Title, &existingTask.Notes, &existingTask.Completed, &existingTask.UpdatedAt)

		if err == sql.ErrNoRows {
			// New reminder - insert
			err = w.insertTask(ctx, reminder)
			if err != nil {
				return nil, fmt.Errorf("failed to insert task: %w", err)
			}
			synced++
		} else if err == nil {
			// Existing - check if Apple version is newer
			if reminder.ModifiedAt.After(existingTask.UpdatedAt) {
				err = w.updateTaskFromApple(ctx, existingTask.ID, reminder)
				if err != nil {
					return nil, fmt.Errorf("failed to update task: %w", err)
				}
				updated++
			} else {
				duplicates++
			}
		} else {
			return nil, fmt.Errorf("database error: %w", err)
		}
	}

	// Update sync timestamp
	w.DB.ExecContext(ctx, "UPDATE tasks SET synced_at = CURRENT_TIMESTAMP WHERE source = 'apple'")

	return json.Marshal(map[string]any{
		"success":    true,
		"synced":     synced,
		"updated":    updated,
		"duplicates": duplicates,
		"total":      len(reminders),
	})
}

// syncFromDB syncs PostgreSQL tasks to Apple Reminders
func (w *RemindersSyncWorkerState) syncFromDB(ctx context.Context, input json.RawMessage) ([]byte, error) {
	if w.DB == nil {
		return nil, fmt.Errorf("database not configured")
	}

	var req struct {
		List string `json:"list"` // optional: specific list to sync to
	}
	json.Unmarshal(input, &req)

	// Fetch tasks that need syncing (source = mymcp, no external_id)
	rows, err := w.DB.QueryContext(ctx,
		`SELECT id, title, notes, list_name, priority, due_date, completed, completed_at, created_at 
		 FROM tasks 
		 WHERE (external_id IS NULL OR external_id = '') AND source = 'mymcp'`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query tasks: %w", err)
	}
	defer rows.Close()

	synced := 0
	var errors []string

	for rows.Next() {
		var task RemindersTask
		var notes, listName, priority sql.NullString
		var dueDate, completedAt sql.NullTime

		err := rows.Scan(
			&task.ID, &task.Title, &notes, &listName, &priority,
			&dueDate, &task.Completed, &completedAt, &task.CreatedAt,
		)
		if err != nil {
			errors = append(errors, fmt.Sprintf("scan error: %v", err))
			continue
		}

		task.Notes = notes.String
		task.ListName = listName.String
		if task.ListName == "" {
			task.ListName = req.List
			if task.ListName == "" {
				task.ListName = "Default"
			}
		}
		task.Priority = priority.String
		task.DueDate = nullTimeToPtr(dueDate)
		task.CompletedAt = nullTimeToPtr(completedAt)

		// Create in Apple Reminders
		externalID, err := w.createAppleReminder(ctx, task)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to create reminder %d: %v", task.ID, err))
			continue
		}

		// Update external_id in database
		_, err = w.DB.ExecContext(ctx,
			"UPDATE tasks SET external_id = $1, source = 'apple', synced_at = CURRENT_TIMESTAMP WHERE id = $2",
			externalID, task.ID,
		)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to update task %d: %v", task.ID, err))
			continue
		}

		synced++
	}

	return json.Marshal(map[string]any{
		"success": true,
		"synced":  synced,
		"errors":  errors,
	})
}

// createReminder creates a reminder in both Apple Reminders and database
func (w *RemindersSyncWorkerState) createReminder(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Title    string     `json:"title"`
		Notes    string     `json:"notes"`
		List     string     `json:"list"`
		Priority string     `json:"priority"`
		DueDate  *time.Time `json:"due_date"`
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	if req.Title == "" {
		return nil, fmt.Errorf("title is required")
	}

	if req.List == "" {
		req.List = "Default"
	}

	// Create in Apple Reminders first
	task := RemindersTask{
		Title:    req.Title,
		Notes:    req.Notes,
		ListName: req.List,
		Priority: req.Priority,
		DueDate:  req.DueDate,
	}

	externalID, err := w.createAppleReminder(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("failed to create Apple Reminder: %w", err)
	}

	// Store in database
	task.ExternalID = externalID
	task.Source = "apple"

	if w.DB != nil {
		var id int64
		err = w.DB.QueryRowContext(ctx,
			`INSERT INTO tasks (title, notes, list_name, priority, due_date, external_id, source, synced_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, CURRENT_TIMESTAMP)
			 RETURNING id`,
			task.Title, task.Notes, task.ListName, task.Priority, task.DueDate, task.ExternalID, task.Source,
		).Scan(&id)
		if err != nil {
			// Don't fail - Apple reminder was created, just log
			return json.Marshal(map[string]any{
				"success":     true,
				"external_id": externalID,
				"warning":     fmt.Sprintf("Apple reminder created but database insert failed: %v", err),
			})
		}
		task.ID = id
	}

	return json.Marshal(map[string]any{
		"success":     true,
		"id":          task.ID,
		"external_id": externalID,
		"title":       task.Title,
		"list":        task.ListName,
	})
}

// completeReminder marks a reminder as complete in both systems
func (w *RemindersSyncWorkerState) completeReminder(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		ID         int64  `json:"id"`
		ExternalID string `json:"external_id"`
	}

	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	if req.ExternalID == "" && req.ID == 0 {
		return nil, fmt.Errorf("either id or external_id is required")
	}

	// Get task if we have ID but no external_id
	if req.ExternalID == "" && w.DB != nil {
		err := w.DB.QueryRowContext(ctx,
			"SELECT external_id FROM tasks WHERE id = $1",
			req.ID,
		).Scan(&req.ExternalID)
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("failed to get external_id: %w", err)
		}
	}

	// Complete in Apple Reminders
	if req.ExternalID != "" {
		if err := w.completeAppleReminder(ctx, req.ExternalID); err != nil {
			return nil, fmt.Errorf("failed to complete Apple Reminder: %w", err)
		}
	}

	// Update database
	if w.DB != nil {
		if req.ID > 0 {
			_, err := w.DB.ExecContext(ctx,
				"UPDATE tasks SET completed = TRUE, completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = $1",
				req.ID,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to update database: %w", err)
			}
		} else if req.ExternalID != "" {
			_, err := w.DB.ExecContext(ctx,
				"UPDATE tasks SET completed = TRUE, completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE external_id = $1",
				req.ExternalID,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to update database: %w", err)
			}
		}
	}

	return json.Marshal(map[string]any{
		"success":     true,
		"id":          req.ID,
		"external_id": req.ExternalID,
	})
}

// listReminders lists reminders from the database
func (w *RemindersSyncWorkerState) listReminders(ctx context.Context, input json.RawMessage) ([]byte, error) {
	if w.DB == nil {
		return nil, fmt.Errorf("database not configured")
	}

	var req struct {
		List      string `json:"list"`
		Completed *bool  `json:"completed"`
		Limit     int    `json:"limit"`
	}
	json.Unmarshal(input, &req)

	if req.Limit == 0 {
		req.Limit = 100
	}

	query := "SELECT id, title, notes, list_name, priority, due_date, completed, completed_at, external_id, source, created_at FROM tasks WHERE 1=1"
	var args []any
	argNum := 1

	if req.List != "" {
		query += fmt.Sprintf(" AND list_name = $%d", argNum)
		args = append(args, req.List)
		argNum++
	}

	if req.Completed != nil {
		query += fmt.Sprintf(" AND completed = $%d", argNum)
		args = append(args, *req.Completed)
		argNum++
	}

	query += fmt.Sprintf(" ORDER BY due_date ASC NULLS LAST, created_at DESC LIMIT $%d", argNum)
	args = append(args, req.Limit)

	rows, err := w.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query tasks: %w", err)
	}
	defer rows.Close()

	var tasks []RemindersTask
	for rows.Next() {
		var task RemindersTask
		var notes, listName, priority, externalID, source sql.NullString
		var dueDate, completedAt sql.NullTime

		err := rows.Scan(
			&task.ID, &task.Title, &notes, &listName, &priority,
			&dueDate, &task.Completed, &completedAt, &externalID, &source, &task.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}

		task.Notes = notes.String
		task.ListName = listName.String
		task.Priority = priority.String
		task.DueDate = nullTimeToPtr(dueDate)
		task.CompletedAt = nullTimeToPtr(completedAt)
		task.ExternalID = externalID.String
		task.Source = source.String

		tasks = append(tasks, task)
	}

	return json.Marshal(map[string]any{
		"tasks": tasks,
		"count": len(tasks),
	})
}

// showReminders fetches reminders directly from Apple Reminders
func (w *RemindersSyncWorkerState) showReminders(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Filter string `json:"filter"` // today, all, overdue, etc.
		List   string `json:"list"`
	}
	json.Unmarshal(input, &req)

	if req.Filter == "" {
		req.Filter = "all"
	}

	reminders, err := w.fetchAppleReminders(ctx, req.List)
	if err != nil {
		return nil, err
	}

	// Apply filter
	if req.Filter != "all" {
		reminders = w.filterReminders(reminders, req.Filter)
	}

	return json.Marshal(map[string]any{
		"reminders": reminders,
		"count":     len(reminders),
	})
}

// syncStatus returns sync status and counts
func (w *RemindersSyncWorkerState) syncStatus(ctx context.Context, input json.RawMessage) ([]byte, error) {
	status := map[string]any{
		"database_connected": w.DB != nil,
		"remindctl_path":     w.remindctlPath,
	}

	if w.DB != nil {
		// Count tasks
		var totalTasks, completedTasks, pendingTasks, syncableTasks int
		w.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks").Scan(&totalTasks)
		w.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = TRUE").Scan(&completedTasks)
		w.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = FALSE").Scan(&pendingTasks)
		w.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE external_id IS NULL OR external_id = ''").Scan(&syncableTasks)

		status["tasks_total"] = totalTasks
		status["tasks_completed"] = completedTasks
		status["tasks_pending"] = pendingTasks
		status["tasks_to_sync"] = syncableTasks

		// Last sync time
		var lastSync sql.NullTime
		w.DB.QueryRowContext(ctx, "SELECT MAX(synced_at) FROM tasks").Scan(&lastSync)
		if lastSync.Valid {
			status["last_sync"] = lastSync.Time
		}
	}

	// Check remindctl availability
	_, err := exec.LookPath(w.remindctlPath)
	status["remindctl_available"] = err == nil

	return json.Marshal(status)
}

// --- Apple Reminders CLI helpers ---

// fetchAppleReminders fetches all reminders from Apple Reminders via remindctl
func (w *RemindersSyncWorkerState) fetchAppleReminders(ctx context.Context, list string) ([]AppleReminder, error) {
	args := []string{"show", "all", "--json"}
	if list != "" {
		args = []string{"show", "--list", list, "--json"}
	}

	output, err := w.runRemindctl(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("remindctl failed: %w", err)
	}

	// Parse JSON output
	type remindctlOutput struct {
		Reminders []struct {
			ID          string     `json:"id"`
			Title       string     `json:"title"`
			Notes       string     `json:"notes"`
			List        string     `json:"list"`
			ListName    string     `json:"listName"`
			Priority    int        `json:"priority"`
			PriorityStr string     `json:"priorityString"`
			DueDate     *time.Time `json:"dueDate"`
			IsCompleted bool       `json:"isCompleted"`
			Completed   *time.Time `json:"completionDate"`
			CreatedAt   *time.Time `json:"creationDate"`
			ModifiedAt  *time.Time `json:"modificationDate"`
		} `json:"reminders"`
	}

	var result remindctlOutput
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse remindctl output: %w", err)
	}

	reminders := make([]AppleReminder, 0, len(result.Reminders))
	for _, r := range result.Reminders {
		listName := r.List
		if listName == "" {
			listName = r.ListName
		}

		priority := "none"
		if r.PriorityStr != "" {
			priority = strings.ToLower(r.PriorityStr)
		} else if r.Priority > 0 {
			switch r.Priority {
			case 1:
				priority = "high"
			case 5:
				priority = "medium"
			case 9:
				priority = "low"
			}
		}

		reminder := AppleReminder{
			ID:         r.ID,
			Title:      r.Title,
			Notes:      r.Notes,
			List:       listName,
			Priority:   priority,
			DueDate:    r.DueDate,
			Completed:  r.IsCompleted,
			CreatedAt:  time.Now(),
			ModifiedAt: time.Now(),
		}

		if r.CreatedAt != nil {
			reminder.CreatedAt = *r.CreatedAt
		}
		if r.ModifiedAt != nil {
			reminder.ModifiedAt = *r.ModifiedAt
		}
		if r.Completed != nil {
			reminder.CompletedAt = r.Completed
		}

		reminders = append(reminders, reminder)
	}

	return reminders, nil
}

// createAppleReminder creates a reminder in Apple Reminders
func (w *RemindersSyncWorkerState) createAppleReminder(ctx context.Context, task RemindersTask) (string, error) {
	args := []string{"add", "--json", "--title", task.Title}

	if task.ListName != "" {
		args = append(args, "--list", task.ListName)
	}
	if task.Notes != "" {
		args = append(args, "--notes", task.Notes)
	}
	if task.Priority != "" && task.Priority != "none" {
		args = append(args, "--priority", task.Priority)
	}
	if task.DueDate != nil {
		args = append(args, "--due", task.DueDate.Format("2006-01-02"))
	}

	output, err := w.runRemindctl(ctx, args...)
	if err != nil {
		return "", err
	}

	// Parse created reminder ID
	var result struct {
		Reminders []struct {
			ID string `json:"id"`
		} `json:"reminders"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return "", fmt.Errorf("failed to parse created reminder: %w", err)
	}

	if len(result.Reminders) > 0 {
		return result.Reminders[0].ID, nil
	}

	// Fallback: try to get ID from plain output
	return fmt.Sprintf("created_%d", time.Now().UnixNano()), nil
}

// completeAppleReminder marks a reminder as complete
func (w *RemindersSyncWorkerState) completeAppleReminder(ctx context.Context, externalID string) error {
	_, err := w.runRemindctl(ctx, "complete", "--json", externalID)
	return err
}

// runRemindctl executes the remindctl CLI
func (w *RemindersSyncWorkerState) runRemindctl(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, w.remindctlPath, args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("remindctl error: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("remindctl failed: %w", err)
	}
	return output, nil
}

// filterReminders filters reminders based on filter type
func (w *RemindersSyncWorkerState) filterReminders(reminders []AppleReminder, filter string) []AppleReminder {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrow := today.Add(24 * time.Hour)
	weekEnd := today.Add(7 * 24 * time.Hour)

	var result []AppleReminder
	for _, r := range reminders {
		include := false

		switch strings.ToLower(filter) {
		case "today":
			include = r.DueDate != nil && r.DueDate.After(today) && r.DueDate.Before(tomorrow)
		case "tomorrow":
			include = r.DueDate != nil && r.DueDate.After(tomorrow) && r.DueDate.Before(tomorrow.Add(24*time.Hour))
		case "week":
			include = r.DueDate != nil && r.DueDate.After(today) && r.DueDate.Before(weekEnd)
		case "overdue":
			include = r.DueDate != nil && r.DueDate.Before(now) && !r.Completed
		case "completed":
			include = r.Completed
		case "pending", "incomplete":
			include = !r.Completed
		case "all":
			include = true
		}

		if include {
			result = append(result, r)
		}
	}

	return result
}

// insertTask inserts a task into the database
func (w *RemindersSyncWorkerState) insertTask(ctx context.Context, r AppleReminder) error {
	_, err := w.DB.ExecContext(ctx,
		`INSERT INTO tasks (title, notes, list_name, priority, due_date, completed, completed_at, external_id, source, synced_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'apple', CURRENT_TIMESTAMP)
		 ON CONFLICT (external_id) DO UPDATE SET
		 title = EXCLUDED.title,
		 notes = EXCLUDED.notes,
		 list_name = EXCLUDED.list_name,
		 priority = EXCLUDED.priority,
		 due_date = EXCLUDED.due_date,
		 completed = EXCLUDED.completed,
		 completed_at = EXCLUDED.completed_at,
		 synced_at = CURRENT_TIMESTAMP`,
		r.Title, r.Notes, r.List, r.Priority, r.DueDate, r.Completed, r.CompletedAt, r.ID,
	)
	return err
}

// updateTaskFromApple updates an existing task from Apple Reminders data
func (w *RemindersSyncWorkerState) updateTaskFromApple(ctx context.Context, taskID int64, r AppleReminder) error {
	_, err := w.DB.ExecContext(ctx,
		`UPDATE tasks SET
		 title = $1,
		 notes = $2,
		 list_name = $3,
		 priority = $4,
		 due_date = $5,
		 completed = $6,
		 completed_at = $7,
		 updated_at = CURRENT_TIMESTAMP,
		 synced_at = CURRENT_TIMESTAMP
		 WHERE id = $8`,
		r.Title, r.Notes, r.List, r.Priority, r.DueDate, r.Completed, r.CompletedAt, taskID,
	)
	return err
}

// nullTimeToPtr converts sql.NullTime to *time.Time
func nullTimeToPtr(nt sql.NullTime) *time.Time {
	if nt.Valid {
		return &nt.Time
	}
	return nil
}