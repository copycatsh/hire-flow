package main

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e AppError) Error() string {
	return e.Message
}

func NewAppError(code int, msg string) AppError {
	return AppError{Code: code, Message: msg}
}

func customErrorHandler(err error, c echo.Context) {
	// TODO: log error when response already committed (review issue #5)
	if c.Response().Committed {
		return
	}

	code := http.StatusInternalServerError
	msg := "internal server error"

	var appErr AppError
	var httpErr *echo.HTTPError

	switch {
	case errors.As(err, &appErr):
		code = appErr.Code
		msg = appErr.Message
	case errors.Is(err, pgx.ErrNoRows):
		code = http.StatusNotFound
		msg = "not found"
	case errors.As(err, &httpErr):
		code = httpErr.Code
		if m, ok := httpErr.Message.(string); ok {
			msg = m
		}
	default:
		slog.Error("unhandled error", "error", err)
	}

	// TODO: log writeErr when c.JSON fails (review issue #4)
	_ = c.JSON(code, map[string]string{"error": msg})
}

// TODO: requestLogger swallows handler errors (returns nil) — consider returning
// the error so upstream middleware can observe it (review issue #7)
func requestLogger(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		start := time.Now()
		err := next(c)
		if err != nil {
			c.Error(err)
		}

		req := c.Request()
		slog.Info("request",
			"method", req.Method,
			"path", req.URL.Path,
			"status", c.Response().Status,
			"duration", time.Since(start),
		)

		return nil
	}
}
