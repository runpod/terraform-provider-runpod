package client

import "testing"

func TestGetGraphQLEndpoint(t *testing.T) {
	t.Run("default when unset", func(t *testing.T) {
		t.Setenv("RUNPOD_GRAPHQL_URL", "")
		if got := GetGraphQLEndpoint(); got != "https://api.runpod.io/graphql" {
			t.Errorf("got %q, want default GraphQL endpoint", got)
		}
	})
	t.Run("env override", func(t *testing.T) {
		t.Setenv("RUNPOD_GRAPHQL_URL", "http://localhost:4000/graphql")
		if got := GetGraphQLEndpoint(); got != "http://localhost:4000/graphql" {
			t.Errorf("got %q, want the env override", got)
		}
	})
}

func TestGetRestBaseURL(t *testing.T) {
	t.Run("default when unset", func(t *testing.T) {
		t.Setenv("RUNPOD_BASE_URL", "")
		if got := GetRestBaseURL(); got != "https://api.runpod.io/v2" {
			t.Errorf("got %q, want default REST base URL", got)
		}
	})
	t.Run("env override", func(t *testing.T) {
		t.Setenv("RUNPOD_BASE_URL", "http://localhost:8081/v1")
		if got := GetRestBaseURL(); got != "http://localhost:8081/v1" {
			t.Errorf("got %q, want the env override", got)
		}
	})
}
