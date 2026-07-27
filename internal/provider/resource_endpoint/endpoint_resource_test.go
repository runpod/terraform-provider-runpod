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

// newBaseModel returns an EndpointModel with every List/Map field explicitly
// initialized to a typed Null. tfsdk.State.Set panics on a nil (uninitialized)
// types.List/types.Map value, so all of them must be set even when the test
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
		Env:                 types.MapNull(types.StringType), // env is a MapAttribute(string)
		ExecutionTimeoutMs:  types.Int64Null(),
		Flashboot:           types.BoolNull(),
		GpuCount:            types.Int64Null(),
		GpuTypeIds:          types.ListNull(types.StringType),
		GpuTypePriority:     types.StringNull(),
		Id:                  types.StringNull(),
		IdleTimeout:         types.Int64Null(),
		MinCudaVersion:      types.StringNull(),
		Name:                types.StringNull(),
		NetworkVolumeId:     types.StringNull(),
		NetworkVolumeIds:    types.ListNull(types.StringType),
		ScalerType:          types.StringNull(),
		ScalerValue:         types.Int64Null(),
		TemplateId:          types.StringNull(),
		TemplateVersion:     types.Int64Null(),
		Users:               types.ListNull(types.StringType),
		Version:             types.Int64Null(),
		VcpuCount:           types.Int64Null(),
		Workers:             types.ListNull(workerObjType),
		WorkersMax:          types.Int64Null(),
		WorkersMin:          types.Int64Null(),
	}
}

// stubServer returns an httptest server that records the last request body and
// method/path, and responds with the given status + body.
func stubServer(t *testing.T, status int, respBody string, captured *map[string]interface{}, method, path *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/templates/tmpl-123" || r.URL.Path == "/v2/templates/t-1" {
			_, _ = w.Write([]byte(`{"image":"runpod/worker:latest","args":"","ports":["8000/http"],"env":{},"disk":50,"mounts":{}}`))
			return
		}
		
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
	m.ImageName = types.StringValue("runpod/worker:latest")
	m.GpuTypeId = types.StringValue("GPU-1")
	m.GpuCount = types.Int64Value(1)
	m.Name = types.StringValue("my-ep")
	m.WorkersMin = types.Int64Value(1)
	m.WorkersMax = types.Int64Value(3)

	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config: %v", d)
	}
	cfg := tfsdk.Config{Schema: sch, Raw: st.Raw}

	var captured map[string]interface{}
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
	if captured["image"] != "runpod/worker:latest" {
		t.Errorf("request body image = %v, want runpod/worker:latest", captured["image"])
	}
	gpu := captured["gpu"].(map[string]interface{})
	pools := gpu["pools"].([]interface{})
	if pools[0].(string) != "GPU-1" {
		t.Errorf("request body gpu.pools = %v, want GPU-1", pools)
	}
	if gpu["count"] != float64(1) {
		t.Errorf("request body gpu.count = %v, want 1", gpu["count"])
	}
	if captured["name"] != "my-ep" {
		t.Errorf("request body name = %v, want my-ep", captured["name"])
	}
	workers := captured["workers"].(map[string]interface{})
	if workers["min"] != float64(1) {
		t.Errorf("request body workers.min = %v, want 1", workers["min"])
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

// TestEndpointCreate_Accepts201 locks in CE-1681: POST /v2/serverless returns 201
// Created, so Create must treat 201 as success (not only 200).
func TestEndpointCreate_Accepts201(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.TemplateId = types.StringValue("tmpl-123")
	m.Name = types.StringValue("my-ep")
	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config: %v", d)
	}
	cfg := tfsdk.Config{Schema: sch, Raw: st.Raw}

	srv := stubServer(t, 201, `{"id":"ep-1","templateId":"tmpl-123","name":"my-ep","version":1}`, nil, nil, nil)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	cresp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, cresp)
	if cresp.Diagnostics.HasError() {
		t.Fatalf("Create must accept HTTP 201 (CE-1681): %v", cresp.Diagnostics.Errors())
	}
	var out EndpointModel
	if d := cresp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read state: %v", d)
	}
	if out.Id.ValueString() != "ep-1" {
		t.Errorf("id = %q, want ep-1 (Create must set id on a 201 response)", out.Id.ValueString())
	}
}

// TestEndpointCreate_PartialResponse_NoPanic guards the fix that made Create
// ok-check its response fields (previously result["templateId"]/["name"] were
// unchecked .(string) casts that panicked on a partial response). A response
// with only "id" must NOT panic and must still set the Id into state.
func TestEndpointCreate_PartialResponse_NoPanic(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.TemplateId = types.StringValue("tmpl-123")
	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config: %v", d)
	}
	cfg := tfsdk.Config{Schema: sch, Raw: st.Raw}

	srv := stubServer(t, 200, `{"id":"ep-1"}`, nil, nil, nil)
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	cresp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, cresp)

	if cresp.Diagnostics.HasError() {
		t.Fatalf("Create returned diagnostics error: %v", cresp.Diagnostics.Errors())
	}
	var out EndpointModel
	if d := cresp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read result state: %v", d)
	}
	if out.Id.ValueString() != "ep-1" {
		t.Errorf("state Id = %q, want ep-1", out.Id.ValueString())
	}
}

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
// error rather than mutating state.
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

// TestEndpointCreate_EnvRoundTrips is the regression for CE-1672 (fixed): env is
// now a MapAttribute(string), so a create response echoing env vars round-trips
// into state keyed by the real var names, with no diagnostics error.
func TestEndpointCreate_EnvRoundTrips(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.TemplateId = types.StringValue("tmpl-123")
	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config: %v", d)
	}
	cfg := tfsdk.Config{Schema: sch, Raw: st.Raw}

	resp := `{"id":"ep-1","templateId":"tmpl-123","name":"my-ep","env":{"MY_VAR":"hello"}}`
	srv := stubServer(t, 200, resp, nil, nil, nil)
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	cresp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, cresp)

	if cresp.Diagnostics.HasError() {
		t.Fatalf("Create returned diagnostics error for echoed env vars: %v", cresp.Diagnostics.Errors())
	}

	var out EndpointModel
	if d := cresp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read result state: %v", d)
	}
	if out.Env.IsNull() {
		t.Fatalf("state Env is null, want populated with MY_VAR")
	}
	elems := out.Env.Elements()
	v, ok := elems["MY_VAR"]
	if !ok {
		t.Fatalf("state Env missing key MY_VAR; got elements %v", elems)
	}
	if sv, ok := v.(types.String); !ok || sv.ValueString() != "hello" {
		t.Errorf("state Env[MY_VAR] = %v, want %q", v, "hello")
	}
}

func TestEndpointRead_Success(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.Id = types.StringValue("ep-1")
	m.TemplateId = types.StringValue("tmpl-123")
	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build state: %v", d)
	}
	reqState := tfsdk.State{Schema: sch, Raw: st.Raw}

	var gotMethod, gotPath string
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
	if gotPath != "/v2/serverless/ep-1" {
		t.Errorf("Read path = %q, want /v2/serverless/ep-1", gotPath)
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
	if gotPath != "/v2/serverless/ep-1" {
		t.Errorf("Delete path = %q, want /v2/serverless/ep-1", gotPath)
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

func strList(vals ...string) types.List {
	elems := make([]attr.Value, len(vals))
	for i, v := range vals {
		elems[i] = types.StringValue(v)
	}
	return types.ListValueMust(types.StringType, elems)
}

// TestEndpointCreate_WithListFields exercises Create's list/map body-building
// branches (networkVolumeIds, dataCenterIds, allowedCudaVersions,
// env) — the bulk of Create the happy-path test doesn't reach.
func TestEndpointCreate_WithListFields(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.ImageName = types.StringValue("runpod/worker:latest")
	m.GpuTypeId = types.StringValue("NVIDIA A100")
	m.GpuCount = types.Int64Value(1)
	m.Name = types.StringValue("my-ep")
	m.NetworkVolumeIds = strList("nv-1", "nv-2")
	m.DataCenterIds = strList("US-CA-1")
	m.AllowedCudaVersions = strList("12.0", "12.1")
	m.Env = types.MapValueMust(types.StringType, map[string]attr.Value{"MY_VAR": types.StringValue("x")})

	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config: %v", d)
	}
	cfg := tfsdk.Config{Schema: sch, Raw: st.Raw}

	var body map[string]interface{}
	srv := stubServer(t, 200, `{"id":"ep-1","templateId":"tmpl-123","name":"my-ep","image":"runpod/worker:latest","gpu":{"id":"NVIDIA A100","count":1}}`, &body, nil, nil)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	cresp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, cresp)
	if cresp.Diagnostics.HasError() {
		t.Fatalf("Create errored: %v", cresp.Diagnostics.Errors())
	}

	for _, k := range []string{"networkVolumeIds", "dataCenterIds", "allowedCudaVersions"} {
		arr, ok := body[k].([]interface{})
		if !ok || len(arr) == 0 {
			t.Errorf("body[%q] not a non-empty array; got %v", k, body[k])
		}
	}
	if nv, _ := body["networkVolumeIds"].([]interface{}); len(nv) != 2 {
		t.Errorf("networkVolumeIds = %v, want 2 elements", body["networkVolumeIds"])
	}
	if env, ok := body["env"].(map[string]interface{}); !ok || env["MY_VAR"] != "x" {
		t.Errorf("body env = %v, want MY_VAR=x", body["env"])
	}
}

// TestEndpointUpdate_Success drives Update (reads req.Config + req.State) and
// asserts the PATCH carries the changed field. Update is entirely uncovered by
// the happy-path suite.
func TestEndpointUpdate_Success(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	prior := newBaseModel()
	prior.Id = types.StringValue("ep-1")
	prior.Name = types.StringValue("old-name")
	prior.WorkersMin = types.Int64Value(1)

	desired := newBaseModel()
	desired.Id = types.StringValue("ep-1")
	desired.Name = types.StringValue("old-name")
	desired.WorkersMin = types.Int64Value(5) // changed

	priorSt := tfsdk.State{Schema: sch}
	if d := priorSt.Set(ctx, &prior); d.HasError() {
		t.Fatalf("build prior state: %v", d)
	}
	desiredSt := tfsdk.State{Schema: sch}
	if d := desiredSt.Set(ctx, &desired); d.HasError() {
		t.Fatalf("build desired: %v", d)
	}

	var body map[string]interface{}
	var method, path string
	srv := stubServer(t, 200, `{"id":"ep-1","name":"old-name","workers":{"min":5,"max":5},"scaling":{"idleTimeout":300},"version":3}`, &body, &method, &path)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	uresp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: sch, Raw: desiredSt.Raw},
		State:  priorSt,
	}, uresp)

	if uresp.Diagnostics.HasError() {
		t.Fatalf("Update errored: %v", uresp.Diagnostics.Errors())
	}
	if method != "PATCH" || path != "/v2/serverless/ep-1" {
		t.Errorf("expected PATCH /v2/serverless/ep-1, got %s %s", method, path)
	}
	workers := body["workers"].(map[string]interface{})
	if workers["min"] != float64(5) {
		t.Errorf("PATCH body workers.min = %v, want 5", workers["min"])
	}
}

// TestEndpointUpdate_PreservesComputedState asserts the CORRECT behavior for the
// CE-1688 bug (GitHub #34): EndpointResource.Update builds the merged state from
// prior state + PATCH response, then overwrites it with resp.State.Set(ctx, &config),
// clobbering computed values config leaves null (vcpu_count, compute_type). Through
// the real plugin protocol this surfaces as "Provider produced inconsistent result
// after apply". Skipped until CE-1688 is fixed (drop the trailing State.Set(&config)).
// Note: this test's preservation assertion depends on TWO behaviors — dropping the
// trailing State.Set(&config) AND the merge continuing to seed `state` from prior
// state. If a future change drops State.Set but also stops seeding from prior state,
// the un-skipped test would still fail (for the second reason), so treat a failure as
// "check both" rather than assuming the overwrite is the only cause.
func TestEndpointUpdate_PreservesComputedState(t *testing.T) {
	t.Skip("CE-1688: endpoint Update clobbers computed state (config-overwrites-computed-state / GitHub #34) — un-skip when fixed")
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	// Prior state carries concrete computed values the API resolved earlier.
	prior := newBaseModel()
	prior.Id = types.StringValue("ep-1")
	prior.Name = types.StringValue("ep-1")
	prior.WorkersMin = types.Int64Value(1)
	prior.VcpuCount = types.Int64Value(2)
	prior.ComputeType = types.StringValue("cpu")

	// Config leaves the computed fields null (the reporter's ignore_changes case)
	// and changes only an unrelated mutable field (workers_min).
	desired := newBaseModel()
	desired.Id = types.StringValue("ep-1")
	desired.Name = types.StringValue("ep-1")
	desired.WorkersMin = types.Int64Value(5) // changed
	// VcpuCount / ComputeType intentionally left Null.

	priorSt := tfsdk.State{Schema: sch}
	if d := priorSt.Set(ctx, &prior); d.HasError() {
		t.Fatalf("build prior state: %v", d)
	}
	desiredSt := tfsdk.State{Schema: sch}
	if d := desiredSt.Set(ctx, &desired); d.HasError() {
		t.Fatalf("build desired: %v", d)
	}

	// PATCH response omits vcpuCount/computeType, so the merged `state` keeps the
	// prior concrete values — isolating the overwrite as the sole cause of the null.
	srv := stubServer(t, 200, `{"id":"ep-1","name":"ep-1","workersMin":5}`, nil, nil, nil)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	uresp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: sch, Raw: desiredSt.Raw},
		State:  priorSt,
	}, uresp)
	if uresp.Diagnostics.HasError() {
		t.Fatalf("Update errored: %v", uresp.Diagnostics.Errors())
	}

	var out EndpointModel
	if d := uresp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read result state: %v", d)
	}

	// Correct behavior (post-fix): the merged computed values survive the update.
	if out.VcpuCount.ValueInt64() != 2 {
		t.Errorf("vcpu_count = %v, want 2 preserved (CE-1688: Update must not clobber computed state)", out.VcpuCount)
	}
	if out.ComputeType.ValueString() != "cpu" {
		t.Errorf("compute_type = %q, want \"cpu\" preserved (CE-1688)", out.ComputeType.ValueString())
	}
}

// TestEndpointRead_MapsWorkersAndEnv exercises Read's workers-list and env-map
// field-mapping branches (not reached by the scalar-only Read happy-path test).
func TestEndpointRead_MapsWorkersAndEnv(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.Id = types.StringValue("ep-1")
	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build state: %v", d)
	}

	resp := `{"id":"ep-1","name":"ep","computeType":"GPU","gpuCount":2,
		"env":{"MY_VAR":"hello"},
		"workers":[{"id":"w-1","podId":"p-1","status":"RUNNING","uptimeMs":1000,"startTime":"2024-01-01","lastBusyMs":500}]}`
	srv := stubServer(t, 200, resp, nil, nil, nil)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	rresp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: sch, Raw: st.Raw}}, rresp)

	if rresp.Diagnostics.HasError() {
		t.Fatalf("Read errored: %v", rresp.Diagnostics.Errors())
	}
	var out EndpointModel
	if d := rresp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read state: %v", d)
	}
	if out.ComputeType.ValueString() != "GPU" {
		t.Errorf("ComputeType = %q, want GPU", out.ComputeType.ValueString())
	}
	if out.Workers.IsNull() || len(out.Workers.Elements()) != 1 {
		t.Errorf("Workers = %v, want 1 element", out.Workers)
	}
	if out.Env.IsNull() || out.Env.Elements()["MY_VAR"] == nil {
		t.Errorf("Env = %v, want MY_VAR populated", out.Env)
	}
}

// TestEndpointRead_404_RemovesState asserts CE-1654 fix for endpoint:
// when an endpoint is gone (404), Read must call resp.State.RemoveResource
// so the deleted endpoint is removed from state and planned for recreation.
func TestEndpointRead_404_RemovesState(t *testing.T) {
	ctx := context.Background()
	m := newBaseModel()
	m.Id = types.StringValue("endpoint-gone")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	sch := EndpointResourceSchema(ctx)
	state := tfsdk.State{Schema: sch}
	if d := state.Set(ctx, &m); d.HasError() {
		t.Fatalf("build state: %v", d)
	}
	resp := &resource.ReadResponse{State: state}
	(&EndpointResource{}).Read(ctx, resource.ReadRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("404 should not error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("state was not removed on 404 — CE-1654: deleted endpoint should be removed from state")
	}
}

// TestEndpointCreate_FlashbootTrue verifies flashboot=true is sent in the request body
func TestEndpointCreate_FlashbootTrue(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.TemplateId = types.StringValue("tmpl-123")
	m.Name = types.StringValue("my-ep")
	m.Flashboot = types.BoolValue(true)

	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config: %v", d)
	}
	cfg := tfsdk.Config{Schema: sch, Raw: st.Raw}

	var captured map[string]interface{}
	resp := `{"id":"ep-1","templateId":"tmpl-123","name":"my-ep","flashboot":true}`
	srv := stubServer(t, 200, resp, &captured, nil, nil)
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	cresp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, cresp)

	if cresp.Diagnostics.HasError() {
		t.Fatalf("Create returned diagnostics error: %v", cresp.Diagnostics.Errors())
	}

	if captured["flashboot"] != true {
		t.Errorf("request body flashboot = %v, want true", captured["flashboot"])
	}

	var out EndpointModel
	if d := cresp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read result state: %v", d)
	}
	if !out.Flashboot.ValueBool() {
		t.Errorf("state Flashboot = %v, want true", out.Flashboot.ValueBool())
	}
}

// TestEndpointCreate_FlashbootFalse verifies flashboot=false is sent in the request body
func TestEndpointCreate_FlashbootFalse(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.TemplateId = types.StringValue("tmpl-123")
	m.Name = types.StringValue("my-ep")
	m.Flashboot = types.BoolValue(false)

	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config: %v", d)
	}
	cfg := tfsdk.Config{Schema: sch, Raw: st.Raw}

	var captured map[string]interface{}
	resp := `{"id":"ep-1","templateId":"tmpl-123","name":"my-ep","flashboot":false}`
	srv := stubServer(t, 200, resp, &captured, nil, nil)
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	cresp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, cresp)

	if cresp.Diagnostics.HasError() {
		t.Fatalf("Create returned diagnostics error: %v", cresp.Diagnostics.Errors())
	}

	if captured["flashboot"] != false {
		t.Errorf("request body flashboot = %v, want false", captured["flashboot"])
	}

	var out EndpointModel
	if d := cresp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read result state: %v", d)
	}
	if out.Flashboot.ValueBool() {
		t.Errorf("state Flashboot = %v, want false", out.Flashboot.ValueBool())
	}
}

// TestEndpointCreate_FlashbootNull verifies flashboot is not sent when null
func TestEndpointCreate_FlashbootNull(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.TemplateId = types.StringValue("tmpl-123")
	m.Name = types.StringValue("my-ep")
	// Flashboot left as null (default)

	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config: %v", d)
	}
	cfg := tfsdk.Config{Schema: sch, Raw: st.Raw}

	var captured map[string]interface{}
	resp := `{"id":"ep-1","templateId":"tmpl-123","name":"my-ep"}`
	srv := stubServer(t, 200, resp, &captured, nil, nil)
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	cresp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Create(ctx, resource.CreateRequest{Config: cfg}, cresp)

	if cresp.Diagnostics.HasError() {
		t.Fatalf("Create returned diagnostics error: %v", cresp.Diagnostics.Errors())
	}

	if _, ok := captured["flashboot"]; ok {
		t.Errorf("request body should not include flashboot when null, got %v", captured["flashboot"])
	}

	var out EndpointModel
	if d := cresp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read result state: %v", d)
	}
	if !out.Flashboot.IsNull() {
		t.Errorf("state Flashboot should be null, got %v", out.Flashboot.ValueBool())
	}
}

// TestEndpointRead_Flashboot verifies flashboot is parsed from API response
func TestEndpointRead_Flashboot(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.Id = types.StringValue("ep-1")
	m.TemplateId = types.StringValue("tmpl-123")

	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build state: %v", d)
	}
	reqState := tfsdk.State{Schema: sch, Raw: st.Raw}

	var gotMethod, gotPath string
	resp := `{"id":"ep-1","templateId":"tmpl-123","name":"my-ep","flashboot":true}`
	srv := stubServer(t, 200, resp, nil, &gotMethod, &gotPath)
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	rresp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Read(ctx, resource.ReadRequest{State: reqState}, rresp)

	if rresp.Diagnostics.HasError() {
		t.Fatalf("Read returned diagnostics error: %v", rresp.Diagnostics.Errors())
	}

	var out EndpointModel
	if d := rresp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read result state: %v", d)
	}
	if !out.Flashboot.ValueBool() {
		t.Errorf("state Flashboot = %v, want true", out.Flashboot.ValueBool())
	}
}

// TestEndpointRead_FlashbootFalse verifies flashboot=false is parsed correctly
func TestEndpointRead_FlashbootFalse(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.Id = types.StringValue("ep-1")
	m.TemplateId = types.StringValue("tmpl-123")

	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build state: %v", d)
	}
	reqState := tfsdk.State{Schema: sch, Raw: st.Raw}

	var gotMethod, gotPath string
	resp := `{"id":"ep-1","templateId":"tmpl-123","name":"my-ep","flashboot":false}`
	srv := stubServer(t, 200, resp, nil, &gotMethod, &gotPath)
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	rresp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Read(ctx, resource.ReadRequest{State: reqState}, rresp)

	if rresp.Diagnostics.HasError() {
		t.Fatalf("Read returned diagnostics error: %v", rresp.Diagnostics.Errors())
	}

	var out EndpointModel
	if d := rresp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read result state: %v", d)
	}
	if out.Flashboot.ValueBool() {
		t.Errorf("state Flashboot = %v, want false", out.Flashboot.ValueBool())
	}
}

// TestEndpointRead_FlashbootMissing verifies null when flashboot not in response
func TestEndpointRead_FlashbootMissing(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.Id = types.StringValue("ep-1")
	m.TemplateId = types.StringValue("tmpl-123")

	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build state: %v", d)
	}
	reqState := tfsdk.State{Schema: sch, Raw: st.Raw}

	var gotMethod, gotPath string
	resp := `{"id":"ep-1","templateId":"tmpl-123","name":"my-ep"}`
	srv := stubServer(t, 200, resp, nil, &gotMethod, &gotPath)
	defer srv.Close()

	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	rresp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Read(ctx, resource.ReadRequest{State: reqState}, rresp)

	if rresp.Diagnostics.HasError() {
		t.Fatalf("Read returned diagnostics error: %v", rresp.Diagnostics.Errors())
	}

	var out EndpointModel
	if d := rresp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read result state: %v", d)
	}
	if !out.Flashboot.IsNull() {
		t.Errorf("state Flashboot should be null, got %v", out.Flashboot.ValueBool())
	}
}

// TestEndpointUpdate_Flashboot verifies flashboot update in PATCH request
func TestEndpointUpdate_Flashboot(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	prior := newBaseModel()
	prior.Id = types.StringValue("ep-1")
	prior.Name = types.StringValue("my-ep")
	prior.Flashboot = types.BoolValue(false)

	desired := newBaseModel()
	desired.Id = types.StringValue("ep-1")
	desired.Name = types.StringValue("my-ep")
	desired.Flashboot = types.BoolValue(true)

	priorSt := tfsdk.State{Schema: sch}
	if d := priorSt.Set(ctx, &prior); d.HasError() {
		t.Fatalf("build prior state: %v", d)
	}
	desiredSt := tfsdk.State{Schema: sch}
	if d := desiredSt.Set(ctx, &desired); d.HasError() {
		t.Fatalf("build desired: %v", d)
	}

	var body map[string]interface{}
	var method, path string
	srv := stubServer(t, 200, `{"id":"ep-1","name":"my-ep","flashboot":true}`, &body, &method, &path)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	uresp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: sch, Raw: desiredSt.Raw},
		State:  priorSt,
	}, uresp)

	if uresp.Diagnostics.HasError() {
		t.Fatalf("Update errored: %v", uresp.Diagnostics.Errors())
	}
	if method != "PATCH" || path != "/v2/serverless/ep-1" {
		t.Errorf("expected PATCH /v2/serverless/ep-1, got %s %s", method, path)
	}
	if body["flashboot"] != true {
		t.Errorf("PATCH body flashboot = %v, want true", body["flashboot"])
	}

	var out EndpointModel
	if d := uresp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read result state: %v", d)
	}
	if !out.Flashboot.ValueBool() {
		t.Errorf("state Flashboot = %v, want true", out.Flashboot.ValueBool())
	}
}
