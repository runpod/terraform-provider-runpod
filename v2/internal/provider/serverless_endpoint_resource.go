package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
	"github.com/runpod/go-sdk/v2/lib/serverless"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

var (
	_ resource.Resource                     = (*serverlessEndpointResource)(nil)
	_ resource.ResourceWithConfigure        = (*serverlessEndpointResource)(nil)
	_ resource.ResourceWithImportState      = (*serverlessEndpointResource)(nil)
	_ resource.ResourceWithConfigValidators = (*serverlessEndpointResource)(nil)
	_ resource.ResourceWithValidateConfig   = (*serverlessEndpointResource)(nil)
)

// One worker of the deprecated workers read-back. 2.0.0 reports the same
// workers, with more detail, through the runpod_endpoint_workers data source;
// this shape is the 1.x attribute, kept for configurations that index into it.

type endpointLegacyWorkerModel struct {
	Id         types.String `tfsdk:"id"`
	PodId      types.String `tfsdk:"pod_id"`
	Status     types.String `tfsdk:"status"`
	UptimeMs   types.Int64  `tfsdk:"uptime_ms"`
	StartTime  types.String `tfsdk:"start_time"`
	LastBusyMs types.Int64  `tfsdk:"last_busy_ms"`
}

var endpointLegacyWorkerAttrTypes = map[string]attr.Type{
	"id":           types.StringType,
	"pod_id":       types.StringType,
	"status":       types.StringType,
	"uptime_ms":    types.Int64Type,
	"start_time":   types.StringType,
	"last_busy_ms": types.Int64Type,
}

var endpointLegacyWorkerObjectType = types.ObjectType{AttrTypes: endpointLegacyWorkerAttrTypes}

// endpointWorkersDeprecation is the DeprecationMessage of the two 1.x
// read-backs REST v2 serves through its workers listing.
const endpointWorkersDeprecation = "Use the runpod_endpoint_workers data source, which reads the same REST v2 workers listing"

// endpointComputeTypeDeprecation is the DeprecationMessage of compute_type,
// which restates the compute block.
const endpointComputeTypeDeprecation = "Derived from gpu_pools or cpu; remove it from the configuration"

// computeFamilyReplaceDescription covers both compute blocks: the API
// refuses to move an endpoint between the GPU and CPU families, so the one
// change the plan modifier catches is a block appearing or disappearing.
const computeFamilyReplaceDescription = "Adding or removing the block replaces the endpoint: the API cannot move an endpoint between the GPU and CPU families. Editing the block's contents is applied in place."

func NewServerlessEndpointResource() resource.Resource { return &serverlessEndpointResource{} }

type serverlessEndpointResource struct {
	client *runpod.Client
	legacy *legacy.Client
}

type serverlessEndpointResourceModel struct {
	Id                  types.String  `tfsdk:"id"`
	Name                types.String  `tfsdk:"name"`
	Image               types.String  `tfsdk:"image"`
	TemplateId          types.String  `tfsdk:"template_id"`
	EndpointType        types.String  `tfsdk:"endpoint_type"`
	GpuPools            types.List    `tfsdk:"gpu_pools"`
	GpuCount            types.Int64   `tfsdk:"gpu_count"`
	ExcludedTypes       types.List    `tfsdk:"excluded_types"`
	AllowedCudaVersions types.List    `tfsdk:"allowed_cuda_versions"`
	MinCudaVersion      types.String  `tfsdk:"min_cuda_version"`
	Cpu                 types.List    `tfsdk:"cpu"`
	Flashboot           types.String  `tfsdk:"flashboot"`
	ScalingType         types.String  `tfsdk:"scaling_type"`
	ScalingValue        types.Float64 `tfsdk:"scaling_value"`
	WorkersMin          types.Int64   `tfsdk:"workers_min"`
	WorkersMax          types.Int64   `tfsdk:"workers_max"`
	IdleTimeout         types.Int64   `tfsdk:"idle_timeout"`
	Disk                types.Int64   `tfsdk:"disk"`
	Env                 types.Map     `tfsdk:"env"`
	Timeout             types.Int64   `tfsdk:"timeout"`
	Registry            types.String  `tfsdk:"registry"`
	DataCenterIds       types.List    `tfsdk:"data_center_ids"`
	NetworkVolumes      types.List    `tfsdk:"network_volumes"`
	GlobalVolumes       types.List    `tfsdk:"global_volumes"`
	Ports               types.List    `tfsdk:"ports"`
	RequestUrls         types.Object  `tfsdk:"request_urls"`
	CreatedAt           types.String  `tfsdk:"created_at"`
	ComputeType         types.String  `tfsdk:"compute_type"`
	GpuTypePriority     types.String  `tfsdk:"gpu_type_priority"`
	CpuFlavorPriority   types.String  `tfsdk:"cpu_flavor_priority"`
	TemplateVersion     types.Int64   `tfsdk:"template_version"`
	Users               types.List    `tfsdk:"users"`
	Version             types.Int64   `tfsdk:"version"`
	Workers             types.List    `tfsdk:"workers"`
}

func (r *serverlessEndpointResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_serverless_endpoint"
}

func (r *serverlessEndpointResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Serverless endpoint with autoscaled workers.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Endpoint identifier assigned by the API.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true, Description: "Endpoint name."},
			"image": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Docker image reference for workers. Optional when template_id supplies one; at least one of image or template_id is required.",
			},
			"template_id": schema.StringAttribute{
				Optional: true,
				Description: "Serverless template to base the endpoint on, resolved into the same container settings this resource sets directly (image, args, disk, ports, env, registry); explicit attributes win, except env, which is merged per key with the endpoint's values winning. " +
					"Must be a serverless template. The API applies it once and keeps no link to it, so the value is remembered from the configuration rather than read back.",
			},
			"endpoint_type": schema.StringAttribute{
				Required:    true,
				Description: "Request routing: QUEUE (managed job queue) or LOAD_BALANCER (direct HTTP to worker-defined paths, declared in ports). Changing it replaces the endpoint.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(sdktypes.EndpointTypeQueue),
						string(sdktypes.EndpointTypeLoadBalancer),
					),
				},
			},
			"gpu_pools": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "Serverless GPU pool IDs (see the gpu catalog's pool field), e.g. [\"ADA_24\"]. Workers run on whichever listed pool has capacity. Exactly one of gpu_pools or cpu must be set; the API cannot move an endpoint between the GPU and CPU families, so switching replaces the endpoint.",
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
				PlanModifiers: []planmodifier.List{
					requiresReplaceOnListPresenceChange(computeFamilyReplaceDescription),
				},
			},
			"gpu_count": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "GPUs per worker.",
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
					int64validator.ConflictsWith(path.MatchRoot("cpu")),
				},
			},
			"excluded_types": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Description: "GPU type IDs to subtract from the selected pools, e.g. [\"NVIDIA L40\"]. Workers run on every type in gpu_pools except these. The pool stays the unit of selection, so a card later added to the pool becomes eligible. GPU endpoints only.",
				Validators: []validator.List{
					listvalidator.ConflictsWith(path.MatchRoot("cpu")),
				},
			},
			"allowed_cuda_versions": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Description: "Acceptable CUDA versions for worker placement, as major.minor, e.g. [\"12.8\", \"12.6\"]. Matching is exact. A non-empty list cannot be combined with min_cuda_version; set it to [] to state no constraint. Takes effect as workers are replaced. GPU endpoints only.",
				Validators: []validator.List{
					listvalidator.ConflictsWith(path.MatchRoot("cpu")),
				},
			},
			"min_cuda_version": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Lowest acceptable CUDA version for worker placement, as major.minor, compared numerically: 12.11 is above 12.2. Use it for an open-ended floor and allowed_cuda_versions for an exact set; set it to \"\" to drop the floor. GPU endpoints only.",
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("cpu")),
				},
			},
			"cpu": schema.ListNestedAttribute{
				Optional:    true,
				Description: "Eligible CPU configurations for each worker, for a CPU-only endpoint. Workers are placed on whichever listed configuration has capacity, and memory follows from the flavor's catalog RAM multiplier. Exactly one of gpu_pools or cpu must be set; the API cannot move an endpoint between the GPU and CPU families, so switching replaces the endpoint.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"cpu_type_id": schema.StringAttribute{
							Required:    true,
							Description: "CPU flavor identifier from the catalog, e.g. \"cpu5c\".",
						},
						"vcpu_count": schema.Int64Attribute{
							Required:    true,
							Description: "Number of vCPUs per worker. Must be valid for the flavor and a power of two.",
							Validators: []validator.Int64{
								int64validator.AtLeast(1),
							},
						},
					},
				},
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
				PlanModifiers: []planmodifier.List{
					requiresReplaceOnListPresenceChange(computeFamilyReplaceDescription),
				},
			},
			"flashboot": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "FlashBoot cold-start acceleration: OFF, FLASHBOOT, or PRIORITY_FLASHBOOT (enabled with priority capacity).",
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(sdktypes.FlashBootOff),
						string(sdktypes.FlashBootEnabled),
						string(sdktypes.FlashBootPriority),
					),
				},
			},
			"scaling_type": schema.StringAttribute{
				Required:    true,
				Description: "Autoscaling signal: QUEUE_DELAY (queue endpoints only) or REQUEST_COUNT.",
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(sdktypes.QueueDelayScalingTypeQueueDelay),
						string(sdktypes.RequestCountScalingTypeRequestCount),
					),
				},
			},
			"scaling_value": schema.Float64Attribute{
				Required:    true,
				Description: "Threshold for the scaler: seconds of queue delay (0.5 or more, fractions allowed) for QUEUE_DELAY, or a whole number of in-flight requests (1 or more) for REQUEST_COUNT.",
				Validators: []validator.Float64{
					float64validator.AtLeast(0.5),
				},
			},
			"workers_min": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Minimum number of workers.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"workers_max": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Maximum number of workers.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"idle_timeout": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Seconds before idle workers scale down, between 1 and 3600. Not applicable to REQUEST_COUNT-scaled queue endpoints.",
				Validators: []validator.Int64{
					int64validator.Between(1, 3600),
				},
			},
			"disk": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Worker container disk in GB.",
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"env": schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Description: "Environment variables for workers.",
			},
			"timeout": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Per-request execution timeout in milliseconds.",
			},
			"registry": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Registry credential ID (runpod_registry) for pulling private worker images. A template can supply one. Removing it from the configuration leaves the credential attached: the provider cannot clear it yet. With template_id set and registry left unset, every update plans registry as (known after apply), because the API re-applies the template, and the template's current credential, on each update.",
				PlanModifiers: []planmodifier.String{useEndpointRegistryStateUnlessTemplateSent()},
			},
			"data_center_ids": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Description: "Preferred data centers for workers.",
			},
			"network_volumes": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Description: "Network volume IDs attached to workers.",
			},
			"global_volumes": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Description: "Global volume IDs (runpod_global_volume) mounted on every worker at /runpod-volume. At most one, " +
					"on GPU endpoints only, and not together with network volumes: to switch, set the kind being left to [] " +
					"in the same apply. [] detaches the global volume; the change applies to workers created afterwards.",
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
			},
			"ports": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Description: "Worker HTTP ports a LOAD_BALANCER endpoint routes to, formatted as port/protocol, e.g. [\"8000/http\"]. QUEUE endpoints take requests through the managed queue and do not use it.",
			},
			"request_urls": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "Invocation URLs the API reports for this endpoint. A QUEUE endpoint fills the job operations (run, run_sync, status, stream, cancel, retry, purge_queue, health); a LOAD_BALANCER endpoint fills base and health, since its remaining paths are worker-defined. The attributes that do not apply to the endpoint's routing type are null.",
				Attributes: map[string]schema.Attribute{
					"base": schema.StringAttribute{
						Computed:    true,
						Description: "Base URL for the worker's own HTTP and WebSocket paths. LOAD_BALANCER endpoints only.",
					},
					"cancel": schema.StringAttribute{
						Computed:    true,
						Description: "Cancel an in-progress job or drop a queued one. Takes a job id in place of the {job_id} segment.",
					},
					"health": schema.StringAttribute{
						Computed:    true,
						Description: "Operational status of the endpoint, or the path the load balancer polls for worker health.",
					},
					"purge_queue": schema.StringAttribute{
						Computed:    true,
						Description: "Drop all pending jobs from the queue, leaving in-progress jobs running.",
					},
					"retry": schema.StringAttribute{
						Computed:    true,
						Description: "Requeue a failed or timed-out job. Takes a job id in place of the {job_id} segment.",
					},
					"run": schema.StringAttribute{
						Computed:    true,
						Description: "Submit a job asynchronously.",
					},
					"run_sync": schema.StringAttribute{
						Computed:    true,
						Description: "Submit a job and wait for its result.",
					},
					"status": schema.StringAttribute{
						Computed:    true,
						Description: "State, timings, and result of a submitted job. Takes a job id in place of the {job_id} segment.",
					},
					"stream": schema.StringAttribute{
						Computed:    true,
						Description: "Read incremental results from a job that produces output progressively. Takes a job id in place of the {job_id} segment.",
					},
				},
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "Creation timestamp in RFC 3339 format.",
			},
			"compute_type": schema.StringAttribute{
				Optional:           true,
				Computed:           true,
				Description:        "Deprecated. Worker compute family, GPU or CPU, preserving the 1.x compute_type attribute. It restates the choice gpu_pools and cpu already make, so it is derived from the endpoint the v2 API reports and a value contradicting that choice is rejected; nothing is sent for it beyond the compute block. Removed once 1.x configurations have moved to gpu_pools and cpu.",
				DeprecationMessage: endpointComputeTypeDeprecation,
				Validators: []validator.String{
					stringvalidator.OneOf("GPU", "CPU"),
				},
			},
			"gpu_type_priority": schema.StringAttribute{
				Optional: true,
				Description: "Deprecated. How the scheduler orders the GPU types inside the selected pools, e.g. \"availability\", preserving the 1.x gpu_type_priority attribute. GPU endpoints only. " +
					legacyIgnoredInputDescriptionSuffix,
				DeprecationMessage: legacyIgnoredInputDeprecation,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("cpu")),
				},
			},
			"cpu_flavor_priority": schema.StringAttribute{
				Optional: true,
				Description: "Deprecated. How the scheduler orders the configured CPU flavors, e.g. \"availability\", preserving the 1.x cpu_flavor_priority attribute. CPU endpoints only. " +
					legacyIgnoredInputDescriptionSuffix,
				DeprecationMessage: legacyIgnoredInputDeprecation,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("gpu_pools")),
				},
			},
			"template_version": schema.Int64Attribute{
				Computed: true,
				Description: "Deprecated. Version of the template the endpoint was built from, preserving the 1.x template_version attribute. " +
					legacyNullDescriptionSuffix,
				DeprecationMessage: legacyNullDeprecation,
				// A state written while the attribute still had a source can
				// hold a number; an update must not promise it when the apply
				// nulls it.
				PlanModifiers: []planmodifier.Int64{legacyShimUnknownInt64()},
			},
			"users": schema.ListAttribute{
				ElementType: types.StringType,
				Computed:    true,
				Description: "Deprecated. Account that owns the endpoint, as a one-element list, preserving the 1.x users attribute and its shape. " +
					legacyShimDescriptionSuffix,
				DeprecationMessage: legacyShimDeprecation,
				PlanModifiers:      []planmodifier.List{legacyShimUnknownList(types.StringType)},
			},
			"version": schema.Int64Attribute{
				Computed: true,
				Description: "Deprecated. The endpoint's configuration version, which the platform increments on every change, preserving the 1.x version attribute. " +
					"Read from the REST v2 workers listing; the runpod_endpoint_workers data source reports the same value as endpoint_version.",
				DeprecationMessage: endpointWorkersDeprecation,
				PlanModifiers:      []planmodifier.Int64{legacyShimUnknownInt64()},
			},
			"workers": schema.ListNestedAttribute{
				Computed: true,
				Description: "Deprecated. Workers active on the endpoint at the moment it was read, preserving the 1.x workers attribute. Allocation changes with load, so this is a snapshot. " +
					"Read from the REST v2 workers listing; the runpod_endpoint_workers data source is the supported view of the same workers, with more detail.",
				DeprecationMessage: endpointWorkersDeprecation,
				PlanModifiers:      []planmodifier.List{legacyShimUnknownList(endpointLegacyWorkerObjectType)},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:    true,
							Description: "Worker identifier.",
						},
						"pod_id": schema.StringAttribute{
							Computed:    true,
							Description: "Pod backing the worker. REST v2 lists an endpoint's pods as its workers, so this is the worker's id.",
						},
						"status": schema.StringAttribute{
							Computed:    true,
							Description: "Worker status: IDLE, INITIALIZING, RUNNING, THROTTLED, or UNHEALTHY.",
						},
						"uptime_ms": schema.Int64Attribute{
							Computed:    true,
							Description: "Milliseconds since the worker started, at whole-second precision: REST v2 reports uptime in seconds. Null until the worker is placed and running.",
						},
						"start_time": schema.StringAttribute{
							Computed:    true,
							Description: "When the worker last started, in RFC 3339 format; null if it has not started.",
						},
						"last_busy_ms": schema.Int64Attribute{
							Computed:    true,
							Description: "Milliseconds since the worker last handled a request. Always null: REST v2 does not report it.",
						},
					},
				},
			},
		},
	}
}

func (r *serverlessEndpointResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, &resp.Diagnostics)
	r.legacy = configuredLegacyClient(req.ProviderData, &resp.Diagnostics)
}

// ValidateConfig warns about a configured priority, which nothing can apply,
// and rejects a compute_type that contradicts the compute block the
// configuration carries. The two state the same fact, and the API takes the
// block, so a disagreement is a configuration error rather than a silent
// preference.
func (r *serverlessEndpointResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config serverlessEndpointResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	warnUnappliedLegacyEndpointInputs(&resp.Diagnostics, config)
	if config.ComputeType.IsNull() || config.ComputeType.IsUnknown() {
		return
	}

	computeType := config.ComputeType.ValueString()
	if !config.GpuPools.IsNull() && !config.GpuPools.IsUnknown() && computeType != "GPU" {
		resp.Diagnostics.AddAttributeError(
			path.Root("compute_type"),
			"compute_type contradicts the endpoint's compute family",
			fmt.Sprintf("gpu_pools is set, which makes this a GPU endpoint, but compute_type is %q. Set compute_type to \"GPU\" or drop it; it is derived from the compute block.", computeType),
		)
	}
	if !config.Cpu.IsNull() && !config.Cpu.IsUnknown() && computeType != "CPU" {
		resp.Diagnostics.AddAttributeError(
			path.Root("compute_type"),
			"compute_type contradicts the endpoint's compute family",
			fmt.Sprintf("cpu is set, which makes this a CPU endpoint, but compute_type is %q. Set compute_type to \"CPU\" or drop it; it is derived from the compute block.", computeType),
		)
	}
}

// readLegacyEndpointFields fills the deprecated read-backs after the v2 call
// has settled the rest of the model. None of them is an input, so a read that
// fails warns and nulls what it feeds rather than failing the operation, and
// an endpoint the GraphQL API does not know leaves users null silently. The
// two priorities are not touched: nothing reads them back, so they keep the
// value the configuration set.
func (r *serverlessEndpointResource) readLegacyEndpointFields(ctx context.Context, model *serverlessEndpointResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	model.TemplateVersion = types.Int64Null()
	diags.Append(r.readLegacyEndpointWorkers(ctx, model)...)
	diags.Append(r.readLegacyEndpointOwner(ctx, model)...)
	return diags
}

// readLegacyEndpointWorkers fills workers and version from
// GET /v2/serverless/{id}/workers.
func (r *serverlessEndpointResource) readLegacyEndpointWorkers(ctx context.Context, model *serverlessEndpointResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	apiResp, err := r.client.Serverless.ListEndpointWorkersWithResponse(ctx, model.Id.ValueString())
	if err != nil {
		clearLegacyEndpointWorkers(model)
		appendLegacyWorkersReadFailure(&diags, err)
		return diags
	}
	if apiResp.JSON200 == nil {
		clearLegacyEndpointWorkers(model)
		appendLegacyWorkersReadFailure(&diags, fmt.Errorf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)))
		return diags
	}
	diags.Append(flattenLegacyEndpointWorkers(ctx, apiResp.JSON200, model)...)
	return diags
}

// readLegacyEndpointOwner fills users from the GraphQL shim.
func (r *serverlessEndpointResource) readLegacyEndpointOwner(ctx context.Context, model *serverlessEndpointResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if r.legacy == nil {
		model.Users = types.ListNull(types.StringType)
		return diags
	}
	owner, err := fetchLegacyEndpointOwner(ctx, r.legacy, model.Id.ValueString())
	if err != nil {
		model.Users = types.ListNull(types.StringType)
		appendLegacyReadFailure(&diags, "endpoint", err)
		return diags
	}
	diags.Append(flattenLegacyEndpointOwner(ctx, owner, model)...)
	return diags
}

// ConfigValidators carry the cross-attribute rules the API enforces: workers
// are either GPU or CPU, and their container settings come from an image or a
// template (or both).
func (r *serverlessEndpointResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("gpu_pools"),
			path.MatchRoot("cpu"),
		),
		resourcevalidator.AtLeastOneOf(
			path.MatchRoot("image"),
			path.MatchRoot("template_id"),
		),
	}
}

func (r *serverlessEndpointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverlessEndpointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, diags := expandCreateEndpoint(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.Serverless.CreateEndpointWithResponse(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating serverless endpoint failed", err.Error())
		return
	}
	if apiResp.JSON201 == nil {
		resp.Diagnostics.AddError(
			"Creating serverless endpoint failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	resp.Diagnostics.Append(flattenEndpoint(ctx, apiResp.JSON201, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The endpoint exists from here on, so its state is written whatever the
	// deprecated reads report: returning first would leave it running in the
	// account with nothing in state.
	legacyDiags := r.readLegacyEndpointFields(ctx, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	resp.Diagnostics.Append(legacyDiags...)
}

func (r *serverlessEndpointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverlessEndpointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.Serverless.GetEndpointWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading serverless endpoint failed", err.Error())
		return
	}
	if apiResp.HTTPResponse.StatusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Reading serverless endpoint failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	resp.Diagnostics.Append(flattenEndpoint(ctx, apiResp.JSON200, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// As on a pod, an apply leaves a configured env as planned because the API
	// merges the template's in, so a refresh is where a changed value on a
	// configured key surfaces.
	var apiEnv map[string]string
	if apiResp.JSON200.Env != nil {
		apiEnv = *apiResp.JSON200.Env
	}
	resp.Diagnostics.Append(resyncConfiguredEnv(ctx, apiEnv, &state.Env)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.readLegacyEndpointFields(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serverlessEndpointResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, config serverlessEndpointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := serverless.UpdateEndpointJSONRequestBody{
		Name: plan.Name.ValueStringPointer(),
	}
	if !plan.Image.IsNull() && !plan.Image.IsUnknown() {
		body.Image = plan.Image.ValueStringPointer()
	}
	if !plan.TemplateId.IsNull() && !plan.TemplateId.IsUnknown() {
		body.TemplateId = plan.TemplateId.ValueStringPointer()
	}

	// The compute family is fixed at create (the schema replaces the endpoint
	// when the block that carries it appears or disappears), so exactly one of
	// the two branches applies to an existing endpoint.
	switch {
	case !plan.Cpu.IsNull() && !plan.Cpu.IsUnknown():
		cpu, cpuDiags := expandEndpointCpu(ctx, plan.Cpu)
		resp.Diagnostics.Append(cpuDiags...)
		body.Cpu = &cpu
	case !plan.GpuPools.IsNull() && !plan.GpuPools.IsUnknown():
		var pools []string
		resp.Diagnostics.Append(plan.GpuPools.ElementsAs(ctx, &pools, false)...)
		gpu := &sdktypes.UpdateEndpointGpuConfig{Pools: &pools}
		if !plan.GpuCount.IsNull() && !plan.GpuCount.IsUnknown() {
			count := int(plan.GpuCount.ValueInt64())
			gpu.Count = &count
		}
		// pools and excluded_types are one selection: sending pools without
		// excludedTypes clears the exclusions, so they are always restated.
		if !plan.ExcludedTypes.IsNull() && !plan.ExcludedTypes.IsUnknown() {
			var excluded []string
			resp.Diagnostics.Append(plan.ExcludedTypes.ElementsAs(ctx, &excluded, false)...)
			gpu.ExcludedTypes = &excluded
		}
		if !plan.AllowedCudaVersions.IsNull() && !plan.AllowedCudaVersions.IsUnknown() {
			var versions []string
			resp.Diagnostics.Append(plan.AllowedCudaVersions.ElementsAs(ctx, &versions, false)...)
			gpu.AllowedCudaVersions = &versions
		}
		if !plan.MinCudaVersion.IsNull() && !plan.MinCudaVersion.IsUnknown() {
			gpu.MinCudaVersion = plan.MinCudaVersion.ValueStringPointer()
		}
		body.Gpu = gpu
	}

	if !plan.Flashboot.IsNull() && !plan.Flashboot.IsUnknown() {
		flashboot := sdktypes.FlashBoot(plan.Flashboot.ValueString())
		body.Flashboot = &flashboot
	}

	scaling, scalingDiags := expandEndpointScaling(plan.ScalingType.ValueString(), plan.ScalingValue.ValueFloat64())
	resp.Diagnostics.Append(scalingDiags...)
	body.Scaling = &scaling

	workers := sdktypes.EndpointWorkers{}
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
	// From the configuration: the plan can carry the credential in state
	// forward, and sending that back would make it an explicit field, which
	// wins over the credential a template supplies.
	if !config.Registry.IsNull() && !config.Registry.IsUnknown() {
		body.Registry = config.Registry.ValueStringPointer()
	}
	if !plan.Env.IsNull() && !plan.Env.IsUnknown() {
		env := map[string]string{}
		resp.Diagnostics.Append(plan.Env.ElementsAs(ctx, &env, false)...)
		body.Env = &env
	}
	if !plan.DataCenterIds.IsNull() && !plan.DataCenterIds.IsUnknown() {
		var ids []string
		resp.Diagnostics.Append(plan.DataCenterIds.ElementsAs(ctx, &ids, false)...)
		body.DataCenterIds = &ids
	}
	if !plan.NetworkVolumes.IsNull() && !plan.NetworkVolumes.IsUnknown() {
		var volumes []string
		resp.Diagnostics.Append(plan.NetworkVolumes.ElementsAs(ctx, &volumes, false)...)
		body.NetworkVolumes = &volumes
	}
	mounts, mountDiags := expandEndpointGlobalMounts(ctx, plan.GlobalVolumes)
	resp.Diagnostics.Append(mountDiags...)
	body.Mounts = mounts
	if !plan.Ports.IsNull() && !plan.Ports.IsUnknown() {
		var ports []string
		resp.Diagnostics.Append(plan.Ports.ElementsAs(ctx, &ports, false)...)
		body.Ports = &ports
	}
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.Serverless.UpdateEndpointWithResponse(ctx, state.Id.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Updating serverless endpoint failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Updating serverless endpoint failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	resp.Diagnostics.Append(flattenEndpoint(ctx, apiResp.JSON200, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// As on Create: the v2 update has already landed, so the state is written
	// whatever the deprecated reads report. Returning first would leave state
	// describing the endpoint as it was before this apply.
	legacyDiags := r.readLegacyEndpointFields(ctx, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	resp.Diagnostics.Append(legacyDiags...)
}

func (r *serverlessEndpointResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serverlessEndpointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.Serverless.DeleteEndpointWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Deleting serverless endpoint failed", err.Error())
		return
	}
	switch apiResp.HTTPResponse.StatusCode {
	case http.StatusNoContent, http.StatusNotFound:
		// Deleted, or already gone.
	default:
		resp.Diagnostics.AddError(
			"Deleting serverless endpoint failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
	}
}

func (r *serverlessEndpointResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

const endpointRegistryPlanDescription = "Keeps the prior value on an update that sends no template_id; an update that sends one can pick up the template's current credential."

// useEndpointRegistryStateUnlessTemplateSent is UseStateForUnknown for the
// endpoint's registry, limited to updates that send no template_id. Update
// sends template_id whenever the configuration sets it, and the API re-applies
// the template each time, seeding the registry from the credential the
// template holds now. The same template id can therefore come back with a
// different credential once the template has changed, and promising the old
// one would fail the apply with an inconsistent result.
func useEndpointRegistryStateUnlessTemplateSent() planmodifier.String {
	return endpointRegistryPlanModifier{}
}

type endpointRegistryPlanModifier struct{}

func (endpointRegistryPlanModifier) Description(context.Context) string {
	return endpointRegistryPlanDescription
}

func (endpointRegistryPlanModifier) MarkdownDescription(context.Context) string {
	return endpointRegistryPlanDescription
}

func (endpointRegistryPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// As UseStateForUnknown: nothing to keep on a create, nothing to do for a
	// known plan, and an unknown configuration value stays unknown.
	if req.State.Raw.IsNull() || !req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}

	var configTemplate types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("template_id"), &configTemplate)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Update sends template_id only when the configuration sets it.
	if configTemplate.IsNull() {
		resp.PlanValue = req.StateValue
	}
}
