package resource_endpoint

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// newBaseModel returns an EndpointModel with every List/Object field explicitly
// initialized to a typed Null. tfsdk.State.Set panics on a nil (uninitialized)
// types.List/types.Object value, so all of them must be set even when the test
// only cares about a couple of scalars.
func newBaseModel() EndpointModel {
	workerObjType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"id":           types.StringType,
		"pod_id":       types.StringType,
		"status":       types.StringType,
		"uptime_ms":    types.Int64Type,
		"start_time":   types.StringType,
		"last_busy_ms": types.Int64Type,
	}}
	return EndpointModel{
		AllowedCudaVersions: types.ListNull(types.StringType),
		ComputeType:         types.StringNull(),
		CpuFlavorIds:        types.ListNull(types.StringType),
		CpuFlavorPriority:   types.StringNull(),
		CreatedAt:           types.StringNull(),
		DataCenterIds:       types.ListNull(types.StringType),
		// env ObjectAttribute is declared with no AttributeTypes in the schema,
		// so its underlying type is an object with an empty attr-type map.
		Env:                types.ObjectNull(map[string]attr.Type{}),
		ExecutionTimeoutMs: types.Int64Null(),
		Flashboot:          types.BoolNull(),
		GpuCount:           types.Int64Null(),
		GpuTypeIds:         types.ListNull(types.StringType),
		GpuTypePriority:    types.StringNull(),
		Id:                 types.StringNull(),
		IdleTimeout:        types.Int64Null(),
		MinCudaVersion:     types.StringNull(),
		Name:               types.StringNull(),
		NetworkVolumeId:    types.StringNull(),
		NetworkVolumeIds:   types.ListNull(types.StringType),
		ScalerType:         types.StringNull(),
		ScalerValue:        types.Int64Null(),
		TemplateId:         types.StringNull(),
		TemplateVersion:    types.Int64Null(),
		Users:              types.ListNull(types.StringType),
		Version:            types.Int64Null(),
		VcpuCount:          types.Int64Null(),
		Workers:            types.ListNull(workerObjType),
		WorkersMax:         types.Int64Null(),
		WorkersMin:         types.Int64Null(),
	}
}

// stubServer returns an httptest server that records the last request body and
// method/path, and responds with the given status + body.
func stubServer(t *testing.T, status int, respBody string, captured *map[string]interface{}, method, path *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if method != nil {
			*method = r.Method
		}
		if path != nil {
			*path = r.URL.Path
		}
		if captured != nil {
			b, _ := io.ReadAll(r.Body)
			m := map[string]interface{}{}
			_ = json.Unmarshal(b, &m)
			*captured = m
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
	return srv
}

func TestEndpointCreate_Success(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.TemplateId = types.StringValue("tmpl-123")
	m.Name = types.StringValue("my-ep")
	m.WorkersMin = types.Int64Value(1)
	m.WorkersMax = types.Int64Value(3)

	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config: %v", d)
	}
	cfg := tfsdk.Config{Schema: sch, Raw: st.Raw}

	var captured map[string]interface{}
	// Create reads result["templateId"] and result["name"] via UNCHECKED type
	// assertions (endpoint_resource.go:234-235). The response must include both
	// or Create panics. They are included here so we test the happy path.
	resp := `{"id":"ep-1","templateId":"tmpl-123","name":"my-ep","workersMin":1,"workersMax":3,"version":2}`
	srv := stubServer(t, 200, resp, &captured, nil, nil)
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	cresp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, cresp)

	if cresp.Diagnostics.HasError() {
		t.Fatalf("Create returned diagnostics error: %v", cresp.Diagnostics.Errors())
	}

	// request body assertions
	if captured["templateId"] != "tmpl-123" {
		t.Errorf("request body templateId = %v, want tmpl-123", captured["templateId"])
	}
	if captured["name"] != "my-ep" {
		t.Errorf("request body name = %v, want my-ep", captured["name"])
	}
	if captured["workersMin"] != float64(1) {
		t.Errorf("request body workersMin = %v, want 1", captured["workersMin"])
	}

	// state assertions
	var out EndpointModel
	if d := cresp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read result state: %v", d)
	}
	if out.Id.ValueString() != "ep-1" {
		t.Errorf("state Id = %q, want ep-1", out.Id.ValueString())
	}
	if out.Name.ValueString() != "my-ep" {
		t.Errorf("state Name = %q, want my-ep", out.Name.ValueString())
	}
	if out.Version.ValueInt64() != 2 {
		t.Errorf("state Version = %d, want 2", out.Version.ValueInt64())
	}
}

// TestEndpointCreate_MissingAPIKey verifies the early guard when RUNPOD_API_KEY
// is unset.
func TestEndpointCreate_MissingAPIKey(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.TemplateId = types.StringValue("tmpl-123")
	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config: %v", d)
	}
	cfg := tfsdk.Config{Schema: sch, Raw: st.Raw}

	t.Setenv("RUNPOD_API_KEY", "")

	cresp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, cresp)

	if !cresp.Diagnostics.HasError() {
		t.Fatalf("expected diagnostics error for missing API key, got none")
	}
}

// TestEndpointCreate_NonOKStatus verifies a non-200 status yields a diagnostics
// error rather than mutating state. The body must still be valid JSON because
// Create unmarshals before checking the status code (endpoint_resource.go:216-225).
func TestEndpointCreate_NonOKStatus(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.TemplateId = types.StringValue("tmpl-123")
	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config: %v", d)
	}
	cfg := tfsdk.Config{Schema: sch, Raw: st.Raw}

	srv := stubServer(t, 400, `{"error":"bad request"}`, nil, nil, nil)
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	cresp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, cresp)

	if !cresp.Diagnostics.HasError() {
		t.Fatalf("expected diagnostics error for 400 status, got none")
	}
}

// TestEndpointCreate_EnvResponseBug characterizes a REAL BUG in Create.
//
// When the create response includes a non-empty "env" map
// (endpoint_resource.go:370-387), Create builds a types.ObjectValue using a
// HARDCODED attr-type map of {"key": StringType} but populates it with the
// ACTUAL env keys from the response. Unless the response env has exactly one
// key literally named "key", types.ObjectValue returns a diagnostics error
// ("attribute mismatch") and Create aborts via resp.Diagnostics.Append+return.
//
// This means an endpoint whose create response echoes its env vars (the normal
// case) fails at the Terraform layer. This test documents the behavior; it does
// NOT assert success. Source is not modified.
func TestEndpointCreate_EnvResponseBug(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.TemplateId = types.StringValue("tmpl-123")
	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config: %v", d)
	}
	cfg := tfsdk.Config{Schema: sch, Raw: st.Raw}

	// templateId + name present (required by unchecked casts); env has a
	// realistic key "MY_VAR" which does NOT match the hardcoded {"key":...}.
	resp := `{"id":"ep-1","templateId":"tmpl-123","name":"my-ep","env":{"MY_VAR":"hello"}}`
	srv := stubServer(t, 200, resp, nil, nil, nil)
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	cresp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, cresp)

	// Characterize: the env-attr-type mismatch surfaces as a diagnostics error.
	if !cresp.Diagnostics.HasError() {
		t.Fatalf("BUG REGRESSION? expected diagnostics error from env ObjectValue "+
			"attr-type mismatch, but Create succeeded. diags=%v", cresp.Diagnostics)
	}
}

func TestEndpointRead_Success(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	// seed state with an Id so Read targets /endpoints/ep-1
	m := newBaseModel()
	m.Id = types.StringValue("ep-1")
	m.TemplateId = types.StringValue("tmpl-123")
	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build state: %v", d)
	}
	reqState := tfsdk.State{Schema: sch, Raw: st.Raw}

	var gotMethod, gotPath string
	// Read uses checked casts throughout, so omitting fields is safe.
	resp := `{"id":"ep-1","templateId":"tmpl-123","name":"renamed-ep","version":5,"workersMin":2}`
	srv := stubServer(t, 200, resp, nil, &gotMethod, &gotPath)
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	rresp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Read(ctx, resource.ReadRequest{State: reqState}, rresp)

	if rresp.Diagnostics.HasError() {
		t.Fatalf("Read returned diagnostics error: %v", rresp.Diagnostics.Errors())
	}
	if gotMethod != "GET" {
		t.Errorf("Read method = %q, want GET", gotMethod)
	}
	if gotPath != "/endpoints/ep-1" {
		t.Errorf("Read path = %q, want /endpoints/ep-1", gotPath)
	}

	var out EndpointModel
	if d := rresp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read result state: %v", d)
	}
	if out.Name.ValueString() != "renamed-ep" {
		t.Errorf("state Name = %q, want renamed-ep (refreshed from API)", out.Name.ValueString())
	}
	if out.Version.ValueInt64() != 5 {
		t.Errorf("state Version = %d, want 5", out.Version.ValueInt64())
	}
}

func TestEndpointDelete_Success(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.Id = types.StringValue("ep-1")
	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build state: %v", d)
	}
	reqState := tfsdk.State{Schema: sch, Raw: st.Raw}

	var gotMethod, gotPath string
	// Delete expects HTTP 204 (endpoint_resource.go:1039).
	srv := stubServer(t, 204, ``, nil, &gotMethod, &gotPath)
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	dresp := &resource.DeleteResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Delete(ctx, resource.DeleteRequest{State: reqState}, dresp)

	if dresp.Diagnostics.HasError() {
		t.Fatalf("Delete returned diagnostics error: %v", dresp.Diagnostics.Errors())
	}
	if gotMethod != "DELETE" {
		t.Errorf("Delete method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/endpoints/ep-1" {
		t.Errorf("Delete path = %q, want /endpoints/ep-1", gotPath)
	}
}

// TestEndpointDelete_NonNoContent verifies Delete surfaces an error when the API
// returns a status other than 204.
func TestEndpointDelete_NonNoContent(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.Id = types.StringValue("ep-1")
	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build state: %v", d)
	}
	reqState := tfsdk.State{Schema: sch, Raw: st.Raw}

	srv := stubServer(t, 500, `internal error`, nil, nil, nil)
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	dresp := &resource.DeleteResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Delete(ctx, resource.DeleteRequest{State: reqState}, dresp)

	if !dresp.Diagnostics.HasError() {
		t.Fatalf("expected diagnostics error for 500 status, got none")
	}
}
