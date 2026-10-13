package provider

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
	"github.com/runpod/go-sdk/v2/lib/networkvolumes"
	"github.com/runpod/go-sdk/v2/lib/pods"
	"github.com/runpod/go-sdk/v2/lib/serverless"
	"github.com/runpod/go-sdk/v2/lib/templates"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// The pure API<->state mapping lives here so it is unit-testable without the
// plugin protocol machinery.

// The REST v1 shims. Each of these attributes worked in the 1.0.8 provider and
// has no REST v2 equivalent yet, so it is served from the legacy REST v1 host
// and marked deprecated. They go away when REST v2 grows the field.

const legacyShimDeprecation = "REST v1 shim: REST v2 has no equivalent yet; removed when it does"

const legacyShimDescriptionSuffix = "Served by the deprecated REST v1 shim and removed once REST v2 reports it."

// legacyShimPartialPatchCaveat goes on every attribute the shim writes. 1.0.8
// PATCHed the whole resource body on every update; the shim sends only the one
// or two keys it owns, a shape that release never sent, so the result depends
// on how the route merges a partial body. The attribute text says so.
const legacyShimPartialPatchCaveat = "The shim sends this field on its own, where the 1.x provider sent the whole " +
	"resource body, so applying it relies on the REST v1 route merging a partial request. That behaviour is " +
	"untested."

// appendLegacyReadFailure records a REST v1 read that did not answer. The
// legacy host is expected to disappear before these shims are removed, so a
// read must not break an apply: it warns and the attributes come back null.
// A configuration that sets one of the shim inputs reaches the legacy host
// through a write instead, and that failure is an error; see
// appendLegacyWriteFailure.
//
// A 404 is silent. The v1 route legitimately does not know a CPU pod, a
// cluster pod, or a template created through v2, and those objects have no
// legacy data rather than a failed read. Warning about it on every refresh,
// forever, would be noise the user cannot act on.
func appendLegacyReadFailure(diags *diag.Diagnostics, subject string, err error) {
	if legacy.IsNotFound(err) {
		return
	}
	diags.AddWarning(
		fmt.Sprintf("Reading the deprecated REST v1 %s failed", subject),
		fmt.Sprintf(
			"%s\n\nThe deprecated attributes served by the REST v1 shim are reported as null. Remove them from the configuration to stop depending on that host.",
			err.Error(),
		),
	)
}

// appendLegacyWriteFailure records a REST v1 write that did not land. Unlike a
// read, this is an error: the configuration asked for something only the legacy
// host can apply. Two shapes of failure are reported differently.
//
// A 404 says the legacy host does not know the object at all, the normal case
// for a resource created through REST v2, so the message names that cause
// instead of relaying a bare "templates/tpl_abc was not found", which says
// nothing about what the user can do next.
//
// A body that did not decode is not a failed write at all: the host accepted
// the change and answered something this client cannot read, so it warns and
// the read-backs go null. Failing the apply there would report a change the
// host already made as an error, and the next plan would re-send it.
func appendLegacyWriteFailure(diags *diag.Diagnostics, subject string, err error) {
	if legacy.IsDecodeFailure(err) {
		diags.AddWarning(
			fmt.Sprintf("Reading back the deprecated REST v1 %s settings failed", subject),
			fmt.Sprintf(
				"%s\n\nThe settings were applied; only the response could not be read, so the attributes the REST v1 shim reads back are reported as null until the next refresh.",
				err.Error(),
			),
		)
		return
	}
	if legacy.IsNotFound(err) {
		diags.AddError(
			fmt.Sprintf("Applying the deprecated REST v1 %s settings failed", subject),
			fmt.Sprintf(
				"The deprecated REST v1 host does not know this %s: %s\n\nA %s created through the REST v2 API is invisible to that host, so the attributes only it can apply cannot be applied at all. Drop the deprecated attribute from the configuration.",
				subject, err.Error(), subject,
			),
		)
		return
	}
	diags.AddError(
		fmt.Sprintf("Applying the deprecated REST v1 %s settings failed", subject),
		fmt.Sprintf("%s\n\nThese attributes are served by the REST v1 shim; the REST v2 resource itself was applied.", err.Error()),
	)
}

// legacyTruncatedInt64 truncates a REST v1 JSON number the way 1.0.8 did: it
// read every one of these as a float64 and truncated with int64(val), so a
// fractional value is data to round rather than a decode error. An absent field
// is null rather than zero. Shared by the shim read-backs and the three billing
// data sources, whose diskSpaceBilledGb and timeBilledMs take the same route.
func legacyTruncatedInt64(value *float64) types.Int64 {
	if value == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*value))
}

// legacyNonEmptyString carries a REST v1 string that 1.0.8 wrote only when the
// response reported it non-empty (`ok && val != ""`), leaving the attribute
// null otherwise, so a 1.x configuration's `!= null` check keeps answering
// what it did.
func legacyNonEmptyString(value *string) types.String {
	if value == nil || *value == "" {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

// flattenLegacyPod maps the REST v1 pod onto the two deprecated read-backs.
// The other REST v1 fields the 1.x provider read are served from REST v2
// instead: costPerHr and memoryInGb as cost and memory_in_gb, dockerEntrypoint
// and dockerStartCmd as docker_entrypoint and docker_start_cmd.
func flattenLegacyPod(pod *legacy.PodV1, model *podResourceModel) {
	// Absent is null, not "" or false: a CPU or cluster pod carries no
	// machineId, and 1.0.8 left both attributes unset in that case.
	model.MachineId = legacyNonEmptyString(pod.MachineID)
	model.VolumeEncrypted = types.BoolPointerValue(pod.VolumeEncrypted)
}

// clearLegacyPodFields nulls the shim read-backs, for the path where the REST
// v1 host did not answer. Both are Computed, so a null is a legal value and
// the apply survives the legacy host being gone.
func clearLegacyPodFields(model *podResourceModel) {
	model.MachineId = types.StringNull()
	model.VolumeEncrypted = types.BoolNull()
}

// expandLegacyEndpointPatch collects the two endpoint settings REST v2 has no
// field for. Only what the configuration set is carried, so an endpoint using
// neither never reaches the legacy host; EndpointPatchV1 reports that case as
// empty.
//
// compute_type is not one of them, although 1.0.8 sent it: the v2 create
// already fixes the family through the GPU or CPU block, flattenEndpoint
// derives the attribute from what v2 reports, and ValidateConfig rejects a
// configured value that contradicts the block. A v1 PATCH carrying it could
// change nothing, while its failure is an error, which would break every
// apply of the canonical 1.x migration shape (gpu_pools plus compute_type) as
// soon as the deprecated host answers 502, or 404 for an endpoint it does not
// know.
//
// The patch is read from the configuration rather than the plan, where an
// attribute the configuration leaves out is null on every operation.
func expandLegacyEndpointPatch(config serverlessEndpointResourceModel) legacy.EndpointPatchV1 {
	var patch legacy.EndpointPatchV1
	if !config.GpuTypePriority.IsNull() && !config.GpuTypePriority.IsUnknown() {
		patch.GpuTypePriority = config.GpuTypePriority.ValueStringPointer()
	}
	if !config.CpuFlavorPriority.IsNull() && !config.CpuFlavorPriority.IsUnknown() {
		patch.CpuFlavorPriority = config.CpuFlavorPriority.ValueStringPointer()
	}
	return patch
}

// flattenLegacyEndpoint maps the REST v1 endpoint onto the deprecated
// read-backs. compute_type is not among them: it says the same thing as the
// GPU/CPU block the v2 read already carries, so flattenEndpoint derives it and
// the legacy value is never allowed to contradict it.
func flattenLegacyEndpoint(ctx context.Context, endpoint *legacy.EndpointV1, model *serverlessEndpointResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	model.TemplateVersion = int64PointerValue(endpoint.TemplateVersion)
	model.Version = int64PointerValue(endpoint.Version)

	// v1 reported a single owner; the 1.x attribute wrapped it in a list, and
	// that shape is what a 1.x configuration indexes into.
	if endpoint.UserID == "" {
		model.Users = types.ListNull(types.StringType)
	} else {
		users, userDiags := types.ListValueFrom(ctx, types.StringType, []string{endpoint.UserID})
		diags.Append(userDiags...)
		model.Users = users
	}

	// An endpoint with no workers allocated reports the same null the failed
	// read does: one state for "nothing here", not an empty list a
	// configuration would have to distinguish from it.
	if len(endpoint.Workers) == 0 {
		model.Workers = types.ListNull(endpointLegacyWorkerObjectType)
		return diags
	}

	workers := make([]endpointLegacyWorkerModel, 0, len(endpoint.Workers))
	for _, worker := range endpoint.Workers {
		// A field the response omits is null, the way 1.0.8 left an absent
		// worker field unset; the two millisecond counters are truncated from
		// the JSON number it read them as.
		workers = append(workers, endpointLegacyWorkerModel{
			Id:         types.StringPointerValue(worker.ID),
			PodId:      types.StringPointerValue(worker.PodID),
			Status:     types.StringPointerValue(worker.Status),
			UptimeMs:   legacyTruncatedInt64(worker.UptimeMs),
			StartTime:  legacyNonEmptyString(worker.StartTime),
			LastBusyMs: legacyTruncatedInt64(worker.LastBusyMs),
		})
	}
	workerList, workerDiags := types.ListValueFrom(ctx, endpointLegacyWorkerObjectType, workers)
	diags.Append(workerDiags...)
	model.Workers = workerList

	return diags
}

// clearLegacyEndpointFields nulls the shim read-backs for the path where the
// REST v1 host did not answer. The two write-only priorities are not here:
// nothing reads them back, so they keep the value the configuration set.
func clearLegacyEndpointFields(model *serverlessEndpointResourceModel) {
	model.TemplateVersion = types.Int64Null()
	model.Users = types.ListNull(types.StringType)
	model.Version = types.Int64Null()
	model.Workers = types.ListNull(endpointLegacyWorkerObjectType)
}

// expandLegacyTemplatePatch collects the two template inputs REST v2 has no
// field for. An unset one stays out of the request rather than clearing what
// the template has.
//
// Both are Optional+Computed, so the patch is read from the configuration, not
// the plan: terraform core copies the prior state value into the plan for a
// computed attribute the configuration leaves out, which would PATCH the
// deprecated host on every update of a template that sets neither, and a
// failed write is an error. In the configuration an unset attribute is null on
// every operation.
func expandLegacyTemplatePatch(ctx context.Context, config templateResourceModel) (legacy.TemplatePatchV1, diag.Diagnostics) {
	var diags diag.Diagnostics
	var patch legacy.TemplatePatchV1

	if !config.DockerEntrypoint.IsNull() && !config.DockerEntrypoint.IsUnknown() {
		var entrypoint []string
		diags.Append(config.DockerEntrypoint.ElementsAs(ctx, &entrypoint, false)...)
		patch.DockerEntrypoint = &entrypoint
	}
	if !config.Readme.IsNull() && !config.Readme.IsUnknown() {
		patch.Readme = config.Readme.ValueStringPointer()
	}
	return patch, diags
}

// flattenLegacyTemplate maps the REST v1 template onto the two deprecated
// inputs and the three deprecated read-backs of the resource model. fromPatch
// says which v1 call answered; docker_entrypoint follows the same rule as the
// four flattenLegacyTemplateInto carries, and is set here because the data
// source reads its docker_entrypoint from REST v2 instead.
func flattenLegacyTemplate(ctx context.Context, template *legacy.TemplateV1, fromPatch bool, model *templateResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	switch {
	case template.DockerEntrypoint != nil:
		value, entrypointDiags := types.ListValueFrom(ctx, types.StringType, template.DockerEntrypoint)
		diags.Append(entrypointDiags...)
		model.DockerEntrypoint = value
	case !fromPatch || model.DockerEntrypoint.IsUnknown():
		model.DockerEntrypoint = types.ListNull(types.StringType)
	}

	flattenLegacyTemplateInto(template, fromPatch, &model.Readme, &model.Earned, &model.IsRunpod, &model.RuntimeInMin)
	return diags
}

// flattenLegacyTemplateInto carries the four REST v1 template attributes the
// resource and the data source share onto whichever model declares them: the
// two hold the same four in different structs, the way templateCollections
// already handles the collection fields two template shapes share.
//
// What an absent field means depends on which call answered. A PATCH
// (fromPatch) echoes what it chooses to, and a 204 echoes nothing at all: a
// partial body must leave the value alone rather than report a configured
// readme as cleared or a template's earnings as zero, so the planned value is
// kept. An unknown one still resolves to null, because state cannot hold
// unknown, which is the rule the GraphQL shims apply through jsonStringOr. A
// GET reports the template as it is, so an absent field is a cleared field
// (`"readme": null` and an omitted readme decode to the same nil), and keeping
// the old value there would hide a readme cleared in the console from every
// later plan.
//
// The rule covers earned, is_runpod and runtime_in_min too, not just the
// configurable readme: they are Computed and planned unknown, so nothing flags
// a fabricated zero on the plan, and the wrong value would sit in state until
// the next refresh.
func flattenLegacyTemplateInto(
	template *legacy.TemplateV1,
	fromPatch bool,
	readme *types.String,
	earned *types.Float64,
	isRunpod *types.Bool,
	runtimeInMin *types.Int64,
) {
	switch {
	case template.Readme != nil:
		*readme = types.StringValue(*template.Readme)
	case !fromPatch || readme.IsUnknown():
		*readme = types.StringNull()
	}

	switch {
	case template.Earned != nil:
		*earned = types.Float64Value(*template.Earned)
	case !fromPatch || earned.IsUnknown():
		*earned = types.Float64Null()
	}

	switch {
	case template.IsRunpod != nil:
		*isRunpod = types.BoolValue(*template.IsRunpod)
	case !fromPatch || isRunpod.IsUnknown():
		*isRunpod = types.BoolNull()
	}

	switch {
	case template.RuntimeInMin != nil:
		*runtimeInMin = legacyTruncatedInt64(template.RuntimeInMin)
	case !fromPatch || runtimeInMin.IsUnknown():
		*runtimeInMin = types.Int64Null()
	}
}

// clearLegacyTemplateFields nulls the shim read-backs for the path where the
// REST v1 host did not answer, and keeps the two configured inputs; the data
// source's clearLegacyTemplateDetailFields nulls all four of its shim
// attributes, because nothing there is configured.
func clearLegacyTemplateFields(model *templateResourceModel) {
	clearLegacyTemplateFieldsInto(&model.DockerEntrypoint, &model.Readme,
		&model.Earned, &model.IsRunpod, &model.RuntimeInMin)
}

// clearLegacyTemplateFieldsInto nulls the three read-backs and leaves
// docker_entrypoint and readme at the value the configuration planned,
// resolving an unknown one to null because state cannot hold unknown.
//
// The two configurable attributes are deliberately not nulled, the way
// clearLegacyEndpointFields leaves the two endpoint priorities alone: nothing
// reads them back off a failed call, and nulling a configured value puts state
// permanently at odds with the configuration. A template created through REST
// v2 is a 404 on the v1 route, which appendLegacyReadFailure passes over in
// silence, so every later plan would show a diff nothing explains and every
// later apply would re-run the same failing write, forever, until the
// attribute is deleted from the configuration.
func clearLegacyTemplateFieldsInto(
	entrypoint *types.List,
	readme *types.String,
	earned *types.Float64,
	isRunpod *types.Bool,
	runtimeInMin *types.Int64,
) {
	// A null is rewritten rather than left alone so the list always carries its
	// element type: a model's zero value is a null list with none, which state
	// cannot hold.
	if entrypoint.IsUnknown() || entrypoint.IsNull() {
		*entrypoint = types.ListNull(types.StringType)
	}
	if readme.IsUnknown() {
		*readme = types.StringNull()
	}
	*earned = types.Float64Null()
	*isRunpod = types.BoolNull()
	*runtimeInMin = types.Int64Null()
}

func flattenGpuType(g sdktypes.GpuType) gpuTypeModel {
	return gpuTypeModel{
		Id:             types.StringValue(g.Id),
		Name:           types.StringValue(g.Name),
		Pool:           types.StringPointerValue(g.Pool),
		Memory:         types.Int64Value(int64(g.Memory)),
		Secure:         types.BoolValue(g.Secure),
		Community:      types.BoolValue(g.Community),
		PriceSecure:    types.Float64Value(float64(g.Price.Secure)),
		PriceCommunity: types.Float64Value(float64(g.Price.Community)),
	}
}

func flattenGpuTypes(gpus []sdktypes.GpuType) []gpuTypeModel {
	models := make([]gpuTypeModel, 0, len(gpus))
	for _, g := range gpus {
		models = append(models, flattenGpuType(g))
	}
	return models
}

func flattenCpuType(c sdktypes.CpuType) cpuTypeModel {
	return cpuTypeModel{
		Id:                     types.StringValue(c.Id),
		Name:                   types.StringValue(c.Name),
		Group:                  types.StringValue(c.Group),
		RamGbPerVcpu:           types.Float64Value(float64(c.RamGbPerVcpu)),
		VcpuMin:                types.Int64Value(int64(c.Vcpu.Min)),
		VcpuMax:                types.Int64Value(int64(c.Vcpu.Max)),
		PriceSecurePerVcpu:     types.Float64Value(float64(c.Price.SecurePerVcpu)),
		PriceServerlessPerVcpu: types.Float64Value(float64(c.Price.ServerlessPerVcpu)),
	}
}

func flattenCpuTypes(cpus []sdktypes.CpuType) []cpuTypeModel {
	models := make([]cpuTypeModel, 0, len(cpus))
	for _, c := range cpus {
		models = append(models, flattenCpuType(c))
	}
	return models
}

func flattenDataCenter(ctx context.Context, dc sdktypes.DataCenter) (dataCenterModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	compliance := make([]string, 0, len(dc.Compliance))
	for _, c := range dc.Compliance {
		compliance = append(compliance, string(c))
	}
	volumeTypes := make([]string, 0, len(dc.NetworkVolumeTypes))
	for _, v := range dc.NetworkVolumeTypes {
		volumeTypes = append(volumeTypes, string(v))
	}

	complianceList, d := types.ListValueFrom(ctx, types.StringType, compliance)
	diags.Append(d...)
	volumeTypesList, d := types.ListValueFrom(ctx, types.StringType, volumeTypes)
	diags.Append(d...)

	return dataCenterModel{
		Id:                 types.StringValue(dc.Id),
		Name:               types.StringValue(dc.Name),
		Region:             types.StringValue(string(dc.Region)),
		GlobalNetwork:      types.BoolValue(dc.GlobalNetwork),
		Compliance:         complianceList,
		NetworkVolumeTypes: volumeTypesList,
	}, diags
}

func flattenDataCenters(ctx context.Context, dcs []sdktypes.DataCenter) ([]dataCenterModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	models := make([]dataCenterModel, 0, len(dcs))
	for _, dc := range dcs {
		model, d := flattenDataCenter(ctx, dc)
		diags.Append(d...)
		models = append(models, model)
	}
	return models, diags
}

func expandCreatePod(ctx context.Context, plan podResourceModel) (pods.CreatePodJSONRequestBody, diag.Diagnostics) {
	var diags diag.Diagnostics
	body := pods.CreatePodJSONRequestBody{
		Name: plan.Name.ValueString(),
	}
	if !plan.Image.IsNull() && !plan.Image.IsUnknown() {
		body.Image = plan.Image.ValueStringPointer()
	}
	if !plan.TemplateId.IsNull() && !plan.TemplateId.IsUnknown() {
		body.TemplateId = plan.TemplateId.ValueStringPointer()
	}

	// Exactly one compute config reaches the API: the resource's
	// ExactlyOneOf validator has already rejected a config with neither or
	// both. The CUDA and placement constraints live on the GPU config, which
	// is why a CPU pod cannot express them.
	switch {
	case !plan.GpuTypeId.IsNull() && !plan.GpuTypeId.IsUnknown():
		gpu := &sdktypes.CreateGpuConfig{Id: plan.GpuTypeId.ValueString()}
		if !plan.GpuCount.IsNull() && !plan.GpuCount.IsUnknown() {
			count := int(plan.GpuCount.ValueInt64())
			gpu.Count = &count
		}
		if !plan.AllowedCudaVersions.IsNull() && !plan.AllowedCudaVersions.IsUnknown() {
			var versions []string
			diags.Append(plan.AllowedCudaVersions.ElementsAs(ctx, &versions, false)...)
			gpu.AllowedCudaVersions = &versions
		}
		if !plan.MinCudaVersion.IsNull() && !plan.MinCudaVersion.IsUnknown() {
			gpu.MinCudaVersion = plan.MinCudaVersion.ValueStringPointer()
		}
		if !plan.MinRamPerGpu.IsNull() && !plan.MinRamPerGpu.IsUnknown() {
			ram := int(plan.MinRamPerGpu.ValueInt64())
			gpu.MinRamPerGpu = &ram
		}
		if !plan.MinVcpuCountPerGpu.IsNull() && !plan.MinVcpuCountPerGpu.IsUnknown() {
			vcpu := int(plan.MinVcpuCountPerGpu.ValueInt64())
			gpu.MinVcpuCountPerGpu = &vcpu
		}
		body.Gpu = gpu
	case !plan.CpuTypeId.IsNull() && !plan.CpuTypeId.IsUnknown():
		body.Cpu = &sdktypes.CreateCpuConfig{
			Id:        plan.CpuTypeId.ValueString(),
			VcpuCount: int(plan.VcpuCount.ValueInt64()),
		}
	}

	mounts, mountDiags := expandPodMounts(ctx, plan)
	diags.Append(mountDiags...)
	body.Mounts = mounts

	if !plan.StartSsh.IsNull() && !plan.StartSsh.IsUnknown() {
		body.StartSsh = plan.StartSsh.ValueBoolPointer()
	}
	if !plan.StartJupyter.IsNull() && !plan.StartJupyter.IsUnknown() {
		body.StartJupyter = plan.StartJupyter.ValueBoolPointer()
	}
	if !plan.Cloud.IsNull() && !plan.Cloud.IsUnknown() {
		cloud := sdktypes.Cloud(plan.Cloud.ValueString())
		body.Cloud = &cloud
	}
	if !plan.Disk.IsNull() && !plan.Disk.IsUnknown() {
		disk := int(plan.Disk.ValueInt64())
		body.Disk = &disk
	}
	if !plan.Args.IsNull() && !plan.Args.IsUnknown() {
		body.Args = plan.Args.ValueStringPointer()
	}
	if !plan.GlobalNetworking.IsNull() && !plan.GlobalNetworking.IsUnknown() {
		body.GlobalNetworking = plan.GlobalNetworking.ValueBoolPointer()
	}
	if !plan.Registry.IsNull() && !plan.Registry.IsUnknown() {
		body.Registry = plan.Registry.ValueStringPointer()
	}
	if !plan.DataCenterIds.IsNull() && !plan.DataCenterIds.IsUnknown() {
		var ids []string
		diags.Append(plan.DataCenterIds.ElementsAs(ctx, &ids, false)...)
		body.DataCenterIds = &ids
	}
	if !plan.Env.IsNull() && !plan.Env.IsUnknown() {
		env := map[string]string{}
		diags.Append(plan.Env.ElementsAs(ctx, &env, false)...)
		body.Env = &env
	}
	if !plan.Ports.IsNull() && !plan.Ports.IsUnknown() {
		var ports []string
		diags.Append(plan.Ports.ElementsAs(ctx, &ports, false)...)
		body.Ports = &ports
	}
	return body, diags
}

// expandPodMounts builds the mounts object for a create or an update. An
// unconfigured mount block leaves mounts off the body entirely, which a PATCH
// reads as "leave the existing mount alone"; the API has no way to remove
// one. At most one kind is ever set; the resource's Conflicting validator
// rejects a config that names both.
func expandPodMounts(ctx context.Context, plan podResourceModel) (*sdktypes.Mounts, diag.Diagnostics) {
	var diags diag.Diagnostics

	if !plan.PersistentVolume.IsNull() && !plan.PersistentVolume.IsUnknown() {
		var persistent persistentMountModel
		diags.Append(plan.PersistentVolume.As(ctx, &persistent, basetypes.ObjectAsOptions{})...)
		return &sdktypes.Mounts{Persistent: &sdktypes.PersistentMount{
			Size: int(persistent.Size.ValueInt64()),
			Path: persistent.Path.ValueString(),
		}}, diags
	}

	if !plan.NetworkVolumes.IsNull() && !plan.NetworkVolumes.IsUnknown() {
		var volumes []networkMountModel
		diags.Append(plan.NetworkVolumes.ElementsAs(ctx, &volumes, false)...)
		network := make([]sdktypes.NetworkMount, 0, len(volumes))
		for _, volume := range volumes {
			network = append(network, sdktypes.NetworkMount{
				VolumeId: volume.VolumeId.ValueString(),
				Path:     volume.Path.ValueString(),
			})
		}
		return &sdktypes.Mounts{Network: &network}, diags
	}

	return nil, diags
}

func flattenPod(ctx context.Context, pod *sdktypes.Pod, model *podResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	model.Id = types.StringValue(pod.Id)
	model.Name = types.StringValue(pod.Name)
	model.Image = types.StringValue(pod.Image)
	model.Status = types.StringValue(string(pod.Status))
	model.Cloud = types.StringValue(string(pod.Cloud))
	model.Disk = types.Int64Value(int64(pod.Disk))
	model.Args = types.StringValue(pod.Args)
	model.GlobalNetworking = types.BoolValue(pod.GlobalNetworking.Enabled)
	model.Registry = types.StringPointerValue(pod.Registry)
	model.CreatedAt = types.StringValue(pod.CreatedAt.Format(time.RFC3339))
	model.Locked = types.BoolValue(pod.Locked)
	model.CudaVersion = types.StringPointerValue(pod.CudaVersion)

	if pod.StartedAt != nil {
		model.StartedAt = types.StringValue(pod.StartedAt.Format(time.RFC3339))
	} else {
		model.StartedAt = types.StringNull()
	}

	// The API decodes args into these two exec-form lists; an omitted one is
	// null rather than an empty command.
	entrypoint, entrypointDiags := types.ListValueFrom(ctx, types.StringType, pod.Entrypoint)
	diags.Append(entrypointDiags...)
	model.DockerEntrypoint = entrypoint

	startCmd, startCmdDiags := types.ListValueFrom(ctx, types.StringType, pod.Cmd)
	diags.Append(startCmdDiags...)
	model.DockerStartCmd = startCmd

	// cost is a float32 on the wire; widening it directly would report
	// 0.34999999403953552 for the 0.35 the API sent.
	cost, costErr := widenFloat32(pod.Cost)
	if costErr != nil {
		diags.AddError("Reading pod cost failed", costErr.Error())
	} else {
		model.Cost = types.Float64Value(cost)
	}

	// Cluster membership is read-only here: a member pod is managed through
	// its cluster, not through this resource.
	if pod.Cluster != nil {
		model.ClusterId = types.StringValue(pod.Cluster.Id)
		model.ClusterIp = types.StringPointerValue(pod.Cluster.Ip)
	} else {
		model.ClusterId = types.StringNull()
		model.ClusterIp = types.StringNull()
	}

	runtime, runtimeDiags := flattenPodRuntime(ctx, pod.Runtime)
	diags.Append(runtimeDiags...)
	model.Runtime = runtime

	if pod.DataCenterId != nil {
		model.DataCenterId = types.StringValue(*pod.DataCenterId)
	} else {
		model.DataCenterId = types.StringNull()
	}

	// The API does not link a pod back to the template it was created from, so
	// a configured template_id stays as configured and an unset one resolves
	// to null; a pod that does report a template takes it from the response.
	if pod.Template != nil {
		model.TemplateId = types.StringValue(*pod.Template)
	} else if model.TemplateId.IsUnknown() {
		model.TemplateId = types.StringNull()
	}

	// A pod is GPU or CPU, never both; the response omits the other block.
	// The GPU response carries no CUDA or placement constraints, so
	// allowed_cuda_versions, min_cuda_version, min_ram_per_gpu and
	// min_vcpu_count_per_gpu keep their configured values, as do start_ssh
	// and start_jupyter, which the API never returns.
	// Allocated system RAM comes from whichever compute block the pod has:
	// the GPU total across all requested cards, or the CPU flavor's share.
	switch {
	case pod.Gpu != nil:
		model.MemoryInGb = types.Float64Value(pod.Gpu.Memory)
	case pod.Cpu != nil:
		model.MemoryInGb = types.Float64Value(float64(pod.Cpu.Memory))
	default:
		model.MemoryInGb = types.Float64Null()
	}

	if pod.Gpu != nil {
		model.GpuTypeId = types.StringValue(pod.Gpu.Id)
		model.GpuCount = int64PointerValue(pod.Gpu.Count)
	} else {
		model.GpuTypeId = types.StringNull()
		model.GpuCount = types.Int64Null()
	}

	if pod.Cpu != nil {
		model.CpuTypeId = types.StringValue(pod.Cpu.Id)
		model.VcpuCount = types.Int64Value(int64(pod.Cpu.VcpuCount))
	} else {
		model.CpuTypeId = types.StringNull()
		model.VcpuCount = types.Int64Null()
	}

	if pod.Mounts.Persistent != nil {
		persistent, persistentDiags := types.ObjectValueFrom(ctx, persistentMountAttrTypes, persistentMountModel{
			Size: types.Int64Value(int64(pod.Mounts.Persistent.Size)),
			Path: types.StringValue(pod.Mounts.Persistent.Path),
		})
		diags.Append(persistentDiags...)
		model.PersistentVolume = persistent
	} else {
		model.PersistentVolume = types.ObjectNull(persistentMountAttrTypes)
	}

	if pod.Mounts.Network != nil && len(*pod.Mounts.Network) > 0 {
		volumes := make([]networkMountModel, 0, len(*pod.Mounts.Network))
		for _, mount := range *pod.Mounts.Network {
			volumes = append(volumes, networkMountModel{
				VolumeId: types.StringValue(mount.VolumeId),
				Path:     types.StringValue(mount.Path),
			})
		}
		network, networkDiags := types.ListValueFrom(ctx, networkMountObjectType, volumes)
		diags.Append(networkDiags...)
		model.NetworkVolumes = network
	} else {
		model.NetworkVolumes = types.ListNull(networkMountObjectType)
	}

	// The API adds keys the configuration never held (PUBLIC_KEY for
	// start_ssh, JUPYTER_PASSWORD for start_jupyter, and a template's own env
	// merged per key), so a configured env keeps exactly what was planned and
	// only an unset (unknown) one is filled in from the response. A refresh
	// resyncs the configured keys instead; see resyncConfiguredEnv.
	if model.Env.IsUnknown() {
		env, envDiags := types.MapValueFrom(ctx, types.StringType, pod.Env)
		diags.Append(envDiags...)
		model.Env = env
	}

	ports, portDiags := types.ListValueFrom(ctx, types.StringType, pod.Ports)
	diags.Append(portDiags...)
	model.Ports = ports

	return diags
}

// apiOwnedEnvKeys are the two variables the provisioner writes itself:
// PUBLIC_KEY from start_ssh, JUPYTER_PASSWORD from start_jupyter. The API owns
// their values, so a configuration that also sets one never matches what comes
// back, and resyncing it would put the same diff on every plan forever.
var apiOwnedEnvKeys = map[string]struct{}{
	"PUBLIC_KEY":       {},
	"JUPYTER_PASSWORD": {},
}

// mergeConfiguredEnv builds the full env an update sends from the live env,
// the env in prior state and the planned one. Every planned key is set, so a
// configured PUBLIC_KEY or JUPYTER_PASSWORD wins here as it does on create,
// where the provisioner injects one only when the env has none. A prior key
// the plan no longer has is removed, and every other live key is kept.
//
// priorConfigured says whether the prior env is the configuration's own, as a
// create or update with a configured env leaves it. Otherwise it was read
// whole from the API, after an import or while env was unset, and its
// PUBLIC_KEY and JUPYTER_PASSWORD are the provisioner's, not the
// configuration's: leaving them out of the configuration keeps them on the pod.
func mergeConfiguredEnv(live, prior, planned map[string]string, priorConfigured bool) map[string]string {
	merged := make(map[string]string, len(live)+len(planned))
	for key, value := range live {
		merged[key] = value
	}
	for key := range prior {
		if _, kept := planned[key]; kept {
			continue
		}
		if _, apiOwned := apiOwnedEnvKeys[key]; apiOwned && !priorConfigured {
			continue
		}
		delete(merged, key)
	}
	for key, value := range planned {
		merged[key] = value
	}
	return merged
}

// resyncConfiguredEnv refreshes a configured env from the API response during
// a refresh, limited to the keys the configuration declared and the API does
// not own. Create and update keep the planned map untouched because the API
// merges a template's env in and injects its own keys, which would plan a
// permanent diff; doing the same on a read leaves a value changed outside
// Terraform invisible to plan forever. Reading only the configured keys back
// reports that drift without pulling in the keys the configuration never held.
// A key the response no longer carries drops out, which is the drift it is.
//
// A null env means nothing has been planned for it yet, which is the state an
// import leaves; it takes the whole response, as an unset env does on create.
func resyncConfiguredEnv(ctx context.Context, apiEnv map[string]string, env *types.Map) diag.Diagnostics {
	var diags diag.Diagnostics
	if env.IsUnknown() {
		return diags
	}
	if env.IsNull() {
		value, valueDiags := types.MapValueFrom(ctx, types.StringType, apiEnv)
		diags.Append(valueDiags...)
		if !valueDiags.HasError() {
			*env = value
		}
		return diags
	}

	var configured map[string]string
	diags.Append(env.ElementsAs(ctx, &configured, false)...)
	if diags.HasError() {
		return diags
	}

	refreshed := make(map[string]string, len(configured))
	for key, planned := range configured {
		if _, apiOwned := apiOwnedEnvKeys[key]; apiOwned {
			refreshed[key] = planned
			continue
		}
		if value, ok := apiEnv[key]; ok {
			refreshed[key] = value
		}
	}

	value, valueDiags := types.MapValueFrom(ctx, types.StringType, refreshed)
	diags.Append(valueDiags...)
	if valueDiags.HasError() {
		return diags
	}
	*env = value
	return diags
}

// flattenPodRuntime maps the pod's live utilization for both the resource and
// the data source. The API reports nothing here unless the pod is RUNNING, so
// the whole block is null in every other status, and each reading inside it is
// null when it is missing rather than reported as a zero.
func flattenPodRuntime(ctx context.Context, runtime *sdktypes.PodRuntime) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics
	if runtime == nil {
		return types.ObjectNull(podRuntimeAttrTypes), diags
	}

	metrics := podRuntimeModel{
		UptimeSeconds: int64PointerValue(runtime.Uptime),
		Gpus:          types.ListNull(podGpuUtilizationObjectType),
		Ports:         types.ListNull(podRuntimePortObjectType),
	}
	if runtime.Cpu != nil {
		metrics.CpuUtil = int64PointerValue(runtime.Cpu.Util)
	}
	if runtime.Memory != nil {
		metrics.MemoryUtil = int64PointerValue(runtime.Memory.Util)
	}

	if runtime.Gpus != nil {
		gpus := make([]podGpuUtilizationModel, 0, len(*runtime.Gpus))
		for _, gpu := range *runtime.Gpus {
			gpus = append(gpus, podGpuUtilizationModel{
				Util:       int64PointerValue(gpu.Util),
				MemoryUtil: int64PointerValue(gpu.MemoryUtil),
			})
		}
		list, gpuDiags := types.ListValueFrom(ctx, podGpuUtilizationObjectType, gpus)
		diags.Append(gpuDiags...)
		metrics.Gpus = list
	}

	if runtime.Ports != nil {
		ports := make([]podRuntimePortModel, 0, len(*runtime.Ports))
		for _, port := range *runtime.Ports {
			ports = append(ports, podRuntimePortModel{
				Ip:          types.StringPointerValue(port.Ip),
				PrivatePort: int64PointerValue(port.Private),
				PublicPort:  int64PointerValue(port.Public),
				Type:        types.StringPointerValue(port.Type),
			})
		}
		list, portDiags := types.ListValueFrom(ctx, podRuntimePortObjectType, ports)
		diags.Append(portDiags...)
		metrics.Ports = list
	}

	object, objectDiags := types.ObjectValueFrom(ctx, podRuntimeAttrTypes, metrics)
	diags.Append(objectDiags...)
	return object, diags
}

// podActionForDesiredState maps a desired_state value against the pod's
// current status to the action (if any) that converges them, following the
// transitions the API accepts: a pod that should be RUNNING takes start from
// EXITED and from ERROR, and a pod that should be EXITED takes stop from
// RUNNING and from the two transitional statuses, PROVISIONING and STARTING.
// Stopping from the transitional statuses is what makes a pod created with
// desired_state = EXITED stop on the first apply instead of billing until the
// second one catches it in RUNNING. ERROR only ever takes start; nothing here
// stops an errored pod.
func podActionForDesiredState(desired string, status sdktypes.PodStatus) (sdktypes.PodAction, bool) {
	switch desired {
	case string(sdktypes.PodStatusRunning):
		switch status {
		case sdktypes.PodStatusExited, sdktypes.PodStatusError:
			return sdktypes.PodActionStart, true
		}
	case string(sdktypes.PodStatusExited):
		switch status {
		case sdktypes.PodStatusRunning, sdktypes.PodStatusProvisioning, sdktypes.PodStatusStarting:
			return sdktypes.PodActionStop, true
		}
	}
	return "", false
}

// runStateForStatus maps an API-reported pod status onto the desired_state
// value that describes it. Only the two steady states have one; a pod that is
// provisioning, starting, terminated, or errored is in no state a
// desired_state can be compared against.
func runStateForStatus(status sdktypes.PodStatus) (string, bool) {
	switch status {
	case sdktypes.PodStatusRunning:
		return string(sdktypes.PodStatusRunning), true
	case sdktypes.PodStatusExited:
		return string(sdktypes.PodStatusExited), true
	}
	return "", false
}

// refreshDesiredState writes the pod's actual run state back into
// desired_state during a read, so a pod started or stopped outside Terraform
// produces a plan diff against the configured value instead of silently
// diverging. A pod whose run state is unmanaged (desired_state absent from
// state) stays unmanaged, and a transitional status leaves the previous value
// alone rather than inventing a diff mid-transition.
func refreshDesiredState(model *podResourceModel) {
	if model.DesiredState.IsNull() || model.DesiredState.IsUnknown() {
		return
	}
	if runState, ok := runStateForStatus(sdktypes.PodStatus(model.Status.ValueString())); ok {
		model.DesiredState = types.StringValue(runState)
	}
}

// flattenDelegation maps the API response; the ECR resource ARN is create-only
// input the API does not echo back, so model.Resource stays as configured.
func flattenDelegation(delegation *sdktypes.EcrDelegation, model *registryDelegationResourceModel) {
	model.Id = types.StringValue(delegation.Id)
	model.Name = types.StringPointerValue(delegation.Name)
	model.AwsRegion = types.StringValue(delegation.AwsRegion)
	model.AwsUser = types.StringValue(delegation.AwsUser)
	model.Repository = types.StringValue(delegation.Repository)
	model.Tag = types.StringValue(delegation.Tag)
	model.DockerRegistryUri = types.StringPointerValue(delegation.DockerRegistryUri)
	model.CreatedAt = types.StringValue(delegation.CreatedAt.Format(time.RFC3339))
}

func expandCreateNetworkVolume(plan networkVolumeResourceModel) networkvolumes.CreateNetworkVolumeJSONRequestBody {
	body := networkvolumes.CreateNetworkVolumeJSONRequestBody{
		Name:       plan.Name.ValueString(),
		Size:       int(plan.Size.ValueInt64()),
		DataCenter: plan.DataCenter.ValueString(),
	}
	if !plan.Type.IsNull() && !plan.Type.IsUnknown() {
		volumeType := sdktypes.VolumeType(plan.Type.ValueString())
		body.Type = &volumeType
	}
	return body
}

func flattenNetworkVolume(volume *sdktypes.NetworkVolume, model *networkVolumeResourceModel) {
	model.Id = types.StringValue(volume.Id)
	model.Name = types.StringValue(volume.Name)
	model.Size = types.Int64Value(int64(volume.Size))
	model.DataCenter = types.StringValue(volume.DataCenter)
	model.Type = types.StringValue(string(volume.Type))
}

func expandCreateTemplate(ctx context.Context, plan templateResourceModel) (templates.CreateTemplateJSONRequestBody, diag.Diagnostics) {
	var diags diag.Diagnostics
	body := templates.CreateTemplateJSONRequestBody{
		Name:  plan.Name.ValueString(),
		Image: plan.Image.ValueString(),
	}
	if !plan.Category.IsNull() && !plan.Category.IsUnknown() {
		category := sdktypes.TemplateCategory(plan.Category.ValueString())
		body.Category = &category
	}
	if !plan.Disk.IsNull() && !plan.Disk.IsUnknown() {
		disk := int(plan.Disk.ValueInt64())
		body.Disk = &disk
	}
	if !plan.Args.IsNull() && !plan.Args.IsUnknown() {
		body.Args = plan.Args.ValueStringPointer()
	}
	if !plan.Public.IsNull() && !plan.Public.IsUnknown() {
		body.Public = plan.Public.ValueBoolPointer()
	}
	if !plan.Serverless.IsNull() && !plan.Serverless.IsUnknown() {
		body.Serverless = plan.Serverless.ValueBoolPointer()
	}
	if !plan.Registry.IsNull() && !plan.Registry.IsUnknown() {
		body.Registry = plan.Registry.ValueStringPointer()
	}
	if !plan.StartSsh.IsNull() && !plan.StartSsh.IsUnknown() {
		body.StartSsh = plan.StartSsh.ValueBoolPointer()
	}
	if !plan.StartJupyter.IsNull() && !plan.StartJupyter.IsUnknown() {
		body.StartJupyter = plan.StartJupyter.ValueBoolPointer()
	}
	if !plan.AllowedCudaVersions.IsNull() && !plan.AllowedCudaVersions.IsUnknown() {
		var versions []string
		diags.Append(plan.AllowedCudaVersions.ElementsAs(ctx, &versions, false)...)
		body.AllowedCudaVersions = &versions
	}
	mounts, mountDiags := expandTemplateMounts(ctx, plan)
	diags.Append(mountDiags...)
	body.Mounts = mounts
	if !plan.Env.IsNull() && !plan.Env.IsUnknown() {
		env := map[string]string{}
		diags.Append(plan.Env.ElementsAs(ctx, &env, false)...)
		body.Env = &env
	}
	if !plan.Ports.IsNull() && !plan.Ports.IsUnknown() {
		var ports []string
		diags.Append(plan.Ports.ElementsAs(ctx, &ports, false)...)
		body.Ports = &ports
	}
	return body, diags
}

// expandTemplateMounts builds the template's single persistent mount. A
// template takes no network volume, and an omitted mounts object leaves an
// existing mount in place on a PATCH.
func expandTemplateMounts(ctx context.Context, plan templateResourceModel) (*sdktypes.TemplateMounts, diag.Diagnostics) {
	var diags diag.Diagnostics
	if plan.PersistentVolume.IsNull() || plan.PersistentVolume.IsUnknown() {
		return nil, diags
	}

	var persistent persistentMountModel
	diags.Append(plan.PersistentVolume.As(ctx, &persistent, basetypes.ObjectAsOptions{})...)
	return &sdktypes.TemplateMounts{Persistent: &sdktypes.PersistentMount{
		Size: int(persistent.Size.ValueInt64()),
		Path: persistent.Path.ValueString(),
	}}, diags
}

func flattenTemplate(ctx context.Context, template *sdktypes.Template, model *templateResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	model.Id = types.StringValue(template.Id)
	model.Name = types.StringValue(template.Name)
	model.Image = types.StringValue(template.Image)
	model.Category = types.StringValue(string(template.Category))
	model.Disk = types.Int64Value(int64(template.Disk))
	model.Args = types.StringValue(template.Args)
	model.Public = types.BoolValue(template.Public)
	model.Serverless = types.BoolValue(template.Serverless)
	model.Registry = types.StringPointerValue(template.Registry)
	model.StartSsh = types.BoolValue(template.StartSsh)
	model.StartJupyter = types.BoolValue(template.StartJupyter)

	allowedCuda, allowedDiags := types.ListValueFrom(ctx, types.StringType, template.AllowedCudaVersions)
	diags.Append(allowedDiags...)
	model.AllowedCudaVersions = allowedCuda

	if template.Mounts.Persistent != nil {
		persistent, persistentDiags := types.ObjectValueFrom(ctx, persistentMountAttrTypes, persistentMountModel{
			Size: types.Int64Value(int64(template.Mounts.Persistent.Size)),
			Path: types.StringValue(template.Mounts.Persistent.Path),
		})
		diags.Append(persistentDiags...)
		model.PersistentVolume = persistent
	} else {
		model.PersistentVolume = types.ObjectNull(persistentMountAttrTypes)
	}

	env, envDiags := types.MapValueFrom(ctx, types.StringType, template.Env)
	diags.Append(envDiags...)
	model.Env = env

	ports, portDiags := types.ListValueFrom(ctx, types.StringType, template.Ports)
	diags.Append(portDiags...)
	model.Ports = ports

	return diags
}

func expandCreateEndpoint(ctx context.Context, plan serverlessEndpointResourceModel) (serverless.CreateEndpointJSONRequestBody, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := serverless.CreateEndpointJSONRequestBody{
		Name: plan.Name.ValueString(),
		Type: sdktypes.EndpointType(plan.EndpointType.ValueString()),
	}
	if !plan.Image.IsNull() && !plan.Image.IsUnknown() {
		body.Image = plan.Image.ValueStringPointer()
	}
	if !plan.TemplateId.IsNull() && !plan.TemplateId.IsUnknown() {
		body.TemplateId = plan.TemplateId.ValueStringPointer()
	}

	// Exactly one compute family reaches the API: the resource's ExactlyOneOf
	// validator has already rejected a config with neither or both. The CUDA
	// and exclusion constraints live on the GPU config, which is why a CPU
	// endpoint cannot express them.
	switch {
	case !plan.GpuPools.IsNull() && !plan.GpuPools.IsUnknown():
		var pools []string
		diags.Append(plan.GpuPools.ElementsAs(ctx, &pools, false)...)
		gpu := &sdktypes.CreateEndpointGpuConfig{Pools: pools}
		if !plan.GpuCount.IsNull() && !plan.GpuCount.IsUnknown() {
			count := int(plan.GpuCount.ValueInt64())
			gpu.Count = &count
		}
		if !plan.ExcludedTypes.IsNull() && !plan.ExcludedTypes.IsUnknown() {
			var excluded []string
			diags.Append(plan.ExcludedTypes.ElementsAs(ctx, &excluded, false)...)
			gpu.ExcludedTypes = &excluded
		}
		if !plan.AllowedCudaVersions.IsNull() && !plan.AllowedCudaVersions.IsUnknown() {
			var versions []string
			diags.Append(plan.AllowedCudaVersions.ElementsAs(ctx, &versions, false)...)
			gpu.AllowedCudaVersions = &versions
		}
		if !plan.MinCudaVersion.IsNull() && !plan.MinCudaVersion.IsUnknown() {
			gpu.MinCudaVersion = plan.MinCudaVersion.ValueStringPointer()
		}
		body.Gpu = gpu
	case !plan.Cpu.IsNull() && !plan.Cpu.IsUnknown():
		cpu, cpuDiags := expandEndpointCpu(ctx, plan.Cpu)
		diags.Append(cpuDiags...)
		body.Cpu = &cpu
	}

	if !plan.Flashboot.IsNull() && !plan.Flashboot.IsUnknown() {
		flashboot := sdktypes.FlashBoot(plan.Flashboot.ValueString())
		body.Flashboot = &flashboot
	}

	scaling, scalingDiags := expandEndpointScaling(plan.ScalingType.ValueString(), plan.ScalingValue.ValueFloat64())
	diags.Append(scalingDiags...)
	body.Scaling = scaling

	// workers_min, workers_max, and idle_timeout are Optional + Computed, so an
	// omitted one is Unknown rather than null. The guard has to check both, or
	// the request carries an empty workers object.
	workers := struct {
		IdleTimeout *int `json:"idleTimeout,omitempty"`
		Max         *int `json:"max,omitempty"`
		Min         *int `json:"min,omitempty"`
	}{}
	workersSet := false
	if !plan.WorkersMin.IsNull() && !plan.WorkersMin.IsUnknown() {
		v := int(plan.WorkersMin.ValueInt64())
		workers.Min = &v
		workersSet = true
	}
	if !plan.WorkersMax.IsNull() && !plan.WorkersMax.IsUnknown() {
		v := int(plan.WorkersMax.ValueInt64())
		workers.Max = &v
		workersSet = true
	}
	if !plan.IdleTimeout.IsNull() && !plan.IdleTimeout.IsUnknown() {
		v := int(plan.IdleTimeout.ValueInt64())
		workers.IdleTimeout = &v
		workersSet = true
	}
	if workersSet {
		body.Workers = &workers
	}

	if !plan.Disk.IsNull() && !plan.Disk.IsUnknown() {
		disk := int(plan.Disk.ValueInt64())
		body.Disk = &disk
	}
	if !plan.Timeout.IsNull() && !plan.Timeout.IsUnknown() {
		timeout := int(plan.Timeout.ValueInt64())
		body.Timeout = &timeout
	}
	if !plan.Registry.IsNull() && !plan.Registry.IsUnknown() {
		body.Registry = plan.Registry.ValueStringPointer()
	}
	if !plan.Env.IsNull() && !plan.Env.IsUnknown() {
		env := map[string]string{}
		diags.Append(plan.Env.ElementsAs(ctx, &env, false)...)
		body.Env = &env
	}
	if !plan.DataCenterIds.IsNull() && !plan.DataCenterIds.IsUnknown() {
		var ids []string
		diags.Append(plan.DataCenterIds.ElementsAs(ctx, &ids, false)...)
		body.DataCenterIds = &ids
	}
	if !plan.NetworkVolumes.IsNull() && !plan.NetworkVolumes.IsUnknown() {
		var volumes []string
		diags.Append(plan.NetworkVolumes.ElementsAs(ctx, &volumes, false)...)
		body.NetworkVolumes = &volumes
	}
	if !plan.Ports.IsNull() && !plan.Ports.IsUnknown() {
		var ports []string
		diags.Append(plan.Ports.ElementsAs(ctx, &ports, false)...)
		body.Ports = &ports
	}
	return body, diags
}

// expandEndpointCpu maps the cpu nested list onto the API's CPU selection,
// shared by create and update. An update replaces the selection wholesale.
func expandEndpointCpu(ctx context.Context, cpu types.List) ([]sdktypes.CreateCpuConfig, diag.Diagnostics) {
	var diags diag.Diagnostics
	var configured []endpointCpuModel
	diags.Append(cpu.ElementsAs(ctx, &configured, false)...)

	configs := make([]sdktypes.CreateCpuConfig, 0, len(configured))
	for _, config := range configured {
		configs = append(configs, sdktypes.CreateCpuConfig{
			Id:        config.CpuTypeId.ValueString(),
			VcpuCount: int(config.VcpuCount.ValueInt64()),
		})
	}
	return configs, diags
}

func expandEndpointScaling(scalingType string, value float64) (sdktypes.EndpointScaling, diag.Diagnostics) {
	var diags diag.Diagnostics
	var scaling sdktypes.EndpointScaling
	var err error
	switch scalingType {
	case "QUEUE_DELAY":
		err = scaling.FromQueueDelayScaling(sdktypes.QueueDelayScaling{
			Type:       sdktypes.QueueDelayScalingTypeQueueDelay,
			QueueDelay: float32(value),
		})
	case "REQUEST_COUNT":
		// scaling_value is a float because QUEUE_DELAY takes fractional
		// seconds, but this scaler counts in-flight requests: the API takes a
		// whole number, 1 or more. Narrowing silently would send 2 for a
		// planned 2.7 (and 0 for 0.5, which the API rejects), so the
		// constraint is enforced here rather than lost in the conversion.
		if value != math.Trunc(value) || value < 1 {
			diags.AddError(
				"Invalid scaling_value",
				fmt.Sprintf("REQUEST_COUNT scaling takes a whole number of in-flight requests, 1 or more; got %v", value),
			)
			return scaling, diags
		}
		err = scaling.FromRequestCountScaling(sdktypes.RequestCountScaling{
			Type:         sdktypes.RequestCountScalingTypeRequestCount,
			RequestCount: int(value),
		})
	default:
		// Unreachable through a plan: scaling_type carries a OneOf validator.
		// Kept so a future caller cannot get a silently empty scaling union.
		diags.AddError("Invalid scaling_type", fmt.Sprintf("scaling_type must be QUEUE_DELAY or REQUEST_COUNT, got %q", scalingType))
		return scaling, diags
	}
	if err != nil {
		diags.AddError("Building scaling config failed", err.Error())
	}
	return scaling, diags
}

func flattenEndpointScaling(scaling sdktypes.EndpointScaling) (scalingType string, value float64, err error) {
	discriminator, err := scaling.Discriminator()
	if err != nil {
		return "", 0, err
	}
	switch discriminator {
	case "QUEUE_DELAY":
		queueDelay, asErr := scaling.AsQueueDelayScaling()
		if asErr != nil {
			return "", 0, asErr
		}
		widened, parseErr := widenFloat32(queueDelay.QueueDelay)
		if parseErr != nil {
			return "", 0, parseErr
		}
		return discriminator, widened, nil
	case "REQUEST_COUNT":
		requestCount, asErr := scaling.AsRequestCountScaling()
		if asErr != nil {
			return "", 0, asErr
		}
		return discriminator, float64(requestCount.RequestCount), nil
	default:
		return "", 0, fmt.Errorf("unknown scaling discriminator %q", discriminator)
	}
}

func flattenEndpoint(ctx context.Context, endpoint *sdktypes.Endpoint, model *serverlessEndpointResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	model.Id = types.StringValue(endpoint.Id)
	model.Name = types.StringValue(endpoint.Name)
	model.CreatedAt = types.StringValue(endpoint.CreatedAt.Format(time.RFC3339))

	// image and endpoint_type are optional in the response but not in the
	// plan (endpoint_type is Required and image is Optional+Computed), so a
	// response that omits either keeps the value the plan holds instead of
	// nulling it out from under the apply.
	if endpoint.Image != nil {
		model.Image = types.StringValue(*endpoint.Image)
	} else if model.Image.IsUnknown() {
		model.Image = types.StringNull()
	}
	if endpoint.Type != nil {
		model.EndpointType = types.StringValue(string(*endpoint.Type))
	}
	model.Timeout = types.Int64Value(int64(endpoint.Timeout))
	model.Flashboot = types.StringValue(string(endpoint.Flashboot))

	// A GPU endpoint and a CPU endpoint each omit the other's block. template_id
	// has no counterpart in the response (the API keeps no link to the
	// template), so it stays as configured.
	if endpoint.Gpu != nil {
		pools, poolDiags := types.ListValueFrom(ctx, types.StringType, endpoint.Gpu.Pools)
		diags.Append(poolDiags...)
		model.GpuPools = pools
		model.GpuCount = int64PointerValue(endpoint.Gpu.Count)
		allowedCuda, allowedDiags := types.ListValueFrom(ctx, types.StringType, endpoint.Gpu.AllowedCudaVersions)
		diags.Append(allowedDiags...)
		model.AllowedCudaVersions = allowedCuda
		// An empty string is the API's way to drop the floor, and it answers
		// with null afterwards; keeping a configured "" makes that documented
		// clear round-trip instead of failing the plan comparison.
		switch {
		case endpoint.Gpu.MinCudaVersion != nil:
			model.MinCudaVersion = types.StringValue(*endpoint.Gpu.MinCudaVersion)
		case isEmptyStringValue(model.MinCudaVersion):
		default:
			model.MinCudaVersion = types.StringNull()
		}
		// The API omits excludedTypes when nothing is excluded, so a configured
		// [] reads back as absent; keeping it is what lets that configuration
		// apply instead of failing with an inconsistent result.
		switch {
		case endpoint.Gpu.ExcludedTypes != nil:
			excluded, excludedDiags := types.ListValueFrom(ctx, types.StringType, *endpoint.Gpu.ExcludedTypes)
			diags.Append(excludedDiags...)
			model.ExcludedTypes = excluded
		case isEmptyListValue(model.ExcludedTypes):
		default:
			model.ExcludedTypes = types.ListNull(types.StringType)
		}
	} else {
		model.GpuPools = types.ListNull(types.StringType)
		model.GpuCount = types.Int64Null()
		model.AllowedCudaVersions = types.ListNull(types.StringType)
		model.MinCudaVersion = types.StringNull()
		model.ExcludedTypes = types.ListNull(types.StringType)
	}

	if endpoint.Cpu != nil {
		configs := make([]endpointCpuModel, 0, len(*endpoint.Cpu))
		for _, config := range *endpoint.Cpu {
			configs = append(configs, endpointCpuModel{
				CpuTypeId: types.StringValue(config.Id),
				VcpuCount: types.Int64Value(int64(config.VcpuCount)),
			})
		}
		cpu, cpuDiags := types.ListValueFrom(ctx, endpointCpuObjectType, configs)
		diags.Append(cpuDiags...)
		model.Cpu = cpu
	} else {
		model.Cpu = types.ListNull(endpointCpuObjectType)
	}

	// The deprecated compute_type restates the family the GPU and CPU blocks
	// already carry, so it is derived here rather than read from the legacy
	// host, and the resource rejects a configuration where the two disagree.
	switch {
	case endpoint.Gpu != nil:
		model.ComputeType = types.StringValue("GPU")
	case endpoint.Cpu != nil:
		model.ComputeType = types.StringValue("CPU")
	case model.ComputeType.IsUnknown():
		model.ComputeType = types.StringNull()
	}

	scalingType, scalingValue, err := flattenEndpointScaling(endpoint.Scaling)
	if err != nil {
		diags.AddError("Reading scaling config failed", err.Error())
	} else {
		model.ScalingType = types.StringValue(scalingType)
		model.ScalingValue = types.Float64Value(scalingValue)
	}

	model.WorkersMin = types.Int64Value(int64(endpoint.Workers.Min))
	model.WorkersMax = types.Int64Value(int64(endpoint.Workers.Max))
	model.IdleTimeout = int64PointerValue(endpoint.Workers.IdleTimeout)
	model.Disk = int64PointerValue(endpoint.Disk)
	model.Registry = types.StringPointerValue(endpoint.Registry)

	// As with a pod, a template's env is merged into the endpoint's per key,
	// so a configured env keeps what was planned and only an unset (unknown)
	// one is filled in from the response; a refresh resyncs the configured
	// keys through resyncConfiguredEnv.
	if model.Env.IsUnknown() {
		var env map[string]string
		if endpoint.Env != nil {
			env = *endpoint.Env
		}
		envMap, envDiags := types.MapValueFrom(ctx, types.StringType, env)
		diags.Append(envDiags...)
		model.Env = envMap
	}

	dataCenterIds, dcDiags := types.ListValueFrom(ctx, types.StringType, endpoint.DataCenterIds)
	diags.Append(dcDiags...)
	model.DataCenterIds = dataCenterIds

	networkVolumes, nvDiags := types.ListValueFrom(ctx, types.StringType, endpoint.NetworkVolumes)
	diags.Append(nvDiags...)
	model.NetworkVolumes = networkVolumes

	if endpoint.Ports != nil {
		ports, portDiags := types.ListValueFrom(ctx, types.StringType, *endpoint.Ports)
		diags.Append(portDiags...)
		model.Ports = ports
	} else {
		model.Ports = types.ListNull(types.StringType)
	}

	requestUrls, urlDiags := flattenEndpointRequestUrls(ctx, endpoint.RequestUrls, model.EndpointType.ValueString())
	diags.Append(urlDiags...)
	model.RequestUrls = requestUrls

	return diags
}

// flattenEndpointRequestUrls maps the endpoint's invocation URLs. The API
// returns one of two shapes and, unlike the scaling union, this one carries no
// discriminator: the endpoint's routing type picks the variant, exactly as
// the API documents it. Whichever fields the other shape would have carried
// stay null.
func flattenEndpointRequestUrls(ctx context.Context, requestUrls *sdktypes.EndpointRequestUrls, endpointType string) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics
	if requestUrls == nil {
		return types.ObjectNull(endpointRequestUrlsAttrTypes), diags
	}

	var urls endpointRequestUrlsModel
	if endpointType == string(sdktypes.EndpointTypeLoadBalancer) {
		balancer, err := requestUrls.AsLoadBalancingRequestUrls()
		if err != nil {
			diags.AddError("Reading endpoint request URLs failed", err.Error())
			return types.ObjectNull(endpointRequestUrlsAttrTypes), diags
		}
		urls.Base = types.StringValue(balancer.Base)
		urls.Health = types.StringValue(balancer.Health)
	} else {
		queue, err := requestUrls.AsQueueBasedRequestUrls()
		if err != nil {
			diags.AddError("Reading endpoint request URLs failed", err.Error())
			return types.ObjectNull(endpointRequestUrlsAttrTypes), diags
		}
		urls.Run = types.StringValue(queue.Run)
		urls.RunSync = types.StringValue(queue.RunSync)
		urls.Status = types.StringValue(queue.Status)
		urls.Stream = types.StringValue(queue.Stream)
		urls.Cancel = types.StringValue(queue.Cancel)
		urls.Retry = types.StringValue(queue.Retry)
		urls.PurgeQueue = types.StringValue(queue.PurgeQueue)
		urls.Health = types.StringValue(queue.Health)
	}

	object, objectDiags := types.ObjectValueFrom(ctx, endpointRequestUrlsAttrTypes, urls)
	diags.Append(objectDiags...)
	return object, diags
}
