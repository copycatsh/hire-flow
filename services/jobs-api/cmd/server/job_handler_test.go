package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/copycatsh/hire-flow/pkg/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock stores ---

type mockJobStore struct {
	createFn func(ctx context.Context, db DBTX, req CreateJobRequest) (Job, error)
	getFn    func(ctx context.Context, db DBTX, id uuid.UUID) (Job, error)
	listFn   func(ctx context.Context, db DBTX, params ListJobsParams) ([]Job, error)
	updateFn func(ctx context.Context, db DBTX, id uuid.UUID, req UpdateJobRequest) (Job, error)
}

func (m *mockJobStore) Create(ctx context.Context, db DBTX, req CreateJobRequest) (Job, error) {
	if m.createFn != nil {
		return m.createFn(ctx, db, req)
	}
	return Job{}, nil
}

func (m *mockJobStore) GetByID(ctx context.Context, db DBTX, id uuid.UUID) (Job, error) {
	if m.getFn != nil {
		return m.getFn(ctx, db, id)
	}
	return Job{}, nil
}

func (m *mockJobStore) List(ctx context.Context, db DBTX, params ListJobsParams) ([]Job, error) {
	if m.listFn != nil {
		return m.listFn(ctx, db, params)
	}
	return nil, nil
}

func (m *mockJobStore) Update(ctx context.Context, db DBTX, id uuid.UUID, req UpdateJobRequest) (Job, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, db, id, req)
	}
	return Job{}, nil
}

type mockOutboxStore struct {
	insertFn          func(ctx context.Context, db outbox.DBTX, entry outbox.Entry) error
	fetchUnpublishedFn func(ctx context.Context, db outbox.DBTX, limit int) ([]outbox.Entry, error)
	markPublishedFn   func(ctx context.Context, db outbox.DBTX, ids []uuid.UUID) error
}

func (m *mockOutboxStore) Insert(ctx context.Context, db outbox.DBTX, entry outbox.Entry) error {
	if m.insertFn != nil {
		return m.insertFn(ctx, db, entry)
	}
	return nil
}

func (m *mockOutboxStore) FetchUnpublished(ctx context.Context, db outbox.DBTX, limit int) ([]outbox.Entry, error) {
	if m.fetchUnpublishedFn != nil {
		return m.fetchUnpublishedFn(ctx, db, limit)
	}
	return nil, nil
}

func (m *mockOutboxStore) MarkPublished(ctx context.Context, db outbox.DBTX, ids []uuid.UUID) error {
	if m.markPublishedFn != nil {
		return m.markPublishedFn(ctx, db, ids)
	}
	return nil
}

// --- Tests ---

func TestGetJob_NotFound(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = customErrorHandler

	jobID := uuid.New()
	handler := &JobHandler{
		jobs: &mockJobStore{
			getFn: func(_ context.Context, _ DBTX, _ uuid.UUID) (Job, error) {
				return Job{}, pgx.ErrNoRows
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+jobID.String(), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(jobID.String())

	err := handler.GetByID(c)
	if err != nil {
		e.HTTPErrorHandler(err, c)
	}

	assert.Equal(t, http.StatusNotFound, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "not found", body["error"])
}

func TestGetJob_InvalidID(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = customErrorHandler

	handler := &JobHandler{
		jobs: &mockJobStore{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("not-a-uuid")

	err := handler.GetByID(c)
	if err != nil {
		e.HTTPErrorHandler(err, c)
	}

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "invalid job id", body["error"])
}

func TestListJobs_Success(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = customErrorHandler

	now := time.Now().Truncate(time.Millisecond)
	testJobs := []Job{
		{ID: uuid.New(), Title: "Job 1", Status: "open", ClientID: uuid.New(), CreatedAt: now, UpdatedAt: now},
		{ID: uuid.New(), Title: "Job 2", Status: "open", ClientID: uuid.New(), CreatedAt: now, UpdatedAt: now},
	}

	handler := &JobHandler{
		jobs: &mockJobStore{
			listFn: func(_ context.Context, _ DBTX, _ ListJobsParams) ([]Job, error) {
				return testJobs, nil
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?limit=10", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.List(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, rec.Code)

	var result []Job
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Len(t, result, 2)
	assert.Equal(t, "Job 1", result[0].Title)
	assert.Equal(t, "Job 2", result[1].Title)
}

func TestCreateJob_BadRequest_MissingTitle(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = customErrorHandler

	handler := &JobHandler{
		jobs:   &mockJobStore{},
		outbox: &mockOutboxStore{},
	}

	body := `{"client_id":"` + uuid.New().String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Create(c)
	if err != nil {
		e.HTTPErrorHandler(err, c)
	}

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var respBody map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &respBody))
	assert.Equal(t, "title is required", respBody["error"])
}

func TestCustomErrorHandler(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantCode   int
		wantMsg    string
	}{
		{
			name:     "pgx.ErrNoRows returns 404",
			err:      pgx.ErrNoRows,
			wantCode: http.StatusNotFound,
			wantMsg:  "not found",
		},
		{
			name:     "AppError 400",
			err:      NewAppError(http.StatusBadRequest, "bad input"),
			wantCode: http.StatusBadRequest,
			wantMsg:  "bad input",
		},
		{
			name:     "echo.HTTPError",
			err:      echo.NewHTTPError(http.StatusForbidden, "forbidden"),
			wantCode: http.StatusForbidden,
			wantMsg:  "forbidden",
		},
		{
			name:     "unknown error returns 500",
			err:      errors.New("something broke"),
			wantCode: http.StatusInternalServerError,
			wantMsg:  "internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			customErrorHandler(tt.err, c)

			assert.Equal(t, tt.wantCode, rec.Code)

			var body map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, tt.wantMsg, body["error"])
		})
	}
}
