package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ericksa/mymcp/internal/config"
	"github.com/ericksa/mymcp/internal/middleware"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	healthHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp["status"])
}

func TestListToolsHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tools", nil)
	w := httptest.NewRecorder()

	listToolsHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.NotNil(t, resp["tools"])
}

func TestAuthMiddleware_NoToken(t *testing.T) {
	cfg := &config.Config{
		JWTSecret: "test-secret",
	}
	handler := middleware.AuthMiddleware(cfg)

	router := mux.NewRouter()
	router.Use(handler)
	router.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_WithToken(t *testing.T) {
	cfg := &config.Config{
		JWTSecret: "test-secret",
	}
	handler := middleware.AuthMiddleware(cfg)

	router := mux.NewRouter()
	router.Use(handler)
	router.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test?token=test-token", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestLoggerMiddleware(t *testing.T) {
	nextCalled := false
	handler := middleware.Logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.True(t, nextCalled)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRecovererMiddleware_Panic(t *testing.T) {
	handler := middleware.Recoverer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
