package workers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteWorker_ListTables(t *testing.T) {
	w := NewSQLiteWorkerState()
	defer w.DB.Close()

	result, err := w.Execute(context.Background(), "list_tables", []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, `["audit_log"]`, string(result))
}

func TestSQLiteWorker_SqlQuery(t *testing.T) {
	w := NewSQLiteWorkerState()
	defer w.DB.Close()

	w.DB.Exec("CREATE TABLE test (id INTEGER, name TEXT)")
	w.DB.Exec("INSERT INTO test (id, name) VALUES (1, 'alice'), (2, 'bob')")

	result, err := w.Execute(context.Background(), "sql_query", []byte(`{"query": "SELECT * FROM test"}`))
	require.NoError(t, err)
	assert.Contains(t, string(result), "alice")
	assert.Contains(t, string(result), "bob")
}

func TestSQLiteWorker_SqlInsert(t *testing.T) {
	w := NewSQLiteWorkerState()
	defer w.DB.Close()

	w.DB.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")

	result, err := w.Execute(context.Background(), "sql_insert", []byte(`{"table": "test", "columns": "name", "values": "'charlie'"}`))
	require.NoError(t, err)
	assert.Contains(t, string(result), "success")
}

func TestSQLiteWorker_DescribeTable(t *testing.T) {
	w := NewSQLiteWorkerState()
	defer w.DB.Close()

	w.DB.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)")

	result, err := w.Execute(context.Background(), "describe_table", []byte(`{"table": "users"}`))
	require.NoError(t, err)
	assert.Contains(t, string(result), "name")
	assert.Contains(t, string(result), "email")
}
