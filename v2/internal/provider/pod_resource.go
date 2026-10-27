package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
	"github.com/runpod/go-sdk/v2/lib/pods"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

var (
	_ resource.Resource                     = (*podResource)(nil)
	_ resource.ResourceWithConfigure        = (*podResource)(nil)
	_ resource.ResourceWithImportState      = (*podResource)(nil)
	_ resource.ResourceWithConfigValidators = (*podResource)(nil)
)

// mountPresenceReplaceDescription states what the mount plan modifiers can and
// cannot do: the mount kind is fixed at create, so adding a block replaces the
// pod, but the API has no way to detach a mount — and the blocks are
// Optional+Computed with UseStateForUnknown, so a block dropped from the
// configuration, or one a template supplied, plans the value already in state
// and changes nothing, even when another attribute changes.
const mountPresenceReplaceDescription = "Adding the block replaces the pod; the API cannot remove a mount, so dropping the block from the configuration, or leaving out one a template supplied, keeps the mount. Editing the block's fields is applied in place."

func NewPodResource() resource.Resource { return &podResource{} }

type podResource struct {
	client *runpod.Client
	legacy *legacy.Client
}

type podResourceModel struct {
	Id                  types.String  `tfsdk:"id"`
	Name                types.String  `tfsdk:"name"`
	Image               types.String  `tfsdk:"image"`
	TemplateId          types.String  `tfsdk:"template_id"`
	GpuTypeId           types.String  `tfsdk:"gpu_type_id"`
	GpuCount            types.Int64   `tfsdk:"gpu_count"`
	AllowedCudaVersions types.List    `tfsdk:"allowed_cuda_versions"`
	MinCudaVersion      types.String  `tfsdk:"min_cuda_version"`
	MinRamPerGpu        types.Int64   `tfsdk:"min_ram_per_gpu"`
	MinVcpuCountPerGpu  types.Int64   `tfsdk:"min_vcpu_count_per_gpu"`
	CpuTypeId           types.String  `tfsdk:"cpu_type_id"`
	VcpuCount           types.Int64   `tfsdk:"vcpu_count"`
	Cloud               types.String  `tfsdk:"cloud"`
	Disk                types.Int64   `tfsdk:"disk"`
	PersistentVolume    types.Object  `tfsdk:"persistent_volume"`
	NetworkVolumes      types.List    `tfsdk:"network_volumes"`
	GlobalVolumes       types.List    `tfsdk:"global_volumes"`
	StartSsh            types.Bool    `tfsdk:"start_ssh"`
	StartJupyter        types.Bool    `tfsdk:"start_jupyter"`
	Args                types.String  `tfsdk:"args"`
	Env                 types.Map     `tfsdk:"env"`
	Ports               types.List    `tfsdk:"ports"`
	DataCenterIds       types.List    `tfsdk:"data_center_ids"`
	GlobalNetworking    types.Bool    `tfsdk:"global_networking"`
	Registry            types.String  `tfsdk:"registry"`
	DesiredState        types.String  `tfsdk:"desired_state"`
	Status              types.String  `tfsdk:"status"`
	DataCenterId        types.String  `tfsdk:"data_center_id"`
	CreatedAt           types.String  `tfsdk:"created_at"`
	StartedAt           types.String  `tfsdk:"started_at"`
	Cost                types.Float64 `tfsdk:"cost"`
	Locked              types.Bool    `tfsdk:"locked"`
	CudaVersion         types.String  `tfsdk:"cuda_version"`
	MemoryInGb          types.Float64 `tfsdk:"memory_in_gb"`
	ClusterId           types.String  `tfsdk:"cluster_id"`
	ClusterIp           types.String  `tfsdk:"cluster_ip"`
	Runtime             types.Object  `tfsdk:"runtime"`
	MachineId           types.String  `tfsdk:"machine_id"`
	DockerEntrypoint    types.List    `tfsdk:"docker_entrypoint"`
	DockerStartCmd      types.List    `tfsdk:"docker_start_cmd"`
	VolumeEncrypted     types.Bool    `tfsdk:"volume_encrypted"`
}

func (r *podResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pod"
}

func (r *podResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Runpod pod, GPU or CPU, managed through the v2 REST API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Pod identifier assigned by the API.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Pod display name.",
			},
			"image": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Docker image reference. Optional when template_id supplies one; at least one of image or template_id is required.",
			},
			"template_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "Pod template to base the pod on, resolved at create time into the same container settings this resource sets directly (image, args, disk, ports, env, registry, persistent volume, SSH/Jupyter flags, CUDA versions); explicit attributes win, except env, which is merged per key with the pod's values winning. Must not be a serverless template. Changing it replaces the pod. " +
					"The API does not link the created pod back to the template, so a pod created this way reports no template.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"gpu_type_id": schema.StringAttribute{
				Optional:    true,
				Description: "GPU type identifier from the catalog, e.g. \"NVIDIA GeForce RTX 4090\". Exactly one of gpu_type_id or cpu_type_id must be set. Changing it replaces the pod.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"gpu_count": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Number of GPUs. Server default is 1. Changing it replaces the pod.",
				PlanModifiers: []planmodifier.Int64{
					// Computed with a server default: without this a config that
					// omits gpu_count plans it unknown on any other change, and
					// RequiresReplace on unknown recreates the pod.
					int64planmodifier.UseStateForUnknown(),
					int64planmodifier.RequiresReplace(),
				},
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
					int64validator.ConflictsWith(path.MatchRoot("cpu_type_id")),
				},
			},
			"allowed_cuda_versions": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "Acceptable CUDA versions on the host machine, as major.minor, e.g. [\"12.8\", \"12.6\"]. Matching is exact, so a version no machine reports yields a capacity error rather than a fallback. A non-empty list cannot be combined with min_cuda_version. GPU pods only, and the API neither returns nor updates it, so changing it replaces the pod.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
				Validators: []validator.List{
					listvalidator.ConflictsWith(path.MatchRoot("cpu_type_id")),
				},
			},
			"min_cuda_version": schema.StringAttribute{
				Optional:    true,
				Description: "Lowest acceptable CUDA version on the host machine, as major.minor, compared numerically: 12.11 is above 12.2. Use it for an open-ended floor and allowed_cuda_versions for an exact set; a non-empty set cannot be combined with it. GPU pods only, and the API neither returns nor updates it, so changing it replaces the pod.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("cpu_type_id")),
				},
			},
			"min_ram_per_gpu": schema.Int64Attribute{
				Optional:    true,
				Description: "Minimum host system RAM in GB per requested GPU. A placement filter, not a resource request: the pod may be allocated more. GPU pods only, and the API neither returns nor updates it, so changing it replaces the pod.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
					int64validator.ConflictsWith(path.MatchRoot("cpu_type_id")),
				},
			},
			"min_vcpu_count_per_gpu": schema.Int64Attribute{
				Optional:    true,
				Description: "Minimum host vCPU count per requested GPU. A placement filter, not a resource request: the pod may be allocated more. GPU pods only, and the API neither returns nor updates it, so changing it replaces the pod.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
					int64validator.ConflictsWith(path.MatchRoot("cpu_type_id")),
				},
			},
			"cpu_type_id": schema.StringAttribute{
				Optional:    true,
				Description: "CPU flavor identifier from the catalog, e.g. \"cpu5c\", for a CPU-only pod. Exactly one of gpu_type_id or cpu_type_id must be set, and a CPU pod also needs vcpu_count. Changing it replaces the pod.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRoot("vcpu_count")),
				},
			},
			"vcpu_count": schema.Int64Attribute{
				Optional:    true,
				Description: "Number of vCPUs for a CPU pod. Must be valid for the chosen cpu_type_id and a power of two. Changing it replaces the pod.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
					int64validator.AlsoRequires(path.MatchRoot("cpu_type_id")),
				},
			},
			"cloud": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Cloud tier: SECURE or COMMUNITY. Server default is SECURE. Changing it replaces the pod.",
				PlanModifiers: []planmodifier.String{
					// Same as gpu_count: keep the server default from state so an
					// unrelated change on a pod that omits cloud does not replace it.
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(string(sdktypes.CloudSecure), string(sdktypes.CloudCommunity)),
				},
			},
			"disk": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Container disk in GB (ephemeral).",
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"persistent_volume": schema.SingleNestedAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Host-local persistent storage, replacing the 1.x volume_in_gb and volume_mount_path attributes. Pinned to the pod's host machine, so the data does not survive a host failure; prefer network_volumes for anything you cannot recreate. Not available on CPU pods, and mutually exclusive with network_volumes. A template can supply one; the API cannot remove a mount once the pod has it.",
				Attributes: map[string]schema.Attribute{
					"size": schema.Int64Attribute{
						Required:    true,
						Description: "Persistent storage in GB, 10 or more.",
						Validators: []validator.Int64{
							int64validator.AtLeast(10),
						},
					},
					"path": schema.StringAttribute{
						Required:    true,
						Description: "Mount path inside the container, e.g. \"/workspace\".",
					},
				},
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
					requiresReplaceOnObjectAdded(mountPresenceReplaceDescription),
				},
				Validators: []validator.Object{
					objectvalidator.ConflictsWith(path.MatchRoot("cpu_type_id")),
				},
			},
			"network_volumes": schema.ListNestedAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Network volumes mounted into the pod, replacing the 1.x network_volume_id attribute. The volume must live in the same data center as the pod. The API accepts one entry today; the list is the forward-compatible shape. Mutually exclusive with persistent_volume, and the API cannot remove a mount once the pod has it.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"volume_id": schema.StringAttribute{
							Required:    true,
							Description: "ID of an existing runpod_network_volume. Changing it replaces the pod.",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.RequiresReplace(),
							},
						},
						"path": schema.StringAttribute{
							Required:    true,
							Description: "Mount path inside the container, e.g. \"/runpod-volume\".",
						},
					},
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
					requiresReplaceOnListAdded(mountPresenceReplaceDescription),
				},
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
			},
			"global_volumes": schema.ListNestedAttribute{
				Optional: true,
				Computed: true,
				Description: "Global volumes (runpod_global_volume) mounted into the pod. A global volume has no data center, so " +
					"the pod can run in any data center. At most one, combinable with network_volumes and mutually exclusive " +
					"with persistent_volume. Like network_volumes, the mount " +
					"is fixed at create: adding it replaces the pod, the volume cannot change, and only the path can.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"volume_id": schema.StringAttribute{
							Required:    true,
							Description: "ID of an existing runpod_global_volume. Changing it replaces the pod.",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.RequiresReplace(),
							},
						},
						"path": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Mount path inside the container, e.g. \"/data\". The API chooses one when it is left out.",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
					},
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
					requiresReplaceOnListAdded(mountPresenceReplaceDescription),
				},
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
					listvalidator.SizeAtLeast(1),
				},
			},
			"start_ssh": schema.BoolAttribute{
				Optional:    true,
				Description: "Create-time flag telling the provisioner to set up SSH access by injecting a PUBLIC_KEY variable carrying the account's registered SSH keys. Needs keys registered on the account, and an image that honors the convention (the Runpod official images do); the ssh.direct form also needs \"22/tcp\" in ports. The API never returns it and cannot change it, so changing it replaces the pod.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"start_jupyter": schema.BoolAttribute{
				Optional:    true,
				Description: "Create-time flag telling the provisioner to start JupyterLab by injecting a generated JUPYTER_PASSWORD variable. Needs an image that honors the convention (the Runpod official images do) and \"8888/http\" in ports. The API never returns it and cannot change it, so changing it replaces the pod.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"args": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Arguments passed to the container entrypoint.",
			},
			"env": schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Description: "Environment variables. An update merges the configured keys into the pod's environment: a configured key is set, including a configured PUBLIC_KEY or JUPYTER_PASSWORD, and a key removed from the configuration is removed from the pod. Keys the configuration never held, such as a template's env and the PUBLIC_KEY / JUPYTER_PASSWORD that start_ssh / start_jupyter added, stay on the pod. The exception is the first apply that configures env after an import, or after env was left unset: state then holds the pod's whole environment, so every key the configuration leaves out is removed from the pod, template keys included, and only PUBLIC_KEY and JUPYTER_PASSWORD stay. The plan lists those removals.",
			},
			"ports": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Description: "Exposed ports, e.g. [\"8888/http\", \"22/tcp\"].",
			},
			"data_center_ids": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "Preferred data centers for placement. Create-time hint; the scheduler's pick lands in data_center_id. The API neither returns nor updates it, so changing it replaces the pod.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"global_networking": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Enable global networking (private cross-datacenter IP). Requires an NVIDIA GPU and an eligible data center. Changes take effect on the next pod start/restart.",
			},
			"registry": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Registry credential ID (runpod_registry) for pulling private images. A template can supply one. Removing it from the configuration leaves the credential attached: the provider cannot clear it yet.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"desired_state": schema.StringAttribute{
				Optional: true,
				Description: "Managed lifecycle state: RUNNING or EXITED. When set, the provider " +
					"starts/stops the pod to converge, and a refresh reports the pod's actual " +
					"run state, so a pod stopped outside Terraform shows up as a plan diff and " +
					"is started again on the next apply. Unset leaves the pod's run state unmanaged.",
				Validators: []validator.String{
					stringvalidator.OneOf(string(sdktypes.PodStatusRunning), string(sdktypes.PodStatusExited)),
				},
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Pod lifecycle status as reported by the API, e.g. RUNNING, EXITED, PROVISIONING.",
			},
			"data_center_id": schema.StringAttribute{
				Computed:    true,
				Description: "Data center the scheduler placed the pod in.",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "Creation timestamp in RFC 3339 format.",
			},
			"started_at": schema.StringAttribute{
				Computed:    true,
				Description: "When the pod last started, in RFC 3339 format. Null until it has run.",
			},
			"cost": schema.Float64Attribute{
				Computed:    true,
				Description: "Current cost in USD per hour. This replaces the 1.x cost_per_hr, which read the same number off the REST v1 pod. Zero while the pod is EXITED or TERMINATED.",
			},
			"locked": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether the pod is locked, which prevents stopping or resetting it.",
			},
			"cuda_version": schema.StringAttribute{
				Computed:    true,
				Description: "CUDA version the host machine reports. Kept while the pod is stopped, since it resumes onto the same host. Null when unknown or not applicable, as on a CPU pod.",
			},
			"memory_in_gb": schema.Float64Attribute{
				Computed:    true,
				Description: "System RAM in GB allocated to the pod: across all requested GPUs on a GPU pod, or the flavor's allocation on a CPU pod.",
			},
			"cluster_id": schema.StringAttribute{
				Computed:    true,
				Description: "Cluster this pod belongs to, or null for a standalone pod. Cluster members are managed through the cluster, not this resource.",
			},
			"cluster_ip": schema.StringAttribute{
				Computed:    true,
				Description: "The pod's address on its cluster's private overlay network, or null until the address is assigned.",
			},
			"runtime": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "Live utilization the API reports for a RUNNING pod; null in every other status.",
				Attributes: map[string]schema.Attribute{
					"uptime_seconds": schema.Int64Attribute{
						Computed:    true,
						Description: "Seconds since the container started.",
					},
					"cpu_util": schema.Int64Attribute{
						Computed:    true,
						Description: "CPU utilization percentage, 0 to 100.",
					},
					"memory_util": schema.Int64Attribute{
						Computed:    true,
						Description: "Memory utilization percentage, 0 to 100.",
					},
					"gpus": schema.ListNestedAttribute{
						Computed:    true,
						Description: "One entry per attached GPU, in the order the API reports them.",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"util": schema.Int64Attribute{
									Computed:    true,
									Description: "GPU utilization percentage, 0 to 100.",
								},
								"memory_util": schema.Int64Attribute{
									Computed:    true,
									Description: "GPU memory utilization percentage, 0 to 100.",
								},
							},
						},
					},
					"ports": schema.ListNestedAttribute{
						Computed:    true,
						Description: "Port mappings the running pod was assigned. This is the runtime view of the configurable ports attribute: one entry per exposed port, with the address it is reachable on.",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"ip": schema.StringAttribute{
									Computed:    true,
									Description: "Address the port is reachable on.",
								},
								"private_port": schema.Int64Attribute{
									Computed:    true,
									Description: "Port inside the container.",
								},
								"public_port": schema.Int64Attribute{
									Computed:    true,
									Description: "Port published outside the container.",
								},
								"type": schema.StringAttribute{
									Computed:    true,
									Description: "Protocol of the mapping, e.g. http or tcp.",
								},
							},
						},
					},
				},
			},
			"machine_id": schema.StringAttribute{
				Computed: true,
				Description: "Deprecated. Host machine the pod is placed on, read from the GraphQL API's pod. " +
					"Null when that API does not report the pod, reports an empty machine id, or cannot be reached. " +
					legacyShimDescriptionSuffix,
				DeprecationMessage: legacyShimDeprecation,
				PlanModifiers:      []planmodifier.String{legacyShimUnknownString()},
			},
			"docker_entrypoint": schema.ListAttribute{
				ElementType:   types.StringType,
				Computed:      true,
				Description:   "Container entrypoint the pod runs, in exec form, e.g. [\"/bin/bash\", \"-c\"], as the API decodes it from args. Read-only. Set the command through args.",
				PlanModifiers: []planmodifier.List{usePodCommandStateUnlessArgsChanges()},
			},
			"docker_start_cmd": schema.ListAttribute{
				ElementType:   types.StringType,
				Computed:      true,
				Description:   "Container start command the pod runs, in exec form, as the API decodes it from args; when there is an entrypoint, this is the argument list passed to it. Read-only. Set the command through args.",
				PlanModifiers: []planmodifier.List{usePodCommandStateUnlessArgsChanges()},
			},
			"volume_encrypted": schema.BoolAttribute{
				Computed: true,
				Description: "Deprecated. Whether the pod's volume is encrypted, read from the GraphQL API's pod: " +
					"false for an unencrypted volume, null only when that API does not report the pod or cannot be reached. " +
					legacyShimDescriptionSuffix,
				DeprecationMessage: legacyShimDeprecation,
				PlanModifiers:      []planmodifier.Bool{legacyShimUnknownBool()},
			},
		},
	}
}

func (r *podResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, &resp.Diagnostics)
	r.legacy = configuredLegacyClient(req.ProviderData, &resp.Diagnostics)
}

// ConfigValidators carry the cross-attribute rules the API enforces: a pod is
// either a GPU pod or a CPU pod, its container settings come from an image or
// a template (or both), and a persistent volume excludes network and global
// volumes, which can be combined with each other.
func (r *podResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("gpu_type_id"),
			path.MatchRoot("cpu_type_id"),
		),
		resourcevalidator.AtLeastOneOf(
			path.MatchRoot("image"),
			path.MatchRoot("template_id"),
		),
		resourcevalidator.Conflicting(
			path.MatchRoot("persistent_volume"),
			path.MatchRoot("network_volumes"),
		),
		resourcevalidator.Conflicting(
			path.MatchRoot("persistent_volume"),
			path.MatchRoot("global_volumes"),
		),
	}
}

func (r *podResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan podResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, diags := expandCreatePod(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.Pods.CreatePodWithResponse(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating pod failed", err.Error())
		return
	}
	if apiResp.JSON201 == nil {
		resp.Diagnostics.AddError(
			"Creating pod failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	configuredEnv := !plan.Env.IsNull() && !plan.Env.IsUnknown()
	resp.Diagnostics.Append(flattenPod(ctx, apiResp.JSON201, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if configuredEnv {
		resp.Diagnostics.Append(setEnvConfigured(ctx, resp.Private, true)...)
	}
	// The pod exists and bills from here on, whether or not the desired state
	// converges. A stop from PROVISIONING or STARTING is the designed path
	// and the one the API is most likely to refuse, so the read-backs are
	// filled and the state written before any of that is reported. Returning
	// first would leave the pod running with nothing in state.
	convergeDiags := r.convergeDesiredState(ctx, &plan)
	legacyDiags := r.readLegacyPodFields(ctx, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	resp.Diagnostics.Append(convergeDiags...)
	resp.Diagnostics.Append(legacyDiags...)
}

// readLegacyPodFields fills the deprecated GraphQL read-backs after the v2
// call has settled the rest of the model. This resource has no shim input, so
// nothing the configuration asks for depends on the GraphQL API: a failure
// warns and leaves the two attributes null, and a pod that API does not know
// leaves them null silently.
func (r *podResource) readLegacyPodFields(ctx context.Context, model *podResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if r.legacy == nil {
		clearLegacyPodFields(model)
		return diags
	}

	pod, err := fetchLegacyPodFields(ctx, r.legacy, model.Id.ValueString())
	if err != nil {
		clearLegacyPodFields(model)
		appendLegacyReadFailure(&diags, "pod", err)
		return diags
	}
	if pod == nil {
		clearLegacyPodFields(model)
		return diags
	}
	flattenLegacyPod(pod, model)
	return diags
}

// convergeDesiredState starts or stops the pod when desired_state is set and
// the API-reported status disagrees; the model is re-flattened from the
// action's pod response.
func (r *podResource) convergeDesiredState(ctx context.Context, model *podResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if model.DesiredState.IsNull() || model.DesiredState.IsUnknown() {
		return diags
	}

	action, needed := podActionForDesiredState(
		model.DesiredState.ValueString(),
		sdktypes.PodStatus(model.Status.ValueString()),
	)
	if !needed {
		return diags
	}

	apiResp, err := r.client.Pods.PodActionWithResponse(ctx, model.Id.ValueString(),
		pods.PodActionJSONRequestBody{Action: action})
	if err != nil {
		diags.AddError("Pod state change failed", err.Error())
		return diags
	}
	if apiResp.JSON200 == nil {
		diags.AddError("Pod state change failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)))
		return diags
	}
	desiredState := model.DesiredState
	diags.Append(flattenPod(ctx, apiResp.JSON200, model)...)
	model.DesiredState = desiredState
	return diags
}

func (r *podResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state podResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.Pods.GetPodWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading pod failed", err.Error())
		return
	}
	if apiResp.HTTPResponse.StatusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Reading pod failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	resp.Diagnostics.Append(flattenPod(ctx, apiResp.JSON200, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// An apply leaves a configured env exactly as planned, because the API
	// merges the template's env in and injects its own keys. A refresh is
	// therefore the only place a changed value can surface, and it looks at
	// the configured keys alone.
	resp.Diagnostics.Append(resyncConfiguredEnv(ctx, apiResp.JSON200.Env, &state.Env)...)
	if resp.Diagnostics.HasError() {
		return
	}
	refreshDesiredState(&state)
	resp.Diagnostics.Append(r.readLegacyPodFields(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *podResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state podResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := pods.UpdatePodJSONRequestBody{
		Name: plan.Name.ValueStringPointer(),
	}
	// template_id is create-only for this resource (changing it replaces the
	// pod), so the PATCH never re-applies a template — the pod's own
	// attributes carry the settings the template seeded at create.
	if !plan.Image.IsNull() && !plan.Image.IsUnknown() {
		body.Image = plan.Image.ValueStringPointer()
	}
	// Mount fields are patchable; the mount kind is not, which the schema
	// handles by replacing the pod when a mount block is added.
	mounts, mountDiags := expandPodMounts(ctx, plan)
	resp.Diagnostics.Append(mountDiags...)
	body.Mounts = mounts
	if !plan.Args.IsNull() && !plan.Args.IsUnknown() {
		body.Args = plan.Args.ValueStringPointer()
	}
	if !plan.GlobalNetworking.IsNull() && !plan.GlobalNetworking.IsUnknown() {
		body.GlobalNetworking = plan.GlobalNetworking.ValueBoolPointer()
	}
	if !plan.Registry.IsNull() && !plan.Registry.IsUnknown() {
		body.Registry = plan.Registry.ValueStringPointer()
	}
	if !plan.Disk.IsNull() && !plan.Disk.IsUnknown() {
		disk := int(plan.Disk.ValueInt64())
		body.Disk = &disk
	}
	if !plan.Ports.IsNull() && !plan.Ports.IsUnknown() {
		var ports []string
		resp.Diagnostics.Append(plan.Ports.ElementsAs(ctx, &ports, false)...)
		body.Ports = &ports
	}
	if resp.Diagnostics.HasError() {
		return
	}
	priorConfigured, privateDiags := envConfigured(ctx, req.Private)
	resp.Diagnostics.Append(privateDiags...)
	env, envDiags := r.envForUpdate(ctx, state.Id.ValueString(), plan.Env, state.Env, priorConfigured)
	resp.Diagnostics.Append(envDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	body.Env = env
	plannedConfigured := !plan.Env.IsNull() && !plan.Env.IsUnknown()

	apiResp, err := r.client.Pods.UpdatePodWithResponse(ctx, state.Id.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Updating pod failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Updating pod failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	resp.Diagnostics.Append(flattenPod(ctx, apiResp.JSON200, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if plannedConfigured != priorConfigured {
		resp.Diagnostics.Append(setEnvConfigured(ctx, resp.Private, plannedConfigured)...)
	}
	// As on Create: the v2 update has already changed the pod, so the state is
	// written before a refused state change or a failed legacy read is
	// reported. Returning first would leave state describing the env, ports
	// and disk the pod had before this apply.
	convergeDiags := r.convergeDesiredState(ctx, &plan)
	legacyDiags := r.readLegacyPodFields(ctx, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	resp.Diagnostics.Append(convergeDiags...)
	resp.Diagnostics.Append(legacyDiags...)
}

// envForUpdate returns the env an update sends, or nil to leave the pod's env
// alone. The PATCH replaces the whole env, so it is sent only when the
// configured env changed, and then merged onto the pod's live env: sending the
// configured keys alone would drop every key the configuration never held,
// such as a template's env merged in at create and the PUBLIC_KEY and
// JUPYTER_PASSWORD the provisioner injected.
func (r *podResource) envForUpdate(ctx context.Context, id string, planned, prior types.Map, priorConfigured bool) (*map[string]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if planned.IsNull() || planned.IsUnknown() || planned.Equal(prior) {
		return nil, diags
	}

	var plannedEnv, priorEnv map[string]string
	diags.Append(planned.ElementsAs(ctx, &plannedEnv, false)...)
	if !prior.IsNull() && !prior.IsUnknown() {
		diags.Append(prior.ElementsAs(ctx, &priorEnv, false)...)
	}
	if diags.HasError() {
		return nil, diags
	}

	apiResp, err := r.client.Pods.GetPodWithResponse(ctx, id)
	if err != nil {
		diags.AddError("Reading the pod's environment failed", err.Error())
		return nil, diags
	}
	if apiResp.JSON200 == nil {
		diags.AddError("Reading the pod's environment failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)))
		return nil, diags
	}

	env := mergeConfiguredEnv(apiResp.JSON200.Env, priorEnv, plannedEnv, priorConfigured)
	return &env, diags
}

// envConfiguredKey is the private state key recording that the env in state
// is the configuration's own. Without it, the env was read whole from the API,
// and the merge treats the provisioner's two keys in it as not configured. A
// state written before the key existed reads as not configured, which keeps
// those two keys on the pod rather than deleting them.
const envConfiguredKey = "env_configured"

// privateState is the part of the framework's private state an update reads
// and writes; the framework's own type sits in an internal package.
type privateState interface {
	GetKey(ctx context.Context, key string) ([]byte, diag.Diagnostics)
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

func envConfigured(ctx context.Context, private privateState) (bool, diag.Diagnostics) {
	value, diags := private.GetKey(ctx, envConfiguredKey)
	return string(value) == "true", diags
}

func setEnvConfigured(ctx context.Context, private privateState, configured bool) diag.Diagnostics {
	if !configured {
		// A nil value removes the key.
		return private.SetKey(ctx, envConfiguredKey, nil)
	}
	return private.SetKey(ctx, envConfiguredKey, []byte("true"))
}

func (r *podResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state podResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.Pods.DeletePodWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Deleting pod failed", err.Error())
		return
	}
	switch apiResp.HTTPResponse.StatusCode {
	case http.StatusNoContent, http.StatusNotFound:
		// Deleted, or already gone.
	default:
		resp.Diagnostics.AddError(
			"Deleting pod failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
	}
}

func (r *podResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

const podCommandPlanDescription = "Keeps the prior value on an update unless it sends a different args, the field the API decodes this list from."

// usePodCommandStateUnlessArgsChanges is UseStateForUnknown for
// docker_entrypoint and docker_start_cmd, limited to updates that leave the
// pod's command alone. The API decodes both from args, so an update that sends
// no args, or the args already in state, cannot change them, and without this
// every update would show them as known after apply. An update that sends a
// different args, or one not known until apply, leaves them unknown, because
// promising the old lists there would fail the apply with an inconsistent
// result.
func usePodCommandStateUnlessArgsChanges() planmodifier.List { return podCommandPlanModifier{} }

type podCommandPlanModifier struct{}

func (podCommandPlanModifier) Description(context.Context) string { return podCommandPlanDescription }

func (podCommandPlanModifier) MarkdownDescription(context.Context) string {
	return podCommandPlanDescription
}

func (podCommandPlanModifier) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	// A create has no prior value to keep, and a planned value that is already
	// known needs nothing.
	if req.State.Raw.IsNull() || !req.PlanValue.IsUnknown() {
		return
	}

	var configArgs, stateArgs types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("args"), &configArgs)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("args"), &stateArgs)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Update sends args only when the configuration sets it.
	if configArgs.IsNull() || configArgs.Equal(stateArgs) {
		resp.PlanValue = req.StateValue
	}
}
