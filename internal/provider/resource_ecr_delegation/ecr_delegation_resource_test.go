package resource_ecr_delegation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func ecrDelegationConfig(t *testing.T, m EcrDelegationModel) tfsdk.Config {
	t.Helper()
	ctx := context.Background()
	sch := EcrDelegationResourceSchema(ctx)
	st := tfsdk.State{Schema: sch}
	if diags := st.Set(ctx, &m); diags.HasError() {
		t.Fatalf("building config: %v", diags)
	}
	return tfsdk.Config{Schema: sch, Raw: st.Raw}
}

func ecrDelegationState(t *testing.T, m EcrDelegationModel) tfsdk.State {
	t.Helper()
	ctx := context.Background()
	sch := EcrDelegationResourceSchema(ctx)
	st := tfsdk.State{Schema: sch}
	if diags := st.Set(ctx, &m); diags.HasError() {
		t.Fatalf("building state: %v", diags)
	}
	return st
}

func baseEcrDelegationModel() EcrDelegationModel {
	return EcrDelegationModel{
		Name: types.StringNull(),
	}
}

func TestEcrDelegationCreate_SuccessWithDelegationField(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"data":{"delegation":{"id":"deleg-123","name":"my-deleg","awsUser":"arn:aws:iam::123456789:root","repository":"myrepo","tag":"latest","awsRegion":"us-east-1","dockerRegistryUri":"123456789.dkr.ecr.us-east-1.amazonaws.com","createdAt":"2026-07-14T00:00:00Z"}},"error":null}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseEcrDelegationModel()
	m.Resource = types.StringValue("arn:aws:ecr:us-east-1:123456789:repository/myrepo")
	m.Name = types.StringValue("my-deleg")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: EcrDelegationResourceSchema(context.Background())}}
	(&EcrDelegationResource{}).Create(context.Background(), resource.CreateRequest{Config: ecrDelegationConfig(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotBody["resource"] != "arn:aws:ecr:us-east-1:123456789:repository/myrepo" {
		t.Errorf("resource = %v, want arn:aws:ecr:us-east-1:123456789:repository/myrepo", gotBody["resource"])
	}
	if gotBody["name"] != "my-deleg" {
		t.Errorf("name = %v, want my-deleg", gotBody["name"])
	}

	var out EcrDelegationModel
	resp.State.Get(context.Background(), &out)
	if out.Id.ValueString() != "deleg-123" {
		t.Errorf("id = %q, want deleg-123", out.Id.ValueString())
	}
	if out.Name.ValueString() != "my-deleg" {
		t.Errorf("name = %q, want my-deleg", out.Name.ValueString())
	}
	if out.AwsUser.ValueString() != "arn:aws:iam::123456789:root" {
		t.Errorf("aws_user = %q, want arn:aws:iam::123456789:root", out.AwsUser.ValueString())
	}
}

func TestEcrDelegationCreate_SuccessWithDelegationsArray(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"data":{"delegations":[{"id":"deleg-456","name":"array-deleg","awsUser":"arn:aws:iam::987654321:root","repository":"repo2","tag":"v1","awsRegion":"us-west-2","dockerRegistryUri":"987654321.dkr.ecr.us-west-2.amazonaws.com","createdAt":"2026-07-14T01:00:00Z"}]},"error":null}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseEcrDelegationModel()
	m.Resource = types.StringValue("arn:aws:ecr:us-west-2:987654321:repository/repo2")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: EcrDelegationResourceSchema(context.Background())}}
	(&EcrDelegationResource{}).Create(context.Background(), resource.CreateRequest{Config: ecrDelegationConfig(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var out EcrDelegationModel
	resp.State.Get(context.Background(), &out)
	if out.Id.ValueString() != "deleg-456" {
		t.Errorf("id = %q, want deleg-456", out.Id.ValueString())
	}
}

func TestEcrDelegationCreate_UncheckedAssertion_PanicFix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"delegations":["not-an-object"]}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseEcrDelegationModel()
	m.Resource = types.StringValue("arn:aws:ecr:us-east-1:123456789:repository/test")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: EcrDelegationResourceSchema(context.Background())}}
	(&EcrDelegationResource{}).Create(context.Background(), resource.CreateRequest{Config: ecrDelegationConfig(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when delegations array contains non-object (CE-1677/CE-1682 panic fix)")
	}
}

func TestEcrDelegationCreate_Status200_OK(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"data":{"delegation":{"id":"deleg-789","name":"status200-deleg","awsUser":"arn:aws:iam::111111111:root","repository":"repo3","tag":"prod","awsRegion":"eu-west-1","dockerRegistryUri":"111111111.dkr.ecr.eu-west-1.amazonaws.com","createdAt":"2026-07-14T02:00:00Z"}},"error":null}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseEcrDelegationModel()
	m.Resource = types.StringValue("arn:aws:ecr:eu-west-1:111111111:repository/repo3")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: EcrDelegationResourceSchema(context.Background())}}
	(&EcrDelegationResource{}).Create(context.Background(), resource.CreateRequest{Config: ecrDelegationConfig(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics with 200 status: %v", resp.Diagnostics)
	}

	var out EcrDelegationModel
	resp.State.Get(context.Background(), &out)
	if out.Id.ValueString() != "deleg-789" {
		t.Errorf("id = %q, want deleg-789", out.Id.ValueString())
	}
}

func TestEcrDelegationCreate_MissingID_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"delegation":{"name":"no-id-deleg"}},"error":null}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseEcrDelegationModel()
	m.Resource = types.StringValue("arn:aws:ecr:us-east-1:123456789:repository/test")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: EcrDelegationResourceSchema(context.Background())}}
	(&EcrDelegationResource{}).Create(context.Background(), resource.CreateRequest{Config: ecrDelegationConfig(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when response has no id")
	}
}

func TestEcrDelegationCreate_NoAPIKey_Errors(t *testing.T) {
	t.Setenv("RUNPOD_API_KEY", "")

	m := baseEcrDelegationModel()
	m.Resource = types.StringValue("arn:aws:ecr:us-east-1:123456789:repository/test")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: EcrDelegationResourceSchema(context.Background())}}
	(&EcrDelegationResource{}).Create(context.Background(), resource.CreateRequest{Config: ecrDelegationConfig(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when RUNPOD_API_KEY is unset")
	}
}

func TestEcrDelegationRead_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"delegations":[{"id":"deleg-123","name":"read-deleg","awsUser":"arn:aws:iam::123456789:root","repository":"myrepo","tag":"latest","awsRegion":"us-east-1","dockerRegistryUri":"123456789.dkr.ecr.us-east-1.amazonaws.com","createdAt":"2026-07-14T00:00:00Z"},{"id":"other-deleg","name":"other","awsUser":"arn:aws:iam::999999999:root","repository":"otherrepo","tag":"v1","awsRegion":"us-west-2","dockerRegistryUri":"999999999.dkr.ecr.us-west-2.amazonaws.com","createdAt":"2026-07-13T00:00:00Z"}]}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseEcrDelegationModel()
	m.Id = types.StringValue("deleg-123")

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: EcrDelegationResourceSchema(context.Background())}}
	(&EcrDelegationResource{}).Read(context.Background(), resource.ReadRequest{State: ecrDelegationState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var out EcrDelegationModel
	resp.State.Get(context.Background(), &out)
	if out.Id.ValueString() != "deleg-123" {
		t.Errorf("id = %q, want deleg-123", out.Id.ValueString())
	}
	if out.Name.ValueString() != "read-deleg" {
		t.Errorf("name = %q, want read-deleg", out.Name.ValueString())
	}
}

func TestEcrDelegationRead_EmptyDelegations_RemovesState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"delegations":[]}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseEcrDelegationModel()
	m.Id = types.StringValue("deleg-gone")

	resp := &resource.ReadResponse{State: ecrDelegationState(t, m)}
	(&EcrDelegationResource{}).Read(context.Background(), resource.ReadRequest{State: ecrDelegationState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("empty delegations should not produce error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("state was not removed when no delegations found (CE-1654-style drift fix)")
	}
}

func TestEcrDelegationRead_404_RemovesState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseEcrDelegationModel()
	m.Id = types.StringValue("deleg-gone")

	resp := &resource.ReadResponse{State: ecrDelegationState(t, m)}
	(&EcrDelegationResource{}).Read(context.Background(), resource.ReadRequest{State: ecrDelegationState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("404 should not produce error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("state was not removed on 404")
	}
}

func TestEcrDelegationRead_NotFound_RemovesState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"delegations":[{"id":"other-deleg","name":"other"}]}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseEcrDelegationModel()
	m.Id = types.StringValue("not-found")

	resp := &resource.ReadResponse{State: ecrDelegationState(t, m)}
	(&EcrDelegationResource{}).Read(context.Background(), resource.ReadRequest{State: ecrDelegationState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("not-found should not produce error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("state was not removed when delegation not found in list")
	}
}

func TestEcrDelegationDelete_Success(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseEcrDelegationModel()
	m.Id = types.StringValue("deleg-123")

	resp := &resource.DeleteResponse{State: ecrDelegationState(t, m)}
	(&EcrDelegationResource{}).Delete(context.Background(), resource.DeleteRequest{State: ecrDelegationState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotMethod != "DELETE" {
		t.Errorf("expected DELETE, got %s", gotMethod)
	}
	if gotPath != "/v2/registries/delegations/deleg-123" {
		t.Errorf("expected /v2/registries/delegations/deleg-123, got %s", gotPath)
	}
}

func TestEcrDelegationDelete_Status200_OK(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"deleted"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseEcrDelegationModel()
	m.Id = types.StringValue("deleg-123")

	resp := &resource.DeleteResponse{State: ecrDelegationState(t, m)}
	(&EcrDelegationResource{}).Delete(context.Background(), resource.DeleteRequest{State: ecrDelegationState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics with 200 status: %v", resp.Diagnostics)
	}
	if gotMethod != "DELETE" {
		t.Errorf("expected DELETE, got %s", gotMethod)
	}
}

func TestEcrDelegationDelete_Non204_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("delete failed"))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseEcrDelegationModel()
	m.Id = types.StringValue("deleg-123")

	resp := &resource.DeleteResponse{State: ecrDelegationState(t, m)}
	(&EcrDelegationResource{}).Delete(context.Background(), resource.DeleteRequest{State: ecrDelegationState(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when delete returns non-204/non-200 status")
	}
}

func TestEcrDelegationUpdate_NotSupported(t *testing.T) {
	m := baseEcrDelegationModel()
	m.Id = types.StringValue("deleg-123")
	m.Resource = types.StringValue("arn:aws:ecr:us-east-1:123456789:repository/test")

	resp := &resource.UpdateResponse{State: ecrDelegationState(t, m)}
	(&EcrDelegationResource{}).Update(context.Background(), resource.UpdateRequest{Config: ecrDelegationConfig(t, m), State: ecrDelegationState(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for update (not supported)")
	}
}
