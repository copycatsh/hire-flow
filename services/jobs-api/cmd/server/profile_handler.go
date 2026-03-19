package main

import (
	"encoding/json"
	"net/http"

	"github.com/copycatsh/hire-flow/pkg/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

type ProfileHandler struct {
	pool     *pgxpool.Pool
	profiles ProfileStore
	outbox   outbox.Store
}

func (h *ProfileHandler) Create(c echo.Context) error {
	var req CreateProfileRequest
	if err := c.Bind(&req); err != nil {
		return err
	}

	if req.FullName == "" {
		return NewAppError(http.StatusBadRequest, "full_name is required")
	}

	ctx := c.Request().Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	profile, err := h.profiles.Create(ctx, tx, req)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(profile)
	if err != nil {
		return err
	}

	err = h.outbox.Insert(ctx, tx, outbox.Entry{
		AggregateType: "profile",
		AggregateID:   profile.ID,
		EventType:     EventProfileCreated,
		Payload:       payload,
	})
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, profile)
}

func (h *ProfileHandler) GetByID(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return NewAppError(http.StatusBadRequest, "invalid profile id")
	}

	profile, err := h.profiles.GetByID(c.Request().Context(), h.pool, id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, profile)
}

func (h *ProfileHandler) UpdateSkills(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return NewAppError(http.StatusBadRequest, "invalid profile id")
	}

	var req UpdateProfileSkillsRequest
	if err := c.Bind(&req); err != nil {
		return err
	}

	ctx := c.Request().Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Verify profile exists
	if _, err := h.profiles.GetByID(ctx, tx, id); err != nil {
		return err
	}

	if err := h.profiles.UpdateSkills(ctx, tx, id, req); err != nil {
		return err
	}

	profile, err := h.profiles.GetByID(ctx, tx, id)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(profile)
	if err != nil {
		return err
	}

	err = h.outbox.Insert(ctx, tx, outbox.Entry{
		AggregateType: "profile",
		AggregateID:   profile.ID,
		EventType:     EventProfileUpdated,
		Payload:       payload,
	})
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, profile)
}

func (h *ProfileHandler) RegisterRoutes(g *echo.Group) {
	g.POST("", h.Create)
	g.GET("/:id", h.GetByID)
	g.PUT("/:id/skills", h.UpdateSkills)
}
