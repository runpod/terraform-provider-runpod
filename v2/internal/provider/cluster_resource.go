package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/lib/clusters"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

var (
	_ resource.Resource                = (*clusterResource)(nil)
	_ resource.ResourceWithConfigure   = (*clusterResource)(nil)
	_ resource.ResourceWithImportState = (*clusterResource)(nil)
)

func NewClusterResource() resource.Resource { return &clusterResource{} }

type clusterResource struct {
	client *runpod.Client
}

// clusterMountsModel mirrors the mounts object of the create request. The API
// takes network mounts as an array for forward compatibility but caps it at
// one entry, so the list is validated to that size rather than collapsed.
type clusterMountsModel struct {
	Persistent types.Object `tfsdk:"persistent"`
	Network    types.List   `tfsdk:"network"`
}

// clusterResourceModel carries the create-request fields the API does not echo
// back (image, args, env, ports, disk, mounts, start_ssh, start_jupyter,
// data_center_ids) alongside the cluster fields it does. The echoed ones are
// refreshed on every read; the create-only ones keep their configured value.
type clusterResourceModel struct {
	Id                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Type               types.String `tfsdk:"type"`
	GpuTypeId          types.String `tfsdk:"gpu_type_id"`
	GpuCountPerPod     types.Int64  `tfsdk:"gpu_count_per_pod"`
	PodCount           types.Int64  `tfsdk:"pod_count"`
	Image              types.String `tfsdk:"image"`
	Args               types.String `tfsdk:"args"`
	Disk               types.Int64  `tfsdk:"disk"`
	Env                types.Map    `tfsdk:"env"`
	Ports              types.List   `tfsdk:"ports"`
	DataCenterIds      types.List   `tfsdk:"data_center_ids"`
	Mounts             types.Object `tfsdk:"mounts"`
	StartSsh           types.Bool   `tfsdk:"start_ssh"`
	StartJupyter       types.Bool   `tfsdk:"start_jupyter"`
	DataCenterId       types.String `tfsdk:"data_center_id"`
	Template           types.String `tfsdk:"template"`
	CreatedAt          types.String `tfsdk:"created_at"`
	PodsTotal          types.Int64  `tfsdk:"pods_total"`
	PodsByStatus       types.Map    `tfsdk:"pods_by_status"`
	PrimaryPodId       types.String `tfsdk:"primary_pod_id"`
	PrimaryStatus      types.String `tfsdk:"primary_status"`
	PrimarySshEndpoint types.String `tfsdk:"primary_ssh_endpoint"`
	NetworkCidr        types.String `tfsdk:"network_cidr"`
	NetworkVxlanId     types.Int64  `tfsdk:"network_vxlan_id"`
	NetworkVxlanPort   types.Int64  `tfsdk:"network_vxlan_port"`
}

func (r *clusterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (r *clusterResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A multi-node cluster of identical pods on one private overlay network. " +
			"Only the name can be changed after creation: the compute shape, cluster type, and " +
			"container configuration are fixed, so changing any of them replaces the cluster. " +
			"Member pods are managed through the cluster, not through runpod_pod; read them with " +
			"the runpod_cluster_pods data source.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Cluster identifier assigned by the API.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Cluster display name. The only attribute the API can change in place.",
			},
			"type": schema.StringAttribute{
				Required: true,
				Description: "Cluster type: TRAINING for generic distributed training, SLURM for a " +
					"managed Slurm controller/compute topology, RAY for a managed Ray head/worker " +
					"topology, APPLICATION for a general multi-node application. Changing it replaces the cluster.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(sdktypes.ClusterTypeTraining),
						string(sdktypes.ClusterTypeSlurm),
						string(sdktypes.ClusterTypeRay),
						string(sdktypes.ClusterTypeApplication),
					),
				},
			},
			"gpu_type_id": schema.StringAttribute{
				Required: true,
				Description: "GPU type for every pod in the cluster, e.g. \"NVIDIA H100 80GB HBM3\". " +
					"Changing it replaces the cluster.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"gpu_count_per_pod": schema.Int64Attribute{
				Required: true,
				Description: "Number of GPUs on each pod, bounded by the GPU type's per-cloud maximum. " +
					"Total GPUs are pod_count * gpu_count_per_pod. Changing it replaces the cluster.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"pod_count": schema.Int64Attribute{
				Required:    true,
				Description: "Number of pods (nodes) in the cluster. Changing it replaces the cluster.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"image": schema.StringAttribute{
				Optional: true,
				Description: "Docker image every member pod runs. Public images only, because the cluster " +
					"create input takes no registry credential. Changing it replaces the cluster.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"args": schema.StringAttribute{
				Optional:    true,
				Description: "Arguments passed to the container entrypoint. Changing them replaces the cluster.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"disk": schema.Int64Attribute{
				Optional: true,
				Description: "Container disk in GB on each pod (ephemeral, wiped on restart). " +
					"Changing it replaces the cluster.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"env": schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "Environment variables set on every member pod. Changing them replaces the cluster.",
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
			"ports": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "Exposed ports on every member pod, e.g. [\"8888/http\", \"22/tcp\"]. " +
					"Changing them replaces the cluster.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"data_center_ids": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "Preferred data centers for placement. Omit to let the scheduler choose; " +
					"a cluster is always placed within a single data center, reported in data_center_id. " +
					"Changing the preference replaces the cluster.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"mounts": schema.SingleNestedAttribute{
				Optional: true,
				Description: "Storage mounted into every member pod. Set at most one of persistent and " +
					"network. Changing a mount replaces the cluster.",
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
				Attributes: map[string]schema.Attribute{
					"persistent": schema.SingleNestedAttribute{
						Optional: true,
						Description: "Host-local persistent storage, pinned to each pod's host machine, so " +
							"the data does not survive a host failure. Prefer a network mount for anything " +
							"you cannot recreate.",
						Validators: []validator.Object{
							objectvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("network")),
						},
						Attributes: map[string]schema.Attribute{
							"path": schema.StringAttribute{
								Required:    true,
								Description: "Mount path inside the container, e.g. \"/workspace\".",
							},
							"size": schema.Int64Attribute{
								Required:    true,
								Description: "Host-local storage in GB; the API enforces a 10 GB floor.",
								Validators: []validator.Int64{
									int64validator.AtLeast(10),
								},
							},
						},
					},
					"network": schema.ListNestedAttribute{
						Optional: true,
						Description: "An existing network volume mounted into every member pod. The API " +
							"takes a list for forward compatibility but accepts at most one entry.",
						Validators: []validator.List{
							listvalidator.SizeAtMost(1),
						},
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"path": schema.StringAttribute{
									Required:    true,
									Description: "Mount path inside the container, e.g. \"/runpod-volume\".",
								},
								"volume_id": schema.StringAttribute{
									Required:    true,
									Description: "ID of a runpod_network_volume in the same data center as the cluster.",
								},
							},
						},
					},
				},
			},
			"start_ssh": schema.BoolAttribute{
				Optional: true,
				Description: "Provision SSH access on every member pod by injecting the account's " +
					"registered SSH public keys (see runpod_account_ssh_keys). Changing it replaces the cluster.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"start_jupyter": schema.BoolAttribute{
				Optional:    true,
				Description: "Start Jupyter on every member pod. Changing it replaces the cluster.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"data_center_id": schema.StringAttribute{
				Computed: true,
				Description: "Data center the cluster was placed in, derived from the member pods. " +
					"Null until at least one pod is placed.",
			},
			"template": schema.StringAttribute{
				Computed:    true,
				Description: "Template the member pods were created from; null when they were not created from one.",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "Creation timestamp in RFC 3339 format.",
			},
			"pods_total": schema.Int64Attribute{
				Computed:    true,
				Description: "Number of member pods currently provisioned for the cluster.",
			},
			"pods_by_status": schema.MapAttribute{
				ElementType: types.Int64Type,
				Computed:    true,
				Description: "Member pod counts keyed by pod status, e.g. {RUNNING = 4}. " +
					"Statuses with no pods are absent.",
			},
			"primary_pod_id": schema.StringAttribute{
				Computed: true,
				Description: "Pod id of the primary (rank 0) node: the Slurm controller or Ray head. " +
					"Null until a primary pod is placed.",
			},
			"primary_status": schema.StringAttribute{
				Computed:    true,
				Description: "Lifecycle status of the primary node, e.g. RUNNING, PROVISIONING.",
			},
			"primary_ssh_endpoint": schema.StringAttribute{
				Computed: true,
				Description: "Public SSH endpoint (host:port) of the primary node. Null until it is " +
					"RUNNING with 22/tcp exposed.",
			},
			"network_cidr": schema.StringAttribute{
				Computed:    true,
				Description: "CIDR block of the cluster's private overlay network.",
			},
			"network_vxlan_id": schema.Int64Attribute{
				Computed:    true,
				Description: "VXLAN network identifier; null until assigned.",
			},
			"network_vxlan_port": schema.Int64Attribute{
				Computed:    true,
				Description: "UDP port carrying the VXLAN traffic; null until assigned.",
			},
		},
	}
}

func (r *clusterResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (r *clusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, diags := expandCreateCluster(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.Clusters.CreateClusterWithResponse(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating cluster failed", err.Error())
		return
	}
	if apiResp.JSON201 == nil {
		resp.Diagnostics.AddError(
			"Creating cluster failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	resp.Diagnostics.Append(flattenCluster(ctx, apiResp.JSON201, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.Clusters.GetClusterWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading cluster failed", err.Error())
		return
	}
	if apiResp.HTTPResponse.StatusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Reading cluster failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	resp.Diagnostics.Append(flattenCluster(ctx, apiResp.JSON200, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state clusterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The endpoint is a rename: every other attribute carries RequiresReplace.
	body := clusters.UpdateClusterJSONRequestBody{Name: plan.Name.ValueStringPointer()}

	apiResp, err := r.client.Clusters.UpdateClusterWithResponse(ctx, state.Id.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Updating cluster failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Updating cluster failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	resp.Diagnostics.Append(flattenCluster(ctx, apiResp.JSON200, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.Clusters.DeleteClusterWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Deleting cluster failed", err.Error())
		return
	}
	switch apiResp.HTTPResponse.StatusCode {
	case http.StatusNoContent, http.StatusNotFound:
		// Deleted, or already gone.
	default:
		resp.Diagnostics.AddError(
			"Deleting cluster failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
	}
}

func (r *clusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// expandCluster* / flattenCluster* keep the plan<->request mapping unit-testable
// without the plugin protocol; the cluster files carry their own mapping rather
// than sharing mapping.go so the nested mounts object stays next to its schema.

func expandCreateCluster(ctx context.Context, plan clusterResourceModel) (clusters.CreateClusterJSONRequestBody, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := clusters.CreateClusterJSONRequestBody{
		Name: plan.Name.ValueString(),
		Type: sdktypes.ClusterType(plan.Type.ValueString()),
		Compute: sdktypes.ClusterCompute{
			GpuTypeId:      plan.GpuTypeId.ValueString(),
			GpuCountPerPod: int(plan.GpuCountPerPod.ValueInt64()),
			PodCount:       int(plan.PodCount.ValueInt64()),
		},
	}

	if !plan.Image.IsNull() && !plan.Image.IsUnknown() {
		body.Image = plan.Image.ValueStringPointer()
	}
	if !plan.Args.IsNull() && !plan.Args.IsUnknown() {
		body.Args = plan.Args.ValueStringPointer()
	}
	if !plan.Disk.IsNull() && !plan.Disk.IsUnknown() {
		disk := int(plan.Disk.ValueInt64())
		body.Disk = &disk
	}
	if !plan.StartSsh.IsNull() && !plan.StartSsh.IsUnknown() {
		body.StartSsh = plan.StartSsh.ValueBoolPointer()
	}
	if !plan.StartJupyter.IsNull() && !plan.StartJupyter.IsUnknown() {
		body.StartJupyter = plan.StartJupyter.ValueBoolPointer()
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
	if !plan.DataCenterIds.IsNull() && !plan.DataCenterIds.IsUnknown() {
		var ids []string
		diags.Append(plan.DataCenterIds.ElementsAs(ctx, &ids, false)...)
		body.DataCenterIds = &ids
	}

	mounts, mountDiags := expandClusterMounts(ctx, plan.Mounts)
	diags.Append(mountDiags...)
	body.Mounts = mounts

	return body, diags
}

// expandClusterMounts returns nil when mounts is absent: the API leaves a
// cluster mountless on an omitted mounts object, and an empty one would be a
// different request.
func expandClusterMounts(ctx context.Context, object types.Object) (*sdktypes.ClusterMounts, diag.Diagnostics) {
	var diags diag.Diagnostics
	if object.IsNull() || object.IsUnknown() {
		return nil, diags
	}

	var model clusterMountsModel
	diags.Append(object.As(ctx, &model, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil, diags
	}

	mounts := &sdktypes.ClusterMounts{}
	if !model.Persistent.IsNull() && !model.Persistent.IsUnknown() {
		var persistent persistentMountModel
		diags.Append(model.Persistent.As(ctx, &persistent, basetypes.ObjectAsOptions{})...)
		if diags.HasError() {
			return nil, diags
		}
		mounts.Persistent = &sdktypes.PersistentMount{
			Path: persistent.Path.ValueString(),
			Size: int(persistent.Size.ValueInt64()),
		}
	}
	if !model.Network.IsNull() && !model.Network.IsUnknown() {
		var entries []networkMountModel
		diags.Append(model.Network.ElementsAs(ctx, &entries, false)...)
		if diags.HasError() {
			return nil, diags
		}
		network := make([]sdktypes.NetworkMount, 0, len(entries))
		for _, entry := range entries {
			network = append(network, sdktypes.NetworkMount{
				Path:     entry.Path.ValueString(),
				VolumeId: entry.VolumeId.ValueString(),
			})
		}
		mounts.Network = &network
	}
	if mounts.Persistent == nil && mounts.Network == nil {
		return nil, diags
	}
	return mounts, diags
}

// flattenCluster writes back only what the API echoes. The container
// configuration is create-only input the cluster object does not carry, so
// image, args, disk, env, ports, data_center_ids, mounts, start_ssh, and
// start_jupyter keep their configured values.
func flattenCluster(ctx context.Context, cluster *sdktypes.Cluster, model *clusterResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	model.Id = types.StringValue(cluster.Id)
	model.Name = types.StringValue(cluster.Name)
	model.Type = types.StringValue(string(cluster.Type))
	model.GpuTypeId = types.StringValue(cluster.Compute.GpuTypeId)
	model.GpuCountPerPod = types.Int64Value(int64(cluster.Compute.GpuCountPerPod))
	model.PodCount = types.Int64Value(int64(cluster.Compute.PodCount))
	model.DataCenterId = types.StringPointerValue(cluster.DataCenterId)
	model.Template = types.StringPointerValue(cluster.Template)
	model.CreatedAt = types.StringValue(cluster.CreatedAt.Format(time.RFC3339))
	model.PodsTotal = types.Int64Value(int64(cluster.Pods.Total))

	byStatus := map[string]int64{}
	for status, count := range cluster.Pods.ByStatus {
		byStatus[status] = int64(count)
	}
	podsByStatus, statusDiags := types.MapValueFrom(ctx, types.Int64Type, byStatus)
	diags.Append(statusDiags...)
	model.PodsByStatus = podsByStatus

	if cluster.Primary != nil {
		model.PrimaryPodId = types.StringValue(cluster.Primary.PodId)
		model.PrimaryStatus = types.StringValue(string(cluster.Primary.Status))
		model.PrimarySshEndpoint = types.StringPointerValue(cluster.Primary.SshEndpoint)
	} else {
		model.PrimaryPodId = types.StringNull()
		model.PrimaryStatus = types.StringNull()
		model.PrimarySshEndpoint = types.StringNull()
	}

	if cluster.Network != nil {
		model.NetworkCidr = types.StringValue(cluster.Network.Cidr)
		model.NetworkVxlanId = int64PointerValue(cluster.Network.VxlanId)
		model.NetworkVxlanPort = int64PointerValue(cluster.Network.VxlanPort)
	} else {
		model.NetworkCidr = types.StringNull()
		model.NetworkVxlanId = types.Int64Null()
		model.NetworkVxlanPort = types.Int64Null()
	}

	return diags
}
