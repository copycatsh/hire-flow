package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

type APIError struct {
	StatusCode int
	Body       string
	Service    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s returned %d: %s", e.Service, e.StatusCode, e.Body)
}

type ServiceClient struct {
	BaseURL string
	HTTP    *http.Client
	Name    string
}

func (c *ServiceClient) Do(ctx context.Context, method, path string, body any, dest any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if userID, ok := ctx.Value(ctxKeyUserID).(string); ok && userID != "" {
		req.Header.Set("X-User-ID", userID)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s request: %w", c.Name, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
			Service:    c.Name,
		}
	}

	if dest != nil {
		if err := json.Unmarshal(respBody, dest); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}

	return nil
}

func (c *ServiceClient) Forward(ctx context.Context, w http.ResponseWriter, method, path string, body io.Reader) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		slog.Error("failed to create upstream request", "service", c.Name, "error", err)
		writeError(w, http.StatusBadGateway, fmt.Sprintf("%s: service unavailable", c.Name))
		return
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if userID, ok := ctx.Value(ctxKeyUserID).(string); ok && userID != "" {
		req.Header.Set("X-User-ID", userID)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		slog.Error("upstream request failed", "service", c.Name, "method", method, "path", path, "error", err)
		writeError(w, http.StatusBadGateway, fmt.Sprintf("%s: service unavailable", c.Name))
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		slog.Error("forwarding response body", "service", c.Name, "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}