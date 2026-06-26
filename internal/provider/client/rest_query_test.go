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
	c := NewRunPodClient("testkey123", "")

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
	c := NewRunPodClient("testkey123", "")

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

// TestRestQuery_IgnoresContext documents a bug: RestQuery builds its request
// with http.NewRequest (runpod_client.go:110), NOT http.NewRequestWithContext,
// so the ctx argument is accepted but never wired into the request. A cancelled
// context therefore does NOT cancel the request. This test passes today because
// the request proceeds regardless of cancellation; it exists to pin the current
// behavior so a future fix (switching to NewRequestWithContext) is a deliberate,
// observable change.
func TestRestQuery_IgnoresContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	t.Setenv("RUNPOD_BASE_URL", srv.URL)
	c := NewRunPodClient("testkey123", "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call

	// Despite the cancelled context, the request still succeeds because ctx is
	// not propagated into the *http.Request.
	res, err := c.RestQuery(ctx, "GET", "anything", nil)
	if err != nil {
		t.Fatalf("expected success despite cancelled ctx (ctx is ignored), got error: %v", err)
	}
	if res["ok"] != true {
		t.Errorf("expected decoded body {ok:true}, got %#v", res)
	}
}
