package resource_endpoint

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestEndpointResource_MetadataAndSchema(t *testing.T) {
	ctx := context.Background()
	r := NewEndpointResource()
	mResp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "runpod"}, mResp)
	if mResp.TypeName == "" {
		t.Error("empty TypeName")
	}
	sResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, sResp)
	if len(sResp.Schema.Attributes) == 0 {
		t.Error("no schema attributes")
	}
}

// TestEndpointUpdate_ManyFields exercises Update's conditional body-build branches
// across many attributes (Update is the lowest-covered endpoint method).
func TestEndpointUpdate_ManyFields(t *testing.T) {
	ctx := context.Background()
	sch := EndpointResourceSchema(ctx)

	prior := newBaseModel()
	prior.Id = types.StringValue("ep-1")
	prior.Name = types.StringValue("old")

	desired := newBaseModel()
	desired.Id = types.StringValue("ep-1")
	desired.Name = types.StringValue("new")
	desired.NetworkVolumeIds = strList("nv-1")
	desired.DataCenterIds = strList("US-CA-1")
	desired.GpuTypeIds = strList("NVIDIA A100")
	desired.Env = types.MapValueMust(types.StringType, map[string]attr.Value{"K": types.StringValue("V")})
	desired.WorkersMin = types.Int64Value(2)
	desired.WorkersMax = types.Int64Value(5)
	desired.IdleTimeout = types.Int64Value(30)
	desired.ScalerType = types.StringValue("QUEUE_DELAY")
	desired.ScalerValue = types.Int64Value(4)
	desired.ExecutionTimeoutMs = types.Int64Value(600000)
	desired.ComputeType = types.StringValue("GPU")
	desired.GpuCount = types.Int64Value(1)
	desired.Flashboot = types.BoolValue(true)

	var body map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		_, _ = w.Write([]byte(`{"id":"ep-1","name":"new"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	priorSt := tfsdk.State{Schema: sch}
	if d := priorSt.Set(ctx, &prior); d.HasError() {
		t.Fatalf("build prior: %v", d)
	}
	desiredSt := tfsdk.State{Schema: sch}
	if d := desiredSt.Set(ctx, &desired); d.HasError() {
		t.Fatalf("build desired: %v", d)
	}

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	(&EndpointResource{}).Update(ctx, resource.UpdateRequest{
		Config: tfsdk.Config{Schema: sch, Raw: desiredSt.Raw},
		State:  priorSt,
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update errored: %v", resp.Diagnostics.Errors())
	}
	for _, k := range []string{"name", "networkVolumes", "dataCenterIds", "env", "workers", "scaling", "computeType", "flashboot"} {
		if _, ok := body[k]; !ok {
			t.Errorf("PATCH body missing %q; got %v", k, body)
		}
	}
}
