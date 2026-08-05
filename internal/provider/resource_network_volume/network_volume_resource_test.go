package resource_network_volume

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// All NetworkVolumeModel fields are scalars (no List/Map), so a zero-value model
// Sets cleanly; tests set the fields they care about.
func nvModel() NetworkVolumeModel {
	return NetworkVolumeModel{
		Id:           types.StringNull(),
		Name:         types.StringNull(),
		Size:         types.Int64Null(),
		DataCenterId: types.StringNull(),
		Type:  types.StringNull(),
	}
}

func nvStub(t *testing.T, status int, body string, captured *map[string]interface{}, method, path *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		if status == 204 {
			_, _ = w.Write([]byte(""))
		} else {
			_, _ = w.Write([]byte(fmt.Sprintf(`{"data":{"networkVolume":%s},"meta":{},"error":null}`, body)))
		}
	}))
}

func nvConfig(t *testing.T, m NetworkVolumeModel) tfsdk.Config {
	t.Helper()
	ctx := context.Background()
	sch := NetworkVolumeResourceSchema(ctx)
	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build config: %v", d)
	}
	return tfsdk.Config{Schema: sch, Raw: st.Raw}
}

func nvState(t *testing.T, m NetworkVolumeModel) tfsdk.State {
	t.Helper()
	ctx := context.Background()
	sch := NetworkVolumeResourceSchema(ctx)
	st := tfsdk.State{Schema: sch}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build state: %v", d)
	}
	return st
}

func TestNetworkVolumeCreate_Success(t *testing.T) {
	ctx := context.Background()
	sch := NetworkVolumeResourceSchema(ctx)

	m := nvModel()
	m.Name = types.StringValue("vol-a")
	m.Size = types.Int64Value(50)
	m.DataCenterId = types.StringValue("US-CA-1")

	var body map[string]interface{}
	srv := nvStub(t, 200, `{"id":"nv-1","name":"vol-a","size":50,"dataCenter":"US-CA-1","type":"standard"}`, &body, nil, nil)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&NetworkVolumeResource{}).Create(ctx, resource.CreateRequest{Config: nvConfig(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create errored: %v", resp.Diagnostics.Errors())
	}
	if body["name"] != "vol-a" || body["size"] != float64(50) || body["dataCenter"] != "US-CA-1" {
		t.Errorf("request body missing fields; got %v", body)
	}
	var out NetworkVolumeModel
	if d := resp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read state: %v", d)
	}
	if out.Id.ValueString() != "nv-1" || out.Type.ValueString() != "standard" {
		t.Errorf("state not populated: id=%q tier=%q", out.Id.ValueString(), out.Type.ValueString())
	}
}

// TestNetworkVolumeCreate_Accepts201 locks in CE-1681: the v1 API returns 201
// Created for POST /network-volumes, so Create must treat 201 as success (not only
// 200). Before #44 this failed on a successful create and orphaned the volume.
func TestNetworkVolumeCreate_Accepts201(t *testing.T) {
	ctx := context.Background()
	sch := NetworkVolumeResourceSchema(ctx)
	m := nvModel()
	m.Name = types.StringValue("vol-a")
	m.Size = types.Int64Value(50)
	m.DataCenterId = types.StringValue("US-CA-1")

	srv := nvStub(t, 201, `{"id":"nv-1","name":"vol-a","size":50,"dataCenter":"US-CA-1","type":"standard"}`, nil, nil, nil)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&NetworkVolumeResource{}).Create(ctx, resource.CreateRequest{Config: nvConfig(t, m)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create must accept HTTP 201 (CE-1681): %v", resp.Diagnostics.Errors())
	}
	var out NetworkVolumeModel
	if d := resp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read state: %v", d)
	}
	if out.Id.ValueString() != "nv-1" {
		t.Errorf("id = %q, want nv-1 (Create must set id on a 201 response)", out.Id.ValueString())
	}
}

func TestNetworkVolumeCreate_MissingAPIKey(t *testing.T) {
	ctx := context.Background()
	sch := NetworkVolumeResourceSchema(ctx)
	t.Setenv("RUNPOD_API_KEY", "")
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&NetworkVolumeResource{}).Create(ctx, resource.CreateRequest{Config: nvConfig(t, nvModel())}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for missing API key")
	}
}

func TestNetworkVolumeCreate_NonOKStatus(t *testing.T) {
	ctx := context.Background()
	sch := NetworkVolumeResourceSchema(ctx)
	srv := nvStub(t, 400, `{"error":"bad"}`, nil, nil, nil)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&NetworkVolumeResource{}).Create(ctx, resource.CreateRequest{Config: nvConfig(t, nvModel())}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for 400 status")
	}
}

func TestNetworkVolumeCreate_PartialResponse_ReturnsDiagnostic(t *testing.T) {
	ctx := context.Background()
	sch := NetworkVolumeResourceSchema(ctx)
	srv := nvStub(t, 200, `{"id":"nv-1"}`, nil, nil, nil)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&NetworkVolumeResource{}).Create(ctx, resource.CreateRequest{Config: nvConfig(t, nvModel())}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for a partial response")
	}
}

func TestNetworkVolumeRead_Success(t *testing.T) {
	ctx := context.Background()
	sch := NetworkVolumeResourceSchema(ctx)
	m := nvModel()
	m.Id = types.StringValue("nv-1")
	var method, path string
	srv := nvStub(t, 200, `{"id":"nv-1","name":"renamed","size":100,"dataCenter":"EU-1","type":"premium"}`, nil, &method, &path)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&NetworkVolumeResource{}).Read(ctx, resource.ReadRequest{State: nvState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read errored: %v", resp.Diagnostics.Errors())
	}
	if method != "GET" || path != "/v2/network-volumes/nv-1" {
		t.Errorf("expected GET /v2/network-volumes/nv-1, got %s %s", method, path)
	}
	var out NetworkVolumeModel
	if d := resp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read state: %v", d)
	}
	if out.Name.ValueString() != "renamed" || out.Size.ValueInt64() != 100 {
		t.Errorf("state not refreshed: name=%q size=%d", out.Name.ValueString(), out.Size.ValueInt64())
	}
}

// TestNetworkVolumeRead_404_RemovesState asserts CE-1654 fix for network_volume:
// when a volume is gone (404), Read must call resp.State.RemoveResource so the
// deleted volume is removed from state and planned for recreation.
func TestNetworkVolumeRead_404_RemovesState(t *testing.T) {
	ctx := context.Background()
	m := nvModel()
	m.Id = types.StringValue("nv-1")
	srv := nvStub(t, 404, `{"error":"not found"}`, nil, nil, nil)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &resource.ReadResponse{State: nvState(t, m)}
	(&NetworkVolumeResource{}).Read(ctx, resource.ReadRequest{State: nvState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("404 should not error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("state was not removed on 404 — CE-1654: deleted network volume should be removed from state")
	}
}

func TestNetworkVolumeUpdate_Success(t *testing.T) {
	ctx := context.Background()
	sch := NetworkVolumeResourceSchema(ctx)
	prior := nvModel()
	prior.Id = types.StringValue("nv-1")
	prior.Name = types.StringValue("old-name")

	desired := nvModel()
	desired.Id = types.StringValue("nv-1")
	desired.Name = types.StringValue("new-name")

	var method, path string
	srv := nvStub(t, 200, `{"id":"nv-1","name":"new-name","size":50,"dataCenter":"US-CA-1","type":"standard"}`, nil, &method, &path)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	// Update reads req.Config (desired) + req.State (prior).
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	(&NetworkVolumeResource{}).Update(ctx, resource.UpdateRequest{
		Config: nvConfig(t, desired),
		State:  nvState(t, prior),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update errored: %v", resp.Diagnostics.Errors())
	}
	if method != "PATCH" || path != "/v2/network-volumes/nv-1" {
		t.Errorf("expected PATCH /v2/network-volumes/nv-1, got %s %s", method, path)
	}
}

// TestNetworkVolumeUpdate_PreservesComputedState asserts the CORRECT behavior for a
// second instance of the CE-1688 clobber (GitHub #34). Like EndpointResource.Update,
// NetworkVolumeResource.Update writes the merged state then overwrites it with
// resp.State.Set(ctx, &config), replacing values config leaves null:
//   - id (Computed — never present in config on update) -> clobbered to null
//   - storage_tier (Optional — API-populated but user-omitted) -> clobbered to null
// Through the real plugin protocol, a null computed id surfaces as "Provider
// produced inconsistent result after apply". Skipped until CE-1688 is fixed (drop
// the trailing State.Set(&config)).
// Note: this test's preservation assertion depends on TWO behaviors — dropping the
// trailing State.Set(&config) AND the merge continuing to seed `state` from prior
// state. If a future change drops State.Set but also stops seeding from prior state,
// the un-skipped test would still fail (for the second reason), so treat a failure as
// "check both" rather than assuming the overwrite is the only cause.
func TestNetworkVolumeUpdate_PreservesComputedState(t *testing.T) {
	t.Skip("CE-1688: network_volume Update clobbers computed state (config-overwrites-computed-state / GitHub #34) — un-skip when fixed")
	ctx := context.Background()
	sch := NetworkVolumeResourceSchema(ctx)

	// Prior state holds the resolved id + storage tier from create.
	prior := nvModel()
	prior.Id = types.StringValue("nv-1")
	prior.Name = types.StringValue("old-name")
	prior.Size = types.Int64Value(50)
	prior.DataCenterId = types.StringValue("US-CA-1")
	prior.Type = types.StringValue("standard")

	// Config as real Terraform supplies it on update: id is Computed (null in
	// config) and storage_tier is left unset. Only name changes.
	desired := nvModel()
	desired.Name = types.StringValue("new-name")
	desired.Size = types.Int64Value(50)
	desired.DataCenterId = types.StringValue("US-CA-1")
	// Id and Type intentionally left Null (nvModel defaults).

	// PATCH response returns the full resolved body (id + storageTier present).
	srv := nvStub(t, 200, `{"id":"nv-1","name":"new-name","size":50,"dataCenter":"US-CA-1","type":"standard"}`, nil, nil, nil)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	(&NetworkVolumeResource{}).Update(ctx, resource.UpdateRequest{
		Config: nvConfig(t, desired),
		State:  nvState(t, prior),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update errored: %v", resp.Diagnostics.Errors())
	}

	var out NetworkVolumeModel
	if d := resp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read result state: %v", d)
	}

	// Correct behavior (post-fix): the merged computed values survive the update.
	if out.Id.ValueString() != "nv-1" {
		t.Errorf("id = %q, want \"nv-1\" preserved (CE-1688: Update must not clobber computed state)", out.Id.ValueString())
	}
	if out.Type.ValueString() != "standard" {
		t.Errorf("storage_tier = %q, want \"standard\" preserved (CE-1688)", out.Type.ValueString())
	}
}

// TestNetworkVolumeUpdate_PanicsOnPartialResponse pins the CE-1677 Update half:
// TestNetworkVolumeUpdate_MissingNameReturnsDiagnostic verifies the CE-1677 fix
// (#48 merged): a 200 PATCH response that omits "name" must yield a clean
// diagnostic, not a panic (network_volume Update previously did an unchecked
// `state.Name = result["name"].(string)`).
func TestNetworkVolumeUpdate_MissingNameReturnsDiagnostic(t *testing.T) {
	ctx := context.Background()
	sch := NetworkVolumeResourceSchema(ctx)

	prior := nvModel()
	prior.Id = types.StringValue("nv-1")
	prior.Name = types.StringValue("old-name")

	desired := nvModel()
	desired.Id = types.StringValue("nv-1")
	desired.Name = types.StringValue("new-name") // a change, so Update PATCHes

	// 200 OK (passes the status guard) but the body omits "name".
	srv := nvStub(t, 200, `{"id":"nv-1"}`, nil, nil, nil)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	(&NetworkVolumeResource{}).Update(ctx, resource.UpdateRequest{
		Config: nvConfig(t, desired),
		State:  nvState(t, prior),
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected a diagnostic when the update response omits \"name\" (CE-1677); got none")
	}
}

func TestNetworkVolumeDelete_Success(t *testing.T) {
	ctx := context.Background()
	sch := NetworkVolumeResourceSchema(ctx)
	m := nvModel()
	m.Id = types.StringValue("nv-1")
	var method, path string
	srv := nvStub(t, 204, `{"data":{},"meta":{},"error":null}`, nil, &method, &path)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: sch}}
	(&NetworkVolumeResource{}).Delete(ctx, resource.DeleteRequest{State: nvState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete errored: %v", resp.Diagnostics.Errors())
	}
	if method != "DELETE" || path != "/v2/network-volumes/nv-1" {
		t.Errorf("expected DELETE /v2/network-volumes/nv-1, got %s %s", method, path)
	}
}

func TestNetworkVolumeDelete_NonNoContent(t *testing.T) {
	ctx := context.Background()
	sch := NetworkVolumeResourceSchema(ctx)
	m := nvModel()
	m.Id = types.StringValue("nv-1")
	srv := nvStub(t, 500, `oops`, nil, nil, nil)
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: sch}}
	(&NetworkVolumeResource{}).Delete(ctx, resource.DeleteRequest{State: nvState(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for non-204 delete")
	}
}
