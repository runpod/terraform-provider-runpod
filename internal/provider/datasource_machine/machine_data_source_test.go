package datasource_machine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Regression test for the systemic GraphQL double-unwrap (CE-1652), fixed by
// PR #20: rlClient.Query() now returns the inner GraphQL "data" map, so callers
// read result["machine"] directly. This asserts the FIXED behavior — Read
// decodes the config input first (so we supply a valid config with Id=m1),
// then populates state from every dereferenced machine field without error.
func TestMachineDataSourceRead_PopulatesState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"machine":{
			"id":"m1",
			"name":"machine-one",
			"location":"US-CA",
			"listed":true,
			"gpuType":{"id":"g1","displayName":"NVIDIA A100"},
			"gpuTotal":8,
			"gpuReserved":2,
			"cpuCount":64,
			"cpuTypeId":"epyc-7763",
			"memoryTotal":512,
			"memoryReserved":128,
			"diskTotal":4096,
			"diskReserved":1024,
			"secureCloud":true,
			"maintenanceMode":false,
			"verified":true,
			"hostPricePerGpu":1.5,
			"runpodIp":"10.0.0.1"
		}}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	ctx := context.Background()
	sch := MachineDataSourceSchema(ctx)
	cfgState := tfsdk.State{Schema: sch}
	if d := cfgState.Set(ctx, &MachineModel{Id: types.StringValue("m1")}); d.HasError() {
		t.Fatalf("building config: %v", d)
	}

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&MachineDataSource{}).Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: sch, Raw: cfgState.Raw}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected Read to succeed after CE-1652 fix (PR #20), got: %v", resp.Diagnostics)
	}

	var model MachineModel
	if d := resp.State.Get(ctx, &model); d.HasError() {
		t.Fatalf("reading state: %v", d)
	}
	if got := model.Name.ValueString(); got != "machine-one" {
		t.Errorf("expected name %q, got %q", "machine-one", got)
	}
}
