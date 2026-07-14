package datasource_ecr_delegations

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

func ecrDelegationsDataSourceState(t *testing.T) tfsdk.State {
	t.Helper()
	ctx := context.Background()
	sch := EcrDelegationsDataSourceSchema(ctx)
	st := tfsdk.State{Schema: sch}
	return st
}

func TestEcrDelegationsDataSourceRead_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"delegations":[{"id":"deleg-123","name":"ds-read","awsUser":"arn:aws:iam::123456789:root","repository":"myrepo","tag":"latest","awsRegion":"us-east-1","dockerRegistryUri":"123456789.dkr.ecr.us-east-1.amazonaws.com","createdAt":"2026-07-14T00:00:00Z"},{"id":"deleg-456","name":"second","awsUser":"arn:aws:iam::987654321:root","repository":"repo2","tag":"v1","awsRegion":"us-west-2","dockerRegistryUri":"987654321.dkr.ecr.us-west-2.amazonaws.com","createdAt":"2026-07-13T00:00:00Z"}]}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &datasource.ReadResponse{State: ecrDelegationsDataSourceState(t)}
	(&EcrDelegationsDataSource{}).Read(context.Background(), datasource.ReadRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var out EcrDelegationsDataSourceModel
	resp.State.Get(context.Background(), &out)
	if len(out.EcrDelegations) != 2 {
		t.Errorf("expected 2 delegations, got %d", len(out.EcrDelegations))
	}
	if out.EcrDelegations[0].Id.ValueString() != "deleg-123" {
		t.Errorf("first id = %q, want deleg-123", out.EcrDelegations[0].Id.ValueString())
	}
	if out.EcrDelegations[1].Id.ValueString() != "deleg-456" {
		t.Errorf("second id = %q, want deleg-456", out.EcrDelegations[1].Id.ValueString())
	}
}

func TestEcrDelegationsDataSourceRead_EmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"delegations":[]}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &datasource.ReadResponse{State: ecrDelegationsDataSourceState(t)}
	(&EcrDelegationsDataSource{}).Read(context.Background(), datasource.ReadRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics with empty list: %v", resp.Diagnostics)
	}

	var out EcrDelegationsDataSourceModel
	resp.State.Get(context.Background(), &out)
	if len(out.EcrDelegations) != 0 {
		t.Errorf("expected 0 delegations, got %d", len(out.EcrDelegations))
	}
}

func TestEcrDelegationsDataSourceRead_NoAPIKey_Errors(t *testing.T) {
	t.Setenv("RUNPOD_API_KEY", "")

	resp := &datasource.ReadResponse{State: ecrDelegationsDataSourceState(t)}
	(&EcrDelegationsDataSource{}).Read(context.Background(), datasource.ReadRequest{}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when RUNPOD_API_KEY is unset")
	}
}

func TestEcrDelegationsDataSourceRead_HTTP401_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &datasource.ReadResponse{State: ecrDelegationsDataSourceState(t)}
	(&EcrDelegationsDataSource{}).Read(context.Background(), datasource.ReadRequest{}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on HTTP 401")
	}
}

func TestEcrDelegationsDataSourceRead_MalformedJSON_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &datasource.ReadResponse{State: ecrDelegationsDataSourceState(t)}
	(&EcrDelegationsDataSource{}).Read(context.Background(), datasource.ReadRequest{}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error on malformed JSON")
	}
}

func TestEcrDelegationsDataSourceRead_MissingIDField_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"delegations":[{"name":"no-id"}]}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &datasource.ReadResponse{State: ecrDelegationsDataSourceState(t)}
	(&EcrDelegationsDataSource{}).Read(context.Background(), datasource.ReadRequest{}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when delegation missing id field")
	}
}
