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

func GetRestBaseURL() string {
	url := os.Getenv("RUNPOD_BASE_URL")
	if url == "" {
		url = "https://api.runpod.io/v2"
	}
	return url
}

func (c *RunPodClient) getRestBaseURL() string {
	if c.RestBaseURL != "" {
		return c.RestBaseURL
	}
	return GetRestBaseURL()
}

func (c *RunPodClient) GetTemplateURL(templateId string) string {
	base := c.getRestBaseURL()
	if !strings.HasSuffix(base, "/v2") {
		return base + "/v2/templates/" + templateId
	}
	return base + "/templates/" + templateId
}
