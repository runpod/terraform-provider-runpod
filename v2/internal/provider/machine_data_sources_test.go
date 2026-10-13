package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// dataSourceSchemaFields is the pair of schema strings the deprecation
// assertions look at, collected through the framework's own Schema call.
type dataSourceSchemaFields struct {
	Description        string
	DeprecationMessage string
}

func dataSourceSchemaOf(t *testing.T, d datasource.DataSource) dataSourceSchemaFields {
	t.Helper()
	var resp datasource.SchemaResponse
	d.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return dataSourceSchemaFields{
		Description:        resp.Schema.Description,
		DeprecationMessage: resp.Schema.DeprecationMessage,
	}
}

func TestFlattenMachineDataSource(t *testing.T) {
	machine := map[string]any{
		"id":              "m-1",
		"name":            "rig-01",
		"location":        "RO",
		"listed":          true,
		"gpuType":         map[string]any{"id": "NVIDIA GeForce RTX 4090", "displayName": "RTX 4090"},
		"gpuTotal":        float64(8),
		"gpuReserved":     float64(3),
		"cpuCount":        float64(32),
		"cpuTypeId":       "cpu3c",
		"memoryTotal":     float64(256),
		"memoryReserved":  float64(96),
		"diskTotal":       float64(2048),
		"diskReserved":    float64(512),
		"secureCloud":     true,
		"maintenanceMode": false,
		"verified":        true,
		"hostPricePerGpu": 0.34,
		"runpodIp":        "10.0.0.7",
	}

	model := flattenMachineDataSource(machine, types.StringValue("m-1"))

	if model.Id.ValueString() != "m-1" || model.Name.ValueString() != "rig-01" {
		t.Errorf("id/name: %+v", model)
	}
	if model.GpuTypeId.ValueString() != "NVIDIA GeForce RTX 4090" {
		t.Errorf("gpu_type_id = %q", model.GpuTypeId.ValueString())
	}
	if model.GpuTotal.ValueInt64() != 8 || model.GpuReserved.ValueInt64() != 3 {
		t.Errorf("gpu totals: %+v", model)
	}
	if model.MemoryTotal.ValueInt64() != 256 || model.MemoryReserved.ValueInt64() != 96 {
		t.Errorf("memory totals: %+v", model)
	}
	if model.DiskTotal.ValueInt64() != 2048 || model.DiskReserved.ValueInt64() != 512 {
		t.Errorf("disk totals: %+v", model)
	}
	if model.CpuCount.ValueInt64() != 32 || model.CpuTypeId.ValueString() != "cpu3c" {
		t.Errorf("cpu: %+v", model)
	}
	if !model.Listed.ValueBool() || !model.SecureCloud.ValueBool() ||
		model.MaintenanceMode.ValueBool() || !model.Verified.ValueBool() {
		t.Errorf("flags: %+v", model)
	}
	if model.HostPricePerGpu.ValueFloat64() < 0.33 || model.HostPricePerGpu.ValueFloat64() > 0.35 {
		t.Errorf("host_price_per_gpu = %v, want ~0.34", model.HostPricePerGpu.ValueFloat64())
	}
	if model.Location.ValueString() != "RO" || model.RunpodIp.ValueString() != "10.0.0.7" {
		t.Errorf("location/runpod_ip: %+v", model)
	}
	// The ported GetMachine query does not select dataCenterId.
	if !model.DataCenterId.IsNull() {
		t.Errorf("data_center_id = %v, want null", model.DataCenterId)
	}
}

func TestFlattenMachineDataSourceNullsAbsentFields(t *testing.T) {
	model := flattenMachineDataSource(map[string]any{"id": "m-1"}, types.StringValue("m-1"))

	if model.Id.ValueString() != "m-1" {
		t.Errorf("id = %v", model.Id)
	}
	if !model.Name.IsNull() || !model.Location.IsNull() || !model.Listed.IsNull() ||
		!model.GpuTypeId.IsNull() || !model.GpuTotal.IsNull() || !model.CpuTypeId.IsNull() ||
		!model.HostPricePerGpu.IsNull() || !model.RunpodIp.IsNull() {
		t.Errorf("absent fields should read null rather than fail the read: %+v", model)
	}
}

func TestFlattenMachines(t *testing.T) {
	machines := []any{
		map[string]any{
			"id":           "m-1",
			"name":         "rig-01",
			"location":     "RO",
			"listed":       true,
			"gpuType":      map[string]any{"id": "NVIDIA GeForce RTX 4090"},
			"gpuTotal":     float64(8),
			"secureCloud":  true,
			"dataCenterId": "EU-RO-1",
		},
		// No gpuType and no dataCenterId: a community machine with neither.
		map[string]any{
			"id":          "m-2",
			"name":        "rig-02",
			"gpuTotal":    float64(0),
			"secureCloud": false,
		},
		"not-an-object",
	}

	state := flattenMachines(machines)

	if len(state.Machines) != 2 {
		t.Fatalf("machines = %d, want the two objects", len(state.Machines))
	}
	first := state.Machines[0]
	if first.Id.ValueString() != "m-1" || first.Name.ValueString() != "rig-01" ||
		first.Location.ValueString() != "RO" || !first.Listed.ValueBool() ||
		first.GpuTypeId.ValueString() != "NVIDIA GeForce RTX 4090" ||
		first.GpuTotal.ValueInt64() != 8 || !first.SecureCloud.ValueBool() ||
		first.DataCenterId.ValueString() != "EU-RO-1" {
		t.Errorf("first machine: %+v", first)
	}
	second := state.Machines[1]
	if second.Id.ValueString() != "m-2" || second.GpuTotal.ValueInt64() != 0 || second.SecureCloud.ValueBool() {
		t.Errorf("second machine: %+v", second)
	}
	if !second.GpuTypeId.IsNull() || !second.DataCenterId.IsNull() || !second.Listed.IsNull() {
		t.Errorf("second machine should carry nulls for the absent fields: %+v", second)
	}
}

func TestFlattenMachinesEmptyListIsNotNull(t *testing.T) {
	state := flattenMachines([]any{})
	if state.Machines == nil {
		t.Fatal("machines is nil, want an empty list so the attribute reads []")
	}
	if len(state.Machines) != 0 {
		t.Fatalf("machines = %d, want 0", len(state.Machines))
	}
}

func TestMachineDataSourceFailsOnANullId(t *testing.T) {
	ctx := context.Background()

	// An unknown id comes back as a machine object whose fields are all null;
	// reading that as success would hand the configuration an empty machine.
	d := &machineDataSource{client: graphQLStub(t, func(string, map[string]any) string {
		return `{"data":{"machine":{"id":null,"name":null,"location":null,"listed":null}}}`
	})}

	var schemaResp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type = %T, want an object", schemaResp.Schema.Type().TerraformType(ctx))
	}
	values := map[string]tftypes.Value{}
	for name, attributeType := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	values["id"] = tftypes.NewValue(tftypes.String, "m-missing")

	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, nil)}}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, values)},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("a lookup answered with a null id must fail")
	}
}

func TestMachinesDataSourceReadsTheAccountsMachines(t *testing.T) {
	ctx := context.Background()

	// The root machines field is the public supply and only answers support
	// and host-daemon callers; an account's own machines are myself.machines.
	var gotQuery string
	d := &machinesDataSource{client: graphQLStub(t, func(query string, _ map[string]any) string {
		gotQuery = query
		return `{"data":{"myself":{"machines":[{"id":"m-1","name":"rig-01","location":"RO","listed":true,` +
			`"gpuType":{"id":"NVIDIA GeForce RTX 4090","displayName":"RTX 4090"},"gpuTotal":8,` +
			`"secureCloud":true,"dataCenterId":"EU-RO-1"}]}}}`
	})}

	var state machinesDataSourceModel
	readDataSource(ctx, t, d, nil, &state)

	if !strings.Contains(gotQuery, "myself {") {
		t.Errorf("document = %s, want myself.machines", gotQuery)
	}
	if len(state.Machines) != 1 || state.Machines[0].Id.ValueString() != "m-1" ||
		state.Machines[0].GpuTotal.ValueInt64() != 8 {
		t.Fatalf("machines = %+v", state.Machines)
	}
}
