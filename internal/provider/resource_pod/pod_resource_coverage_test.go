package resource_pod

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestPodResource_MetadataAndSchema covers the boilerplate accessors (Metadata,
// Schema, NewPodResource) and guards against a schema-build panic.
func TestPodResource_MetadataAndSchema(t *testing.T) {
	ctx := context.Background()
	r := NewPodResource()
	mResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "runpod"}, mResp)
	if mResp.TypeName == "" {
		t.Error("Metadata produced an empty TypeName")
	}
	sResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, sResp)
	if len(sResp.Schema.Attributes) == 0 {
		t.Error("Schema produced no attributes")
	}
}

// TestPodRead_FullFieldMapping exercises every field-mapping branch in Read by
// returning a fully-populated body (cloudType, containerDiskInGb, costPerHr,
// created_at, dockerEntrypoint, dockerStartCmd, gpuTypeId, interruptible,
// machineId, memoryInGb, networkVolume, status, templateId, volumeEncrypted,
// volumeInGb).
func TestPodRead_FullFieldMapping(t *testing.T) {
	ctx := context.Background()
	sch := PodResourceSchema(ctx)
	// v2 envelope format
	body := `{
		"data": {
			"pod": {
				"id":"pod-1","status":"RUNNING","gpuTypeId":"NVIDIA A100","machineId":"m-1",
				"costPerHr":1.5,"created_at":"2024-01-01T00:00:00Z","memoryInGb":16,
				"volumeInGb":50,"containerDiskInGb":20,"templateId":"t-1","cloudType":"SECURE",
				"networkVolume":{"id":"nv-1"},"dockerEntrypoint":["/bin/sh"],
				"dockerStartCmd":["run"],"interruptible":true,"volumeEncrypted":true,"type":"ON_DEMAND"
			}
		},
		"meta": {"requestId":"test"},
		"error": null
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModel()
	m.Id = types.StringValue("pod-1")
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&PodResource{}).Read(ctx, resource.ReadRequest{State: podState(t, m)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read errored: %v", resp.Diagnostics.Errors())
	}

	var out PodModel
	if d := resp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read state: %v", d)
	}
	if out.MachineId.ValueString() != "m-1" {
		t.Errorf("MachineId = %q, want m-1", out.MachineId.ValueString())
	}
	if out.CostPerHr.ValueFloat64() != 1.5 {
		t.Errorf("CostPerHr = %v, want 1.5", out.CostPerHr.ValueFloat64())
	}
	if out.ContainerDiskInGb.ValueInt64() != 20 {
		t.Errorf("ContainerDiskInGb = %d, want 20", out.ContainerDiskInGb.ValueInt64())
	}
	if !out.Interruptible.ValueBool() || !out.VolumeEncrypted.ValueBool() {
		t.Errorf("interruptible/volumeEncrypted not mapped: %v / %v", out.Interruptible, out.VolumeEncrypted)
	}
	if out.NetworkVolumeId.ValueString() != "nv-1" {
		t.Errorf("NetworkVolumeId = %q, want nv-1 (from networkVolume.id)", out.NetworkVolumeId.ValueString())
	}
	if out.DockerEntrypoint.IsNull() || len(out.DockerEntrypoint.Elements()) != 1 {
		t.Errorf("DockerEntrypoint not mapped: %v", out.DockerEntrypoint)
	}
}

// TestPodUpdate_ManyFieldsInBody exercises Update's conditional body-build
// branches by changing fields that are valid for v1 PATCH /pods endpoint.
// Valid PATCH fields: name, env, ports (array), volumeInGb, volumeMountPath, containerDiskInGb.
func TestPodUpdate_ManyFieldsInBody(t *testing.T) {
	ctx := context.Background()
	sch := PodResourceSchema(ctx)

	prior := baseModel()
	prior.Id = types.StringValue("pod-1")
	prior.Name = types.StringValue("old")
	prior.VolumeInGb = types.Float64Value(40)
	prior.VolumeMountPath = types.StringValue("/old")
	prior.ContainerDiskInGb = types.Int64Value(20)

	desired := baseModel()
	desired.Id = types.StringValue("pod-1")
	desired.Name = types.StringValue("new")
	desired.VolumeInGb = types.Float64Value(50)
	desired.VolumeMountPath = types.StringValue("/data")
	desired.ContainerDiskInGb = types.Int64Value(30)

	envList := types.ListValueMust(types.StringType, []attr.Value{
		types.StringValue("K=V"),
	})
	desired.Env = envList

	desired.Ports = types.StringValue("8080/http,8443/https")

	var body map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		// v2 envelope format for response
		_, _ = w.Write([]byte(`{"data":{"pod":{"id":"pod-1"}},"meta":{"requestId":"test"},"error":null}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	(&PodResource{}).Update(ctx, resource.UpdateRequest{Config: podConfig(t, desired), State: podState(t, prior)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update errored: %v", resp.Diagnostics.Errors())
	}

	validFields := []string{"name", "env", "ports", "volumeInGb", "volumeMountPath", "containerDiskInGb"}
	for _, k := range validFields {
		if _, ok := body[k]; !ok {
			t.Errorf("PATCH body missing %q; got %v", k, body)
		}
	}
	if body["name"] != "new" {
		t.Errorf("body name = %v, want new", body["name"])
	}

	if portsArray, ok := body["ports"].([]interface{}); ok {
		if len(portsArray) != 2 {
			t.Errorf("ports array length = %d, want 2", len(portsArray))
		}
		if portsArray[0] != "8080/http" || portsArray[1] != "8443/https" {
			t.Errorf("ports array = %v, want [\"8080/http\",\"8443/https\"]", portsArray)
		}
	} else {
		t.Errorf("ports not found or not an array: %v", body["ports"])
	}
}
