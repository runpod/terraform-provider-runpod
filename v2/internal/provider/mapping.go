package provider

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/runpod/go-sdk/v2/lib/networkvolumes"
	"github.com/runpod/go-sdk/v2/lib/pods"
	"github.com/runpod/go-sdk/v2/lib/serverless"
	"github.com/runpod/go-sdk/v2/lib/templates"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// The pure API<->state mapping lives here so it is unit-testable without the
// plugin protocol machinery.

// The deprecated runpod_template attributes REST v2 does not report (readme,
// earned, is_runpod and runtime_in_min) are served by the GraphQL shim in
// legacy_graphql.go, with the shared plumbing in legacy_shim.go.
// docker_entrypoint is REST v2's entrypoint, mapped with the rest of the
// template further down.

// configuredTemplateReadme returns the readme to write through the GraphQL
// shim, or nil when there is nothing to write.
//
// It is read from the configuration, not the plan: readme is
// Optional+Computed, so terraform core copies the prior state value into the
// plan for a configuration that leaves it out, which would rewrite the
// template through saveTemplate on every update of a template that never set
// it, and a failed write is an error. In the configuration an unset attribute
// is null on every operation, and an unresolved one is not a value to send.
func configuredTemplateReadme(config templateResourceModel) *string {
	if config.Readme.IsNull() || config.Readme.IsUnknown() {
		return nil
	}
	return config.Readme.ValueStringPointer()
}

// validateTemplateReadme rejects a readme the GraphQL shim cannot apply.
//
// saveTemplate stores trim(readme) and skips the field when it is empty, so
// "" changes nothing and the read-back keeps the old text: a plan promising
// "" would fail the apply with an inconsistent result, and a warning instead
// would leave the same diff on every later plan. A whitespace-only value is refused too: it would only
// clear the text by way of that trim, and read back as something other than
// what was planned. Null and unknown are not values to judge.
func validateTemplateReadme(diags *diag.Diagnostics, readme types.String) {
	if readme.IsNull() || readme.IsUnknown() || strings.TrimSpace(readme.ValueString()) != "" {
		return
	}
	diags.AddAttributeError(
		path.Root("readme"),
		"Empty template readme cannot be applied",
		"readme is written through the GraphQL API, which ignores an empty readme, so the text cannot be cleared. "+
			"Remove readme from the configuration to leave the template's text as it is, or set the text it should carry.",
	)
}

// expandTemplateEntrypoint returns the entrypoint to send to REST v2, or nil
// to leave the stored one alone.
//
// It is read from the configuration for the reason configuredTemplateReadme
// gives: the plan carries the prior entrypoint when the configuration leaves
// it out, and sending that back would rewrite a template whose stored command
// is a bare string into the JSON form the API encodes an entrypoint into. An
// empty list is sent as [], which is how the API clears the entrypoint.
func expandTemplateEntrypoint(ctx context.Context, config templateResourceModel) (*[]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if config.DockerEntrypoint.IsNull() || config.DockerEntrypoint.IsUnknown() {
		return nil, diags
	}
	entrypoint := []string{}
	diags.Append(config.DockerEntrypoint.ElementsAs(ctx, &entrypoint, false)...)
	return &entrypoint, diags
}

// flattenTemplateEntrypoint maps the entrypoint REST v2 decodes from the
// template's stored command. The API stores a cleared entrypoint by leaving it
// out of that command, so a cleared one can come back absent: a planned []
// is kept then, or a configuration clearing the entrypoint would fail the
// apply against a null. Any other absent entrypoint is null.
func flattenTemplateEntrypoint(ctx context.Context, reported *[]string, planned types.List) (types.List, diag.Diagnostics) {
	if (reported == nil || len(*reported) == 0) &&
		!planned.IsNull() && !planned.IsUnknown() && len(planned.Elements()) == 0 {
		return types.ListValueMust(types.StringType, []attr.Value{}), nil
	}
	return types.ListValueFrom(ctx, types.StringType, reported)
}

// flattenLegacyTemplateInto carries the four GraphQL template fields onto
// whichever model declares them: the resource and the data source hold the
// same four in different structs, the way templateCollections already handles
// the collection fields two template shapes share.
//
// The query and the saveTemplate mutation both select all four, so an absent
// or null field is one the template does not have, and maps to null; a 0 is a
// reported value. readme keeps the value it holds when the answer differs
// from it only by the surrounding whitespace saveTemplate trims off: a heredoc
// readme ends in a newline the stored text never carries, so taking the
// answer would fail the apply with an inconsistent result and then plan the
// same write on every later run.
func flattenLegacyTemplateInto(
	template *legacyTemplateFields,
	readme *types.String,
	earned *types.Float64,
	isRunpod *types.Bool,
	runtimeInMin *types.Int64,
) {
	switch {
	case template.Readme == nil:
		*readme = types.StringNull()
	case readme.IsNull() || readme.IsUnknown() || strings.TrimSpace(readme.ValueString()) != *template.Readme:
		*readme = types.StringValue(*template.Readme)
	}
	*earned = types.Float64PointerValue(template.Earned)
	*isRunpod = types.BoolPointerValue(template.IsRunpod)
	*runtimeInMin = types.Int64PointerValue(template.RuntimeInMin)
}

// clearLegacyTemplateFields nulls the three GraphQL read-backs for the path
// where the shim did not answer, and keeps readme at the value the
// configuration planned, resolving an unknown one to null because state
// cannot hold unknown.
//
// readme is deliberately not nulled: nothing reads it back off a failed call,
// and nulling a configured value would put state at odds with the
// configuration on every later plan, so every later apply would re-run the
// same failing write until the attribute is removed. The data source's
// clearLegacyTemplateDetailFields nulls all four, because nothing there is
// configured.
func clearLegacyTemplateFields(model *templateResourceModel) {
	if model.Readme.IsUnknown() {
		model.Readme = types.StringNull()
	}
	model.Earned = types.Float64Null()
	model.IsRunpod = types.BoolNull()
	model.RuntimeInMin = types.Int64Null()
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
// unconfigured mount block leaves its kind off the body, which a PATCH reads
// as "leave the existing mount alone"; the API has no way to remove one. A
// persistent volume goes alone; network and global volumes may go together.
// The resource's Conflicting validators reject a persistent volume with
// either.
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

	var mounts *sdktypes.Mounts
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
		mounts = &sdktypes.Mounts{Network: &network}
	}

	if !plan.GlobalVolumes.IsNull() && !plan.GlobalVolumes.IsUnknown() {
		var volumes []networkMountModel
		diags.Append(plan.GlobalVolumes.ElementsAs(ctx, &volumes, false)...)
		global := make([]sdktypes.GlobalMount, 0, len(volumes))
		for _, volume := range volumes {
			mount := sdktypes.GlobalMount{VolumeId: volume.VolumeId.ValueString()}
			// An unset path lets the API choose one on create and keeps the
			// current one on a PATCH.
			if !volume.Path.IsNull() && !volume.Path.IsUnknown() {
				mount.Path = volume.Path.ValueStringPointer()
			}
			global = append(global, mount)
		}
		if mounts == nil {
			mounts = &sdktypes.Mounts{}
		}
		mounts.Global = &global
	}

	return mounts, diags
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

	if pod.Mounts.Global != nil && len(*pod.Mounts.Global) > 0 {
		volumes := make([]networkMountModel, 0, len(*pod.Mounts.Global))
		for _, mount := range *pod.Mounts.Global {
			volumes = append(volumes, networkMountModel{
				VolumeId: types.StringValue(mount.VolumeId),
				Path:     types.StringPointerValue(mount.Path),
			})
		}
		global, globalDiags := types.ListValueFrom(ctx, networkMountObjectType, volumes)
		diags.Append(globalDiags...)
		model.GlobalVolumes = global
	} else {
		model.GlobalVolumes = types.ListNull(networkMountObjectType)
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

	entrypoint, entrypointDiags := flattenTemplateEntrypoint(ctx, template.Entrypoint, model.DockerEntrypoint)
	diags.Append(entrypointDiags...)
	model.DockerEntrypoint = entrypoint

	return diags
}

// expandEndpointGlobalMounts builds the endpoint mounts for a create or an
// update from global_volumes. A null or unknown list leaves mounts off the
// body, so an update keeps the endpoint's global volumes; a known list,
// including [], replaces them, which is how a global volume is detached.
func expandEndpointGlobalMounts(ctx context.Context, globalVolumes types.List) (*sdktypes.EndpointMountsInput, diag.Diagnostics) {
	var diags diag.Diagnostics
	if globalVolumes.IsNull() || globalVolumes.IsUnknown() {
		return nil, diags
	}
	var ids []string
	diags.Append(globalVolumes.ElementsAs(ctx, &ids, false)...)
	global := make([]sdktypes.EndpointMount, 0, len(ids))
	for _, id := range ids {
		global = append(global, sdktypes.EndpointMount{VolumeId: id})
	}
	return &sdktypes.EndpointMountsInput{Global: &global}, diags
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
	mounts, mountDiags := expandEndpointGlobalMounts(ctx, plan.GlobalVolumes)
	diags.Append(mountDiags...)
	body.Mounts = mounts
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

	globalVolumes := make([]string, 0, len(endpoint.Mounts.Global))
	for _, mount := range endpoint.Mounts.Global {
		globalVolumes = append(globalVolumes, mount.VolumeId)
	}
	globals, gvDiags := types.ListValueFrom(ctx, types.StringType, globalVolumes)
	diags.Append(gvDiags...)
	model.GlobalVolumes = globals

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
