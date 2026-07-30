package resource_endpoint

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestEndpointRead_FullFieldMapping exercises Read's scalar + list mapping
// branches with a fully-populated response.
func TestEndpointRead_FullFieldMapping(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)
	body := `{
		"id":"ep-1","name":"ep","templateId":"t-1","userId":"u-1","computeType":"GPU",
		"gpuCount":2,"vcpuCount":4,"workersMin":1,"workersMax":5,"idleTimeout":30,
		"scalerType":"QUEUE_DELAY","scalerValue":4,"executionTimeoutMs":600000,
		"createdAt":"2024-01-01T00:00:00Z","version":3,
		"networkVolumeId":"nv-1","networkVolumeIds":["nv-1","nv-2"],
		"dataCenterIds":["US-CA-1"],"env":{"K":"V"},
		"workers":[{"id":"w-1","podId":"p-1","status":"RUNNING","uptimeMs":1000,"startTime":"2024-01-01","lastBusyMs":500}]
	}`
	srv := stubServer(t, 200, body, nil, nil, nil)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := newBaseModel()
	m.Id = types.StringValue("ep-1")
	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build state: %v", d)
	}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: sch, Raw: st.Raw}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read errored: %v", resp.Diagnostics.Errors())
	}
	var out EndpointModel
	if d := resp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read state: %v", d)
	}
	if out.ComputeType.ValueString() != "GPU" {
		t.Errorf("ComputeType = %q, want GPU", out.ComputeType.ValueString())
	}
	if out.ScalerType.ValueString() != "QUEUE_DELAY" {
		t.Errorf("ScalerType = %q, want QUEUE_DELAY", out.ScalerType.ValueString())
	}
	if out.Version.ValueInt64() != 3 {
		t.Errorf("Version = %d, want 3", out.Version.ValueInt64())
	}
	if out.NetworkVolumeIds.IsNull() || len(out.NetworkVolumeIds.Elements()) != 2 {
		t.Errorf("NetworkVolumeIds = %v, want 2", out.NetworkVolumeIds)
	}
	if out.Workers.IsNull() || len(out.Workers.Elements()) != 1 {
		t.Errorf("Workers = %v, want 1", out.Workers)
	}
}

// TestEndpointCreate_AllScalars exercises Create's scalar body-build branches
// (the list/env branches are covered by TestEndpointCreate_WithListFields).
func TestEndpointCreate_AllScalars(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	m := newBaseModel()
	m.TemplateId = types.StringValue("t-1")
	m.Name = types.StringValue("ep")
	m.ComputeType = types.StringValue("GPU")
	m.GpuCount = types.Int64Value(2)
	m.VcpuCount = types.Int64Value(4)
	m.WorkersMin = types.Int64Value(1)
	m.WorkersMax = types.Int64Value(5)
	m.IdleTimeout = types.Int64Value(30)
	m.ScalerType = types.StringValue("QUEUE_DELAY")
	m.ScalerValue = types.Int64Value(4)
	m.ExecutionTimeoutMs = types.Int64Value(600000)
	m.Flashboot = types.BoolValue(true)
	m.GpuTypePriority = types.StringValue("availability")
	m.CpuFlavorPriority = types.StringValue("cost")
	m.NetworkVolumeId = types.StringValue("nv-1")

	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config: %v", d)
	}

	var body map[string]interface{}
	srv := stubServer(t, 200, `{"id":"ep-1","templateId":"t-1","name":"ep"}`, &body, nil, nil)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Create(ctx, resource.CreateRequest{Config: tfsdk.Config{Schema: sch, Raw: st.Raw}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create errored: %v", resp.Diagnostics.Errors())
	}
	for _, k := range []string{"gpu", "workers", "scaling", "timeout", "flashboot", "networkVolumes", "type"} {
		if _, ok := body[k]; !ok {
			t.Errorf("POST body missing %q; got %v", k, body)
		}
	}
}
