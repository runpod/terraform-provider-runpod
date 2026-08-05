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
	t.Run("env override without version is normalized", func(t *testing.T) {
		t.Setenv("RUNPOD_BASE_URL", "http://localhost:8081")
		if got := GetRestBaseURL(); got != "http://localhost:8081/v2" {
			t.Errorf("got %q, want %q", got, "http://localhost:8081/v2")
		}
	})
	t.Run("env override already versioned is unchanged", func(t *testing.T) {
		t.Setenv("RUNPOD_BASE_URL", "http://localhost:8081/v2")
		if got := GetRestBaseURL(); got != "http://localhost:8081/v2" {
			t.Errorf("got %q, want %q", got, "http://localhost:8081/v2")
		}
	})
	t.Run("trailing slash is trimmed", func(t *testing.T) {
		t.Setenv("RUNPOD_BASE_URL", "http://localhost:8081/v2/")
		if got := GetRestBaseURL(); got != "http://localhost:8081/v2" {
			t.Errorf("got %q, want %q", got, "http://localhost:8081/v2")
		}
	})
}

func TestBaseURL(t *testing.T) {
	t.Run("unversioned client field gets /v2", func(t *testing.T) {
		c := NewRunPodClient("key", "http://g", "http://localhost:8081")
		if got := c.BaseURL(); got != "http://localhost:8081/v2" {
			t.Errorf("got %q, want %q", got, "http://localhost:8081/v2")
		}
	})
	t.Run("versioned client field is unchanged", func(t *testing.T) {
		c := NewRunPodClient("key", "http://g", "http://localhost:8081/v2")
		if got := c.BaseURL(); got != "http://localhost:8081/v2" {
			t.Errorf("got %q, want %q", got, "http://localhost:8081/v2")
		}
	})
	t.Run("empty client field falls back to env/default", func(t *testing.T) {
		t.Setenv("RUNPOD_BASE_URL", "")
		c := NewRunPodClient("key", "http://g", "")
		if got := c.BaseURL(); got != "https://api.runpod.io/v2" {
			t.Errorf("got %q, want default REST base URL", got)
		}
	})
}

func TestGetTemplateURL(t *testing.T) {
	t.Run("bare base is not double-versioned", func(t *testing.T) {
		c := NewRunPodClient("key", "http://g", "http://localhost:8081")
		if got := c.GetTemplateURL("tmpl-1"); got != "http://localhost:8081/v2/templates/tmpl-1" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("versioned base is not double-versioned", func(t *testing.T) {
		c := NewRunPodClient("key", "http://g", "http://localhost:8081/v2")
		if got := c.GetTemplateURL("tmpl-1"); got != "http://localhost:8081/v2/templates/tmpl-1" {
			t.Errorf("got %q", got)
		}
	})
}
