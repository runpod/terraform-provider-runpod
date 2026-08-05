package datasource_pod

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Correct behavior for the pod data source Read: given a valid response, it
// TestPodDataSourceRead_PopulatesState stubs the GraphQL pod endpoint and
// asserts a clean Read. The query uses the fields the live schema actually has:
// status comes from desiredStatus, created_at from lastStatusChange, and
// gpu_type_id from machine.gpuType.id (null-safe while transitioning).
func TestPodDataSourceRead_PopulatesState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"pod":{
			"id":"p1","name":"my-pod","desiredStatus":"RUNNING",
			"imageName":"runpod/base:latest","machineId":"m1","machineType":"NVIDIA",
			"machine":{"gpuType":{"id":"NVIDIA A100 80GB"}},
			"gpuCount":2,"costPerHr":1.89,"memoryInGb":128,
			"volumeInGb":50,"volumeMountPath":"/workspace","volumeKey":"vk1",
			"ports":"8888/http","lastStatusChange":"Rented by User: Mon Jan 1 2024",
			"dockerArgs":"","env":[],"templateId":"t1","containerDiskInGb":20
		}}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	ctx := context.Background()
	sch := PodDataSourceSchema(ctx)
	cfgState := tfsdk.State{Schema: sch}
	if d := cfgState.Set(ctx, &PodModel{Id: types.StringValue("p1"), Env: types.ListNull(types.StringType)}); d.HasError() {
		t.Fatalf("building config: %v", d)
	}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&PodDataSource{}).Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: sch, Raw: cfgState.Raw}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected Read to populate state, got: %v", resp.Diagnostics)
	}
	var m PodModel
	if d := resp.State.Get(ctx, &m); d.HasError() {
		t.Fatalf("reading state: %v", d)
	}
	if m.Name.ValueString() != "my-pod" {
		t.Errorf("state Name = %q, want my-pod", m.Name.ValueString())
	}
	if m.Status.ValueString() != "RUNNING" {
		t.Errorf("state Status = %q, want RUNNING (mapped from desiredStatus)", m.Status.ValueString())
	}
	if m.GpuTypeId.ValueString() != "NVIDIA A100 80GB" {
		t.Errorf("state GpuTypeId = %q, want NVIDIA A100 80GB (mapped from machine.gpuType.id)", m.GpuTypeId.ValueString())
	}
	if m.CreatedAt.ValueString() != "Rented by User: Mon Jan 1 2024" {
		t.Errorf("state CreatedAt = %q, want lastStatusChange value", m.CreatedAt.ValueString())
	}
}
