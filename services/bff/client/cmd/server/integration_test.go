//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bffURL() string {
	return cmpOr(os.Getenv("BFF_URL"), "http://localhost:8010")
}

func traefikURL() string {
	return cmpOr(os.Getenv("TRAEFIK_URL"), "http://localhost")
}

// cmpOr returns the first non-empty string (can't import cmp in test without build issues).
func cmpOr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func integrationClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func login(t *testing.T, client *http.Client, base string) []*http.Cookie {
	t.Helper()
	body := `{"email":"client@example.com","password":"password"}`
	resp, err := client.Post(base+"/auth/login", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	return resp.Cookies()
}

func addCookies(req *http.Request, cookies []*http.Cookie) {
	for _, c := range cookies {
		req.AddCookie(c)
	}
}

func TestIntegration_LoginAndCreateJob(t *testing.T) {
	base := bffURL()
	client := integrationClient()

	// Step 1: Login.
	cookies := login(t, client, base)

	var accessCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "access_token" {
			accessCookie = c
			break
		}
	}
	require.NotNil(t, accessCookie, "access_token cookie must be set")
	assert.True(t, accessCookie.HttpOnly)

	// Step 2: Create job with auth.
	jobBody := `{"title":"Go Developer","description":"Build microservices","budget_min":5000,"budget_max":10000,"client_id":"11111111-1111-1111-1111-111111111111"}`
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/jobs", strings.NewReader(jobBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	addCookies(req, cookies)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var job map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&job))
	assert.Equal(t, "Go Developer", job["title"])
	jobID := fmt.Sprint(job["id"])
	require.NotEmpty(t, jobID)

	// Step 3: List jobs.
	req, _ = http.NewRequest(http.MethodGet, base+"/api/v1/jobs", nil)
	addCookies(req, cookies)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Step 4: Get job by ID.
	req, _ = http.NewRequest(http.MethodGet, base+"/api/v1/jobs/"+jobID, nil)
	addCookies(req, cookies)
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestIntegration_UnauthenticatedRequest(t *testing.T) {
	client := integrationClient()
	resp, err := client.Get(bffURL() + "/api/v1/jobs")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestIntegration_TraefikRouting(t *testing.T) {
	client := integrationClient()
	resp, err := client.Get(traefikURL() + "/client/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var health map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&health))
	assert.Equal(t, "ok", health["status"])
}

func TestIntegration_RefreshTokenRotation(t *testing.T) {
	base := bffURL()
	client := integrationClient()

	// Login to get tokens.
	cookies := login(t, client, base)
	var refreshCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "refresh_token" {
			refreshCookie = c
			break
		}
	}
	require.NotNil(t, refreshCookie)

	// Refresh.
	req, _ := http.NewRequest(http.MethodPost, base+"/auth/refresh", nil)
	req.AddCookie(refreshCookie)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// New cookies should be set.
	var newAccess, newRefresh bool
	for _, c := range resp.Cookies() {
		if c.Name == "access_token" {
			newAccess = true
		}
		if c.Name == "refresh_token" {
			newRefresh = true
		}
	}
	assert.True(t, newAccess, "new access token must be set")
	assert.True(t, newRefresh, "new refresh token must be set")
}