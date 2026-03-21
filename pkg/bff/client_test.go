package bff

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoRequest_Success(t *testing.T) {
	var gotUserID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = r.Header.Get("X-User-ID")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"123","name":"test"}`))
	}))
	defer srv.Close()

	client := &ServiceClient{
		BaseURL: srv.URL,
		HTTP:    srv.Client(),
		Name:    "test-service",
	}

	ctx := WithUserID(t.Context(), "user-42")

	var dest struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	err := client.Do(ctx, http.MethodGet, "/items", nil, &dest)
	require.NoError(t, err)
	assert.Equal(t, "123", dest.ID)
	assert.Equal(t, "test", dest.Name)
	assert.Equal(t, "user-42", gotUserID)
}

func TestDoRequest_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`not found`))
	}))
	defer srv.Close()

	client := &ServiceClient{
		BaseURL: srv.URL,
		HTTP:    srv.Client(),
		Name:    "test-service",
	}

	err := client.Do(t.Context(), http.MethodGet, "/missing", nil, nil)
	require.Error(t, err)

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
	assert.Equal(t, "test-service", apiErr.Service)
	assert.Equal(t, "not found", apiErr.Body)
}

func TestDoRequest_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &ServiceClient{
		BaseURL: srv.URL,
		HTTP:    srv.Client(),
		Name:    "test-service",
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	err := client.Do(ctx, http.MethodGet, "/slow", nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestForward_Success(t *testing.T) {
	var gotUserID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = r.Header.Get("X-User-ID")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client := &ServiceClient{
		BaseURL: srv.URL,
		HTTP:    srv.Client(),
		Name:    "test-service",
	}

	ctx := WithUserID(t.Context(), "user-42")
	rec := httptest.NewRecorder()
	client.Forward(ctx, rec, http.MethodGet, "/items", nil)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "user-42", gotUserID)
}

func TestForward_ServiceDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	client := &ServiceClient{
		BaseURL: srv.URL,
		HTTP:    srv.Client(),
		Name:    "test-service",
	}

	rec := httptest.NewRecorder()
	client.Forward(t.Context(), rec, http.MethodGet, "/items", nil)

	assert.Equal(t, http.StatusBadGateway, rec.Code)
}
