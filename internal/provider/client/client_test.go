package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient returns a client pointed at the given test server URL.
func newTestClient(url string) *RunPodClient {
	return NewRunPodClient("test-key", url)
}

// TestQuery_Success_ReturnsInnerData documents that Query unwraps the GraphQL
// envelope once and returns the *inner* `data` object — not the full response.
// This is the basis of risk R1: callers (machine, pod_action) that do
// result["data"] again are double-unwrapping.
func TestQuery_Success_ReturnsInnerData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization header = %q, want %q", got, "Bearer test-key")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"podStop":{"id":"p1","status":"STOPPED"}}}`))
	}))
	defer srv.Close()

	got, err := newTestClient(srv.URL).Query(context.Background(), "query{}", nil)
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	if _, ok := got["data"]; ok {
		t.Errorf("Query returned the full envelope (still has [\"data\"]); expected the inner data object: %v", got)
	}
	podStop, ok := got["podStop"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected inner key \"podStop\" in result, got: %v", got)
	}
	if podStop["status"] != "STOPPED" {
		t.Errorf("status = %v, want STOPPED", podStop["status"])
	}
}

// TestQuery_SendsCorrectRequest verifies what the client puts on the wire:
// the GraphQL query + variables in the JSON body, plus the auth and
// content-type headers. We only tested response handling before, not the
// request we send.
func TestQuery_SendsCorrectRequest(t *testing.T) {
	var gotBody map[string]interface{}
	var gotAuth, gotContentType, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer srv.Close()

	vars := map[string]interface{}{"podId": "p1", "count": float64(2)}
	_, err := newTestClient(srv.URL).Query(context.Background(), "mutation Foo { x }", vars)
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want 'Bearer test-key'", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody["query"] != "mutation Foo { x }" {
		t.Errorf("body.query = %v, want the query string", gotBody["query"])
	}
	gotVars, ok := gotBody["variables"].(map[string]interface{})
	if !ok || gotVars["podId"] != "p1" || gotVars["count"] != float64(2) {
		t.Errorf("body.variables = %v, want podId=p1 count=2", gotBody["variables"])
	}
}

// TestQuery_IgnoresContextCancellation characterizes R10: Query builds its
// request with http.NewRequest (not NewRequestWithContext), so the ctx is never
// attached. A pre-cancelled context does NOT abort the call — the request still
// succeeds. When fixed (context-aware request), a cancelled ctx should error;
// flip this test then.
func TestQuery_IgnoresContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	_, err := newTestClient(srv.URL).Query(ctx, "query{}", nil)
	if err != nil {
		t.Fatalf("expected the cancelled ctx to be IGNORED (R10), but Query errored: %v — ctx may now be wired; flip this test", err)
	}
}

func TestQuery_GraphQLErrors_Aggregated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"boom"},{"message":"bang"}]}`))
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Query(context.Background(), "query{}", nil)
	if err == nil {
		t.Fatal("expected error for GraphQL errors, got nil")
	}
	if !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "bang") {
		t.Errorf("error should contain both messages, got: %v", err)
	}
}

func TestQuery_Non200_ReturnsStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream exploded"))
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Query(context.Background(), "query{}", nil)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "upstream exploded") {
		t.Errorf("error should contain status and body, got: %v", err)
	}
}

func TestQuery_MalformedJSON_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("this is not json"))
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Query(context.Background(), "query{}", nil)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "parse response") {
		t.Errorf("error should mention parse failure, got: %v", err)
	}
}

func TestQuery_NoDataNoErrors_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"extensions":{}}`))
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Query(context.Background(), "query{}", nil)
	if err == nil || !strings.Contains(err.Error(), "no data in response") {
		t.Errorf("expected \"no data in response\" error, got: %v", err)
	}
}
