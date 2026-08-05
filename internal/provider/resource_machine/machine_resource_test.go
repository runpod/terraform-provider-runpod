package resource_machine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func machineConfig(t *testing.T, m MachineModel) tfsdk.Config {
	t.Helper()
	ctx := context.Background()
	sch := MachineResourceSchema(ctx)
	st := tfsdk.State{Schema: sch}
	if diags := st.Set(ctx, &m); diags.HasError() {
		t.Fatalf("building config: %v", diags)
	}
	return tfsdk.Config{Schema: sch, Raw: st.Raw}
}

// TestMachineCreate_SetsID is a regression test for the CE-1652 fix (PR #20).
// rlClient.Query() now returns the inner GraphQL `data` map directly, so Create
// reads result["machineAdd"]["id"] and sets config.Id. A valid GraphQL
// response must now succeed and populate the state Id.
func TestMachineCreate_SetsID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"machineAdd":{"id":"m1","name":"n"}}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	m := MachineModel{
		Name:      types.StringValue("my-machine"),
		GpuCount:  types.Int64Value(1),
		GpuTypeId: types.StringValue("NVIDIA A100"),
	}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: MachineResourceSchema(context.Background())}}
	(&MachineResource{}).Create(context.Background(), resource.CreateRequest{Config: machineConfig(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	var got MachineModel
	if d := resp.State.Get(context.Background(), &got); d.HasError() {
		t.Fatalf("reading state: %v", d)
	}
	if got.Id.ValueString() != "m1" {
		t.Errorf("expected Id == m1, got %q", got.Id.ValueString())
	}
}

// TestMachineRead_PopulatesState is a regression test for the CE-1652 fix (PR #20).
// rlClient.Query() now returns the inner GraphQL `data` map directly, so Read
// reads result["machine"] and populates the state. The stub must supply every
// dereferenced field (name, gpuCount, gpuType, cpuCount, memoryInGb,
// diskSizeInGb, region, listed, secureCloud, maintenanceMode, verified,
// hostPricePerGpu) because Read does unchecked type assertions on each.
func TestMachineRead_PopulatesState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"machine":{"name":"n","gpuType":{"id":"A100","displayName":"NVIDIA A100"},"cpuCount":8,"gpuTotal":1,"memoryTotal":64,"diskTotal":100,"location":"EU","listed":true,"secureCloud":true,"maintenanceMode":false,"verified":true,"hostPricePerGpu":1.5}}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	m := MachineModel{Id: types.StringValue("m1")}
	sch := MachineResourceSchema(context.Background())
	state := tfsdk.State{Schema: sch}
	if d := state.Set(context.Background(), &m); d.HasError() {
		t.Fatalf("building state: %v", d)
	}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&MachineResource{}).Read(context.Background(), resource.ReadRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	var got MachineModel
	if d := resp.State.Get(context.Background(), &got); d.HasError() {
		t.Fatalf("reading state: %v", d)
	}
	if got.Name.ValueString() != "n" {
		t.Errorf("expected name == n, got %q", got.Name.ValueString())
	}
}

// TestMachineUpdate_UsesConfiguredEndpoint verifies the CE-1651 fix: Update must
// POST to RUNPOD_GRAPHQL_URL, not the hardcoded prod URL. Update ignores the
// response body (it only checks for an error), so a valid GraphQL 200 succeeds.
// If the endpoint were still hardcoded, the test server would never be hit.
func TestMachineUpdate_UsesConfiguredEndpoint(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		_, _ = w.Write([]byte(`{"data":{"machineEditName":{"id":"m1"}}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	m := MachineModel{
		Id:        types.StringValue("m1"),
		Name:      types.StringValue("n"),
		GpuCount:  types.Int64Value(1),
		GpuTypeId: types.StringValue("A100"),
	}
	sch := MachineResourceSchema(context.Background())
	plan := tfsdk.Plan{Schema: sch}
	if d := plan.Set(context.Background(), &m); d.HasError() {
		t.Fatalf("building plan: %v", d)
	}
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	(&MachineResource{}).Update(context.Background(), resource.UpdateRequest{Plan: plan}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !hit {
		t.Error("Update did not hit RUNPOD_GRAPHQL_URL — CE-1651 regression (endpoint hardcoded)")
	}
}

func TestMachineDelete_UsesConfiguredEndpoint(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		_, _ = w.Write([]byte(`{"data":{"machineDelete":{"id":"m1","status":"DELETED"}}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	m := MachineModel{Id: types.StringValue("m1")}
	sch := MachineResourceSchema(context.Background())
	state := tfsdk.State{Schema: sch}
	if d := state.Set(context.Background(), &m); d.HasError() {
		t.Fatalf("building state: %v", d)
	}
	resp := &resource.DeleteResponse{State: state}
	(&MachineResource{}).Delete(context.Background(), resource.DeleteRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !hit {
		t.Error("Delete did not hit RUNPOD_GRAPHQL_URL — CE-1651 regression (endpoint hardcoded)")
	}
}
