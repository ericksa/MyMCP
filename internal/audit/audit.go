package audit

import (
	"database/sql"
	"encoding/json"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Auditor struct {
	db *sql.DB
}

type AuditEntry struct {
	ID        int64     `json:"id"`
	Tool      string    `json:"tool"`
	Input     string    `json:"input"`
	Output    string    `json:"output"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

func NewAuditor() *Auditor {
	db, err := sql.Open("sqlite3", "/tmp/mymcp_audit.db")
	if err != nil {
		log.Printf("Failed to open audit DB: %v", err)
		return &Auditor{}
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tool TEXT NOT NULL,
		input TEXT,
		output TEXT,
		error TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		log.Printf("Failed to create audit table: %v", err)
	}
	return &Auditor{db: db}
}

func (a *Auditor) Log(tool string, input json.RawMessage, output []byte, err error) {
	if a.db == nil {
		return
	}
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	_, err = a.db.Exec(
		"INSERT INTO audit_log (tool, input, output, error) VALUES (?, ?, ?, ?)",
		tool, string(input), string(output), errStr,
	)
	if err != nil {
		log.Printf("Failed to write audit log: %v", err)
	}
}

func (a *Auditor) GetLogs(limit int) ([]AuditEntry, error) {
	if a.db == nil {
		return nil, nil
	}
	rows, err := a.db.Query("SELECT id, tool, input, output, error, timestamp FROM audit_log ORDER BY timestamp DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.Tool, &e.Input, &e.Output, &e.Error, &e.Timestamp); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func (a *Auditor) Close() {
	if a.db != nil {
		a.db.Close()
	}
}
