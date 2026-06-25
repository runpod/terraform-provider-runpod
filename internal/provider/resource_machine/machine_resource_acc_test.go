package resource_machine

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestAccMachineCreate_riab asserts the DESIRED behavior: creating a machine
// against the live GraphQL endpoint returns an id. It FAILS today due to
// CE-1652 (double-unwrap, "data not in response"). Green here == CE-1652
// fixed and the machine create path works end-to-end.
//
// Gated on RIAB_ACC=1 with RUNPOD_API_KEY=$TEST_USER_JWT and
// RUNPOD_GRAPHQL_URL=http://localhost:4000/graphql.
func TestAccMachineCreate_riab(t *testing.T) {
	if os.Getenv("RIAB_ACC") != "1" {
		t.Skip("set RIAB_ACC=1 + RUNPOD_API_KEY + RUNPOD_GRAPHQL_URL to run live riab tests")
	}
	if os.Getenv("RUNPOD_API_KEY") == "" || os.Getenv("RUNPOD_GRAPHQL_URL") == "" {
		t.Fatal("RUNPOD_API_KEY and RUNPOD_GRAPHQL_URL must be set")
	}

	ctx := context.Background()
	m := MachineModel{
		Name:      types.StringValue("tf-acc-machine"),
		GpuCount:  types.Int64Value(1),
		GpuTypeId: types.StringValue("NVIDIA GeForce RTX 4090"),
	}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: MachineResourceSchema(ctx)}}
	(&MachineResource{}).Create(ctx, resource.CreateRequest{Config: machineConfig(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("machine Create failed (RED until CE-1652 is fixed): %v", resp.Diagnostics)
	}
	var created MachineModel
	resp.State.Get(ctx, &created)
	if created.Id.ValueString() == "" {
		t.Fatal("machine Create returned an empty id")
	}
	t.Logf("created machine id=%s", created.Id.ValueString())

	// Clean up once this actually starts passing.
	dResp := &resource.DeleteResponse{State: tfsdk.State{Schema: MachineResourceSchema(ctx)}}
	st := tfsdk.State{Schema: MachineResourceSchema(ctx)}
	_ = st.Set(ctx, &created)
	(&MachineResource{}).Delete(ctx, resource.DeleteRequest{State: st}, dResp)
}
