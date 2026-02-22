package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"

	_ "github.com/lib/pq"
)

// Task represents a task from the database
type Task struct {
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	Client         string     `json:"client"`
	Project        string     `json:"project"`
	EmailSubject   string     `json:"email_subject"`
	EmailFrom      string     `json:"email_from"`
	DueDate        *time.Time `json:"due_date"`
	Status         string     `json:"status"`
	Priority       int        `json:"priority"`
	Urgency        string     `json:"urgency"`
	AssignedAgent  string     `json:"assigned_agent"`
	Source         string     `json:"source"`
	EstimatedHours float64    `json:"estimated_hours"`
	ActualHours    float64    `json:"actual_hours"`
	BillingStatus  string     `json:"billing_status"`
	Tags           []string   `json:"tags"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TimeEntry represents a time entry from the database
type TimeEntry struct {
	ID              string     `json:"id"`
	TaskID          string     `json:"task_id"`
	StartedAt       *time.Time `json:"started_at"`
	EndedAt         *time.Time `json:"ended_at"`
	DurationMinutes int        `json:"duration_minutes"`
	Description     string     `json:"description"`
	AgentID         string     `json:"agent_id"`
}

// StandupReport represents the generated standup report
type StandupReport struct {
	GeneratedAt     time.Time `json:"generated_at"`
	DateRange       string    `json:"date_range"`
	TotalTasks      int       `json:"total_tasks"`
	OverdueTasks    []Task    `json:"overdue_tasks"`
	DueTodayTasks   []Task    `json:"due_today_tasks"`
	InProgressTasks []Task    `json:"in_progress_tasks"`
	CompletedTasks  []Task    `json:"completed_tasks"`
	Summary         Summary   `json:"summary"`
}

// Summary provides high-level stats
type Summary struct {
	OverdueCount    int     `json:"overdue_count"`
	DueTodayCount   int     `json:"due_today_count"`
	InProgressCount int     `json:"in_progress_count"`
	CompletedCount  int     `json:"completed_count"`
	TotalHours      float64 `json:"total_hours"`
	BilledHours     float64 `json:"billed_hours"`
	UnbilledHours   float64 `json:"unbilled_hours"`
}

// FilterOptions for query filtering
type FilterOptions struct {
	Client    string
	Status    string
	StartDate *time.Time
	EndDate   *time.Time
}

// Config holds database connection configuration
type Config struct {
	DatabaseURL string
}

func main() {
	// Command-line flags
	var (
		output      = flag.String("output", "console", "Output format: console, json, or file path")
		client      = flag.String("client", "", "Filter by client name")
		status      = flag.String("status", "", "Filter by status")
		startDate   = flag.String("start", "", "Start date for range (YYYY-MM-DD)")
		endDate     = flag.String("end", "", "End date for range (YYYY-MM-DD)")
		dbURL       = flag.String("db", "", "Database URL (default: from DATABASE_URL env)")
		includeDone = flag.Bool("done", false, "Include completed tasks in report")
		help        = flag.Bool("help", false, "Show help")
	)
	flag.Parse()

	if *help {
		printHelp()
		os.Exit(0)
	}

	// Get database URL
	databaseURL := *dbURL
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
		if databaseURL == "" {
			// Default from config.yaml
			databaseURL = "postgres://llm:lom@localhost:5432/llm?sslmode=disable"
		}
	}

	// Build filter options
	filter := FilterOptions{
		Client: *client,
		Status: *status,
	}

	if *startDate != "" {
		t, err := time.Parse("2006-01-02", *startDate)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid start date: %v\n", err)
			os.Exit(1)
		}
		filter.StartDate = &t
	}

	if *endDate != "" {
		t, err := time.Parse("2006-01-02", *endDate)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid end date: %v\n", err)
			os.Exit(1)
		}
		filter.EndDate = &t
	}

	// Generate report
	report, err := generateReport(databaseURL, filter, *includeDone)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating report: %v\n", err)
		os.Exit(1)
	}

	// Output report
	switch {
	case *output == "console":
		printConsoleReport(report)
	case *output == "json":
		printJSONReport(report)
	case strings.HasSuffix(*output, ".json"):
		if err := writeJSONReport(report, *output); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Report written to: %s\n", *output)
	case strings.HasSuffix(*output, ".md") || strings.HasSuffix(*output, ".txt"):
		if err := writeMarkdownReport(report, *output); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Report written to: %s\n", *output)
	default:
		// Default to console for unknown output
		printConsoleReport(report)
	}
}

func printHelp() {
	fmt.Println(`MyMCP Daily Standup Report Generator

USAGE:
    standup [OPTIONS]

OPTIONS:
    -output <format>   Output format: console, json, or file path (.json, .md, .txt)
                       (default: console)
    -client <name>     Filter by client name
    -status <status>   Filter by status (e.g., open, in_progress, completed)
    -start <date>      Start date for range filter (YYYY-MM-DD)
    -end <date>        End date for range filter (YYYY-MM-DD)
    -db <url>          Database URL (default: from DATABASE_URL env)
    -done              Include completed tasks in the report
    -help              Show this help message

EXAMPLES:
    # Basic daily standup
    standup

    # Filter by client
    standup -client "Acme Corp"

    # Export to JSON file
    standup -output /tmp/standup-2024-01-15.json

    # Date range with specific status
    standup -start 2024-01-01 -end 2024-01-31 -status in_progress

    # Full report including completed tasks
    standup -done -output standup.md
`)
}

func generateReport(dbURL string, filter FilterOptions, includeDone bool) (*StandupReport, error) {
	db, err := openDB(dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	report := &StandupReport{
		GeneratedAt: now,
		DateRange:   today.Format("2006-01-02"),
	}

	// Fetch overdue tasks
	overdue, err := fetchTasks(db, TaskQuery{
		Filter:      filter,
		Overdue:     true,
		Today:       today,
		ExcludeDone: !includeDone,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch overdue tasks: %w", err)
	}
	report.OverdueTasks = overdue

	// Fetch due today tasks
	dueToday, err := fetchTasks(db, TaskQuery{
		Filter:      filter,
		DueToday:    true,
		Today:       today,
		ExcludeDone: !includeDone,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch due today tasks: %w", err)
	}
	report.DueTodayTasks = dueToday

	// Fetch in progress tasks
	inProgress, err := fetchTasks(db, TaskQuery{
		Filter:       filter,
		StatusFilter: "in_progress",
		ExcludeDone:  !includeDone,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch in progress tasks: %w", err)
	}
	report.InProgressTasks = inProgress

	// Fetch completed tasks if requested
	if includeDone {
		completed, err := fetchTasks(db, TaskQuery{
			Filter:       filter,
			StatusFilter: "completed",
			CompletedToday: true,
			Today:         today,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch completed tasks: %w", err)
		}
		report.CompletedTasks = completed
	}

	// Calculate summary
	report.Summary = Summary{
		OverdueCount:    len(report.OverdueTasks),
		DueTodayCount:   len(report.DueTodayTasks),
		InProgressCount: len(report.InProgressTasks),
		CompletedCount:  len(report.CompletedTasks),
	}

	for _, t := range report.OverdueTasks {
		report.Summary.TotalHours += t.ActualHours
		if t.BillingStatus == "billed" {
			report.Summary.BilledHours += t.ActualHours
		} else {
			report.Summary.UnbilledHours += t.ActualHours
		}
	}
	for _, t := range report.DueTodayTasks {
		report.Summary.TotalHours += t.ActualHours
	}
	for _, t := range report.InProgressTasks {
		report.Summary.TotalHours += t.ActualHours
	}

	report.TotalTasks = report.Summary.OverdueCount + report.Summary.DueTodayCount +
		report.Summary.InProgressCount + report.Summary.CompletedCount

	return report, nil
}

// TaskQuery specifies query parameters
type TaskQuery struct {
	Filter        FilterOptions
	Overdue       bool
	DueToday      bool
	StatusFilter  string
	CompletedToday bool
	Today         time.Time
	ExcludeDone   bool
}

func fetchTasks(db *DB, query TaskQuery) ([]Task, error) {
	var conditions []string
	var args []interface{}
	argNum := 1

	// Base condition: not deleted
	conditions = append(conditions, "1=1")

	// Client filter
	if query.Filter.Client != "" {
		conditions = append(conditions, fmt.Sprintf("client ILIKE $%d", argNum))
		args = append(args, "%"+query.Filter.Client+"%")
		argNum++
	}

	// Status filter
	if query.StatusFilter != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argNum))
		args = append(args, query.StatusFilter)
		argNum++
	} else if query.Filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argNum))
		args = append(args, query.Filter.Status)
		argNum++
	}

	// Date range filter
	if query.Filter.StartDate != nil {
		conditions = append(conditions, fmt.Sprintf("due_date >= $%d", argNum))
		args = append(args, *query.Filter.StartDate)
		argNum++
	}
	if query.Filter.EndDate != nil {
		conditions = append(conditions, fmt.Sprintf("due_date <= $%d", argNum))
		args = append(args, *query.Filter.EndDate)
		argNum++
	}

	// Overdue condition
	if query.Overdue {
		conditions = append(conditions, fmt.Sprintf("due_date < $%d", argNum))
		args = append(args, query.Today)
		argNum++
		conditions = append(conditions, "status NOT IN ('completed', 'cancelled')")
	}

	// Due today condition
	if query.DueToday {
		tomorrow := query.Today.Add(24 * time.Hour)
		conditions = append(conditions, fmt.Sprintf("due_date >= $%d AND due_date < $%d", argNum, argNum+1))
		args = append(args, query.Today, tomorrow)
		argNum += 2
		conditions = append(conditions, "status NOT IN ('completed', 'cancelled')")
	}

	// Completed today
	if query.CompletedToday {
		conditions = append(conditions, fmt.Sprintf("DATE(updated_at) = $%d", argNum))
		args = append(args, query.Today.Format("2006-01-02"))
		argNum++
	}

	// Exclude done
	if query.ExcludeDone && query.StatusFilter == "" {
		conditions = append(conditions, "status NOT IN ('completed', 'cancelled')")
	}

	// Build query
	whereClause := strings.Join(conditions, " AND ")
	querySQL := fmt.Sprintf(`
		SELECT id, title, description, client, project, email_subject, email_from,
		       due_date, status, priority, urgency, assigned_agent, source,
		       estimated_hours, actual_hours, billing_status, tags, created_at, updated_at
		FROM tasks
		WHERE %s
		ORDER BY 
			CASE priority 
				WHEN 1 THEN 1 
				WHEN 2 THEN 2 
				WHEN 3 THEN 3 
				WHEN 4 THEN 4 
				ELSE 5 
			END,
			due_date NULLS LAST,
			created_at DESC
	`, whereClause)

	rows, err := db.Query(querySQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var dueDate, emailSubject, emailFrom, description, client, project, assignedAgent sql.NullString
		var tags []byte

		err := rows.Scan(
			&t.ID, &t.Title, &description, &client, &project, &emailSubject, &emailFrom,
			&dueDate, &t.Status, &t.Priority, &t.Urgency, &assignedAgent, &t.Source,
			&t.EstimatedHours, &t.ActualHours, &t.BillingStatus, &tags, &t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		t.Description = nullToString(description)
		t.Client = nullToString(client)
		t.Project = nullToString(project)
		t.EmailSubject = nullToString(emailSubject)
		t.EmailFrom = nullToString(emailFrom)
		t.AssignedAgent = nullToString(assignedAgent)

		if dueDate.Valid {
			if parsed, err := time.Parse("2006-01-02 15:04:05", dueDate.String); err == nil {
				t.DueDate = &parsed
			}
		}

		if len(tags) > 0 {
			json.Unmarshal(tags, &t.Tags)
		}

		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

// DB wraps database connection
type DB struct {
	conn interface {
		Query(query string, args ...interface{}) (*sql.Rows, error)
		Close() error
	}
}

func openDB(dbURL string) (*DB, error) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &DB{conn: db}, nil
}

func (db *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return db.conn.Query(query, args...)
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func nullToString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func printConsoleReport(report *StandupReport) {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("                    DAILY STANDUP REPORT")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Printf("Generated: %s\n", report.GeneratedAt.Format("Mon Jan 2, 2006 3:04 PM"))
	fmt.Println()

	// Summary
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│                        SUMMARY                              │")
	fmt.Println("├─────────────────────────────────────────────────────────────┤")
	fmt.Printf("│  Overdue:      %3d tasks                                    │\n", report.Summary.OverdueCount)
	fmt.Printf("│  Due Today:    %3d tasks                                    │\n", report.Summary.DueTodayCount)
	fmt.Printf("│  In Progress:  %3d tasks                                    │\n", report.Summary.InProgressCount)
	fmt.Printf("│  Completed:    %3d tasks                                    │\n", report.Summary.CompletedCount)
	fmt.Printf("│  Total Active: %3d tasks                                    │\n", report.Summary.OverdueCount+report.Summary.DueTodayCount+report.Summary.InProgressCount)
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	fmt.Println()

	// Overdue Tasks
	if len(report.OverdueTasks) > 0 {
		fmt.Printf("\n🔴 OVERDUE TASKS (%d)\n", len(report.OverdueTasks))
		fmt.Println("─────────────────────────────────────────────────────────────────")
		for _, t := range report.OverdueTasks {
			printTaskCard(t, true)
		}
	}

	// Due Today Tasks
	if len(report.DueTodayTasks) > 0 {
		fmt.Printf("\n🟡 DUE TODAY (%d)\n", len(report.DueTodayTasks))
		fmt.Println("─────────────────────────────────────────────────────────────────")
		for _, t := range report.DueTodayTasks {
			printTaskCard(t, false)
		}
	}

	// In Progress Tasks
	if len(report.InProgressTasks) > 0 {
		fmt.Printf("\n🟢 IN PROGRESS (%d)\n", len(report.InProgressTasks))
		fmt.Println("─────────────────────────────────────────────────────────────────")
		for _, t := range report.InProgressTasks {
			printTaskCard(t, false)
		}
	}

	// Completed Tasks
	if len(report.CompletedTasks) > 0 {
		fmt.Printf("\n✅ COMPLETED TODAY (%d)\n", len(report.CompletedTasks))
		fmt.Println("─────────────────────────────────────────────────────────────────")
		for _, t := range report.CompletedTasks {
			printTaskCard(t, false)
		}
	}

	// No tasks message
	if report.TotalTasks == 0 {
		fmt.Println("No tasks found matching the criteria.")
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
}

func printTaskCard(t Task, showOverdue bool) {
	priorityIcon := getPriorityIcon(t.Priority)
	client := t.Client
	if client == "" {
		client = "No Client"
	}

	fmt.Printf("\n  %s [%s] %s\n", priorityIcon, t.ID[:8], t.Title)
	if t.Description != "" {
		desc := t.Description
		if len(desc) > 80 {
			desc = desc[:77] + "..."
		}
		fmt.Printf("      %s\n", desc)
	}

	fmt.Printf("      Client: %s", client)
	if t.Project != "" {
		fmt.Printf(" | Project: %s", t.Project)
	}
	fmt.Println()

	if t.DueDate != nil {
		if showOverdue {
			fmt.Printf("      ⚠️  DUE: %s (OVERDUE)\n", t.DueDate.Format("Jan 2"))
		} else {
			fmt.Printf("      📅 Due: %s\n", t.DueDate.Format("Jan 2, 2006"))
		}
	}

	if t.Tags != nil && len(t.Tags) > 0 {
		fmt.Printf("      🏷️  %s\n", strings.Join(t.Tags, ", "))
	}

	if t.ActualHours > 0 {
		fmt.Printf("      ⏱️  %.1f hours", t.ActualHours)
		if t.EstimatedHours > 0 {
			fmt.Printf(" / %.1f estimated", t.EstimatedHours)
		}
		fmt.Println()
	}
}

func getPriorityIcon(priority int) string {
	switch priority {
	case 1:
		return "🔥" // Critical
	case 2:
		return "⬆️" // High
	case 3:
		return "➡️" // Medium
	case 4:
		return "⬇️" // Low
	default:
		return "⚪"
	}
}

func printJSONReport(report *StandupReport) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func writeJSONReport(report *StandupReport, path string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func writeMarkdownReport(report *StandupReport, path string) error {
	tmpl := `# Daily Standup Report
**Generated:** {{.GeneratedAt.Format "Mon Jan 2, 2006 3:04 PM"}}

## Summary

| Category | Count |
|----------|-------|
| Overdue | {{.Summary.OverdueCount}} |
| Due Today | {{.Summary.DueTodayCount}} |
| In Progress | {{.Summary.InProgressCount}} |
| Completed | {{.Summary.CompletedCount}} |
| **Total** | **{{.TotalTasks}}** |

{{if gt (len .OverdueTasks) 0}}
## 🔴 Overdue Tasks ({{len .OverdueTasks}})

{{range .OverdueTasks}}
- **[{{.ID | printf "%.8s"}}]** {{.Title}}
  - Client: {{if .Client}}{{.Client}}{{else}}No Client{{end}}
  - Priority: {{.Priority}} | Status: {{.Status}}
  {{- if .DueDate}}
  - Due: {{.DueDate.Format "Jan 2, 2006"}} ⚠️ OVERDUE
  {{- end}}
{{end}}
{{end}}

{{if gt (len .DueTodayTasks) 0}}
## 🟡 Due Today ({{len .DueTodayTasks}})

{{range .DueTodayTasks}}
- **[{{.ID | printf "%.8s"}}]** {{.Title}}
  - Client: {{if .Client}}{{.Client}}{{else}}No Client{{end}}
  - Priority: {{.Priority}} | Status: {{.Status}}
{{end}}
{{end}}

{{if gt (len .InProgressTasks) 0}}
## 🟢 In Progress ({{len .InProgressTasks}})

{{range .InProgressTasks}}
- **[{{.ID | printf "%.8s"}}]** {{.Title}}
  - Client: {{if .Client}}{{.Client}}{{else}}No Client{{end}}
  - Priority: {{.Priority}} | Status: {{.Status}}
  {{- if .DueDate}}
  - Due: {{.DueDate.Format "Jan 2, 2006"}}
  {{- end}}
{{end}}
{{end}}

{{if gt (len .CompletedTasks) 0}}
## ✅ Completed Today ({{len .CompletedTasks}})

{{range .CompletedTasks}}
- **[{{.ID | printf "%.8s"}}]** {{.Title}}
  - Client: {{if .Client}}{{.Client}}{{else}}No Client{{end}}
  - 🎉 Completed
{{end}}
{{end}}

{{if eq .TotalTasks 0}}
No tasks found matching the criteria.
{{end}}
`
	t, err := template.New("report").Parse(tmpl)
	if err != nil {
		return err
	}

	var buf strings.Builder
	if err := t.Execute(&buf, report); err != nil {
		return err
	}

	return os.WriteFile(path, []byte(buf.String()), 0644)
}