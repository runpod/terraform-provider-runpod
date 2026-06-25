package resource_machine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestMachineCreate_DoubleUnwrap_R1 is a characterization test for bug R1.
// Same root cause as pod_action: client.Query() returns the inner `data`, but
// MachineResource.Create does result["data"].(map) again (always nil), so even
// a valid GraphQL response fails with "data not in response".
//
// When R1 is fixed, Create will read machineCreate.id and set Id — flip this
// test to assert that.
func TestMachineCreate_DoubleUnwrap_R1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"machineCreate":{"id":"m1","name":"n"}}}`))
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

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected R1 double-unwrap failure; if Create now succeeds, R1 is FIXED — flip to assert Id == m1")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "data not in response") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'data not in response' (R1), got: %v", resp.Diagnostics)
	}
}

// TestMachineRead_DoubleUnwrap_R1 confirms R1 in Read, and documents that R6
// (unchecked type assertions like machine["name"].(string)) is currently
// MASKED by R1: the result["data"] double-unwrap errors out before any
// assertion runs. Fixing R1 without also hardening these assertions will turn
// this error into a panic on a null/missing field.
func TestMachineRead_DoubleUnwrap_R1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"machine":{"name":"n","gpuCount":1,"gpuType":"A100","cpuCount":8,"memoryInGb":64,"diskSizeInGb":100,"region":"EU","listed":true,"secureCloud":true,"maintenanceMode":false,"verified":true,"hostPricePerGpu":1.5}}}`))
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

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected R1 double-unwrap failure in Read; if it now succeeds, R1 is FIXED (watch for R6 panics)")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "Failed to get data from response") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Failed to get data from response' (R1), got: %v", resp.Diagnostics)
	}
}

// TestMachineUpdate_UsesConfiguredEndpoint_R7 verifies the R7 fix: Update must
// POST to RUNPOD_GRAPHQL_URL, not the hardcoded prod URL. Update ignores the
// response body (it only checks for an error), so a valid GraphQL 200 succeeds.
// If the endpoint were still hardcoded, the test server would never be hit.
func TestMachineUpdate_UsesConfiguredEndpoint_R7(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		_, _ = w.Write([]byte(`{"data":{"machineEdit":{"id":"m1"}}}`))
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
		t.Error("Update did not hit RUNPOD_GRAPHQL_URL — R7 regression (endpoint hardcoded)")
	}
}

func TestMachineDelete_UsesConfiguredEndpoint_R7(t *testing.T) {
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
		t.Error("Delete did not hit RUNPOD_GRAPHQL_URL — R7 regression (endpoint hardcoded)")
	}
}
