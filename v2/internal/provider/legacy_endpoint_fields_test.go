package provider

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

func TestFlattenLegacyEndpointWorkersMapsTheV2Listing(t *testing.T) {
	ctx := context.Background()
	version := 4
	uptime := 12
	started := time.Date(2026, 9, 17, 9, 0, 0, 0, time.UTC)

	var model serverlessEndpointResourceModel
	diags := flattenLegacyEndpointWorkers(ctx, &sdktypes.ListEndpointWorkersResponse{
		EndpointVersion: &version,
		Workers: []sdktypes.Worker{{
			Id:            "worker-1",
			Status:        "RUNNING",
			UptimeSeconds: &uptime,
			StartedAt:     &started,
		}},
	}, &model)
	if diags.HasError() {
		t.Fatal(diags)
	}

	if model.Version.ValueInt64() != 4 {
		t.Fatalf("version = %v, want the listing's endpointVersion", model.Version)
	}
	var workers []endpointLegacyWorkerModel
	if d := model.Workers.ElementsAs(ctx, &workers, false); d.HasError() {
		t.Fatal(d)
	}
	if len(workers) != 1 {
		t.Fatalf("len(workers) = %d, want 1", len(workers))
	}
	worker := workers[0]
	if worker.Id.ValueString() != "worker-1" || worker.Status.ValueString() != "RUNNING" {
		t.Fatalf("worker id/status: %+v", worker)
	}
	if worker.UptimeMs.ValueInt64() != 12000 {
		t.Fatalf("uptime_ms = %v, want the 12 seconds v2 reports as 12000", worker.UptimeMs)
	}
	if worker.StartTime.ValueString() != "2026-09-17T09:00:00Z" {
		t.Fatalf("start_time = %v, want startedAt in RFC 3339", worker.StartTime)
	}
	// REST v2 lists an endpoint's pods as its workers, so the worker id is the
	// pod id. No API reports a last-busy counter.
	if worker.PodId.ValueString() != "worker-1" {
		t.Fatalf("pod_id = %v, want the worker id", worker.PodId)
	}
	if !worker.LastBusyMs.IsNull() {
		t.Fatalf("last_busy_ms must be null: %+v", worker)
	}
}

func TestFlattenLegacyEndpointWorkersAbsentValuesAreNull(t *testing.T) {
	ctx := context.Background()

	// A fresh endpoint has no active workers and may report no version: both
	// go null, the same null a failed read produces, so "nothing here" has
	// one shape.
	var model serverlessEndpointResourceModel
	if diags := flattenLegacyEndpointWorkers(ctx, &sdktypes.ListEndpointWorkersResponse{}, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if !model.Version.IsNull() {
		t.Fatalf("version = %v, want null", model.Version)
	}
	if !model.Workers.IsNull() {
		t.Fatalf("an endpoint with no workers must report null, not an empty list: %v", model.Workers)
	}

	// A worker that is not placed yet has no uptime or start time.
	if diags := flattenLegacyEndpointWorkers(ctx, &sdktypes.ListEndpointWorkersResponse{
		Workers: []sdktypes.Worker{{Id: "worker-1", Status: "INITIALIZING"}},
	}, &model); diags.HasError() {
		t.Fatal(diags)
	}
	var workers []endpointLegacyWorkerModel
	if d := model.Workers.ElementsAs(ctx, &workers, false); d.HasError() {
		t.Fatal(d)
	}
	if len(workers) != 1 || !workers[0].UptimeMs.IsNull() || !workers[0].StartTime.IsNull() {
		t.Fatalf("an unplaced worker must report null uptime_ms and start_time: %+v", workers)
	}

	clearLegacyEndpointWorkers(&model)
	if !model.Version.IsNull() || !model.Workers.IsNull() {
		t.Fatalf("version and workers must be null after a failed read: %+v", model)
	}
}

func TestFlattenLegacyEndpointOwner(t *testing.T) {
	ctx := context.Background()
	owner := "user-1"
	empty := ""

	var model serverlessEndpointResourceModel
	if diags := flattenLegacyEndpointOwner(ctx, &owner, &model); diags.HasError() {
		t.Fatal(diags)
	}
	var users []string
	if d := model.Users.ElementsAs(ctx, &users, false); d.HasError() {
		t.Fatal(d)
	}
	if len(users) != 1 || users[0] != "user-1" {
		t.Fatalf("users must be the owner wrapped in a one-element list: %v", users)
	}

	for name, value := range map[string]*string{"no owner": nil, "empty owner": &empty} {
		if diags := flattenLegacyEndpointOwner(ctx, value, &model); diags.HasError() {
			t.Fatal(diags)
		}
		if !model.Users.IsNull() {
			t.Errorf("%s: users = %v, want null", name, model.Users)
		}
	}
}

func TestServerlessEndpointValidateConfigWarnsAboutEachConfiguredPriority(t *testing.T) {
	ctx := context.Background()
	r := &serverlessEndpointResource{}
	s, raw := shimResourceSchema(ctx, t, r)
	pools, diags := types.ListValueFrom(ctx, types.StringType, []string{"ADA_24"})
	if diags.HasError() {
		t.Fatal(diags)
	}

	for _, tc := range []struct {
		name  string
		set   map[string]attr.Value
		warns []string
	}{
		{"no priority", map[string]attr.Value{"gpu_pools": pools}, nil},
		{"gpu_type_priority", map[string]attr.Value{
			"gpu_pools": pools, "gpu_type_priority": types.StringValue("availability"),
		}, []string{"gpu_type_priority"}},
		{"gpu_type_priority not known until apply", map[string]attr.Value{
			"gpu_pools": pools, "gpu_type_priority": types.StringUnknown(),
		}, []string{"gpu_type_priority"}},
		{"cpu_flavor_priority", map[string]attr.Value{
			"cpu_flavor_priority": types.StringValue("availability"),
		}, []string{"cpu_flavor_priority"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &resource.ValidateConfigResponse{}
			r.ValidateConfig(ctx, resource.ValidateConfigRequest{
				Config: tfsdk.Config{Schema: s, Raw: shimObject(ctx, t, s, raw, tc.set)},
			}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("a priority is not applied, which is a warning, not an error: %v", resp.Diagnostics)
			}
			warnings := resp.Diagnostics.Warnings()
			if len(warnings) != len(tc.warns) {
				t.Fatalf("warnings = %v, want one each for %v", warnings, tc.warns)
			}
			for i, want := range tc.warns {
				withPath, ok := warnings[i].(diag.DiagnosticWithPath)
				if !ok || !withPath.Path().Equal(path.Root(want)) {
					t.Errorf("warning %d = %v, want it on %s", i, warnings[i], want)
				}
			}
		})
	}
}
