package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
)

// runpod_machine and runpod_machines, ported from the 1.x provider's
// datasource_machine and datasource_machines. The singular document is the 1.x
// document unchanged, which is why that data source keeps its data_center_id
// attribute even though the query does not select it: the attribute exists for
// 1.x configuration compatibility and always reads null. The list document
// reads myself.machines rather than the 1.x root machines field: the root
// field lists the public supply and only answers support and host-daemon
// callers, while myself.machines is the account's own machines.
// A field the query omits, or returns null for, reads null here rather than
// failing the read as it did in 1.x.

var (
	_ datasource.DataSource              = (*machineDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*machineDataSource)(nil)
	_ datasource.DataSource              = (*machinesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*machinesDataSource)(nil)
)

const getMachineDetailQuery = `
	query GetMachine($machineId: String!) {
		machine(input: { machineId: $machineId }) {
			id
			name
			location
			listed
			gpuType {
				id
				displayName
			}
			gpuTotal
			gpuReserved
			cpuCount
			cpuTypeId
			memoryTotal
			memoryReserved
			diskTotal
			diskReserved
			secureCloud
			maintenanceMode
			verified
			hostPricePerGpu
			runpodIp
		}
	}
`

const getMachinesQuery = `
	query GetMachines {
		myself {
			machines {
				id
				name
				location
				listed
				gpuType {
					id
					displayName
				}
				gpuTotal
				secureCloud
				dataCenterId
			}
		}
	}
`

func NewMachineDataSource() datasource.DataSource { return &machineDataSource{} }

type machineDataSource struct {
	client *legacy.Client
}

type machineDataSourceModel struct {
	Id              types.String  `tfsdk:"id"`
	Name            types.String  `tfsdk:"name"`
	Location        types.String  `tfsdk:"location"`
	Listed          types.Bool    `tfsdk:"listed"`
	GpuTypeId       types.String  `tfsdk:"gpu_type_id"`
	GpuTotal        types.Int64   `tfsdk:"gpu_total"`
	GpuReserved     types.Int64   `tfsdk:"gpu_reserved"`
	CpuCount        types.Int64   `tfsdk:"cpu_count"`
	CpuTypeId       types.String  `tfsdk:"cpu_type_id"`
	MemoryTotal     types.Int64   `tfsdk:"memory_total"`
	MemoryReserved  types.Int64   `tfsdk:"memory_reserved"`
	DiskTotal       types.Int64   `tfsdk:"disk_total"`
	DiskReserved    types.Int64   `tfsdk:"disk_reserved"`
	SecureCloud     types.Bool    `tfsdk:"secure_cloud"`
	MaintenanceMode types.Bool    `tfsdk:"maintenance_mode"`
	Verified        types.Bool    `tfsdk:"verified"`
	HostPricePerGpu types.Float64 `tfsdk:"host_price_per_gpu"`
	DataCenterId    types.String  `tfsdk:"data_center_id"`
	RunpodIp        types.String  `tfsdk:"runpod_ip"`
}

func (d *machineDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_machine"
}

func (d *machineDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:        "A single host machine looked up by identifier, with its reserved-versus-total capacity. Deprecated: " + machineDeprecation,
		DeprecationMessage: machineDeprecation,
		Attributes: map[string]schema.Attribute{
			"id":                 schema.StringAttribute{Required: true, Description: "Machine identifier to look up."},
			"name":               schema.StringAttribute{Computed: true, Description: "Machine name."},
			"location":           schema.StringAttribute{Computed: true, Description: "Location or country code the machine sits in."},
			"listed":             schema.BoolAttribute{Computed: true, Description: "Whether the machine is listed for others to rent."},
			"gpu_type_id":        schema.StringAttribute{Computed: true, Description: "GPU type identifier of the GPUs installed in the machine."},
			"gpu_total":          schema.Int64Attribute{Computed: true, Description: "Number of GPUs installed."},
			"gpu_reserved":       schema.Int64Attribute{Computed: true, Description: "Number of GPUs currently reserved."},
			"cpu_count":          schema.Int64Attribute{Computed: true, Description: "Number of CPU cores."},
			"cpu_type_id":        schema.StringAttribute{Computed: true, Description: "CPU flavor identifier."},
			"memory_total":       schema.Int64Attribute{Computed: true, Description: "Installed memory in GB."},
			"memory_reserved":    schema.Int64Attribute{Computed: true, Description: "Memory currently reserved in GB."},
			"disk_total":         schema.Int64Attribute{Computed: true, Description: "Installed disk in GB."},
			"disk_reserved":      schema.Int64Attribute{Computed: true, Description: "Disk currently reserved in GB."},
			"secure_cloud":       schema.BoolAttribute{Computed: true, Description: "Whether the machine runs in secure cloud."},
			"maintenance_mode":   schema.BoolAttribute{Computed: true, Description: "Whether the machine is in maintenance mode."},
			"verified":           schema.BoolAttribute{Computed: true, Description: "Whether the machine is verified."},
			"host_price_per_gpu": schema.Float64Attribute{Computed: true, Description: "Host price per GPU in USD/hr."},
			"data_center_id":     schema.StringAttribute{Computed: true, Description: "Always null: the 1.x GraphQL query this data source ports does not select it. Use runpod_machines for the data center."},
			"runpod_ip":          schema.StringAttribute{Computed: true, Description: "Machine IP address as Runpod sees it."},
		},
	}
}

func (d *machineDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredLegacyClient(req.ProviderData, &resp.Diagnostics)
}

func (d *machineDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config machineDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	variables := map[string]any{"machineId": config.Id.ValueString()}

	data, err := d.client.Query(ctx, getMachineDetailQuery, variables)
	if err != nil {
		resp.Diagnostics.AddError("Reading machine failed", err.Error())
		return
	}
	machine, ok := data["machine"].(map[string]any)
	if !ok || !machineHasID(machine) {
		resp.Diagnostics.AddError("Reading machine failed", "no machine with id "+config.Id.ValueString())
		return
	}

	state := flattenMachineDataSource(machine, config.Id)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func NewMachinesDataSource() datasource.DataSource { return &machinesDataSource{} }

type machinesDataSource struct {
	client *legacy.Client
}

type machineListItemModel struct {
	Id           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Location     types.String `tfsdk:"location"`
	Listed       types.Bool   `tfsdk:"listed"`
	GpuTypeId    types.String `tfsdk:"gpu_type_id"`
	GpuTotal     types.Int64  `tfsdk:"gpu_total"`
	SecureCloud  types.Bool   `tfsdk:"secure_cloud"`
	DataCenterId types.String `tfsdk:"data_center_id"`
}

type machinesDataSourceModel struct {
	Machines []machineListItemModel `tfsdk:"machines"`
}

func (d *machinesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_machines"
}

func (d *machinesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:        "All host machines owned by the authenticated account. Deprecated: " + machineDeprecation,
		DeprecationMessage: machineDeprecation,
		Attributes: map[string]schema.Attribute{
			"machines": schema.ListNestedAttribute{
				Computed:    true,
				Description: "One entry per machine.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":             schema.StringAttribute{Computed: true, Description: "Machine identifier."},
						"name":           schema.StringAttribute{Computed: true, Description: "Machine name."},
						"location":       schema.StringAttribute{Computed: true, Description: "Location or country code the machine sits in."},
						"listed":         schema.BoolAttribute{Computed: true, Description: "Whether the machine is listed for others to rent."},
						"gpu_type_id":    schema.StringAttribute{Computed: true, Description: "GPU type identifier of the GPUs installed in the machine."},
						"gpu_total":      schema.Int64Attribute{Computed: true, Description: "Number of GPUs installed."},
						"secure_cloud":   schema.BoolAttribute{Computed: true, Description: "Whether the machine runs in secure cloud."},
						"data_center_id": schema.StringAttribute{Computed: true, Description: "Data center the machine belongs to."},
					},
				},
			},
		},
	}
}

func (d *machinesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredLegacyClient(req.ProviderData, &resp.Diagnostics)
}

func (d *machinesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	data, err := d.client.Query(ctx, getMachinesQuery, nil)
	if err != nil {
		resp.Diagnostics.AddError("Listing machines failed", err.Error())
		return
	}
	myself, ok := data["myself"].(map[string]any)
	if !ok {
		resp.Diagnostics.AddError("Listing machines failed", "the myself query returned no account")
		return
	}
	machines, ok := myself["machines"].([]any)
	if !ok {
		resp.Diagnostics.AddError("Listing machines failed", "the myself query returned no machines list")
		return
	}

	state := flattenMachines(machines)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenMachineDataSource(machine map[string]any, id types.String) machineDataSourceModel {
	return machineDataSourceModel{
		Id:              id,
		Name:            jsonString(machine, "name"),
		Location:        jsonString(machine, "location"),
		Listed:          jsonBool(machine, "listed"),
		GpuTypeId:       jsonNestedString(machine, "gpuType", "id"),
		GpuTotal:        jsonInt64(machine, "gpuTotal"),
		GpuReserved:     jsonInt64(machine, "gpuReserved"),
		CpuCount:        jsonInt64(machine, "cpuCount"),
		CpuTypeId:       jsonString(machine, "cpuTypeId"),
		MemoryTotal:     jsonInt64(machine, "memoryTotal"),
		MemoryReserved:  jsonInt64(machine, "memoryReserved"),
		DiskTotal:       jsonInt64(machine, "diskTotal"),
		DiskReserved:    jsonInt64(machine, "diskReserved"),
		SecureCloud:     jsonBool(machine, "secureCloud"),
		MaintenanceMode: jsonBool(machine, "maintenanceMode"),
		Verified:        jsonBool(machine, "verified"),
		HostPricePerGpu: jsonFloat64(machine, "hostPricePerGpu"),
		DataCenterId:    jsonString(machine, "dataCenterId"),
		RunpodIp:        jsonString(machine, "runpodIp"),
	}
}

func flattenMachines(machines []any) machinesDataSourceModel {
	state := machinesDataSourceModel{Machines: []machineListItemModel{}}
	for _, entry := range machines {
		machine, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		state.Machines = append(state.Machines, machineListItemModel{
			Id:           jsonString(machine, "id"),
			Name:         jsonString(machine, "name"),
			Location:     jsonString(machine, "location"),
			Listed:       jsonBool(machine, "listed"),
			GpuTypeId:    jsonNestedString(machine, "gpuType", "id"),
			GpuTotal:     jsonInt64(machine, "gpuTotal"),
			SecureCloud:  jsonBool(machine, "secureCloud"),
			DataCenterId: jsonString(machine, "dataCenterId"),
		})
	}
	return state
}
