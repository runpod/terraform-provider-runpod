package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// The deprecated 1.x attributes of runpod_serverless_endpoint that the REST
// v2 endpoint object does not carry. workers and version come from the REST
// v2 workers route (GET /v2/serverless/{id}/workers), users from the GraphQL
// shim. template_version and the two priorities have a source in neither API:
// the REST v2 Template has no version, the GraphQL Endpoint.template is a
// PodTemplate whose only version link is templateVersionId, pointing at a
// TemplateVersion whose version is a release label string rather than the
// 1.x integer, and no endpoint input (REST v2 CreateEndpointRequest and
// UpdateEndpointRequest, GraphQL EndpointInput) orders GPU types or CPU
// flavors.
//
// compute_type is not among them: it restates the GPU or CPU block the v2
// endpoint already reports, so flattenEndpoint derives it and ValidateConfig
// rejects a configured value that contradicts the block.

// legacyGpuTypePriorityReason and legacyCpuFlavorPriorityReason complete the
// warnUnappliedLegacyInput sentence for the two priorities.
const (
	legacyGpuTypePriorityReason = "neither REST v2 nor the GraphQL API has a setting for how the scheduler orders " +
		"the GPU types inside the selected pools (gpu_pools and excluded_types choose the types, not their order)"
	legacyCpuFlavorPriorityReason = "neither REST v2 nor the GraphQL API has a setting for how the scheduler orders " +
		"the configured CPU flavors (cpu chooses the flavors, not their order)"
)

// warnUnappliedLegacyEndpointInputs warns at plan time for each configured
// priority. The value stays in state as configured, so a 1.x configuration
// keeps planning clean, but it is never sent.
func warnUnappliedLegacyEndpointInputs(diags *diag.Diagnostics, config serverlessEndpointResourceModel) {
	warnUnappliedLegacyInput(diags, path.Root("gpu_type_priority"), config.GpuTypePriority, legacyGpuTypePriorityReason)
	warnUnappliedLegacyInput(diags, path.Root("cpu_flavor_priority"), config.CpuFlavorPriority, legacyCpuFlavorPriorityReason)
}

// flattenLegacyEndpointWorkers maps the REST v2 workers listing onto the 1.x
// version and workers read-backs. REST v2 lists an endpoint's pods as its
// workers, so a worker's id is its pod's id and fills pod_id too. No API
// reports a last-busy counter for a worker (REST v2 Worker, GraphQL Pod), so
// last_busy_ms stays null, and both report uptime in whole seconds, which the
// 1.x millisecond attribute carries multiplied out.
func flattenLegacyEndpointWorkers(ctx context.Context, response *sdktypes.ListEndpointWorkersResponse, model *serverlessEndpointResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	model.Version = int64PointerValue(response.EndpointVersion)

	// An endpoint with no active workers reports the same null the failed
	// read does: one state for "nothing here", not an empty list a
	// configuration would have to distinguish from it.
	if len(response.Workers) == 0 {
		model.Workers = types.ListNull(endpointLegacyWorkerObjectType)
		return diags
	}

	workers := make([]endpointLegacyWorkerModel, 0, len(response.Workers))
	for _, worker := range response.Workers {
		status := string(worker.Status)
		item := endpointLegacyWorkerModel{
			Id:         legacyNonEmptyString(&worker.Id),
			PodId:      legacyNonEmptyString(&worker.Id),
			Status:     legacyNonEmptyString(&status),
			UptimeMs:   types.Int64Null(),
			StartTime:  types.StringNull(),
			LastBusyMs: types.Int64Null(),
		}
		if worker.UptimeSeconds != nil {
			item.UptimeMs = types.Int64Value(int64(*worker.UptimeSeconds) * 1000)
		}
		if worker.StartedAt != nil {
			item.StartTime = types.StringValue(worker.StartedAt.Format(time.RFC3339))
		}
		workers = append(workers, item)
	}
	workerList, workerDiags := types.ListValueFrom(ctx, endpointLegacyWorkerObjectType, workers)
	diags.Append(workerDiags...)
	model.Workers = workerList
	return diags
}

// flattenLegacyEndpointOwner maps the GraphQL endpoint owner onto users. The
// 1.x attribute wrapped the single owner in a list, and that shape is what a
// 1.x configuration indexes into.
func flattenLegacyEndpointOwner(ctx context.Context, owner *string, model *serverlessEndpointResourceModel) diag.Diagnostics {
	if owner == nil || *owner == "" {
		model.Users = types.ListNull(types.StringType)
		return nil
	}
	users, diags := types.ListValueFrom(ctx, types.StringType, []string{*owner})
	model.Users = users
	return diags
}

// clearLegacyEndpointWorkers nulls the two read-backs the workers route
// feeds, for the path where it did not answer.
func clearLegacyEndpointWorkers(model *serverlessEndpointResourceModel) {
	model.Version = types.Int64Null()
	model.Workers = types.ListNull(endpointLegacyWorkerObjectType)
}

// appendLegacyWorkersReadFailure records a workers read that did not answer.
// Nothing the configuration sets depends on it, so it warns rather than
// failing an apply whose endpoint change has already landed.
func appendLegacyWorkersReadFailure(diags *diag.Diagnostics, err error) {
	diags.AddWarning(
		"Reading the deprecated endpoint workers and version failed",
		fmt.Sprintf(
			"%s\n\nThe deprecated workers and version attributes are reported as null. The runpod_endpoint_workers data source reads the same workers and endpoint version, and reports a failed read as an error.",
			err.Error(),
		),
	)
}
