package resource_pod

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

// baseModel returns a PodModel with all attributes null except the list-typed
// fields, which must carry an element type to be a valid value.
func baseModel() PodModel {
	return PodModel{
		Env:              types.ListNull(types.StringType),
		DockerEntrypoint: types.ListNull(types.StringType),
		DockerStartCmd:   types.ListNull(types.StringType),
	}
}

func podConfig(t *testing.T, m PodModel) tfsdk.Config {
	t.Helper()
	ctx := context.Background()
	sch := PodResourceSchema(ctx)
	st := tfsdk.State{Schema: sch}
	if diags := st.Set(ctx, &m); diags.HasError() {
		t.Fatalf("building config: %v", diags)
	}
	return tfsdk.Config{Schema: sch, Raw: st.Raw}
}

func podState(t *testing.T, m PodModel) tfsdk.State {
	t.Helper()
	ctx := context.Background()
	sch := PodResourceSchema(ctx)
	st := tfsdk.State{Schema: sch}
	if diags := st.Set(ctx, &m); diags.HasError() {
		t.Fatalf("building state: %v", diags)
	}
	return st
}

func configureResourceWithTestClient(t *testing.T, r interface{ Configure(context.Context, resource.ConfigureRequest, *resource.ConfigureResponse) }, srv *httptest.Server) {
	t.Helper()
	req := resource.ConfigureRequest{
		ProviderData: client.NewRunPodClient("test-key", "https://api.runpod.io/graphql", srv.URL),
	}
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("failed to configure resource: %v", resp.Diagnostics)
	}
}

func TestPodCreate_BothTemplateAndImage_Errors(t *testing.T) {
	m := baseModelWithListTypes()
	m.TemplateId = types.StringValue("tmpl-1")
	m.ImageName = types.StringValue("img:latest")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodResourceSchema(context.Background())}}
	(&PodResource{}).Create(context.Background(), resource.CreateRequest{Config: podConfig(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when both template_id and image_name are set")
	}
}

func TestPodCreate_NeitherTemplateNorImage_Errors(t *testing.T) {
	m := baseModelWithListTypes() // both null

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodResourceSchema(context.Background())}}
	(&PodResource{}).Create(context.Background(), resource.CreateRequest{Config: podConfig(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when neither template_id nor image_name is set")
	}
}

func TestPodCreate_ImageName_BuildsBodyAndSetsID(t *testing.T) {
	var gotBody map[string]interface{}
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
			_, _ = w.Write([]byte(`{"data":{"pod":{"id":"pod-xyz"}},"meta":{"requestId":"test"},"error":null}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModelWithListTypes()
	m.Name = types.StringValue("my-pod")
	m.ImageName = types.StringValue("img:latest")
	m.GpuCount = types.Int64Value(2)
	m.GpuTypeId = types.StringValue("NVIDIA A100 80GB")
	m.Interruptible = types.BoolValue(false)

	sch := PodResourceSchema(context.Background())
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&PodResource{}).Create(context.Background(), resource.CreateRequest{Config: podConfig(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotMethod != "POST" || gotPath != "/v2/pods" {
		t.Errorf("request = %s %s, want POST /v2/pods", gotMethod, gotPath)
	}
	// v2 format: flat fields with v2 names
	if gotBody["image"] != "img:latest" {
		t.Errorf("image = %v, want img:latest", gotBody["image"])
	}
	if gotBody["name"] != "my-pod" {
		t.Errorf("name = %v, want my-pod", gotBody["name"])
	}
	if gpu, ok := gotBody["gpu"].(map[string]interface{}); ok {
		if gpu["count"] != float64(2) {
			t.Errorf("gpu.count = %v, want 2", gpu["count"])
		}
	} else {
		t.Errorf("gpu object not found or not a map: %v", gotBody["gpu"])
	}

	var out PodModel
	resp.State.Get(context.Background(), &out)
	if out.Id.ValueString() != "pod-xyz" {
		t.Errorf("id = %q, want pod-xyz", out.Id.ValueString())
	}
}

func TestPodCreate_TemplateID_BuildsBody(t *testing.T) {
	var gotPodBody map[string]interface{}
	
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/templates/tmpl-1" {
			_, _ = w.Write([]byte(`{"image":"runpod/pytorch:2.1.1","args":"","ports":["8888/http","22/tcp"],"env":{"JUPYTER_PASSWORD":"test"},"disk":50,"mounts":{}}`))
			return
		}
		
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotPodBody)
		_, _ = w.Write([]byte(`{"data":{"pod":{"id":"pod-tmpl"}},"meta":{"requestId":"test"},"error":null}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModelWithListTypes()
	m.Name = types.StringValue("tmpl-pod")
	m.TemplateId = types.StringValue("tmpl-1")
	m.GpuCount = types.Int64Value(1)
	m.GpuTypeId = types.StringValue("NVIDIA A100 80GB")

	r := &PodResource{}
	configureResourceWithTestClient(t, r, srv)
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodResourceSchema(context.Background())}}
	r.Create(context.Background(), resource.CreateRequest{Config: podConfig(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotPodBody["name"] != "tmpl-pod" {
		t.Errorf("name = %v, want tmpl-pod", gotPodBody["name"])
	}
	if _, ok := gotPodBody["templateId"]; ok {
		t.Errorf("templateId should not be present, got %v", gotPodBody["templateId"])
	}
	if gotPodBody["image"] != "runpod/pytorch:2.1.1" {
		t.Errorf("image = %v, want runpod/pytorch:2.1.1", gotPodBody["image"])
	}
	if gpu, ok := gotPodBody["gpu"].(map[string]interface{}); ok {
		if gpu["count"] != float64(1) {
			t.Errorf("gpu.count = %v, want 1", gpu["count"])
		}
	} else {
		t.Errorf("gpu object not found or not a map: %v", gotPodBody["gpu"])
	}
}

// TestPodRead_UsesConfiguredBaseURL exercises the CE-1650 fix: Read must honor
// RUNPOD_BASE_URL, not the hardcoded prod URL. Without the fix this request
// would never reach the test server.
func TestPodRead_UsesConfiguredBaseURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"data":{"pod":{"id":"pod-1","desiredStatus":"RUNNING","costPerHr":0.5,"type":"ON_DEMAND"}},"meta":{"requestId":"test"},"error":null}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModelWithListTypes()
	m.Id = types.StringValue("pod-1")

	sch := PodResourceSchema(context.Background())
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&PodResource{}).Read(context.Background(), resource.ReadRequest{State: podState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotPath != "/v2/pods/pod-1" {
		t.Errorf("Read hit path %q, want /v2/pods/pod-1 (CE-1650: must honor RUNPOD_BASE_URL)", gotPath)
	}
	var out PodModel
	resp.State.Get(context.Background(), &out)
	if out.Status.ValueString() != "RUNNING" {
		t.Errorf("status = %q, want RUNNING", out.Status.ValueString())
	}
}

// TestPodRead_404_RemovesState asserts CE-1654 is fixed: when a pod is gone (404),
// Read must call resp.State.RemoveResource so the deleted pod is removed from state.
func TestPodRead_404_RemovesState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"pod not found"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModelWithListTypes()
	m.Id = types.StringValue("gone")

	resp := &resource.ReadResponse{State: podState(t, m)} // pre-populated, mirrors framework
	(&PodResource{}).Read(context.Background(), resource.ReadRequest{State: podState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("404 should not produce an error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("state was not removed on 404 — CE-1654: deleted pod should be removed from state")
	}
}

func TestPodCreate_NoIDInResponse_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":"accepted but no id"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModelWithListTypes()
	m.Name = types.StringValue("my-pod")
	m.ImageName = types.StringValue("img:latest")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodResourceSchema(context.Background())}}
	(&PodResource{}).Create(context.Background(), resource.CreateRequest{Config: podConfig(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when the API response has no pod id")
	}
}

// TestPodCreate_RequiresEnvApiKey characterizes CE-1653: resources read the API
// key straight from os.Getenv("RUNPOD_API_KEY") and have no Configure wiring,
// so provider-block credentials never reach them. With the env var empty,
// Create fails regardless of any provider config.
func TestPodCreate_RequiresEnvApiKey(t *testing.T) {
	t.Setenv("RUNPOD_API_KEY", "") // empty = unset

	m := baseModelWithListTypes()
	m.Name = types.StringValue("p")
	m.ImageName = types.StringValue("img") // valid config so we reach the api-key check

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodResourceSchema(context.Background())}}
	(&PodResource{}).Create(context.Background(), resource.CreateRequest{Config: podConfig(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when RUNPOD_API_KEY is unset (CE-1653: env-only auth)")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "RUNPOD_API_KEY environment variable must be set") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the RUNPOD_API_KEY env error, got: %v", resp.Diagnostics)
	}
}

// TestPodUpdate_AppliesChanges is the positive regression for CE-1655 (closed):
// PodResource.Update now diffs config against prior state and PATCHes the changed
// fields. A name change must trigger one API call (PATCH /pods/{id}) carrying the
// new name, and the resulting state must reflect it. (Update reads req.Config and
// req.State — not req.Plan.)
func TestPodUpdate_AppliesChanges(t *testing.T) {
	var hit bool
	var method, path string
	var body map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		method, path = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		_, _ = w.Write([]byte(`{"id":"pod-1"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	prior := baseModelWithListTypes()
	prior.Id = types.StringValue("pod-1")
	prior.Name = types.StringValue("orig-name")

	desired := baseModelWithListTypes()
	desired.Id = types.StringValue("pod-1")
	desired.Name = types.StringValue("changed-name")

	sch := PodResourceSchema(context.Background())
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	(&PodResource{}).Update(context.Background(), resource.UpdateRequest{
		Config: podConfig(t, desired),
		State:  podState(t, prior),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update should not error: %v", resp.Diagnostics)
	}
	if !hit {
		t.Fatal("Update made no API call — change was silently dropped (CE-1655 regression)")
	}
	if method != "PATCH" {
		t.Errorf("expected PATCH, got %s", method)
	}
	if path != "/v2/pods/pod-1" {
		t.Errorf("expected /v2/pods/pod-1, got %s", path)
	}
	if body["name"] != "changed-name" {
		t.Errorf("expected name=changed-name in PATCH body, got %v", body)
	}
	if resp.State.Raw.IsNull() {
		t.Error("Update wrote no state")
	}
}

// TestPodUpdate_MissingIDReturnsDiagnostic verifies the CE-1684 fix (#48 merged):
// a 200 PATCH response without "id" must yield a clean diagnostic, not a panic
// (previously `config.Id = result["id"].(string)` was an unchecked assertion).
func TestPodUpdate_MissingIDReturnsDiagnostic(t *testing.T) {
	// 200 OK (passes the != 200 guard) but no "id" in the body.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":"ok but no id"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	prior := baseModelWithListTypes()
	prior.Id = types.StringValue("pod-1")
	prior.Name = types.StringValue("orig-name")

	desired := baseModelWithListTypes()
	desired.Id = types.StringValue("pod-1")
	desired.Name = types.StringValue("changed-name") // a diff, so Update PATCHes and reaches the id extraction

	sch := PodResourceSchema(context.Background())
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	(&PodResource{}).Update(context.Background(), resource.UpdateRequest{
		Config: podConfig(t, desired),
		State:  podState(t, prior),
	}, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected a diagnostic when the update response has no id (CE-1684); got none")
	}
}

// TestPodRead_FieldMapping is a unit characterization of CE-1658 using
// a realistic v1 API response: the API returns desiredStatus/createdAt and nests
// gpuType/secureCloud under "machine", but Read maps top-level
// status/created_at/gpuTypeId/cloudType — so those come back empty while
// costPerHr (correctly named) populates. Catches CE-1658 without riab. Green now;
// flips to failing when CE-1658 is fixed (then assert the populated values).
func TestPodRead_FieldMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"desiredStatus":"RUNNING","createdAt":"2026-06-25T00:00:00Z","costPerHr":0.5,"machine":{"gpuTypeId":"NVIDIA GeForce RTX 4090","secureCloud":true}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModelWithListTypes()
	m.Id = types.StringValue("pod-1")
	sch := PodResourceSchema(context.Background())
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&PodResource{}).Read(context.Background(), resource.ReadRequest{State: podState(t, m)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var out PodModel
	resp.State.Get(context.Background(), &out)
	// Correctly-named field populates — proves Read parsed the response.
	if out.CostPerHr.ValueFloat64() != 0.5 {
		t.Errorf("costPerHr = %v, want 0.5", out.CostPerHr.ValueFloat64())
	}
	// CE-1658 FIXED: Read now maps the correct API field names.
	if out.Status.ValueString() != "RUNNING" {
		t.Errorf("status = %q, want RUNNING (CE-1658: API returns 'desiredStatus')", out.Status.ValueString())
	}
	if out.CreatedAt.ValueString() != "2026-06-25T00:00:00Z" {
		t.Errorf("created_at = %q, want 2026-06-25T00:00:00Z (CE-1658: API returns 'createdAt')", out.CreatedAt.ValueString())
	}
	if out.GpuTypeId.ValueString() != "NVIDIA GeForce RTX 4090" {
		t.Errorf("gpu_type_id = %q, want NVIDIA GeForce RTX 4090 (CE-1658: read from machine.gpuTypeId)", out.GpuTypeId.ValueString())
	}
	if out.CloudType.ValueString() != "SECURE" {
		t.Errorf("cloud_type = %q, want SECURE (CE-1658: read from machine.secureCloud=true)", out.CloudType.ValueString())
	}
}

func TestPodRead_CommunityMachineCloudType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"desiredStatus":"RUNNING","createdAt":"2026-06-25T00:00:00Z","costPerHr":0.5,"machine":{"gpuTypeId":"NVIDIA GeForce RTX 3060","secureCloud":false}}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModelWithListTypes()
	m.Id = types.StringValue("pod-1")
	sch := PodResourceSchema(context.Background())
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&PodResource{}).Read(context.Background(), resource.ReadRequest{State: podState(t, m)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var out PodModel
	resp.State.Get(context.Background(), &out)
	if out.CloudType.ValueString() != "COMMUNITY" {
		t.Errorf("cloud_type = %q, want COMMUNITY (CE-1658: secureCloud=false means COMMUNITY)", out.CloudType.ValueString())
	}
}

func TestPodRead_TopLevelCloudTypeFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"desiredStatus":"RUNNING","createdAt":"2026-06-25T00:00:00Z","costPerHr":0.5,"cloudType":"COMMUNITY"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModelWithListTypes()
	m.Id = types.StringValue("pod-1")
	sch := PodResourceSchema(context.Background())
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&PodResource{}).Read(context.Background(), resource.ReadRequest{State: podState(t, m)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var out PodModel
	resp.State.Get(context.Background(), &out)
	if out.CloudType.ValueString() != "COMMUNITY" {
		t.Errorf("cloud_type = %q, want COMMUNITY (CE-1658: fallback to top-level cloudType)", out.CloudType.ValueString())
	}
}

// TestPodCreate_ValidAttributes verifies that Create only forwards attributes
// that are valid for the v1 POST /pods endpoint. Fields like gpuTypeId (scalar),
// machineId, dockerArgs, startSsh, startJupyter, stopAfter, etc. are not in the
// v1 CREATE schema and would cause HTTP 400 errors. Only these fields are valid:
// gpuCount, name, templateId|imageName, cloudType, volumeInGb, networkVolumeId,
// containerDiskInGb, volumeMountPath, env.
func TestPodCreate_ValidAttributes(t *testing.T) {
	var body map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
			_, _ = w.Write([]byte(`{"data":{"pod":{"id":"pod-x"}},"meta":{"requestId":"test"},"error":null}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModelWithListTypes()
	m.Name = types.StringValue("p")
	m.ImageName = types.StringValue("img")
	m.GpuCount = types.Int64Value(1)
	m.GpuTypeId = types.StringValue("NVIDIA A100 80GB")
	m.CloudType = types.StringValue("SECURE")
	m.VolumeInGb = types.Float64Value(50)
	m.ContainerDiskInGb = types.Int64Value(20)
	m.VolumeMountPath = types.StringValue("/data")
	m.Interruptible = types.BoolValue(false)

	envList := types.ListValueMust(types.StringType, []attr.Value{
		types.StringValue("ENV1=value1"),
		types.StringValue("ENV2=value2"),
	})
	m.Env = envList

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodResourceSchema(context.Background())}}
	(&PodResource{}).Create(context.Background(), resource.CreateRequest{Config: podConfig(t, m)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	// v2 format: flat fields with v2 names
	if body["image"] != "img" {
		t.Errorf("image = %v, want img", body["image"])
	}
	if body["name"] != "p" {
		t.Errorf("name = %v, want p", body["name"])
	}
	if gpu, ok := body["gpu"].(map[string]interface{}); ok {
		if gpu["count"] != float64(1) {
			t.Errorf("gpu.count = %v, want 1", gpu["count"])
		}
	} else {
		t.Errorf("gpu object not found or not a map: %v", body["gpu"])
	}
	if body["cloud"] != "SECURE" {
		t.Errorf("cloud = %v, want SECURE", body["cloud"])
	}
	mounts, ok := body["mounts"].(map[string]interface{})
	if !ok {
		t.Fatalf("mounts object not found or not correct type: %v", body["mounts"])
	}
	pers, ok := mounts["persistent"].(map[string]interface{})
	if !ok {
		t.Fatalf("mounts.persistent not found: %v", mounts)
	}
	if pers["size"] != float64(50) {
		t.Errorf("mounts.persistent.size = %v, want 50", pers["size"])
	}
	if pers["path"] != "/data" {
		t.Errorf("mounts.persistent.path = %v, want /data", pers["path"])
	}
	if body["disk"] != float64(20) {
		t.Errorf("disk = %v, want 20", body["disk"])
	}
	if envMap, ok := body["env"].(map[string]interface{}); ok {
		if len(envMap) != 2 {
			t.Errorf("env map length = %d, want 2", len(envMap))
		}
	} else {
		t.Errorf("env not found or not a map: %v", body["env"])
	}
}

// TestPodRead_MapsCorrectlyNamedFields is green coverage of the Read mappings that
// DO work (correctly-named top-level fields), complementing the field-mapping bug
// (CE-1658) which covers the mis-named ones.
func TestPodRead_MapsCorrectlyNamedFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"costPerHr":0.5,"memoryInGb":32,"volumeInGb":40,"containerDiskInGb":50,"machineId":"m9"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModelWithListTypes()
	m.Id = types.StringValue("pod-1")
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: PodResourceSchema(context.Background())}}
	(&PodResource{}).Read(context.Background(), resource.ReadRequest{State: podState(t, m)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var out PodModel
	resp.State.Get(context.Background(), &out)
	if out.CostPerHr.ValueFloat64() != 0.5 {
		t.Errorf("cost_per_hr = %v, want 0.5", out.CostPerHr.ValueFloat64())
	}
	if out.MemoryInGb.ValueFloat64() != 32 {
		t.Errorf("memory_in_gb = %v, want 32", out.MemoryInGb.ValueFloat64())
	}
	if out.VolumeInGb.ValueFloat64() != 40 {
		t.Errorf("volume_in_gb = %v, want 40", out.VolumeInGb.ValueFloat64())
	}
	if out.ContainerDiskInGb.ValueInt64() != 50 {
		t.Errorf("container_disk_in_gb = %v, want 50", out.ContainerDiskInGb.ValueInt64())
	}
	if out.MachineId.ValueString() != "m9" {
		t.Errorf("machine_id = %q, want m9", out.MachineId.ValueString())
	}
}



func TestPodCreate_ConditionalBodyFields(t *testing.T) {
	capture := func(t *testing.T, m PodModel) map[string]interface{} {
		var body map[string]interface{}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &body)
		_, _ = w.Write([]byte(`{"data":{"pod":{"id":"pod-x"}},"meta":{"requestId":"test"},"error":null}`))
		}))
		defer srv.Close()
		t.Setenv("RUNPOD_API_KEY", "testkey123")
		t.Setenv("RUNPOD_BASE_URL", srv.URL)
	r := &PodResource{}
	configureResourceWithTestClient(t, r, srv)
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodResourceSchema(context.Background())}}
	r.Create(context.Background(), resource.CreateRequest{Config: podConfig(t, m)}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}
		return body
	}

	t.Run("cloud and volume present when set", func(t *testing.T) {
		m := baseModelWithListTypes()
		m.Name = types.StringValue("p")
		m.ImageName = types.StringValue("img")
		m.CloudType = types.StringValue("SECURE")
		m.VolumeInGb = types.Float64Value(50)
		m.Interruptible = types.BoolValue(false)
		body := capture(t, m)
		
		if body["cloud"] != "SECURE" {
			t.Errorf("cloud = %v, want SECURE", body["cloud"])
		}
		
		mounts, ok := body["mounts"].(map[string]interface{})
		if !ok {
			t.Fatalf("mounts object not found: %v", body["mounts"])
		}
		pers, ok := mounts["persistent"].(map[string]interface{})
		if !ok || pers["size"] != float64(50) {
			t.Errorf("mounts.persistent = %v, want size=50", mounts["persistent"])
		}

	})

	t.Run("cloud_type and volume absent when unset/zero", func(t *testing.T) {
		m := baseModelWithListTypes()
		m.Name = types.StringValue("p")
		m.ImageName = types.StringValue("img")
		m.VolumeInGb = types.Float64Value(0)
		body := capture(t, m)
		if _, ok := body["cloudType"]; ok {
			t.Error("cloudType should be absent when not set")
		}
		if _, ok := body["volumeInGb"]; ok {
			t.Error("volumeInGb should be absent when zero")
		}
	})
	t.Run("volumeEncrypted present when set", func(t *testing.T) {
		m := baseModelWithListTypes()
		m.Name = types.StringValue("p")
		m.ImageName = types.StringValue("img")
		m.VolumeEncrypted = types.BoolValue(true)
		body := capture(t, m)
		if body["volumeEncrypted"] != true {
			t.Errorf("volumeEncrypted = %v, want true", body["volumeEncrypted"])
		}
	})

	t.Run("volumeEncrypted absent when unset", func(t *testing.T) {
		m := baseModelWithListTypes()
		m.Name = types.StringValue("p")
		m.ImageName = types.StringValue("img")
		body := capture(t, m)
		if _, ok := body["volumeEncrypted"]; ok {
			t.Error("volumeEncrypted should be absent when not set")
		}
	})
}

func TestPodDelete_Success(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent) // 204
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModelWithListTypes()
	m.Id = types.StringValue("pod-1")

	resp := &resource.DeleteResponse{State: podState(t, m)}
	(&PodResource{}).Delete(context.Background(), resource.DeleteRequest{State: podState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotMethod != "DELETE" || gotPath != "/v2/pods/pod-1" {
		t.Errorf("request = %s %s, want DELETE /v2/pods/pod-1", gotMethod, gotPath)
	}
}

func TestPodDelete_Non204_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("delete failed"))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModelWithListTypes()
	m.Id = types.StringValue("pod-1")

	resp := &resource.DeleteResponse{State: podState(t, m)}
	(&PodResource{}).Delete(context.Background(), resource.DeleteRequest{State: podState(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when delete returns a non-204 status")
	}
}

func TestPodCreate_OmittedStartSshStartJupyter_DefaultsToFalse(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/templates/tmpl-1" {
			_, _ = w.Write([]byte(`{"image":"runpod/pytorch:2.1.1","args":"","ports":["8888/http","22/tcp"],"env":{},"disk":50,"mounts":{}}`))
			return
		}
		
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"id":"pod-1"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModelWithListTypes()
	m.Name = types.StringValue("test-pod")
	m.TemplateId = types.StringValue("tmpl-1")
	m.GpuCount = types.Int64Value(1)
	m.GpuTypeId = types.StringValue("NVIDIA A100 80GB")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodResourceSchema(context.Background())}}
	(&PodResource{}).Create(context.Background(), resource.CreateRequest{Config: podConfig(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if _, ok := gotBody["startSsh"]; ok {
		t.Error("startSsh should not be in request body when omitted")
	}
	if _, ok := gotBody["startJupyter"]; ok {
		t.Error("startJupyter should not be in request body when omitted")
	}

	var out PodModel
	resp.State.Get(context.Background(), &out)
	if out.StartSsh.ValueBool() != false {
		t.Errorf("startSsh in state = %v, want false", out.StartSsh.ValueBool())
	}
	if out.StartJupyter.ValueBool() != false {
		t.Errorf("startJupyter in state = %v, want false", out.StartJupyter.ValueBool())
	}
}

// TestPodCreate_NetworkVolumeMount verifies the v2 network mount shape:
// mounts = { network: [ { volumeId, path } ] } (maxItems 1 in v2 today).
func TestPodCreate_NetworkVolumeMount(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"data":{"pod":{"id":"pod-nv"}},"meta":{"requestId":"test"},"error":null}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModelWithListTypes()
	m.Name = types.StringValue("nv-pod")
	m.ImageName = types.StringValue("img:latest")
	m.GpuCount = types.Int64Value(1)
	m.GpuTypeId = types.StringValue("NVIDIA A100 80GB")
	m.NetworkVolumeIds = strList("nv-1")
	m.VolumeMountPath = types.StringValue("/data")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodResourceSchema(context.Background())}}
	(&PodResource{}).Create(context.Background(), resource.CreateRequest{Config: podConfig(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	mounts, ok := gotBody["mounts"].(map[string]interface{})
	if !ok {
		t.Fatalf("mounts object not found: %v", gotBody["mounts"])
	}
	network, ok := mounts["network"].([]interface{})
	if !ok || len(network) != 1 {
		t.Fatalf("mounts.network = %v, want one entry", mounts)
	}
	nm := network[0].(map[string]interface{})
	if nm["volumeId"] != "nv-1" {
		t.Errorf("mounts.network[0].volumeId = %v, want nv-1", nm["volumeId"])
	}
	if nm["path"] != "/data" {
		t.Errorf("mounts.network[0].path = %v, want /data", nm["path"])
	}
}

// TestPodCreate_SecondNetworkVolumeRejected documents v2's maxItems-1 network mount.
func TestPodCreate_SecondNetworkVolumeRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"pod":{"id":"pod-nv-err"}},"meta":{},"error":null}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModelWithListTypes()
	m.Name = types.StringValue("nv-err")
	m.ImageName = types.StringValue("img:latest")
	m.GpuCount = types.Int64Value(1)
	m.GpuTypeId = types.StringValue("NVIDIA A100 80GB")
	m.NetworkVolumeIds = strList("nv-1", "nv-2")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodResourceSchema(context.Background())}}
	(&PodResource{}).Create(context.Background(), resource.CreateRequest{Config: podConfig(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected Invalid Configuration for 2 network volumes")
	}
}

// TestPodCreate_WithSingleNetworkVolume verifies the deprecated scalar maps to
// the v2 network mount (mutex with volume_in_gb).
func TestPodCreate_WithSingleNetworkVolume(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"data":{"pod":{"id":"pod-single-nv"}},"meta":{"requestId":"test"},"error":null}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModelWithListTypes()
	m.Name = types.StringValue("single-nv-pod")
	m.ImageName = types.StringValue("img:latest")
	m.GpuCount = types.Int64Value(1)
	m.GpuTypeId = types.StringValue("NVIDIA A100 80GB")
	m.NetworkVolumeId = types.StringValue("nv-single")
	m.VolumeMountPath = types.StringValue("/data")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodResourceSchema(context.Background())}}
	(&PodResource{}).Create(context.Background(), resource.CreateRequest{Config: podConfig(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	mounts, ok := gotBody["mounts"].(map[string]interface{})
	if !ok {
		t.Fatalf("mounts object not found: %v", gotBody["mounts"])
	}
	network, ok := mounts["network"].([]interface{})
	if !ok || len(network) != 1 {
		t.Fatalf("mounts.network = %v, want one entry", mounts)
	}
	if nm := network[0].(map[string]interface{}); nm["volumeId"] != "nv-single" {
		t.Errorf("mounts.network[0].volumeId = %v, want nv-single", nm["volumeId"])
	}
}

// TestPodCreate_MixedNetworkVolumeFields documents that combining the
// deprecated scalar network_volume_id with the list exceeds v2's limit.
func TestPodCreate_MixedNetworkVolumeFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"pod":{"id":"pod-mixed-nv"}},"meta":{},"error":null}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModelWithListTypes()
	m.Name = types.StringValue("mixed-nv-pod")
	m.ImageName = types.StringValue("img:latest")
	m.GpuCount = types.Int64Value(1)
	m.GpuTypeId = types.StringValue("NVIDIA A100 80GB")

	m.NetworkVolumeId = types.StringValue("nv-old")
	m.NetworkVolumeIds = strList("nv-new-1")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodResourceSchema(context.Background())}}
	(&PodResource{}).Create(context.Background(), resource.CreateRequest{Config: podConfig(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected Invalid Configuration: combined fields exceed v2's one-network-volume limit")
	}
}

// TestPodUpdate_NetworkVolume verifies Update builds the v2 network mount.
func TestPodUpdate_NetworkVolume(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"id":"pod-single-nv"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	prior := baseModelWithListTypes()
	prior.Id = types.StringValue("pod-single-nv")

	desired := baseModelWithListTypes()
	desired.Id = types.StringValue("pod-single-nv")
	desired.NetworkVolumeIds = strList("nv-1")

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: PodResourceSchema(context.Background())}}
	(&PodResource{}).Update(context.Background(), resource.UpdateRequest{
		Config: podConfig(t, desired),
		State:  podState(t, prior),
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	mounts, ok := gotBody["mounts"].(map[string]interface{})
	if !ok {
		t.Fatalf("mounts not in PATCH body: %v", gotBody)
	}
	network, ok := mounts["network"].([]interface{})
	if !ok || len(network) != 1 {
		t.Fatalf("mounts.network = %v, want one entry", mounts)
	}
	if nm := network[0].(map[string]interface{}); nm["volumeId"] != "nv-1" {
		t.Errorf("mounts.network[0].volumeId = %v, want nv-1", nm["volumeId"])
	}
}

// strList is a helper to create a types.List from string values
func strList(vals ...string) types.List {
	elems := make([]attr.Value, len(vals))
	for i, v := range vals {
		elems[i] = types.StringValue(v)
	}
	return types.ListValueMust(types.StringType, elems)
}

// baseModelWithListTypes returns a PodModel with properly typed List fields
func baseModelWithListTypes() PodModel {
	return PodModel{
		Env:              types.ListNull(types.StringType),
		DockerEntrypoint: types.ListNull(types.StringType),
		DockerStartCmd:   types.ListNull(types.StringType),
		NetworkVolumeIds: types.ListNull(types.StringType),
	}
}
