package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
)

// runpod_machine, ported from the 1.x provider's resource_machine. The create
// document is the 1.x document unchanged, and the read document adds only
// markedForDeletion. The rename and delete documents follow the backend schema
// instead, because the 1.x ones never matched it: MachineEditNameInput takes
// machineId rather than id, and machineDelete takes a MachineDeleteInput and
// returns Void, so it has no selection set.
//
// The mutation accepts nothing but a name, so name and bid_price_per_gpu are
// the only settable attributes and everything else is Computed-only. Leaving
// the rest Optional, as 1.x had them, makes any configured value a permanent
// diff: the read writes what the API reports, the plan writes the configuration
// back, and no mutation can reconcile the two. The three client-side defaults
// 1.x carried (gpu_count 1, listed true, secure_cloud false) are gone for the
// same reason. A 1.x configuration that set one of these attributes now fails
// validation instead of drifting forever.

var (
	_ resource.Resource                = (*machineResource)(nil)
	_ resource.ResourceWithConfigure   = (*machineResource)(nil)
	_ resource.ResourceWithImportState = (*machineResource)(nil)
)

const createMachineMutation = `
	mutation CreateMachine($input: MachineAddInput!) {
		machineAdd(input: $input) {
			id
			name
			gpuType {
				id
				displayName
			}
			cpuCount
			gpuTotal
			memoryTotal
			diskTotal
			listed
			secureCloud
			location
		}
	}
`

const getMachineQuery = `
	query GetMachine($machineId: String!) {
		machine(input: { machineId: $machineId }) {
			id
			name
			gpuType {
				id
				displayName
			}
			cpuCount
			gpuTotal
			memoryTotal
			diskTotal
			listed
			secureCloud
			location
			maintenanceMode
			verified
			hostPricePerGpu
			dataCenterId
			runpodIp
			markedForDeletion
		}
	}
`

const editMachineNameMutation = `
	mutation EditMachineName($input: MachineEditNameInput!) {
		machineEditName(input: $input) {
			id
			name
		}
	}
`

const deleteMachineMutation = `
	mutation DeleteMachine($input: MachineDeleteInput!) {
		machineDelete(input: $input)
	}
`

func NewMachineResource() resource.Resource { return &machineResource{} }

type machineResource struct {
	client *legacy.Client
}

type machineResourceModel struct {
	Id              types.String  `tfsdk:"id"`
	Name            types.String  `tfsdk:"name"`
	GpuTypeId       types.String  `tfsdk:"gpu_type_id"`
	GpuCount        types.Int64   `tfsdk:"gpu_count"`
	CpuCount        types.Int64   `tfsdk:"cpu_count"`
	MemoryInGb      types.Int64   `tfsdk:"memory_in_gb"`
	DiskInGb        types.Int64   `tfsdk:"disk_in_gb"`
	Listed          types.Bool    `tfsdk:"listed"`
	SecureCloud     types.Bool    `tfsdk:"secure_cloud"`
	Location        types.String  `tfsdk:"location"`
	DataCenterId    types.String  `tfsdk:"data_center_id"`
	BidPricePerGpu  types.Float64 `tfsdk:"bid_price_per_gpu"`
	HostPricePerGpu types.Float64 `tfsdk:"host_price_per_gpu"`
	MaintenanceMode types.Bool    `tfsdk:"maintenance_mode"`
	Verified        types.Bool    `tfsdk:"verified"`
	RunpodIp        types.String  `tfsdk:"runpod_ip"`
}

func (r *machineResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_machine"
}

func (r *machineResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A host machine listed on Runpod. Deprecated: " + machineDeprecation +
			" Only name and bid_price_per_gpu are settable, because the create mutation takes no " +
			"other input; every other attribute reports what the API assigned.",
		DeprecationMessage: machineDeprecation,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Machine identifier assigned by the API.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Machine name. The only attribute the API accepts on create, and the only one an update changes.",
			},
			"gpu_type_id": schema.StringAttribute{
				Computed:    true,
				Description: "GPU type identifier of the GPUs installed in the machine.",
			},
			"gpu_count": schema.Int64Attribute{
				Computed:    true,
				Description: "Number of GPUs installed in the machine (gpuTotal).",
			},
			"cpu_count": schema.Int64Attribute{
				Computed:    true,
				Description: "Number of CPU cores.",
			},
			"memory_in_gb": schema.Int64Attribute{
				Computed:    true,
				Description: "Installed memory in GB (memoryTotal).",
			},
			"disk_in_gb": schema.Int64Attribute{
				Computed:    true,
				Description: "Installed disk in GB (diskTotal).",
			},
			"listed": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether the machine is listed for others to rent.",
			},
			"secure_cloud": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether the machine runs in secure cloud.",
			},
			"location": schema.StringAttribute{
				Computed:    true,
				Description: "Location or country code the machine sits in.",
			},
			"data_center_id": schema.StringAttribute{
				Computed:    true,
				Description: "Data center the machine belongs to.",
			},
			"bid_price_per_gpu": schema.Float64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Bid price per GPU in USD/hr. Write-only in 1.x and absent from every GraphQL document, so it reports back what was configured.",
			},
			"host_price_per_gpu": schema.Float64Attribute{
				Computed:    true,
				Description: "Host price per GPU in USD/hr.",
			},
			"maintenance_mode": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether the machine is in maintenance mode.",
			},
			"verified": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether the machine is verified.",
			},
			"runpod_ip": schema.StringAttribute{
				Computed:    true,
				Description: "Machine IP address as Runpod sees it.",
			},
		},
	}
}

func (r *machineResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredLegacyClient(req.ProviderData, &resp.Diagnostics)
}

func (r *machineResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan machineResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	variables := map[string]any{
		"input": map[string]any{
			"name": plan.Name.ValueString(),
		},
	}

	data, err := r.client.Query(ctx, createMachineMutation, variables)
	if err != nil {
		resp.Diagnostics.AddError("Creating machine failed", err.Error())
		return
	}
	machine, ok := data["machineAdd"].(map[string]any)
	if !ok {
		resp.Diagnostics.AddError("Creating machine failed", "the machineAdd mutation returned no machine")
		return
	}

	flattenMachineResource(machine, &plan)
	if plan.Id.IsNull() || plan.Id.ValueString() == "" {
		resp.Diagnostics.AddError("Creating machine failed", "the machineAdd mutation returned no machine id")
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *machineResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state machineResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	variables := map[string]any{"machineId": state.Id.ValueString()}

	data, err := r.client.Query(ctx, getMachineQuery, variables)
	if err != nil {
		resp.Diagnostics.AddError("Reading machine failed", err.Error())
		return
	}
	machine, ok := data["machine"].(map[string]any)
	// A machine the key cannot see, or an id that matches none, comes back as
	// an object whose fields are all null rather than as an error. A deleted
	// machine still comes back whole: machineDelete only sets
	// markedForDeletion, and the machine query does not filter on it.
	if !ok || !machineHasID(machine) || machine["markedForDeletion"] == true {
		resp.State.RemoveResource(ctx)
		return
	}

	flattenMachineResource(machine, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *machineResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan machineResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	variables := map[string]any{
		"input": map[string]any{
			"machineId": plan.Id.ValueString(),
			"name":      plan.Name.ValueString(),
		},
	}

	data, err := r.client.Query(ctx, editMachineNameMutation, variables)
	if err != nil {
		resp.Diagnostics.AddError("Updating machine failed", err.Error())
		return
	}
	machine, ok := data["machineEditName"].(map[string]any)
	if !ok || !machineHasID(machine) {
		resp.Diagnostics.AddError("Updating machine failed", "the machineEditName mutation returned no machine")
		return
	}

	// The mutation returns id and name only; the rest of the plan carries over.
	flattenMachineResource(machine, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *machineResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state machineResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	variables := map[string]any{
		"input": map[string]any{"machineId": state.Id.ValueString()},
	}

	if _, err := r.client.Query(ctx, deleteMachineMutation, variables); err != nil {
		resp.Diagnostics.AddError("Deleting machine failed", err.Error())
	}
}

func (r *machineResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// machineHasID reports whether a GraphQL machine object names a machine. The
// machine query answers an id it cannot match, or a machine the caller does
// not own, with an empty object, so a non-null field alone proves nothing.
func machineHasID(machine map[string]any) bool {
	id, ok := machine["id"].(string)
	return ok && id != ""
}

// flattenMachineResource copies a GraphQL machine object onto the model. The
// create and update documents select fewer fields than the read query, so a
// field the response omits keeps its planned value (see jsonStringOr).
func flattenMachineResource(machine map[string]any, model *machineResourceModel) {
	model.Id = jsonStringOr(machine, "id", model.Id)
	model.Name = jsonStringOr(machine, "name", model.Name)
	model.GpuTypeId = jsonNestedStringOr(machine, "gpuType", "id", model.GpuTypeId)
	model.GpuCount = jsonInt64Or(machine, "gpuTotal", model.GpuCount)
	model.CpuCount = jsonInt64Or(machine, "cpuCount", model.CpuCount)
	model.MemoryInGb = jsonInt64Or(machine, "memoryTotal", model.MemoryInGb)
	model.DiskInGb = jsonInt64Or(machine, "diskTotal", model.DiskInGb)
	model.Listed = jsonBoolOr(machine, "listed", model.Listed)
	model.SecureCloud = jsonBoolOr(machine, "secureCloud", model.SecureCloud)
	model.Location = jsonStringOr(machine, "location", model.Location)
	model.DataCenterId = jsonStringOr(machine, "dataCenterId", model.DataCenterId)
	model.BidPricePerGpu = jsonFloat64Or(machine, "bidPricePerGpu", model.BidPricePerGpu)
	model.HostPricePerGpu = jsonFloat64Or(machine, "hostPricePerGpu", model.HostPricePerGpu)
	model.MaintenanceMode = jsonBoolOr(machine, "maintenanceMode", model.MaintenanceMode)
	model.Verified = jsonBoolOr(machine, "verified", model.Verified)
	model.RunpodIp = jsonStringOr(machine, "runpodIp", model.RunpodIp)
}
