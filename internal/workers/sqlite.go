package workers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type SQLiteWorkerState struct {
	DB *sql.DB
}

func NewSQLiteWorkerState() *SQLiteWorkerState {
	db, _ := sql.Open("sqlite3", ":memory:")
	db.Exec("CREATE TABLE IF NOT EXISTS audit_log (id INTEGER PRIMARY KEY, tool TEXT, input TEXT, output TEXT, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP)")
	return &SQLiteWorkerState{DB: db}
}

func (w *SQLiteWorkerState) GetTools() []ToolDef {
	return []ToolDef{
		{Name: "sql_query", Description: "Execute a SELECT SQL query"},
		{Name: "sql_insert", Description: "Execute an INSERT SQL statement"},
		{Name: "sql_update", Description: "Execute an UPDATE SQL statement"},
		{Name: "sql_delete", Description: "Execute a DELETE SQL statement"},
		{Name: "list_tables", Description: "List all tables in the database"},
		{Name: "describe_table", Description: "Get schema info for a table"},
	}
}

func (w *SQLiteWorkerState) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "sqlite_sql_query", "sql_query":
		return w.sqlQuery(ctx, input)
	case "sqlite_sql_insert", "sql_insert":
		return w.sqlInsert(ctx, input)
	case "sqlite_sql_update", "sql_update":
		return w.sqlUpdate(ctx, input)
	case "sqlite_sql_delete", "sql_delete":
		return w.sqlDelete(ctx, input)
	case "sqlite_list_tables", "list_tables":
		return w.listTables(ctx, input)
	case "sqlite_describe_table", "describe_table":
		return w.describeTable(ctx, input)
	default:
		return nil, nil
	}
}

func (w *SQLiteWorkerState) sqlQuery(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	rows, err := w.DB.QueryContext(ctx, req.Query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}
		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}
	return json.Marshal(results)
}

func (w *SQLiteWorkerState) sqlInsert(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Table   string `json:"table"`
		Columns string `json:"columns"`
		Values  string `json:"values"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", req.Table, req.Columns, req.Values)
	result, err := w.DB.ExecContext(ctx, query)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	return json.Marshal(map[string]interface{}{"success": true, "last_insert_id": id})
}

func (w *SQLiteWorkerState) sqlUpdate(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Table string `json:"table"`
		Set   string `json:"set"`
		Where string `json:"where"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s", req.Table, req.Set, req.Where)
	result, err := w.DB.ExecContext(ctx, query)
	if err != nil {
		return nil, err
	}
	rows, _ := result.RowsAffected()
	return json.Marshal(map[string]interface{}{"success": true, "rows_affected": rows})
}

func (w *SQLiteWorkerState) sqlDelete(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Table string `json:"table"`
		Where string `json:"where"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	query := fmt.Sprintf("DELETE FROM %s WHERE %s", req.Table, req.Where)
	result, err := w.DB.ExecContext(ctx, query)
	if err != nil {
		return nil, err
	}
	rows, _ := result.RowsAffected()
	return json.Marshal(map[string]interface{}{"success": true, "rows_affected": rows})
}

func (w *SQLiteWorkerState) listTables(ctx context.Context, input json.RawMessage) ([]byte, error) {
	rows, err := w.DB.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return json.Marshal(tables)
}

func (w *SQLiteWorkerState) describeTable(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Table string `json:"table"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	rows, err := w.DB.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", req.Table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []map[string]interface{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, colType string
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notnull, &dfltValue, &pk); err != nil {
			return nil, err
		}
		columns = append(columns, map[string]interface{}{
			"name":          name,
			"type":          colType,
			"notnull":       notnull == 1,
			"default_value": dfltValue.String,
			"primary_key":   pk == 1,
		})
	}
	return json.Marshal(columns)
}
