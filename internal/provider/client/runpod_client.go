package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type RunPodClient struct {
	APIKey   string
	Endpoint string
	Client   *http.Client
}

func NewRunPodClient(apiKey, endpoint string) *RunPodClient {
	return &RunPodClient{
		APIKey:   apiKey,
		Endpoint: endpoint,
		Client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *RunPodClient) Query(ctx context.Context, query string, variables map[string]interface{}) (map[string]interface{}, error) {
	requestBody := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.Endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if errors, ok := result["errors"].([]interface{}); ok {
		errorMessages := make([]string, len(errors))
		for i, err := range errors {
			if errMap, ok := err.(map[string]interface{}); ok {
				if msg, ok := errMap["message"].(string); ok {
					errorMessages[i] = msg
				} else {
					errorMessages[i] = fmt.Sprintf("%v", err)
				}
			} else {
				errorMessages[i] = fmt.Sprintf("%v", err)
			}
		}
		return nil, fmt.Errorf("GraphQL errors: %v", errorMessages)
	}

	if data, ok := result["data"].(map[string]interface{}); ok {
		return data, nil
	}

	return nil, fmt.Errorf("no data in response")
}

func (c *RunPodClient) QueryRaw(ctx context.Context, query string) (map[string]interface{}, error) {
	return c.Query(ctx, query, map[string]interface{}{})
}
