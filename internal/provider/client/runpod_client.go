package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
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

func (c *RunPodClient) RestQuery(ctx context.Context, method, path string, params map[string]string) (map[string]interface{}, error) {
	baseUrl := os.Getenv("RUNPOD_BASE_URL")
	if baseUrl == "" {
		baseUrl = "https://rest.runpod.io/v1"
	}
	
	url := baseUrl + "/" + path
	
	if len(params) > 0 {
		queryParts := make([]string, 0, len(params))
		for k, v := range params {
			encodedValue := urlEncode(v)
			queryParts = append(queryParts, k+"="+encodedValue)
		}
		url += "?" + strings.Join(queryParts, "&")
	}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

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

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if resultMap, ok := result.(map[string]interface{}); ok {
		return resultMap, nil
	}
	if resultArray, ok := result.([]interface{}); ok {
		return map[string]interface{}{
			"billing": resultArray,
		}, nil
	}
	return nil, fmt.Errorf("unexpected response format")
}

func urlEncode(s string) string {
	result := ""
	for _, c := range s {
		switch c {
		case ' ', '!', '"', '#', '$', '%', '&', '\'', '(', ')', '*', '+', ',', '/', ':', ';', '<', '=', '>', '?', '@', '[', '\\', ']', '^', '`', '{', '|', '}', '~':
			result += "%" + formatHex(int(c))
		default:
			result += string(c)
		}
	}
	return result
}

func formatHex(n int) string {
	hex := "0123456789ABCDEF"
	result := ""
	for n > 0 {
		result = string(hex[n%16]) + result
		n /= 16
	}
	if len(result) == 0 {
		return "00"
	}
	if len(result) == 1 {
		return "0" + result
	}
	return result
}
