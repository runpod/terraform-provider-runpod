package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRestQuery_BuildsURLAndDecodes characterizes the happy path of RestQuery:
// it reads RUNPOD_BASE_URL, joins it with the path, appends url-encoded params,
// sets a Bearer Authorization header, uses the provided HTTP method, and decodes
// the JSON object response body into a map.
func TestRestQuery_BuildsURLAndDecodes(t *testing.T) {
	var gotPath, gotMethod, gotAuth, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"billing":[{"id":"b1"}]}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	// NewRunPodClient(apiKey, endpoint). NOTE: the second arg (Endpoint) is
	// only used by the GraphQL Query method; RestQuery ignores it entirely and
	// derives its base URL from the RUNPOD_BASE_URL env var (set above).
	c := NewRunPodClient("testkey123", "https://api.runpod.io/graphql", GetRestBaseURL())

	res, err := c.RestQuery(context.Background(), "GET", "billing/pods", map[string]string{"granularity": "daily"})
	if err != nil {
		t.Fatalf("RestQuery returned error: %v", err)
	}

	if gotMethod != "GET" {
		t.Errorf("method: got %q, want %q", gotMethod, "GET")
	}
	if gotPath != "/billing/pods" {
		t.Errorf("path: got %q, want %q", gotPath, "/billing/pods")
	}
	if gotAuth != "Bearer testkey123" {
		t.Errorf("authorization: got %q, want %q", gotAuth, "Bearer testkey123")
	}
	if !strings.Contains(gotQuery, "granularity=daily") {
		t.Errorf("query: got %q, want it to contain %q", gotQuery, "granularity=daily")
	}

	if _, ok := res["billing"]; !ok {
		t.Errorf("response: expected key %q to be present, got %#v", "billing", res)
	}
}

// TestRestQuery_NonOK_Errors verifies that a non-200 response status is turned
// into an error and no result map is returned.
func TestRestQuery_NonOK_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_BASE_URL", srv.URL)
	c := NewRunPodClient("testkey123", "https://api.runpod.io/graphql", GetRestBaseURL())

	res, err := c.RestQuery(context.Background(), "GET", "billing/pods", nil)
	if err == nil {
		t.Fatalf("expected error for non-200 response, got nil (res=%#v)", res)
	}
	if res != nil {
		t.Errorf("expected nil result on error, got %#v", res)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error: got %q, want it to mention status 500", err.Error())
	}
}

// TestRestQuery_RespectsContextCancellation asserts the CORRECT behavior:
// when RestQuery is called with an already-cancelled context, it should return
// a context-cancellation error and must NOT perform the HTTP request. This is
// currently broken because RestQuery builds its request with http.NewRequest
// instead of http.NewRequestWithContext, so the ctx is never wired in. The test
// is skipped until that is fixed so the package stays green.
func TestRestQuery_RespectsContextCancellation(t *testing.T) {


	var requested bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = true
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_BASE_URL", srv.URL)
	c := NewRunPodClient("testkey123", "https://api.runpod.io/graphql", GetRestBaseURL())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call

	// With a cancelled context, RestQuery should fail fast and never hit the server.
	res, err := c.RestQuery(ctx, "GET", "anything", nil)
	if err == nil {
		t.Fatalf("expected context-cancellation error, got nil (res=%#v)", res)
	}
	if !strings.Contains(err.Error(), "context") {
		t.Errorf("error: got %q, want it to mention context cancellation", err.Error())
	}
	if requested {
		t.Errorf("expected no HTTP request to be made with a cancelled context, but the server was hit")
	}
}
