package resource_pod

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestAccPodLifecycle_riab drives the real PodResource Create→Read→Delete against
// a live local rphttp v1 endpoint (runpod-in-a-box). It is gated on RIAB_ACC=1
// and requires:
//
//	RUNPOD_BASE_URL=http://localhost:8081/v1
//	RUNPOD_API_KEY=$TEST_USER_JWT   (mint via riab scripts/mint-test-jwts.sh)
//
// Unlike the httptest unit tests, this exercises the actual HTTP path, JWT auth,
// and real API response shapes. Only the REST pod resource is exercised — the
// GraphQL resources/data sources are non-functional until CE-1652 is fixed.
func TestAccPodLifecycle_riab(t *testing.T) {
	if os.Getenv("RIAB_ACC") != "1" {
		t.Skip("set RIAB_ACC=1 + RUNPOD_BASE_URL + RUNPOD_API_KEY to run the live riab pod lifecycle")
	}
	if os.Getenv("RUNPOD_API_KEY") == "" || os.Getenv("RUNPOD_BASE_URL") == "" {
		t.Fatal("RUNPOD_API_KEY and RUNPOD_BASE_URL must be set for the acceptance test")
	}

	ctx := context.Background()
	sch := PodResourceSchema(ctx)

	// --- Create ---
	m := baseModel()
	m.Name = types.StringValue("tf-acc-lifecycle")
	m.ImageName = types.StringValue("runpod/base:0.0.0")
	m.GpuCount = types.Int64Value(1)
	m.CloudType = types.StringValue("SECURE")

	cResp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	(&PodResource{}).Create(ctx, resource.CreateRequest{Config: podConfig(t, m)}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics)
	}
	var created PodModel
	cResp.State.Get(ctx, &created)
	id := created.Id.ValueString()
	if id == "" {
		t.Fatal("Create returned an empty pod id")
	}
	t.Logf("created pod id=%s", id)

	// Always clean up, even if Read fails.
	defer func() {
		dResp := &resource.DeleteResponse{State: podState(t, created)}
		(&PodResource{}).Delete(ctx, resource.DeleteRequest{State: podState(t, created)}, dResp)
		if dResp.Diagnostics.HasError() {
			t.Errorf("Delete: %v", dResp.Diagnostics)
		} else {
			t.Logf("deleted pod id=%s", id)
		}
	}()

	// --- Read (exercises the CE-1650 fix against a real server) ---
	rResp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
	(&PodResource{}).Read(ctx, resource.ReadRequest{State: podState(t, created)}, rResp)
	if rResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", rResp.Diagnostics)
	}
	var readBack PodModel
	rResp.State.Get(ctx, &readBack)

	// costPerHr proves the GET succeeded and the body parsed.
	if readBack.CostPerHr.ValueFloat64() == 0 {
		t.Error("Read returned costPerHr=0; expected a populated cost from the live API")
	}

	// DESIRED behavior. FAILS today due to CE-1658: the v1 API returns
	// "desiredStatus" but Read maps the non-existent top-level "status", so it
	// comes back empty. Green here == CE-1658 fixed.
	if got := readBack.Status.ValueString(); got != "RUNNING" {
		t.Errorf("Status = %q, want \"RUNNING\" — FAILS until CE-1658 is fixed: Read maps 'status' but the v1 API returns 'desiredStatus'", got)
	}
	t.Logf("read pod id=%s status=%q costPerHr=%v", id, readBack.Status.ValueString(), readBack.CostPerHr.ValueFloat64())
}

func skipUnlessRiab(t *testing.T) {
	t.Helper()
	if os.Getenv("RIAB_ACC") != "1" {
		t.Skip("set RIAB_ACC=1 + RUNPOD_BASE_URL + RUNPOD_API_KEY to run live riab tests")
	}
	if os.Getenv("RUNPOD_API_KEY") == "" || os.Getenv("RUNPOD_BASE_URL") == "" {
		t.Fatal("RUNPOD_API_KEY and RUNPOD_BASE_URL must be set")
	}
}

// createPod runs Create and returns the resulting model + a cleanup func.
func createPod(t *testing.T, m PodModel) (PodModel, func()) {
	t.Helper()
	ctx := context.Background()
	cResp := &resource.CreateResponse{State: tfsdk.State{Schema: PodResourceSchema(ctx)}}
	(&PodResource{}).Create(ctx, resource.CreateRequest{Config: podConfig(t, m)}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics)
	}
	var created PodModel
	cResp.State.Get(ctx, &created)
	if created.Id.ValueString() == "" {
		t.Fatal("Create returned empty id")
	}
	return created, func() {
		dResp := &resource.DeleteResponse{State: podState(t, created)}
		(&PodResource{}).Delete(ctx, resource.DeleteRequest{State: podState(t, created)}, dResp)
		if dResp.Diagnostics.HasError() {
			t.Errorf("cleanup Delete(%s): %v", created.Id.ValueString(), dResp.Diagnostics)
		}
	}
}

// TestAccPodCreateVariations_riab covers the pod-creation variations the REST
// API supports: image_name, an explicit volume, and (when a template is
// available) template_id. The template_id case is skipped unless
// RUNPOD_TEST_TEMPLATE_ID is set, since riab seeds no templates.
func TestAccPodCreateVariations_riab(t *testing.T) {
	skipUnlessRiab(t)
	ctx := context.Background()
	sch := PodResourceSchema(ctx)

	cases := []struct {
		name   string
		mutate func(*PodModel)
		check  func(*testing.T, PodModel)
	}{
		{"image_name", func(m *PodModel) { m.ImageName = types.StringValue("runpod/test:latest") }, nil},
		{"template_id", func(m *PodModel) { m.TemplateId = types.StringValue(os.Getenv("RUNPOD_TEST_TEMPLATE_ID")) }, nil},
		{"with_volume", func(m *PodModel) {
			m.ImageName = types.StringValue("runpod/test:latest")
			m.VolumeInGb = types.Float64Value(40)
		}, func(t *testing.T, rb PodModel) {
			if rb.VolumeInGb.ValueFloat64() <= 0 {
				t.Errorf("expected a positive volumeInGb read back, got %v", rb.VolumeInGb.ValueFloat64())
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "template_id" && os.Getenv("RUNPOD_TEST_TEMPLATE_ID") == "" {
				t.Skip("set RUNPOD_TEST_TEMPLATE_ID to a seeded template id to exercise the template_id path (riab seeds none)")
			}
			m := baseModel()
			m.Name = types.StringValue("tf-acc-" + tc.name)
			m.GpuCount = types.Int64Value(1)
			tc.mutate(&m)

			created, cleanup := createPod(t, m)
			defer cleanup()
			t.Logf("[%s] created id=%s", tc.name, created.Id.ValueString())

			rResp := &resource.ReadResponse{State: tfsdk.State{Schema: sch}}
			(&PodResource{}).Read(ctx, resource.ReadRequest{State: podState(t, created)}, rResp)
			if rResp.Diagnostics.HasError() {
				t.Fatalf("[%s] Read: %v", tc.name, rResp.Diagnostics)
			}
			var rb PodModel
			rResp.State.Get(ctx, &rb)
			if rb.CostPerHr.ValueFloat64() == 0 {
				t.Errorf("[%s] Read returned costPerHr=0", tc.name)
			}
			if tc.check != nil {
				tc.check(t, rb)
			}
			t.Logf("[%s] read costPerHr=%v volumeInGb=%v", tc.name, rb.CostPerHr.ValueFloat64(), rb.VolumeInGb.ValueFloat64())
		})
	}
}

// TestAccPodRead_NotFound_riab exercises CE-1654 against a real 404: Read emits a
// warning and keeps the resource in state (instead of RemoveResource).
func TestAccPodRead_NotFound_riab(t *testing.T) {
	skipUnlessRiab(t)
	ctx := context.Background()
	m := baseModel()
	m.Id = types.StringValue("doesnotexist-acc")

	rResp := &resource.ReadResponse{State: podState(t, m)}
	(&PodResource{}).Read(ctx, resource.ReadRequest{State: podState(t, m)}, rResp)

	if rResp.Diagnostics.HasError() {
		t.Fatalf("404 should be a warning, not an error: %v", rResp.Diagnostics)
	}
	// DESIRED behavior: a 404 should remove the resource from state so it gets
	// recreated. FAILS today due to CE-1654: Read only warns and leaves
	// stale state. Green here == CE-1654 fixed.
	if !rResp.State.Raw.IsNull() {
		t.Error("state was NOT removed on 404 — FAILS until CE-1654 is fixed (Read should call resp.State.RemoveResource)")
	}
}

// TestAccPodDelete_NotFound_riab: deleting a nonexistent pod returns a non-204
// status, which the provider surfaces as an error.
func TestAccPodDelete_NotFound_riab(t *testing.T) {
	skipUnlessRiab(t)
	ctx := context.Background()
	m := baseModel()
	m.Id = types.StringValue("doesnotexist-acc")

	dResp := &resource.DeleteResponse{State: podState(t, m)}
	(&PodResource{}).Delete(ctx, resource.DeleteRequest{State: podState(t, m)}, dResp)

	if !dResp.Diagnostics.HasError() {
		t.Error("expected an error deleting a nonexistent pod (non-204 status)")
	}
}
