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

// errServer returns a stub GraphQL server that always replies with a non-200
// status, forcing rlClient.Query() to return a transport-style error. This drives
// the `resp.Diagnostics.AddError("API Error", ...)` branches in every CRUD op.
func errServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
	}))
}

// --- Metadata / Schema / constructor smoke (0% funcs) ---

func TestMachineResource_NewMetadataSchema(t *testing.T) {
	ctx := context.Background()
	r := NewMachineResource()
	if r == nil {
		t.Fatal("NewMachineResource returned nil")
	}
	mr, ok := r.(*MachineResource)
	if !ok {
		t.Fatalf("NewMachineResource returned %T, want *MachineResource", r)
	}

	metaResp := &resource.MetadataResponse{}
	mr.Metadata(ctx, resource.MetadataRequest{}, metaResp)
	if metaResp.TypeName != "runpod_machine" {
		t.Errorf("TypeName = %q, want runpod_machine", metaResp.TypeName)
	}

	schemaResp := &resource.SchemaResponse{}
	mr.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Schema.Attributes == nil {
		t.Fatal("Schema returned nil Attributes")
	}
	// A representative subset of attributes Read/Create depend on must exist.
	for _, attr := range []string{"id", "name", "gpu_count", "gpu_type_id", "host_price_per_gpu"} {
		if _, ok := schemaResp.Schema.Attributes[attr]; !ok {
			t.Errorf("Schema missing attribute %q", attr)
		}
	}
}

// --- Read: full field mapping (all dereferenced fields asserted into state) ---

func TestMachineRead_MapsAllFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"machine":{` +
			`"name":"box-1","gpuType":{"id":"NVIDIA H100","displayName":"NVIDIA H100"},"cpuCount":32,` +
			`"gpuTotal":4,"memoryTotal":256,"diskTotal":2000,"location":"US-CA","listed":true,` +
			`"secureCloud":false,"maintenanceMode":true,"verified":true,"hostPricePerGpu":2.75}}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	ctx := context.Background()
	sch := MachineResourceSchema(ctx)
	state := tfsdk.State{Schema: sch}
	if d := state.Set(ctx, &MachineModel{Id: types.StringValue("m1")}); d.HasError() {
		t.Fatalf("building state: %v", d)
	}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&MachineResource{}).Read(ctx, resource.ReadRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	var got MachineModel
	if d := resp.State.Get(ctx, &got); d.HasError() {
		t.Fatalf("reading state: %v", d)
	}

	if got.Name.ValueString() != "box-1" {
		t.Errorf("Name = %q, want box-1", got.Name.ValueString())
	}
	if got.GpuCount.ValueInt64() != 4 {
		t.Errorf("GpuCount = %d, want 4", got.GpuCount.ValueInt64())
	}
	if got.GpuTypeId.ValueString() != "NVIDIA H100" {
		t.Errorf("GpuTypeId = %q, want NVIDIA H100", got.GpuTypeId.ValueString())
	}
	if got.CpuCount.ValueInt64() != 32 {
		t.Errorf("CpuCount = %d, want 32", got.CpuCount.ValueInt64())
	}
	if got.MemoryInGb.ValueInt64() != 256 {
		t.Errorf("MemoryInGb = %d, want 256", got.MemoryInGb.ValueInt64())
	}
	if got.DiskInGb.ValueInt64() != 2000 {
		t.Errorf("DiskInGb = %d, want 2000", got.DiskInGb.ValueInt64())
	}
	if got.Location.ValueString() != "US-CA" {
		t.Errorf("Location = %q, want US-CA", got.Location.ValueString())
	}
	if !got.Listed.ValueBool() {
		t.Error("Listed = false, want true")
	}
	if got.SecureCloud.ValueBool() {
		t.Error("SecureCloud = true, want false")
	}
	if !got.MaintenanceMode.ValueBool() {
		t.Error("MaintenanceMode = false, want true")
	}
	if !got.Verified.ValueBool() {
		t.Error("Verified = false, want true")
	}
	if got.HostPricePerGpu.ValueFloat64() != 2.75 {
		t.Errorf("HostPricePerGpu = %v, want 2.75", got.HostPricePerGpu.ValueFloat64())
	}
}

// --- Read error branches ---

func TestMachineRead_APIError(t *testing.T) {
	srv := errServer(t)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	ctx := context.Background()
	sch := MachineResourceSchema(ctx)
	state := tfsdk.State{Schema: sch}
	if d := state.Set(ctx, &MachineModel{Id: types.StringValue("m1")}); d.HasError() {
		t.Fatalf("building state: %v", d)
	}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&MachineResource{}).Read(ctx, resource.ReadRequest{State: state}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error on Query failure, got none")
	}
}

func TestMachineRead_MachineMissing(t *testing.T) {
	// Valid 200 with data but no "machine" key -> "Machine not found" branch.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"somethingElse":{}}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	ctx := context.Background()
	sch := MachineResourceSchema(ctx)
	state := tfsdk.State{Schema: sch}
	if d := state.Set(ctx, &MachineModel{Id: types.StringValue("m1")}); d.HasError() {
		t.Fatalf("building state: %v", d)
	}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&MachineResource{}).Read(ctx, resource.ReadRequest{State: state}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 'Machine not found' diagnostics error, got none")
	}
}

// --- Create error branches ---

func TestMachineCreate_APIError(t *testing.T) {
	srv := errServer(t)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	m := MachineModel{Name: types.StringValue("n"), GpuCount: types.Int64Value(1), GpuTypeId: types.StringValue("A100")}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: MachineResourceSchema(context.Background())}}
	(&MachineResource{}).Create(context.Background(), resource.CreateRequest{Config: machineConfig(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error on Query failure, got none")
	}
}

func TestMachineCreate_MachineCreateMissing(t *testing.T) {
	// 200 with data but no machineAdd key -> "machineAdd not in response".
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"nope":{}}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	m := MachineModel{Name: types.StringValue("n"), GpuCount: types.Int64Value(1), GpuTypeId: types.StringValue("A100")}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: MachineResourceSchema(context.Background())}}
	(&MachineResource{}).Create(context.Background(), resource.CreateRequest{Config: machineConfig(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 'machineAdd not in response' diagnostics error, got none")
	}
}

func TestMachineCreate_IDMissing(t *testing.T) {
	// machineCreate present but without an "id" -> "Failed to get machine ID" branch.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"machineAdd":{"name":"n"}}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	m := MachineModel{Name: types.StringValue("n"), GpuCount: types.Int64Value(1), GpuTypeId: types.StringValue("A100")}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: MachineResourceSchema(context.Background())}}
	(&MachineResource{}).Create(context.Background(), resource.CreateRequest{Config: machineConfig(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 'Failed to get machine ID' diagnostics error, got none")
	}
}

// --- Update / Delete error branches ---

func TestMachineUpdate_APIError(t *testing.T) {
	srv := errServer(t)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	ctx := context.Background()
	sch := MachineResourceSchema(ctx)
	plan := tfsdk.Plan{Schema: sch}
	m := MachineModel{Id: types.StringValue("m1"), Name: types.StringValue("n"), GpuCount: types.Int64Value(1), GpuTypeId: types.StringValue("A100")}
	if d := plan.Set(ctx, &m); d.HasError() {
		t.Fatalf("building plan: %v", d)
	}
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	(&MachineResource{}).Update(ctx, resource.UpdateRequest{Plan: plan}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error on Query failure, got none")
	}
}

func TestMachineDelete_APIError(t *testing.T) {
	srv := errServer(t)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_GRAPHQL_URL", srv.URL)

	ctx := context.Background()
	sch := MachineResourceSchema(ctx)
	state := tfsdk.State{Schema: sch}
	if d := state.Set(ctx, &MachineModel{Id: types.StringValue("m1")}); d.HasError() {
		t.Fatalf("building state: %v", d)
	}
	resp := &resource.DeleteResponse{State: state}
	(&MachineResource{}).Delete(ctx, resource.DeleteRequest{State: state}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error on Query failure, got none")
	}
}
