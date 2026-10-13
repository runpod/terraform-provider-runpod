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

// Cluster datasources: runpod_clusters (all clusters), runpod_cluster (one by
// id), runpod_cluster_pods (the member pods of one cluster). The first two
// share clusterSummaryModel — the cluster object's own fields, without the
// create-only container configuration the resource also carries.

// clusterSummaryModel is every field a cluster read returns. The tfsdk names
// match the computed attributes of runpod_cluster so a data source and a
// managed cluster read the same.
type clusterSummaryModel struct {
	Id                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Type               types.String `tfsdk:"type"`
	GpuTypeId          types.String `tfsdk:"gpu_type_id"`
	GpuCountPerPod     types.Int64  `tfsdk:"gpu_count_per_pod"`
	PodCount           types.Int64  `tfsdk:"pod_count"`
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

func flattenClusterSummary(ctx context.Context, cluster sdktypes.Cluster) (clusterSummaryModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	model := clusterSummaryModel{
		Id:             types.StringValue(cluster.Id),
		Name:           types.StringValue(cluster.Name),
		Type:           types.StringValue(string(cluster.Type)),
		GpuTypeId:      types.StringValue(cluster.Compute.GpuTypeId),
		GpuCountPerPod: types.Int64Value(int64(cluster.Compute.GpuCountPerPod)),
		PodCount:       types.Int64Value(int64(cluster.Compute.PodCount)),
		DataCenterId:   types.StringPointerValue(cluster.DataCenterId),
		Template:       types.StringPointerValue(cluster.Template),
		CreatedAt:      types.StringValue(cluster.CreatedAt.Format(time.RFC3339)),
		PodsTotal:      types.Int64Value(int64(cluster.Pods.Total)),
	}

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
	}
	if cluster.Network != nil {
		model.NetworkCidr = types.StringValue(cluster.Network.Cidr)
		model.NetworkVxlanId = int64PointerValue(cluster.Network.VxlanId)
		model.NetworkVxlanPort = int64PointerValue(cluster.Network.VxlanPort)
	}

	return model, diags
}

// clusterSummaryAttributes is the shared attribute set; idRequired switches the
// by-id lookup's input attribute on.
func clusterSummaryAttributes(idRequired bool) map[string]schema.Attribute {
	id := schema.StringAttribute{Computed: true, Description: "Cluster identifier."}
	if idRequired {
		id = schema.StringAttribute{Required: true, Description: "Cluster identifier, e.g. \"cluster_abc123\"."}
	}
	return map[string]schema.Attribute{
		"id":   id,
		"name": schema.StringAttribute{Computed: true, Description: "Cluster display name."},
		"type": schema.StringAttribute{
			Computed:    true,
			Description: "Cluster type: TRAINING, SLURM, RAY, or APPLICATION.",
		},
		"gpu_type_id": schema.StringAttribute{
			Computed:    true,
			Description: "GPU type every pod in the cluster runs on.",
		},
		"gpu_count_per_pod": schema.Int64Attribute{
			Computed:    true,
			Description: "Number of GPUs on each pod.",
		},
		"pod_count": schema.Int64Attribute{
			Computed:    true,
			Description: "Number of pods (nodes) the cluster was created with.",
		},
		"data_center_id": schema.StringAttribute{
			Computed:    true,
			Description: "Data center the cluster is placed in; null until at least one pod is placed.",
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
			Description: "Number of member pods currently provisioned.",
		},
		"pods_by_status": schema.MapAttribute{
			ElementType: types.Int64Type,
			Computed:    true,
			Description: "Member pod counts keyed by pod status; statuses with no pods are absent.",
		},
		"primary_pod_id": schema.StringAttribute{
			Computed:    true,
			Description: "Pod id of the primary (rank 0) node; null until a primary pod is placed.",
		},
		"primary_status": schema.StringAttribute{
			Computed:    true,
			Description: "Lifecycle status of the primary node.",
		},
		"primary_ssh_endpoint": schema.StringAttribute{
			Computed:    true,
			Description: "Public SSH endpoint (host:port) of the primary node; null until it is RUNNING with SSH exposed.",
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
	}
}

var _ datasource.DataSourceWithConfigure = (*clustersDataSource)(nil)

func NewClustersDataSource() datasource.DataSource { return &clustersDataSource{} }

type clustersDataSource struct {
	client *runpod.Client
}

type clustersDataSourceModel struct {
	Clusters []clusterSummaryModel `tfsdk:"clusters"`
}

func (d *clustersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clusters"
}

func (d *clustersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "All clusters owned by the authenticated account.",
		Attributes: map[string]schema.Attribute{
			"clusters": schema.ListNestedAttribute{
				Computed:     true,
				Description:  "One entry per cluster.",
				NestedObject: schema.NestedAttributeObject{Attributes: clusterSummaryAttributes(false)},
			},
		},
	}
}

func (d *clustersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *clustersDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	apiResp, err := d.client.Clusters.ListClustersWithResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Listing clusters failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Listing clusters failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)))
		return
	}

	state := clustersDataSourceModel{Clusters: []clusterSummaryModel{}}
	for _, cluster := range apiResp.JSON200.Clusters {
		model, diags := flattenClusterSummary(ctx, cluster)
		resp.Diagnostics.Append(diags...)
		state.Clusters = append(state.Clusters, model)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

var _ datasource.DataSourceWithConfigure = (*clusterDataSource)(nil)

func NewClusterDataSource() datasource.DataSource { return &clusterDataSource{} }

type clusterDataSource struct {
	client *runpod.Client
}

func (d *clusterDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (d *clusterDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A single cluster looked up by identifier. Member pods are listed by the " +
			"runpod_cluster_pods data source.",
		Attributes: clusterSummaryAttributes(true),
	}
}

func (d *clusterDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *clusterDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config clusterSummaryModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := d.client.Clusters.GetClusterWithResponse(ctx, config.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading cluster failed", err.Error())
		return
	}
	if apiResp.HTTPResponse.StatusCode == http.StatusNotFound || apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Reading cluster failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	state, diags := flattenClusterSummary(ctx, *apiResp.JSON200)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

var _ datasource.DataSourceWithConfigure = (*clusterPodsDataSource)(nil)

func NewClusterPodsDataSource() datasource.DataSource { return &clusterPodsDataSource{} }

type clusterPodsDataSource struct {
	client *runpod.Client
}

// clusterPodListItemModel is a member pod: the pod fields the account-wide
// runpod_pods listing carries, plus its place in the cluster.
type clusterPodListItemModel struct {
	Id           types.String  `tfsdk:"id"`
	Name         types.String  `tfsdk:"name"`
	Status       types.String  `tfsdk:"status"`
	Image        types.String  `tfsdk:"image"`
	Cloud        types.String  `tfsdk:"cloud"`
	Disk         types.Int64   `tfsdk:"disk"`
	Cost         types.Float64 `tfsdk:"cost"`
	GpuTypeId    types.String  `tfsdk:"gpu_type_id"`
	GpuCount     types.Int64   `tfsdk:"gpu_count"`
	DataCenterId types.String  `tfsdk:"data_center_id"`
	CreatedAt    types.String  `tfsdk:"created_at"`
	ClusterRank  types.Int64   `tfsdk:"cluster_rank"`
	ClusterRole  types.String  `tfsdk:"cluster_role"`
	ClusterIp    types.String  `tfsdk:"cluster_ip"`
}

type clusterPodsDataSourceModel struct {
	ClusterId types.String              `tfsdk:"cluster_id"`
	Pods      []clusterPodListItemModel `tfsdk:"pods"`
}

func (d *clusterPodsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_pods"
}

func (d *clusterPodsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The member pods of one cluster, with their runtime state and cluster ranks. " +
			"Member pods are excluded from the account-wide runpod_pods listing unless it sets include_cluster_pods.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Required:    true,
				Description: "Cluster whose pods to list.",
			},
			"pods": schema.ListNestedAttribute{
				Computed:    true,
				Description: "One entry per member pod.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":             schema.StringAttribute{Computed: true, Description: "Pod identifier."},
						"name":           schema.StringAttribute{Computed: true, Description: "Pod display name."},
						"status":         schema.StringAttribute{Computed: true, Description: "Pod lifecycle status, e.g. RUNNING, PROVISIONING."},
						"image":          schema.StringAttribute{Computed: true, Description: "Docker image reference."},
						"cloud":          schema.StringAttribute{Computed: true, Description: "Cloud tier: SECURE or COMMUNITY."},
						"disk":           schema.Int64Attribute{Computed: true, Description: "Container disk in GB (ephemeral)."},
						"cost":           schema.Float64Attribute{Computed: true, Description: "Current cost in USD per hour; 0 when the pod is stopped."},
						"gpu_type_id":    schema.StringAttribute{Computed: true, Description: "GPU type identifier; null for a pod with no GPU."},
						"gpu_count":      schema.Int64Attribute{Computed: true, Description: "Number of GPUs attached to the pod."},
						"data_center_id": schema.StringAttribute{Computed: true, Description: "Data center the pod runs in."},
						"created_at":     schema.StringAttribute{Computed: true, Description: "Creation timestamp in RFC 3339 format."},
						"cluster_rank":   schema.Int64Attribute{Computed: true, Description: "Node rank (NODE_RANK) within the cluster; rank 0 is the primary. Null until assigned."},
						"cluster_role":   schema.StringAttribute{Computed: true, Description: "SLURM or RAY role, e.g. SLURM_CONTROLLER. Null for TRAINING and APPLICATION clusters, which assign no roles."},
						"cluster_ip":     schema.StringAttribute{Computed: true, Description: "The pod's address on the cluster's private overlay network; null until assigned."},
					},
				},
			},
		},
	}
}

func (d *clusterPodsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *clusterPodsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config clusterPodsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := d.client.Clusters.ListClusterPodsWithResponse(ctx, config.ClusterId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Listing cluster pods failed", err.Error())
		return
	}
	if apiResp.HTTPResponse.StatusCode == http.StatusNotFound || apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Listing cluster pods failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)))
		return
	}

	state := clusterPodsDataSourceModel{
		ClusterId: config.ClusterId,
		Pods:      []clusterPodListItemModel{},
	}
	for _, pod := range apiResp.JSON200.Pods {
		state.Pods = append(state.Pods, flattenClusterPod(pod))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenClusterPod(pod sdktypes.Pod) clusterPodListItemModel {
	item := clusterPodListItemModel{
		Id:           types.StringValue(pod.Id),
		Name:         types.StringValue(pod.Name),
		Status:       types.StringValue(string(pod.Status)),
		Image:        types.StringValue(pod.Image),
		Cloud:        types.StringValue(string(pod.Cloud)),
		Disk:         types.Int64Value(int64(pod.Disk)),
		Cost:         types.Float64Value(float64(pod.Cost)),
		DataCenterId: types.StringPointerValue(pod.DataCenterId),
		CreatedAt:    types.StringValue(pod.CreatedAt.Format(time.RFC3339)),
	}
	if pod.Gpu != nil {
		item.GpuTypeId = types.StringValue(pod.Gpu.Id)
		item.GpuCount = int64PointerValue(pod.Gpu.Count)
	}
	if pod.Cluster != nil {
		item.ClusterRank = int64PointerValue(pod.Cluster.Rank)
		item.ClusterIp = types.StringPointerValue(pod.Cluster.Ip)
		if pod.Cluster.Role != nil {
			item.ClusterRole = types.StringValue(string(*pod.Cluster.Role))
		}
	}
	return item
}
