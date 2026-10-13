package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
)

// readQueryResponse is the machine object the GetMachine query in
// machine_resource.go selects.
func readQueryResponse() map[string]any {
	return map[string]any{
		"id":              "m-1",
		"name":            "rig-01",
		"gpuType":         map[string]any{"id": "NVIDIA GeForce RTX 4090", "displayName": "RTX 4090"},
		"cpuCount":        float64(32),
		"gpuTotal":        float64(8),
		"memoryTotal":     float64(256),
		"diskTotal":       float64(2048),
		"listed":          true,
		"secureCloud":     false,
		"location":        "RO",
		"maintenanceMode": false,
		"verified":        true,
		"hostPricePerGpu": 0.34,
		"dataCenterId":    "EU-RO-1",
		"runpodIp":        "10.0.0.7",
	}
}

func TestFlattenMachineResourceReadResponse(t *testing.T) {
	model := machineResourceModel{}
	flattenMachineResource(readQueryResponse(), &model)

	if model.Id.ValueString() != "m-1" || model.Name.ValueString() != "rig-01" {
		t.Errorf("id/name: %+v", model)
	}
	if model.GpuTypeId.ValueString() != "NVIDIA GeForce RTX 4090" {
		t.Errorf("gpu_type_id = %q, want the nested gpuType id", model.GpuTypeId.ValueString())
	}
	if model.GpuCount.ValueInt64() != 8 || model.CpuCount.ValueInt64() != 32 {
		t.Errorf("gpu/cpu count: %+v", model)
	}
	if model.MemoryInGb.ValueInt64() != 256 || model.DiskInGb.ValueInt64() != 2048 {
		t.Errorf("memory/disk: %+v", model)
	}
	if !model.Listed.ValueBool() || model.SecureCloud.ValueBool() {
		t.Errorf("listed/secure_cloud: %+v", model)
	}
	if model.Location.ValueString() != "RO" || model.DataCenterId.ValueString() != "EU-RO-1" {
		t.Errorf("location/data_center_id: %+v", model)
	}
	if model.MaintenanceMode.ValueBool() || !model.Verified.ValueBool() {
		t.Errorf("maintenance_mode/verified: %+v", model)
	}
	if model.HostPricePerGpu.ValueFloat64() < 0.33 || model.HostPricePerGpu.ValueFloat64() > 0.35 {
		t.Errorf("host_price_per_gpu = %v, want ~0.34", model.HostPricePerGpu.ValueFloat64())
	}
	if model.RunpodIp.ValueString() != "10.0.0.7" {
		t.Errorf("runpod_ip = %q", model.RunpodIp.ValueString())
	}
	// No GraphQL document selects a bid price, so it stays null here.
	if !model.BidPricePerGpu.IsNull() {
		t.Errorf("bid_price_per_gpu = %v, want null", model.BidPricePerGpu)
	}
}

func TestFlattenMachineResourceKeepsPlannedValuesTheResponseOmits(t *testing.T) {
	// The CreateMachine mutation selects fewer fields than the read query, and
	// EditMachineName returns only id and name. A planned value the response
	// cannot confirm (a configured bid_price_per_gpu, or a computed attribute
	// carrying prior state through an update) has to survive, or Terraform
	// reports an inconsistent result after apply.
	createResponse := map[string]any{
		"id":          "m-1",
		"name":        "rig-01",
		"cpuCount":    float64(32),
		"gpuTotal":    float64(8),
		"memoryTotal": float64(256),
		"diskTotal":   float64(2048),
		"listed":      true,
		"secureCloud": false,
		"location":    "RO",
	}

	plan := machineResourceModel{
		Id:              types.StringUnknown(),
		BidPricePerGpu:  types.Float64Value(0.25),
		DataCenterId:    types.StringValue("EU-RO-1"),
		HostPricePerGpu: types.Float64Unknown(),
		MaintenanceMode: types.BoolUnknown(),
		Verified:        types.BoolUnknown(),
		RunpodIp:        types.StringUnknown(),
		GpuTypeId:       types.StringValue("NVIDIA GeForce RTX 4090"),
	}
	flattenMachineResource(createResponse, &plan)

	if plan.BidPricePerGpu.ValueFloat64() != 0.25 {
		t.Errorf("bid_price_per_gpu = %v, want the configured 0.25", plan.BidPricePerGpu)
	}
	if plan.DataCenterId.ValueString() != "EU-RO-1" {
		t.Errorf("data_center_id = %v, want the prior state value", plan.DataCenterId)
	}
	if plan.GpuTypeId.ValueString() != "NVIDIA GeForce RTX 4090" {
		t.Errorf("gpu_type_id = %v, want the prior state value when gpuType is absent", plan.GpuTypeId)
	}
	// Unknown is not a value state can hold: those become null.
	if !plan.HostPricePerGpu.IsNull() || !plan.MaintenanceMode.IsNull() ||
		!plan.Verified.IsNull() || !plan.RunpodIp.IsNull() {
		t.Errorf("attributes left unknown by the plan should read null: %+v", plan)
	}
	if plan.Id.ValueString() != "m-1" {
		t.Errorf("id = %v, want the created id", plan.Id)
	}
}

func TestFlattenMachineResourceIgnoresWronglyTypedFields(t *testing.T) {
	model := machineResourceModel{}
	flattenMachineResource(map[string]any{
		"id":       "m-1",
		"gpuTotal": "eight",          // a string where a number is expected
		"gpuType":  "RTX 4090",       // a scalar where an object is expected
		"listed":   float64(1),       // a number where a bool is expected
		"location": map[string]any{}, // an object where a string is expected
	}, &model)

	if model.Id.ValueString() != "m-1" {
		t.Errorf("id = %v", model.Id)
	}
	if !model.GpuCount.IsNull() || !model.GpuTypeId.IsNull() || !model.Listed.IsNull() || !model.Location.IsNull() {
		t.Errorf("unexpectedly typed fields should read null: %+v", model)
	}
}

func TestMachineResourceSettableAttributes(t *testing.T) {
	// The create mutation accepts only a name and no mutation accepts the
	// rest, so anything else offered as Optional would be a permanent diff:
	// the read writes what the API reports, the plan writes the configuration
	// back, and nothing reconciles them.
	settable := map[string]bool{"name": true, "bid_price_per_gpu": true}

	var schemaResp resource.SchemaResponse
	NewMachineResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatal(schemaResp.Diagnostics)
	}

	for name, attribute := range schemaResp.Schema.Attributes {
		if attribute.IsOptional() != settable[name] {
			t.Errorf("%s: optional = %v, want %v", name, attribute.IsOptional(), settable[name])
		}
		if attribute.IsRequired() {
			t.Errorf("%s: required, but the API assigns every machine attribute", name)
		}
		if !attribute.IsComputed() {
			t.Errorf("%s: not computed, so the API's value could not be stored", name)
		}
	}
}

func TestMachineResourceAndDataSourcesAreDeprecated(t *testing.T) {
	ctx := context.Background()

	var schemaResp resource.SchemaResponse
	NewMachineResource().Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Schema.DeprecationMessage != machineDeprecation {
		t.Errorf("runpod_machine resource deprecation = %q", schemaResp.Schema.DeprecationMessage)
	}
	if !strings.Contains(schemaResp.Schema.Description, machineDeprecation) {
		t.Errorf("runpod_machine resource description does not repeat the deprecation: %q", schemaResp.Schema.Description)
	}

	for name, schema := range map[string]dataSourceSchemaFields{
		"runpod_machine":  dataSourceSchemaOf(t, NewMachineDataSource()),
		"runpod_machines": dataSourceSchemaOf(t, NewMachinesDataSource()),
	} {
		if schema.DeprecationMessage != machineDeprecation {
			t.Errorf("%s data source deprecation = %q", name, schema.DeprecationMessage)
		}
		if !strings.Contains(schema.Description, machineDeprecation) {
			t.Errorf("%s data source description does not repeat the deprecation: %q", name, schema.Description)
		}
	}
}

// graphQLStub answers each GraphQL request with what answer returns for its
// document and variables. The documents are checked against the backend
// schema's shapes here, because a mismatch only surfaces as a GraphQL error
// against the live host.
func graphQLStub(t *testing.T, answer func(query string, variables map[string]any) string) *legacy.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decoding the GraphQL request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(answer(request.Query, request.Variables)))
	}))
	t.Cleanup(srv.Close)
	return newTestLegacyClient(t, legacy.Config{APIKey: "test-key", GraphQLURL: srv.URL})
}

func machineState(ctx context.Context, t *testing.T, r resource.Resource, name string) (resourceschema.Schema, tftypes.Value) {
	t.Helper()
	s, raw := shimResourceSchema(ctx, t, r)
	return s, shimObject(ctx, t, s, raw, map[string]attr.Value{
		"id":   types.StringValue("m-1"),
		"name": types.StringValue(name),
	})
}

func TestMachineUpdateSendsMachineId(t *testing.T) {
	ctx := context.Background()

	// MachineEditNameInput is { machineId: String!, name: String! }; an input
	// keyed id fails validation before the resolver runs.
	var gotInput map[string]any
	r := &machineResource{client: graphQLStub(t, func(query string, variables map[string]any) string {
		if !strings.Contains(query, "machineEditName(input: $input)") {
			t.Errorf("unexpected document: %s", query)
		}
		gotInput, _ = variables["input"].(map[string]any)
		return `{"data":{"machineEditName":{"id":"m-1","name":"rig-02"}}}`
	})}
	s, state := machineState(ctx, t, r, "rig-01")
	_, plan := machineState(ctx, t, r, "rig-02")

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: plan},
		State:  tfsdk.State{Schema: s, Raw: state},
		Config: tfsdk.Config{Schema: s, Raw: plan},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if gotInput["machineId"] != "m-1" || gotInput["name"] != "rig-02" {
		t.Errorf("input = %v, want machineId and name", gotInput)
	}
	if _, ok := gotInput["id"]; ok {
		t.Errorf("input = %v, carries an id key the schema does not declare", gotInput)
	}
}

func TestMachineUpdateRejectsANullId(t *testing.T) {
	ctx := context.Background()

	r := &machineResource{client: graphQLStub(t, func(string, map[string]any) string {
		return `{"data":{"machineEditName":{"id":null,"name":null}}}`
	})}
	s, state := machineState(ctx, t, r, "rig-01")
	_, plan := machineState(ctx, t, r, "rig-02")

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: plan},
		State:  tfsdk.State{Schema: s, Raw: state},
		Config: tfsdk.Config{Schema: s, Raw: plan},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("a rename answered with no machine id must fail")
	}
}

func TestMachineDeleteSendsAnInputAndExpectsVoid(t *testing.T) {
	ctx := context.Background()

	// machineDelete(input: MachineDeleteInput): Void. A Void field cannot take
	// a selection set, and the machine id rides inside the input object.
	var gotQuery string
	var gotInput map[string]any
	r := &machineResource{client: graphQLStub(t, func(query string, variables map[string]any) string {
		gotQuery = query
		gotInput, _ = variables["input"].(map[string]any)
		return `{"data":{"machineDelete":null}}`
	})}
	s, state := machineState(ctx, t, r, "rig-01")

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: state}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !strings.Contains(gotQuery, "($input: MachineDeleteInput!)") || !strings.Contains(gotQuery, "machineDelete(input: $input)") {
		t.Errorf("document = %s, want machineDelete(input: $input)", gotQuery)
	}
	if strings.Contains(gotQuery, "machineDelete(input: $input) {") {
		t.Errorf("document = %s, selects fields on a Void result", gotQuery)
	}
	if gotInput["machineId"] != "m-1" {
		t.Errorf("input = %v, want machineId m-1", gotInput)
	}
}

func TestMachineReadRemovesAMachineAnsweredWithANullId(t *testing.T) {
	ctx := context.Background()

	// getMachineById answers a machine it cannot find, or one the caller does
	// not own, with an empty object, which GraphQL serializes as null fields.
	r := &machineResource{client: graphQLStub(t, func(string, map[string]any) string {
		return `{"data":{"machine":{"id":null,"name":null,"gpuType":null,"cpuCount":null,"gpuTotal":null,` +
			`"memoryTotal":null,"diskTotal":null,"listed":null,"secureCloud":null,"location":null,` +
			`"maintenanceMode":null,"verified":null,"hostPricePerGpu":null,"dataCenterId":null,"runpodIp":null}}}`
	})}
	s, state := machineState(ctx, t, r, "rig-01")

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: state}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Errorf("state = %v, want the machine removed", resp.State.Raw)
	}
}

func TestMachineReadRemovesAMachineMarkedForDeletion(t *testing.T) {
	ctx := context.Background()

	// machineDelete keeps the row and sets markedForDeletion, and the machine
	// query does not filter on it, so a machine deleted outside Terraform
	// still answers with its id. A live one stays in state.
	for _, tc := range []struct {
		marked      string
		wantRemoved bool
	}{
		{"true", true},
		{"false", false},
		{"null", false},
	} {
		var query string
		r := &machineResource{client: graphQLStub(t, func(q string, _ map[string]any) string {
			query = q
			return `{"data":{"machine":{"id":"m-1","name":"rig-01","gpuType":{"id":"NVIDIA A40","displayName":"A40"},` +
				`"cpuCount":16,"gpuTotal":2,"memoryTotal":64,"diskTotal":500,"listed":false,"secureCloud":false,` +
				`"location":"EU","maintenanceMode":true,"verified":false,"hostPricePerGpu":0.2,"dataCenterId":"EU-RO-1",` +
				`"runpodIp":null,"markedForDeletion":` + tc.marked + `}}}`
		})}
		s, state := machineState(ctx, t, r, "rig-01")

		resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state}}
		r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: state}}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
		if !strings.Contains(query, "markedForDeletion") {
			t.Errorf("machine query = %q, want markedForDeletion selected", query)
		}
		if removed := resp.State.Raw.IsNull(); removed != tc.wantRemoved {
			t.Errorf("markedForDeletion = %s: removed = %v, want %v", tc.marked, removed, tc.wantRemoved)
		}
	}
}
