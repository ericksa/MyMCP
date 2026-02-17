package workers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVectorWorker_StoreAndSearch(t *testing.T) {
	w := NewVectorWorkerState()

	_, err := w.Execute(context.Background(), "vector_store", []byte(`{"id": "doc1", "text": "hello world"}`))
	require.NoError(t, err)

	_, err = w.Execute(context.Background(), "vector_store", []byte(`{"id": "doc2", "text": "goodbye world"}`))
	require.NoError(t, err)

	result, err := w.Execute(context.Background(), "vector_search", []byte(`{"query": "hello", "top_k": 1}`))
	require.NoError(t, err)
	assert.Contains(t, string(result), "doc1")
}

func TestVectorWorker_List(t *testing.T) {
	w := NewVectorWorkerState()

	w.Execute(context.Background(), "vector_store", []byte(`{"id": "doc1", "text": "test"}`))
	w.Execute(context.Background(), "vector_store", []byte(`{"id": "doc2", "text": "test2"}`))

	result, err := w.Execute(context.Background(), "vector_list", []byte(`{}`))
	require.NoError(t, err)
	assert.Contains(t, string(result), "doc1")
	assert.Contains(t, string(result), "doc2")
}

func TestVectorWorker_Delete(t *testing.T) {
	w := NewVectorWorkerState()

	w.Execute(context.Background(), "vector_store", []byte(`{"id": "doc1", "text": "test"}`))
	result, err := w.Execute(context.Background(), "vector_delete", []byte(`{"id": "doc1"}`))
	require.NoError(t, err)
	assert.Contains(t, string(result), "success")
}
