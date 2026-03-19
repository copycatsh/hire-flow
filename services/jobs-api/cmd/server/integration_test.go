package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/copycatsh/hire-flow/pkg/outbox"
	"github.com/copycatsh/hire-flow/services/jobs-api/migrations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func setupPostgres(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	pgContainer, err := tcpg.Run(ctx, "postgres:16-alpine",
		tcpg.WithDatabase("test_jobs"),
		tcpg.WithUsername("test"),
		tcpg.WithPassword("test"),
		tcpg.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pgContainer.Terminate(context.Background()) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	sqlDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	defer sqlDB.Close()

	goose.SetBaseFS(migrations.FS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(sqlDB, "."))

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	return pool
}

func setupNATS(t *testing.T, ctx context.Context) *NATSClient {
	t.Helper()

	natsContainer, err := tcnats.Run(ctx, "nats:2-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = natsContainer.Terminate(context.Background()) })

	natsURL, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err)

	nc, err := NewNATSClient(natsURL)
	require.NoError(t, err)
	t.Cleanup(func() { nc.Close() })

	require.NoError(t, nc.EnsureStream(ctx))
	return nc
}

func setupEcho(pool *pgxpool.Pool, jobStore JobStore, profileStore ProfileStore, outboxStore outbox.Store) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = customErrorHandler
	e.Use(requestLogger)

	jh := &JobHandler{pool: pool, jobs: jobStore, outbox: outboxStore}
	jh.RegisterRoutes(e.Group("/api/v1/jobs"))

	ph := &ProfileHandler{pool: pool, profiles: profileStore, outbox: outboxStore}
	ph.RegisterRoutes(e.Group("/api/v1/profiles"))

	return e
}

func TestJobCRUD_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	pool := setupPostgres(t, ctx)

	jobStore := &PostgresJobStore{}
	outboxStore := &outbox.PostgresStore{}
	e := setupEcho(pool, jobStore, &PostgresProfileStore{}, outboxStore)

	clientID := uuid.New()

	// Create
	createBody := `{"title":"Test Job","description":"A test job","budget_min":100,"budget_max":500,"client_id":"` + clientID.String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(createBody))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var created Job
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, "Test Job", created.Title)
	assert.Equal(t, "A test job", created.Description)
	assert.Equal(t, 100, created.BudgetMin)
	assert.Equal(t, 500, created.BudgetMax)
	assert.Equal(t, "draft", created.Status)
	assert.Equal(t, clientID, created.ClientID)
	assert.NotEqual(t, uuid.Nil, created.ID)

	// Get
	req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+created.ID.String(), nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var fetched Job
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &fetched))
	assert.Equal(t, created.ID, fetched.ID)
	assert.Equal(t, "Test Job", fetched.Title)

	// Update
	updateBody := `{"title":"Updated Job","status":"open"}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/jobs/"+created.ID.String(), strings.NewReader(updateBody))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var updated Job
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, "Updated Job", updated.Title)
	assert.Equal(t, "open", updated.Status)

	// List
	req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs?limit=10", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var jobs []Job
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &jobs))
	require.NotEmpty(t, jobs)

	found := false
	for _, j := range jobs {
		if j.ID == created.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "created job should appear in list")

	// Verify outbox has 2 entries (job.created + job.updated)
	var count int
	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE aggregate_id = $1`, created.ID,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestOutboxPublisher_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	pool := setupPostgres(t, ctx)
	nc := setupNATS(t, ctx)

	jobStore := &PostgresJobStore{}
	outboxStore := &outbox.PostgresStore{}

	// Create a job + outbox entry via store directly
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)

	job, err := jobStore.Create(ctx, tx, CreateJobRequest{
		Title:    "Outbox Test Job",
		ClientID: uuid.New(),
	})
	require.NoError(t, err)

	payload, err := json.Marshal(job)
	require.NoError(t, err)

	err = outboxStore.Insert(ctx, tx, outbox.Entry{
		AggregateType: "job",
		AggregateID:   job.ID,
		EventType:     EventJobCreated,
		Payload:       payload,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	// Subscribe NATS consumer to "jobs.job.created"
	rawConn, err := nats.Connect(nc.conn.ConnectedUrl())
	require.NoError(t, err)
	defer rawConn.Close()

	js, err := jetstream.New(rawConn)
	require.NoError(t, err)

	consumer, err := js.CreateConsumer(ctx, "JOBS", jetstream.ConsumerConfig{
		FilterSubject: "jobs.job.created",
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)

	// Run publisher batch
	publisher := outbox.NewPublisher(outboxStore, pool, nc, time.Second)
	require.NoError(t, publisher.PublishBatch(ctx))

	// Verify message received
	msg, err := consumer.Next(jetstream.FetchMaxWait(5 * time.Second))
	require.NoError(t, err)
	require.NotNil(t, msg)

	var receivedJob Job
	require.NoError(t, json.Unmarshal(msg.Data(), &receivedJob))
	assert.Equal(t, job.ID, receivedJob.ID)
	assert.Equal(t, "Outbox Test Job", receivedJob.Title)

	// Verify outbox marked published
	var publishedAt *time.Time
	err = pool.QueryRow(ctx,
		`SELECT published_at FROM outbox WHERE aggregate_id = $1`, job.ID,
	).Scan(&publishedAt)
	require.NoError(t, err)
	assert.NotNil(t, publishedAt)
}

func TestProfileCRUD_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	pool := setupPostgres(t, ctx)

	profileStore := &PostgresProfileStore{}
	outboxStore := &outbox.PostgresStore{}
	e := setupEcho(pool, &PostgresJobStore{}, profileStore, outboxStore)

	// Create profile
	createBody := `{"full_name":"Jane Doe","bio":"Go developer","hourly_rate":120}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles", strings.NewReader(createBody))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var created Profile
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, "Jane Doe", created.FullName)
	assert.Equal(t, 120, created.HourlyRate)
	assert.True(t, created.Available)
	assert.NotEqual(t, uuid.Nil, created.ID)

	// Get profile
	req = httptest.NewRequest(http.MethodGet, "/api/v1/profiles/"+created.ID.String(), nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var fetched Profile
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &fetched))
	assert.Equal(t, created.ID, fetched.ID)
	assert.Empty(t, fetched.Skills)

	// Get non-existent profile → 404
	req = httptest.NewRequest(http.MethodGet, "/api/v1/profiles/"+uuid.New().String(), nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// Create skills for testing UpdateSkills
	_, err := pool.Exec(ctx, `INSERT INTO skills (id, name, category) VALUES ($1, 'Go', 'language'), ($2, 'PostgreSQL', 'database')`,
		uuid.New(), uuid.New())
	require.NoError(t, err)

	var skillIDs []uuid.UUID
	rows, err := pool.Query(ctx, `SELECT id FROM skills ORDER BY name`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		require.NoError(t, rows.Scan(&id))
		skillIDs = append(skillIDs, id)
	}

	// Update skills
	skillsBody, _ := json.Marshal(UpdateProfileSkillsRequest{SkillIDs: skillIDs})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/profiles/"+created.ID.String()+"/skills", strings.NewReader(string(skillsBody)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var withSkills Profile
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &withSkills))
	assert.Len(t, withSkills.Skills, 2)

	// Verify outbox entries (profile.created + profile.updated)
	var events []string
	eventRows, err := pool.Query(ctx, `SELECT event_type FROM outbox WHERE aggregate_id = $1 ORDER BY created_at`, created.ID)
	require.NoError(t, err)
	defer eventRows.Close()
	for eventRows.Next() {
		var et string
		require.NoError(t, eventRows.Scan(&et))
		events = append(events, et)
	}
	assert.Equal(t, []string{EventProfileCreated, EventProfileUpdated}, events)
}

func TestTransactionRollback_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := t.Context()
	pool := setupPostgres(t, ctx)

	jobStore := &PostgresJobStore{}

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)

	job, err := jobStore.Create(ctx, tx, CreateJobRequest{
		Title:    "Rollback Job",
		ClientID: uuid.New(),
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, job.ID)

	// Rollback instead of commit
	require.NoError(t, tx.Rollback(ctx))

	// Verify job doesn't exist
	_, err = jobStore.GetByID(ctx, pool, job.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, pgx.ErrNoRows)
}
