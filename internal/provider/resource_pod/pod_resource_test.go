package resource_pod

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// baseModel returns a PodModel with all attributes null except the env list,
// which must carry an element type to be a valid value.
func baseModel() PodModel {
	return PodModel{Env: types.ListNull(types.StringType)}
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

func TestPodCreate_BothTemplateAndImage_Errors(t *testing.T) {
	m := baseModel()
	m.TemplateId = types.StringValue("tmpl-1")
	m.ImageName = types.StringValue("img:latest")

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodResourceSchema(context.Background())}}
	(&PodResource{}).Create(context.Background(), resource.CreateRequest{Config: podConfig(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error when both template_id and image_name are set")
	}
}

func TestPodCreate_NeitherTemplateNorImage_Errors(t *testing.T) {
	m := baseModel() // both null

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
		_, _ = w.Write([]byte(`{"id":"pod-xyz"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModel()
	m.Name = types.StringValue("my-pod")
	m.ImageName = types.StringValue("img:latest")
	m.GpuCount = types.Int64Value(2)

	sch := PodResourceSchema(context.Background())
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&PodResource{}).Create(context.Background(), resource.CreateRequest{Config: podConfig(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotMethod != "POST" || gotPath != "/pods" {
		t.Errorf("request = %s %s, want POST /pods", gotMethod, gotPath)
	}
	if gotBody["imageName"] != "img:latest" {
		t.Errorf("imageName = %v, want img:latest", gotBody["imageName"])
	}
	if _, ok := gotBody["templateId"]; ok {
		t.Error("templateId should be absent for an image-name deploy")
	}
	if gotBody["name"] != "my-pod" {
		t.Errorf("name = %v, want my-pod", gotBody["name"])
	}
	if gotBody["gpuCount"] != float64(2) {
		t.Errorf("gpuCount = %v, want 2", gotBody["gpuCount"])
	}

	var out PodModel
	resp.State.Get(context.Background(), &out)
	if out.Id.ValueString() != "pod-xyz" {
		t.Errorf("id = %q, want pod-xyz", out.Id.ValueString())
	}
}

func TestPodCreate_TemplateID_BuildsBody(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"id":"pod-tmpl"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModel()
	m.Name = types.StringValue("tmpl-pod")
	m.TemplateId = types.StringValue("tmpl-1")
	m.GpuCount = types.Int64Value(1)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodResourceSchema(context.Background())}}
	(&PodResource{}).Create(context.Background(), resource.CreateRequest{Config: podConfig(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotBody["templateId"] != "tmpl-1" {
		t.Errorf("templateId = %v, want tmpl-1", gotBody["templateId"])
	}
	if _, ok := gotBody["imageName"]; ok {
		t.Error("imageName should be absent for a template deploy")
	}
}

// TestPodRead_UsesConfiguredBaseURL exercises the CE-1650 fix: Read must honor
// RUNPOD_BASE_URL, not the hardcoded prod URL. Without the fix this request
// would never reach the test server.
func TestPodRead_UsesConfiguredBaseURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"status":"RUNNING","costPerHr":0.5}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModel()
	m.Id = types.StringValue("pod-1")

	sch := PodResourceSchema(context.Background())
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&PodResource{}).Read(context.Background(), resource.ReadRequest{State: podState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotPath != "/pods/pod-1" {
		t.Errorf("Read hit path %q, want /pods/pod-1 (CE-1650: must honor RUNPOD_BASE_URL)", gotPath)
	}
	var out PodModel
	resp.State.Get(context.Background(), &out)
	if out.Status.ValueString() != "RUNNING" {
		t.Errorf("status = %q, want RUNNING", out.Status.ValueString())
	}
}

// TestPodRead_404_DoesNotRemoveState is a characterization test for bug CE-1654.
// When a pod is gone (404), Read only emits a warning and returns — it never
// calls resp.State.RemoveResource, so the deleted pod stays in Terraform state
// (plan stays dirty; it's never recreated).
//
// This asserts current behavior: no error, and state NOT removed. When CE-1654 is
// fixed (RemoveResource on 404), resp.State.Raw becomes null — flip this test.
func TestPodRead_404_DoesNotRemoveState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"pod not found"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModel()
	m.Id = types.StringValue("gone")

	resp := &resource.ReadResponse{State: podState(t, m)} // pre-populated, mirrors framework
	(&PodResource{}).Read(context.Background(), resource.ReadRequest{State: podState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("404 should be a warning, not an error: %v", resp.Diagnostics)
	}
	if resp.State.Raw.IsNull() {
		t.Error("state was removed on 404 — CE-1654 appears FIXED; flip this test to assert removal")
	}
}

func TestPodCreate_NoIDInResponse_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":"accepted but no id"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModel()
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

	m := baseModel()
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

// TestPodUpdate_IsNoOp characterizes CE-1655: PodResource.Update has an empty
// body, so an in-place change is silently dropped — no API call is made and no
// new state is written. When CE-1655 is fixed (Update actually applies changes),
// this test will need updating.
func TestPodUpdate_IsNoOp(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		_, _ = w.Write([]byte(`{"id":"pod-1"}`))
	}))
	defer srv.Close()
	t.Setenv("RUNPOD_API_KEY", "testkey123")
	t.Setenv("RUNPOD_BASE_URL", srv.URL)

	m := baseModel()
	m.Id = types.StringValue("pod-1")
	m.Name = types.StringValue("changed-name")

	sch := PodResourceSchema(context.Background())
	plan := tfsdk.Plan{Schema: sch}
	if d := plan.Set(context.Background(), &m); d.HasError() {
		t.Fatalf("building plan: %v", d)
	}
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	(&PodResource{}).Update(context.Background(), resource.UpdateRequest{Plan: plan, State: podState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("empty Update should not error: %v", resp.Diagnostics)
	}
	if hit {
		t.Error("Update made an API call — it is no longer a no-op (CE-1655 may be fixed)")
	}
	if !resp.State.Raw.IsNull() {
		t.Error("Update wrote state — it is no longer a pure no-op (CE-1655 may be fixed)")
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

	m := baseModel()
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
	// CE-1658: these stay empty because Read maps field names the v1 API doesn't use.
	if out.Status.ValueString() != "" {
		t.Errorf("status = %q; expected empty (CE-1658: Read maps 'status', API returns 'desiredStatus') — CE-1658 may be FIXED", out.Status.ValueString())
	}
	if out.CreatedAt.ValueString() != "" {
		t.Errorf("created_at = %q; expected empty (CE-1658: Read maps 'created_at', API returns 'createdAt') — CE-1658 may be FIXED", out.CreatedAt.ValueString())
	}
	if out.GpuTypeId.ValueString() != "" {
		t.Errorf("gpu_type_id = %q; expected empty (CE-1658: not read from nested machine.gpuTypeId) — CE-1658 may be FIXED", out.GpuTypeId.ValueString())
	}
}

func TestPodCreate_ConditionalBodyFields(t *testing.T) {
	capture := func(t *testing.T, m PodModel) map[string]interface{} {
		var body map[string]interface{}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &body)
			_, _ = w.Write([]byte(`{"id":"pod-x"}`))
		}))
		defer srv.Close()
		t.Setenv("RUNPOD_API_KEY", "testkey123")
		t.Setenv("RUNPOD_BASE_URL", srv.URL)
		resp := &resource.CreateResponse{State: tfsdk.State{Schema: PodResourceSchema(context.Background())}}
		(&PodResource{}).Create(context.Background(), resource.CreateRequest{Config: podConfig(t, m)}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}
		return body
	}

	t.Run("cloud_type and volume present when set", func(t *testing.T) {
		m := baseModel()
		m.Name = types.StringValue("p")
		m.ImageName = types.StringValue("img")
		m.CloudType = types.StringValue("SECURE")
		m.VolumeInGb = types.Float64Value(50)
		body := capture(t, m)
		if body["cloudType"] != "SECURE" {
			t.Errorf("cloudType = %v, want SECURE", body["cloudType"])
		}
		if body["volumeInGb"] != float64(50) {
			t.Errorf("volumeInGb = %v, want 50", body["volumeInGb"])
		}
	})

	t.Run("cloud_type and volume absent when unset/zero", func(t *testing.T) {
		m := baseModel()
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

	m := baseModel()
	m.Id = types.StringValue("pod-1")

	resp := &resource.DeleteResponse{State: podState(t, m)}
	(&PodResource{}).Delete(context.Background(), resource.DeleteRequest{State: podState(t, m)}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotMethod != "DELETE" || gotPath != "/pods/pod-1" {
		t.Errorf("request = %s %s, want DELETE /pods/pod-1", gotMethod, gotPath)
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

	m := baseModel()
	m.Id = types.StringValue("pod-1")

	resp := &resource.DeleteResponse{State: podState(t, m)}
	(&PodResource{}).Delete(context.Background(), resource.DeleteRequest{State: podState(t, m)}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when delete returns a non-204 status")
	}
}
