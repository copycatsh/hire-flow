package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type PaymentsClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewPaymentsClient(baseURL string) *PaymentsClient {
	return &PaymentsClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type HoldRequest struct {
	WalletID   string `json:"wallet_id"`
	Amount     int64  `json:"amount"`
	ContractID string `json:"contract_id"`
}

type HoldResponse struct {
	ID         string `json:"id"`
	WalletID   string `json:"wallet_id"`
	Amount     int64  `json:"amount"`
	Status     string `json:"status"`
	ContractID string `json:"contract_id"`
}

type ReleaseRequest struct {
	HoldID string `json:"hold_id"`
}

type TransferRequest struct {
	HoldID            string `json:"hold_id"`
	RecipientWalletID string `json:"recipient_wallet_id"`
}

func (c *PaymentsClient) HoldFunds(ctx context.Context, req HoldRequest) (HoldResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return HoldResponse{}, fmt.Errorf("payments hold marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/payments/hold", bytes.NewReader(body))
	if err != nil {
		return HoldResponse{}, fmt.Errorf("payments hold request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return HoldResponse{}, fmt.Errorf("payments hold call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return HoldResponse{}, fmt.Errorf("payments hold read body: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return HoldResponse{}, fmt.Errorf("payments hold failed: status=%d body=%s", resp.StatusCode, respBody)
	}

	var holdResp HoldResponse
	if err := json.Unmarshal(respBody, &holdResp); err != nil {
		return HoldResponse{}, fmt.Errorf("payments hold unmarshal: %w", err)
	}
	return holdResp, nil
}

func (c *PaymentsClient) ReleaseFunds(ctx context.Context, holdID string) error {
	body, err := json.Marshal(ReleaseRequest{HoldID: holdID})
	if err != nil {
		return fmt.Errorf("payments release marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/payments/release", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("payments release request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("payments release call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("payments release failed: status=%d body=%s", resp.StatusCode, respBody)
	}
	return nil
}

func (c *PaymentsClient) TransferFunds(ctx context.Context, holdID string, recipientWalletID string) error {
	body, err := json.Marshal(TransferRequest{HoldID: holdID, RecipientWalletID: recipientWalletID})
	if err != nil {
		return fmt.Errorf("payments transfer marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/payments/transfer", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("payments transfer request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("payments transfer call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("payments transfer failed: status=%d body=%s", resp.StatusCode, respBody)
	}
	return nil
}
