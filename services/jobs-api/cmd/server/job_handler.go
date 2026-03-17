package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

type JobHandler struct {
	pool   *pgxpool.Pool
	jobs   JobStore
	outbox OutboxStore
}

func (h *JobHandler) Create(c echo.Context) error {
	var req CreateJobRequest
	if err := c.Bind(&req); err != nil {
		return err
	}

	if req.Title == "" {
		return NewAppError(http.StatusBadRequest, "title is required")
	}
	if req.ClientID == uuid.Nil {
		return NewAppError(http.StatusBadRequest, "client_id is required")
	}

	ctx := c.Request().Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	job, err := h.jobs.Create(ctx, tx, req)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}

	err = h.outbox.Insert(ctx, tx, OutboxEntry{
		AggregateType: "job",
		AggregateID:   job.ID,
		EventType:     EventJobCreated,
		Payload:       payload,
	})
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, job)
}

func (h *JobHandler) GetByID(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return NewAppError(http.StatusBadRequest, "invalid job id")
	}

	job, err := h.jobs.GetByID(c.Request().Context(), h.pool, id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, job)
}

func (h *JobHandler) List(c echo.Context) error {
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if offset < 0 {
		offset = 0
	}

	params := ListJobsParams{
		Limit:  limit,
		Offset: offset,
	}
	if s := c.QueryParam("status"); s != "" {
		if !validJobStatuses[s] {
			return NewAppError(http.StatusBadRequest, "invalid status filter")
		}
		params.Status = &s
	}

	jobs, err := h.jobs.List(c.Request().Context(), h.pool, params)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, jobs)
}

func (h *JobHandler) Update(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return NewAppError(http.StatusBadRequest, "invalid job id")
	}

	var req UpdateJobRequest
	if err := c.Bind(&req); err != nil {
		return err
	}
	if req.Status != nil && !validJobStatuses[*req.Status] {
		return NewAppError(http.StatusBadRequest, "invalid status")
	}

	ctx := c.Request().Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	job, err := h.jobs.Update(ctx, tx, id, req)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}

	err = h.outbox.Insert(ctx, tx, OutboxEntry{
		AggregateType: "job",
		AggregateID:   job.ID,
		EventType:     EventJobUpdated,
		Payload:       payload,
	})
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, job)
}

func (h *JobHandler) RegisterRoutes(g *echo.Group) {
	g.POST("", h.Create)
	g.GET("", h.List)
	g.GET("/:id", h.GetByID)
	g.PUT("/:id", h.Update)
}
