package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// runpod_pod: one pod by id, with every field the API reports. The attributes
// the runpod_pod resource also carries keep the resource's names, so a
// configuration can move between the two without renaming references; the
// fields the resource does not manage (cost, runtime, ssh, cluster, mounts)
// are added here.

var (
	_ datasource.DataSource              = (*podDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*podDataSource)(nil)
)

func NewPodDataSource() datasource.DataSource { return &podDataSource{} }

type podDataSource struct {
	client *runpod.Client
}

type podSshEndpointModel struct {
	Command  types.String `tfsdk:"command"`
	Host     types.String `tfsdk:"host"`
	Port     types.Int64  `tfsdk:"port"`
	Username types.String `tfsdk:"username"`
}

type podSshModel struct {
	Proxy  *podSshEndpointModel `tfsdk:"proxy"`
	Direct *podSshEndpointModel `tfsdk:"direct"`
}

type podClusterModel struct {
	Id   types.String `tfsdk:"id"`
	Ip   types.String `tfsdk:"ip"`
	Rank types.Int64  `tfsdk:"rank"`
	Role types.String `tfsdk:"role"`
}

type podMountsModel struct {
	Persistent *persistentMountModel `tfsdk:"persistent"`
	Network    []networkMountModel   `tfsdk:"network"`
}

type podDetailModel struct {
	Id                          types.String     `tfsdk:"id"`
	Name                        types.String     `tfsdk:"name"`
	Image                       types.String     `tfsdk:"image"`
	Status                      types.String     `tfsdk:"status"`
	Cloud                       types.String     `tfsdk:"cloud"`
	Disk                        types.Int64      `tfsdk:"disk"`
	Args                        types.String     `tfsdk:"args"`
	Cost                        types.Float64    `tfsdk:"cost"`
	Locked                      types.Bool       `tfsdk:"locked"`
	CudaVersion                 types.String     `tfsdk:"cuda_version"`
	Template                    types.String     `tfsdk:"template"`
	Registry                    types.String     `tfsdk:"registry"`
	DataCenterId                types.String     `tfsdk:"data_center_id"`
	CreatedAt                   types.String     `tfsdk:"created_at"`
	StartedAt                   types.String     `tfsdk:"started_at"`
	Env                         types.Map        `tfsdk:"env"`
	Ports                       types.List       `tfsdk:"ports"`
	Actions                     types.List       `tfsdk:"actions"`
	GlobalNetworking            types.Bool       `tfsdk:"global_networking"`
	GlobalNetworkingIp          types.String     `tfsdk:"global_networking_ip"`
	GlobalNetworkingInternalDns types.String     `tfsdk:"global_networking_internal_dns"`
	GpuTypeId                   types.String     `tfsdk:"gpu_type_id"`
	GpuCount                    types.Int64      `tfsdk:"gpu_count"`
	GpuMemory                   types.Float64    `tfsdk:"gpu_memory"`
	GpuVcpuCount                types.Float64    `tfsdk:"gpu_vcpu_count"`
	CpuTypeId                   types.String     `tfsdk:"cpu_type_id"`
	CpuMemory                   types.Int64      `tfsdk:"cpu_memory"`
	CpuVcpuCount                types.Int64      `tfsdk:"cpu_vcpu_count"`
	Cluster                     *podClusterModel `tfsdk:"cluster"`
	Mounts                      *podMountsModel  `tfsdk:"mounts"`
	Runtime                     types.Object     `tfsdk:"runtime"`
	Ssh                         *podSshModel     `tfsdk:"ssh"`
}

func (d *podDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pod"
}

func podSshEndpointAttributes(role string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"command":  schema.StringAttribute{Computed: true, Description: "Ready-to-run ssh command for the " + role + " connection."},
		"host":     schema.StringAttribute{Computed: true, Description: "Host to connect to."},
		"port":     schema.Int64Attribute{Computed: true, Description: "TCP port to connect to."},
		"username": schema.StringAttribute{Computed: true, Description: "SSH user name."},
	}
}

func (d *podDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A single pod looked up by identifier. Reports everything the API returns for the pod, including the live cost, runtime utilization, SSH details, mounts, and cluster membership that the runpod_pod resource does not manage.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Required: true, Description: "Pod identifier, e.g. \"pod_abc123\"."},
			"name":           schema.StringAttribute{Computed: true, Description: "Pod display name."},
			"image":          schema.StringAttribute{Computed: true, Description: "Docker image reference."},
			"status":         schema.StringAttribute{Computed: true, Description: "Pod lifecycle status: PROVISIONING, STARTING, RUNNING, EXITED, ERROR, or TERMINATED."},
			"cloud":          schema.StringAttribute{Computed: true, Description: "Cloud tier: SECURE or COMMUNITY."},
			"disk":           schema.Int64Attribute{Computed: true, Description: "Container disk in GB (ephemeral)."},
			"args":           schema.StringAttribute{Computed: true, Description: "Arguments passed to the container entrypoint."},
			"cost":           schema.Float64Attribute{Computed: true, Description: "Current cost in USD per hour; 0 while the pod is EXITED or TERMINATED."},
			"locked":         schema.BoolAttribute{Computed: true, Description: "Whether the pod is locked, which blocks stopping and resetting it."},
			"cuda_version":   schema.StringAttribute{Computed: true, Description: "CUDA version reported by the host machine; null for CPU pods and for hosts that report none."},
			"template":       schema.StringAttribute{Computed: true, Description: "Identifier of the template the pod was created from; null when it was created without one."},
			"registry":       schema.StringAttribute{Computed: true, Description: "Registry credential ID used to pull the image; null for public images."},
			"data_center_id": schema.StringAttribute{Computed: true, Description: "Data center the pod runs in."},
			"created_at":     schema.StringAttribute{Computed: true, Description: "Creation timestamp in RFC 3339 format."},
			"started_at":     schema.StringAttribute{Computed: true, Description: "Timestamp of the pod's last start in RFC 3339 format; null until it has started."},
			"env": schema.MapAttribute{
				ElementType: types.StringType,
				Computed:    true,
				Description: "Environment variables set in the container.",
			},
			"ports": schema.ListAttribute{
				ElementType: types.StringType,
				Computed:    true,
				Description: "Exposed ports, formatted as port/protocol.",
			},
			"actions": schema.ListAttribute{
				ElementType: types.StringType,
				Computed:    true,
				Description: "State transitions the pod accepts in its current status.",
			},
			"global_networking":              schema.BoolAttribute{Computed: true, Description: "Whether global networking is enabled, giving the pod a private IP reachable across data centers."},
			"global_networking_ip":           schema.StringAttribute{Computed: true, Description: "Global-networking IP; null unless global networking is enabled."},
			"global_networking_internal_dns": schema.StringAttribute{Computed: true, Description: "Internal DNS name reachable from other globally-networked pods in the account; null unless global networking is enabled."},
			"gpu_type_id":                    schema.StringAttribute{Computed: true, Description: "GPU type identifier; null for a CPU pod."},
			"gpu_count":                      schema.Int64Attribute{Computed: true, Description: "Number of GPUs attached; null for a CPU pod."},
			"gpu_memory":                     schema.Float64Attribute{Computed: true, Description: "System RAM in GB across all requested GPUs; null for a CPU pod."},
			"gpu_vcpu_count":                 schema.Float64Attribute{Computed: true, Description: "vCPU count across all requested GPUs; null for a CPU pod."},
			"cpu_type_id":                    schema.StringAttribute{Computed: true, Description: "CPU flavor identifier; null for a GPU pod."},
			"cpu_memory":                     schema.Int64Attribute{Computed: true, Description: "Memory in GB allocated to a CPU pod; null for a GPU pod."},
			"cpu_vcpu_count":                 schema.Int64Attribute{Computed: true, Description: "vCPU count allocated to a CPU pod; null for a GPU pod."},
			"cluster": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "Cluster membership; null for a standalone pod. Member pods are managed through the cluster, not the pod endpoints.",
				Attributes: map[string]schema.Attribute{
					"id":   schema.StringAttribute{Computed: true, Description: "Identifier of the cluster the pod belongs to."},
					"ip":   schema.StringAttribute{Computed: true, Description: "Address on the cluster's private overlay network; null until assigned."},
					"rank": schema.Int64Attribute{Computed: true, Description: "Node rank within the cluster; rank 0 is the entry node. Null until assigned."},
					"role": schema.StringAttribute{Computed: true, Description: "SLURM or RAY role; null for TRAINING and APPLICATION clusters, which assign no roles."},
				},
			},
			"mounts": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "Storage mounted into the pod. At most one of persistent or network is set.",
				Attributes: map[string]schema.Attribute{
					"persistent": schema.SingleNestedAttribute{
						Computed:    true,
						Description: "Host-local persistent storage, pinned to the pod's host machine; null when the pod has no persistent mount.",
						Attributes: map[string]schema.Attribute{
							"path": schema.StringAttribute{Computed: true, Description: "Mount path inside the container."},
							"size": schema.Int64Attribute{Computed: true, Description: "Allocated storage in GB."},
						},
					},
					"network": schema.ListNestedAttribute{
						Computed:    true,
						Description: "Network volumes mounted into the pod; empty when the pod has none.",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"path":      schema.StringAttribute{Computed: true, Description: "Mount path inside the container."},
								"volume_id": schema.StringAttribute{Computed: true, Description: "Identifier of the mounted network volume."},
							},
						},
					},
				},
			},
			"runtime": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "Live utilization metrics; null unless the pod is RUNNING.",
				Attributes: map[string]schema.Attribute{
					"uptime_seconds": schema.Int64Attribute{Computed: true, Description: "Seconds since the container started."},
					"cpu_util":       schema.Int64Attribute{Computed: true, Description: "CPU utilization percentage, 0 to 100."},
					"memory_util":    schema.Int64Attribute{Computed: true, Description: "Memory utilization percentage, 0 to 100."},
					"gpus": schema.ListNestedAttribute{
						Computed:    true,
						Description: "One entry per attached GPU.",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"util":        schema.Int64Attribute{Computed: true, Description: "GPU utilization percentage, 0 to 100."},
								"memory_util": schema.Int64Attribute{Computed: true, Description: "GPU memory utilization percentage, 0 to 100."},
							},
						},
					},
					"ports": schema.ListNestedAttribute{
						Computed:    true,
						Description: "Port mappings assigned to the running pod.",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"ip":           schema.StringAttribute{Computed: true, Description: "Address the port is reachable on."},
								"private_port": schema.Int64Attribute{Computed: true, Description: "Port inside the container."},
								"public_port":  schema.Int64Attribute{Computed: true, Description: "Port published outside the container."},
								"type":         schema.StringAttribute{Computed: true, Description: "Protocol of the mapping, e.g. http or tcp."},
							},
						},
					},
				},
			},
			"ssh": schema.SingleNestedAttribute{
				Computed: true,
				Description: "How to reach the pod over SSH. Both variants authenticate with the account's registered SSH public keys, " +
					"which only reach a pod created with SSH provisioning enabled.",
				Attributes: map[string]schema.Attribute{
					"proxy": schema.SingleNestedAttribute{
						Computed:    true,
						Description: "Connection through the Runpod SSH proxy: interactive shell only, no port forwarding or file copy. Null until the pod has a machine assignment.",
						Attributes:  podSshEndpointAttributes("proxy"),
					},
					"direct": schema.SingleNestedAttribute{
						Computed:    true,
						Description: "Connection straight to the pod's sshd over its published 22/tcp mapping, with the full SSH feature set. Null unless 22/tcp is exposed and the running pod has a public port for it.",
						Attributes:  podSshEndpointAttributes("direct"),
					},
				},
			},
		},
	}
}

func (d *podDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *podDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config podDetailModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := d.client.Pods.GetPodWithResponse(ctx, config.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading pod failed", err.Error())
		return
	}
	if apiResp.HTTPResponse.StatusCode == http.StatusNotFound {
		resp.Diagnostics.AddError("Reading pod failed", "no pod with id "+config.Id.ValueString())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Reading pod failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	state, diags := flattenPodDetail(ctx, *apiResp.JSON200)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenPodDetail(ctx context.Context, pod sdktypes.Pod) (podDetailModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	model := podDetailModel{
		Id:                          types.StringValue(pod.Id),
		Name:                        types.StringValue(pod.Name),
		Image:                       types.StringValue(pod.Image),
		Status:                      types.StringValue(string(pod.Status)),
		Cloud:                       types.StringValue(string(pod.Cloud)),
		Disk:                        types.Int64Value(int64(pod.Disk)),
		Args:                        types.StringValue(pod.Args),
		Cost:                        types.Float64Value(float64(pod.Cost)),
		Locked:                      types.BoolValue(pod.Locked),
		CudaVersion:                 types.StringPointerValue(pod.CudaVersion),
		Template:                    types.StringPointerValue(pod.Template),
		Registry:                    types.StringPointerValue(pod.Registry),
		DataCenterId:                types.StringPointerValue(pod.DataCenterId),
		CreatedAt:                   types.StringValue(pod.CreatedAt.Format(time.RFC3339)),
		StartedAt:                   types.StringNull(),
		GlobalNetworking:            types.BoolValue(pod.GlobalNetworking.Enabled),
		GlobalNetworkingIp:          types.StringPointerValue(pod.GlobalNetworking.Ip),
		GlobalNetworkingInternalDns: types.StringPointerValue(pod.GlobalNetworking.InternalDns),
		GpuTypeId:                   types.StringNull(),
		GpuCount:                    types.Int64Null(),
		GpuMemory:                   types.Float64Null(),
		GpuVcpuCount:                types.Float64Null(),
		CpuTypeId:                   types.StringNull(),
		CpuMemory:                   types.Int64Null(),
		CpuVcpuCount:                types.Int64Null(),
	}

	if pod.StartedAt != nil {
		model.StartedAt = types.StringValue(pod.StartedAt.Format(time.RFC3339))
	}

	env, envDiags := types.MapValueFrom(ctx, types.StringType, pod.Env)
	diags.Append(envDiags...)
	model.Env = env

	ports, portDiags := types.ListValueFrom(ctx, types.StringType, pod.Ports)
	diags.Append(portDiags...)
	model.Ports = ports

	actions := make([]string, 0, len(pod.Actions))
	for _, action := range pod.Actions {
		actions = append(actions, string(action))
	}
	actionList, actionDiags := types.ListValueFrom(ctx, types.StringType, actions)
	diags.Append(actionDiags...)
	model.Actions = actionList

	if pod.Gpu != nil {
		model.GpuTypeId = types.StringValue(pod.Gpu.Id)
		model.GpuCount = int64PointerValue(pod.Gpu.Count)
		model.GpuMemory = types.Float64Value(pod.Gpu.Memory)
		model.GpuVcpuCount = types.Float64Value(pod.Gpu.VcpuCount)
	}
	if pod.Cpu != nil {
		model.CpuTypeId = types.StringValue(pod.Cpu.Id)
		model.CpuMemory = types.Int64Value(int64(pod.Cpu.Memory))
		model.CpuVcpuCount = types.Int64Value(int64(pod.Cpu.VcpuCount))
	}

	if pod.Cluster != nil {
		cluster := podClusterModel{
			Id:   types.StringValue(pod.Cluster.Id),
			Ip:   types.StringPointerValue(pod.Cluster.Ip),
			Rank: int64PointerValue(pod.Cluster.Rank),
			Role: types.StringNull(),
		}
		if pod.Cluster.Role != nil {
			cluster.Role = types.StringValue(string(*pod.Cluster.Role))
		}
		model.Cluster = &cluster
	}

	model.Mounts = flattenPodMounts(pod.Mounts)
	runtime, runtimeDiags := flattenPodRuntime(ctx, pod.Runtime)
	diags.Append(runtimeDiags...)
	model.Runtime = runtime
	model.Ssh = &podSshModel{
		Proxy:  flattenPodSshEndpoint(pod.Ssh.Proxy),
		Direct: flattenPodSshEndpoint(pod.Ssh.Direct),
	}

	return model, diags
}

func flattenPodMounts(mounts sdktypes.Mounts) *podMountsModel {
	model := podMountsModel{Network: []networkMountModel{}}
	if mounts.Persistent != nil {
		model.Persistent = &persistentMountModel{
			Path: types.StringValue(mounts.Persistent.Path),
			Size: types.Int64Value(int64(mounts.Persistent.Size)),
		}
	}
	if mounts.Network != nil {
		for _, mount := range *mounts.Network {
			model.Network = append(model.Network, networkMountModel{
				Path:     types.StringValue(mount.Path),
				VolumeId: types.StringValue(mount.VolumeId),
			})
		}
	}
	return &model
}

func flattenPodSshEndpoint(endpoint *sdktypes.PodSshEndpoint) *podSshEndpointModel {
	if endpoint == nil {
		return nil
	}
	return &podSshEndpointModel{
		Command:  types.StringValue(endpoint.Command),
		Host:     types.StringValue(endpoint.Host),
		Port:     types.Int64Value(int64(endpoint.Port)),
		Username: types.StringValue(endpoint.Username),
	}
}
