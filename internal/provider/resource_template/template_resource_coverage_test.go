package resource_template

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

func TestTemplateResource_MetadataAndSchema(t *testing.T) {
	ctx := context.Background()
	r := NewTemplateResource()
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

func tplState(t *testing.T, ctx context.Context, m TemplateModel) tfsdk.State {
	t.Helper()
	st := tfsdk.State{Schema: TemplateResourceSchema(ctx)}
	if d := st.Set(ctx, &m); d.HasError() {
		t.Fatalf("build state: %v", d)
	}
	return st
}

// TestTemplateRead_FullFieldMapping exercises Read's field-mapping branches with
// a fully-populated response (incl. the computed fields earned/isRunpod/runtimeInMin).
func TestTemplateRead_FullFieldMapping(t *testing.T) {
	ctx := context.Background()
	sch := TemplateResourceSchema(ctx)
	body := `{
		"id":"tpl-1","name":"n","imageName":"img","category":"NVIDIA",
		"containerDiskInGb":20,"containerRegistryAuthId":"cra-1",
		"dockerEntrypoint":["/bin/sh"],"dockerStartCmd":["run"],
		"env":{"K":"V"},"isPublic":true,"isServerless":false,
		"ports":["8888/http"],"readme":"hi","volumeInGb":10,"volumeMountPath":"/workspace",
		"earned":42.5,"isRunpod":true,"runtimeInMin":5
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := newBaseModel()
	m.Id = types.StringValue("tpl-1")
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&TemplateResource{}).Read(ctx, resource.ReadRequest{State: tplState(t, ctx, m)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read errored: %v", resp.Diagnostics.Errors())
	}
	var out TemplateModel
	if d := resp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("read state: %v", d)
	}
	if out.Category.ValueString() != "NVIDIA" || out.ContainerRegistryAuthId.ValueString() != "cra-1" {
		t.Errorf("scalars not mapped: category=%q craId=%q", out.Category.ValueString(), out.ContainerRegistryAuthId.ValueString())
	}
	if out.Earned.ValueFloat64() != 42.5 {
		t.Errorf("Earned = %v, want 42.5", out.Earned.ValueFloat64())
	}
	if out.DockerEntrypoint.IsNull() || len(out.DockerEntrypoint.Elements()) != 1 {
		t.Errorf("DockerEntrypoint not mapped: %v", out.DockerEntrypoint)
	}
}

// TestTemplateUpdate_ManyFields exercises Update's conditional body-build branches.
func TestTemplateUpdate_ManyFields(t *testing.T) {
	ctx := context.Background()
	sch := TemplateResourceSchema(ctx)

	prior := newBaseModel()
	prior.Id = types.StringValue("tpl-1")
	prior.Name = types.StringValue("old")

	desired := newBaseModel()
	desired.Id = types.StringValue("tpl-1")
	desired.Name = types.StringValue("new")
	desired.ImageName = types.StringValue("img2")
	desired.Category = types.StringValue("NVIDIA")
	desired.ContainerDiskInGb = types.Int64Value(30)
	desired.ContainerRegistryAuthId = types.StringValue("cra-2")
	desired.DockerEntrypoint = types.ListValueMust(types.StringType, []attr.Value{types.StringValue("/bin/sh")})
	desired.Ports = types.ListValueMust(types.StringType, []attr.Value{types.StringValue("8080/http")})
	desired.Env = types.MapValueMust(types.StringType, map[string]attr.Value{"K": types.StringValue("V")})
	desired.IsPublic = types.BoolValue(true)
	desired.Readme = types.StringValue("readme")
	desired.VolumeInGb = types.Int64Value(20)
	desired.VolumeMountPath = types.StringValue("/data")

	var body map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		_, _ = w.Write([]byte(`{"id":"tpl-1","name":"new","imageName":"img2"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	(&TemplateResource{}).Update(ctx, resource.UpdateRequest{Config: buildConfig(t, ctx, desired), State: tplState(t, ctx, prior)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update errored: %v", resp.Diagnostics.Errors())
	}
  // NOTE: 'category' is a computed field in the v1 templates PATCH API and
  // should NOT be included in the PATCH request body. It is read from the API
  // response during Read/Create but excluded from Update requests.
  for _, k := range []string{"name", "imageName", "containerDiskInGb", "containerRegistryAuthId", "env", "isPublic", "readme", "volumeMountPath"} {
    if _, ok := body[k]; !ok {
      t.Errorf("PATCH body missing %q; got %v", k, body)
    }
  }

  if _, ok := body["category"]; ok {
    t.Errorf("PATCH body should NOT contain 'category'; got %v", body)
  }
}
