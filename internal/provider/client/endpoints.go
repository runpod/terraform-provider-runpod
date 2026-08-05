package client

import (
	"os"
	"strings"
)

func GetGraphQLEndpoint() string {
	endpoint := os.Getenv("RUNPOD_GRAPHQL_URL")
	if endpoint == "" {
		endpoint = "https://api.runpod.io/graphql"
	}
	return endpoint
}

// GetRestBaseURL returns the v2 REST base URL from RUNPOD_BASE_URL, or the
// production default. The value is normalized so it ends with "/v2" exactly
// once, regardless of how the environment variable was written.
func GetRestBaseURL() string {
	url := os.Getenv("RUNPOD_BASE_URL")
	if url == "" {
		return "https://api.runpod.io/v2"
	}
	return NormalizeRestBaseURL(url)
}

// NormalizeRestBaseURL trims any trailing slash and ensures the URL ends with
// the /v2 API version segment, so callers can safely append resource paths
// ("/pods", "/serverless", ...) without producing missing or duplicated
// version segments.
func NormalizeRestBaseURL(base string) string {
	base = strings.TrimSuffix(base, "/")
	if !strings.HasSuffix(base, "/v2") {
		base += "/v2"
	}
	return base
}

// BaseURL returns the client's normalized v2 REST base URL. Callers should
// prefer this over reading RestBaseURL directly.
func (c *RunPodClient) BaseURL() string {
	if c.RestBaseURL != "" {
		return NormalizeRestBaseURL(c.RestBaseURL)
	}
	return GetRestBaseURL()
}

func (c *RunPodClient) GetTemplateURL(templateId string) string {
	return c.BaseURL() + "/templates/" + templateId
}
