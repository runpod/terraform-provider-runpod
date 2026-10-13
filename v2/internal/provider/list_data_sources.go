package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/lib/pods"
	"github.com/runpod/go-sdk/v2/lib/serverless"
	"github.com/runpod/go-sdk/v2/lib/templates"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// Account-wide list datasources: runpod_pods, runpod_templates,
// runpod_network_volumes, runpod_serverless_endpoints, runpod_registries.
// Items carry the core identification fields; full detail lives on the
// corresponding resource read.

var _ datasource.DataSourceWithConfigure = (*podsDataSource)(nil)

func NewPodsDataSource() datasource.DataSource { return &podsDataSource{} }

type podsDataSource struct {
	client *runpod.Client
}

type podListItemModel struct {
	Id           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Image        types.String `tfsdk:"image"`
	Status       types.String `tfsdk:"status"`
	Cloud        types.String `tfsdk:"cloud"`
	GpuTypeId    types.String `tfsdk:"gpu_type_id"`
	GpuCount     types.Int64  `tfsdk:"gpu_count"`
	Disk         types.Int64  `tfsdk:"disk"`
	DataCenterId types.String `tfsdk:"data_center_id"`
	CreatedAt    types.String `tfsdk:"created_at"`
}

type podsDataSourceModel struct {
	IncludeClusterPods types.Bool         `tfsdk:"include_cluster_pods"`
	Pods               []podListItemModel `tfsdk:"pods"`
}

func (d *podsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pods"
}

func (d *podsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "All pods owned by the authenticated account. Pods that belong to a cluster are left out unless include_cluster_pods is set.",
		Attributes: map[string]schema.Attribute{
			"include_cluster_pods": schema.BoolAttribute{
				Optional:    true,
				Description: "Include pods that belong to a cluster. Defaults to false; runpod_cluster_pods lists one cluster's members on their own.",
			},
			"pods": schema.ListNestedAttribute{
				Computed:    true,
				Description: "One entry per pod.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":             schema.StringAttribute{Computed: true, Description: "Pod identifier."},
						"name":           schema.StringAttribute{Computed: true, Description: "Pod display name."},
						"image":          schema.StringAttribute{Computed: true, Description: "Docker image reference."},
						"status":         schema.StringAttribute{Computed: true, Description: "Pod lifecycle status, e.g. RUNNING, EXITED, PROVISIONING."},
						"cloud":          schema.StringAttribute{Computed: true, Description: "Cloud tier: SECURE or COMMUNITY."},
						"gpu_type_id":    schema.StringAttribute{Computed: true, Description: "GPU type identifier; null for a pod with no GPU."},
						"gpu_count":      schema.Int64Attribute{Computed: true, Description: "Number of GPUs attached to the pod."},
						"disk":           schema.Int64Attribute{Computed: true, Description: "Container disk in GB (ephemeral)."},
						"data_center_id": schema.StringAttribute{Computed: true, Description: "Data center the pod runs in."},
						"created_at":     schema.StringAttribute{Computed: true, Description: "Creation timestamp in RFC 3339 format."},
					},
				},
			},
		},
	}
}

func (d *podsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *podsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config podsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var includeClusterPods *bool
	if !config.IncludeClusterPods.IsNull() && !config.IncludeClusterPods.IsUnknown() {
		includeClusterPods = config.IncludeClusterPods.ValueBoolPointer()
	}

	// With cluster members filtered out a page can come back short, even empty,
	// while more pods follow, so only hasNextPage says when the list is done.
	allPods, err := listAllPages(ctx, func(ctx context.Context, cursor *string) ([]sdktypes.Pod, sdktypes.Pagination, error) {
		apiResp, err := d.client.Pods.ListPodsWithResponse(ctx, &pods.ListPodsParams{
			IncludeClusterPods: includeClusterPods,
			Cursor:             cursor,
		})
		if err != nil {
			return nil, sdktypes.Pagination{}, err
		}
		if apiResp.JSON200 == nil {
			return nil, sdktypes.Pagination{}, fmt.Errorf("API returned status %d: %s",
				apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body))
		}
		return apiResp.JSON200.Pods, apiResp.JSON200.Pagination, nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Listing pods failed", err.Error())
		return
	}

	state := podsDataSourceModel{IncludeClusterPods: config.IncludeClusterPods, Pods: []podListItemModel{}}
	for _, pod := range allPods {
		item := podListItemModel{
			Id:           types.StringValue(pod.Id),
			Name:         types.StringValue(pod.Name),
			Image:        types.StringValue(pod.Image),
			Status:       types.StringValue(string(pod.Status)),
			Cloud:        types.StringValue(string(pod.Cloud)),
			Disk:         types.Int64Value(int64(pod.Disk)),
			DataCenterId: types.StringPointerValue(pod.DataCenterId),
			CreatedAt:    types.StringValue(pod.CreatedAt.Format(time.RFC3339)),
		}
		if pod.Gpu != nil {
			item.GpuTypeId = types.StringValue(pod.Gpu.Id)
			if pod.Gpu.Count != nil {
				item.GpuCount = types.Int64Value(int64(*pod.Gpu.Count))
			}
		}
		state.Pods = append(state.Pods, item)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

var _ datasource.DataSourceWithConfigure = (*templatesDataSource)(nil)

func NewTemplatesDataSource() datasource.DataSource { return &templatesDataSource{} }

type templatesDataSource struct {
	client *runpod.Client
}

type templateListItemModel struct {
	Id         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Image      types.String `tfsdk:"image"`
	Category   types.String `tfsdk:"category"`
	Disk       types.Int64  `tfsdk:"disk"`
	Public     types.Bool   `tfsdk:"public"`
	Serverless types.Bool   `tfsdk:"serverless"`
}

type templatesDataSourceModel struct {
	Templates []templateListItemModel `tfsdk:"templates"`
}

func (d *templatesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_templates"
}

func (d *templatesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "All templates visible to the authenticated account.",
		Attributes: map[string]schema.Attribute{
			"templates": schema.ListNestedAttribute{
				Computed:    true,
				Description: "One entry per template.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":         schema.StringAttribute{Computed: true, Description: "Template identifier."},
						"name":       schema.StringAttribute{Computed: true, Description: "Template name."},
						"image":      schema.StringAttribute{Computed: true, Description: "Docker image reference."},
						"category":   schema.StringAttribute{Computed: true, Description: "Console grouping: CPU, NVIDIA, or AMD."},
						"disk":       schema.Int64Attribute{Computed: true, Description: "Container disk in GB."},
						"public":     schema.BoolAttribute{Computed: true, Description: "Whether the template is visible to other Runpod users."},
						"serverless": schema.BoolAttribute{Computed: true, Description: "Whether the template configures Serverless workers (true) or pods (false)."},
					},
				},
			},
		},
	}
}

func (d *templatesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *templatesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	allTemplates, err := listAllPages(ctx, func(ctx context.Context, cursor *string) ([]sdktypes.Template, sdktypes.Pagination, error) {
		apiResp, err := d.client.Templates.ListTemplatesWithResponse(ctx, &templates.ListTemplatesParams{Cursor: cursor})
		if err != nil {
			return nil, sdktypes.Pagination{}, err
		}
		if apiResp.JSON200 == nil {
			return nil, sdktypes.Pagination{}, fmt.Errorf("API returned status %d: %s",
				apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body))
		}
		return apiResp.JSON200.Templates, apiResp.JSON200.Pagination, nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Listing templates failed", err.Error())
		return
	}

	state := templatesDataSourceModel{Templates: []templateListItemModel{}}
	for _, template := range allTemplates {
		state.Templates = append(state.Templates, templateListItemModel{
			Id:         types.StringValue(template.Id),
			Name:       types.StringValue(template.Name),
			Image:      types.StringValue(template.Image),
			Category:   types.StringValue(string(template.Category)),
			Disk:       types.Int64Value(int64(template.Disk)),
			Public:     types.BoolValue(template.Public),
			Serverless: types.BoolValue(template.Serverless),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

var _ datasource.DataSourceWithConfigure = (*networkVolumesDataSource)(nil)

func NewNetworkVolumesDataSource() datasource.DataSource { return &networkVolumesDataSource{} }

type networkVolumesDataSource struct {
	client *runpod.Client
}

type networkVolumesDataSourceModel struct {
	NetworkVolumes []networkVolumeResourceModel `tfsdk:"network_volumes"`
}

func (d *networkVolumesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_volumes"
}

func (d *networkVolumesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "All network volumes owned by the authenticated account.",
		Attributes: map[string]schema.Attribute{
			"network_volumes": schema.ListNestedAttribute{
				Computed:    true,
				Description: "One entry per network volume.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          schema.StringAttribute{Computed: true, Description: "Volume identifier."},
						"name":        schema.StringAttribute{Computed: true, Description: "Human-readable volume name."},
						"size":        schema.Int64Attribute{Computed: true, Description: "Allocated storage in GB."},
						"data_center": schema.StringAttribute{Computed: true, Description: "Data center holding the volume."},
						"type":        schema.StringAttribute{Computed: true, Description: "Storage tier: STANDARD or HIGH_PERFORMANCE."},
					},
				},
			},
		},
	}
}

func (d *networkVolumesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *networkVolumesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	apiResp, err := d.client.NetworkVolumes.ListNetworkVolumesWithResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Listing network volumes failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Listing network volumes failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)))
		return
	}

	state := networkVolumesDataSourceModel{NetworkVolumes: []networkVolumeResourceModel{}}
	for _, volume := range apiResp.JSON200.NetworkVolumes {
		var model networkVolumeResourceModel
		flattenNetworkVolume(&volume, &model)
		state.NetworkVolumes = append(state.NetworkVolumes, model)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

var _ datasource.DataSourceWithConfigure = (*serverlessEndpointsDataSource)(nil)

func NewServerlessEndpointsDataSource() datasource.DataSource {
	return &serverlessEndpointsDataSource{}
}

type serverlessEndpointsDataSource struct {
	client *runpod.Client
}

type endpointListItemModel struct {
	Id           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Image        types.String `tfsdk:"image"`
	EndpointType types.String `tfsdk:"endpoint_type"`
	WorkersMin   types.Int64  `tfsdk:"workers_min"`
	WorkersMax   types.Int64  `tfsdk:"workers_max"`
	RequestUrls  types.Object `tfsdk:"request_urls"`
	CreatedAt    types.String `tfsdk:"created_at"`
}

type serverlessEndpointsDataSourceModel struct {
	Endpoints []endpointListItemModel `tfsdk:"endpoints"`
}

func (d *serverlessEndpointsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_serverless_endpoints"
}

func (d *serverlessEndpointsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "All Serverless endpoints owned by the authenticated account.",
		Attributes: map[string]schema.Attribute{
			"endpoints": schema.ListNestedAttribute{
				Computed:    true,
				Description: "One entry per Serverless endpoint.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":            schema.StringAttribute{Computed: true, Description: "Endpoint identifier."},
						"name":          schema.StringAttribute{Computed: true, Description: "Endpoint name."},
						"image":         schema.StringAttribute{Computed: true, Description: "Docker image reference for workers."},
						"endpoint_type": schema.StringAttribute{Computed: true, Description: "Request routing: QUEUE or LOAD_BALANCER."},
						"workers_min":   schema.Int64Attribute{Computed: true, Description: "Minimum number of workers."},
						"workers_max":   schema.Int64Attribute{Computed: true, Description: "Maximum number of workers."},
						"request_urls": schema.SingleNestedAttribute{
							Computed: true,
							Description: "Invocation URLs the API reports for this endpoint, the same set the runpod_serverless_endpoint resource reports. " +
								"A QUEUE endpoint fills the job operations (run, run_sync, status, stream, cancel, retry, purge_queue, health); a " +
								"LOAD_BALANCER endpoint fills base and health, since its remaining paths are worker-defined. The attributes that do " +
								"not apply to the endpoint's routing type are null.",
							Attributes: map[string]schema.Attribute{
								"base":        schema.StringAttribute{Computed: true, Description: "Base URL for the worker's own HTTP and WebSocket paths. LOAD_BALANCER endpoints only."},
								"cancel":      schema.StringAttribute{Computed: true, Description: "Cancel an in-progress job or drop a queued one. Takes a job id in place of the {job_id} segment."},
								"health":      schema.StringAttribute{Computed: true, Description: "Operational status of the endpoint, or the path the load balancer polls for worker health."},
								"purge_queue": schema.StringAttribute{Computed: true, Description: "Drop all pending jobs from the queue, leaving in-progress jobs running."},
								"retry":       schema.StringAttribute{Computed: true, Description: "Requeue a failed or timed-out job. Takes a job id in place of the {job_id} segment."},
								"run":         schema.StringAttribute{Computed: true, Description: "Submit a job asynchronously."},
								"run_sync":    schema.StringAttribute{Computed: true, Description: "Submit a job and wait for its result."},
								"status":      schema.StringAttribute{Computed: true, Description: "State, timings, and result of a submitted job. Takes a job id in place of the {job_id} segment."},
								"stream":      schema.StringAttribute{Computed: true, Description: "Read incremental results from a job that produces output progressively. Takes a job id in place of the {job_id} segment."},
							},
						},
						"created_at": schema.StringAttribute{Computed: true, Description: "Creation timestamp in RFC 3339 format."},
					},
				},
			},
		},
	}
}

func (d *serverlessEndpointsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *serverlessEndpointsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	allEndpoints, err := listAllPages(ctx, func(ctx context.Context, cursor *string) ([]sdktypes.Endpoint, sdktypes.Pagination, error) {
		apiResp, err := d.client.Serverless.ListEndpointsWithResponse(ctx, &serverless.ListEndpointsParams{Cursor: cursor})
		if err != nil {
			return nil, sdktypes.Pagination{}, err
		}
		if apiResp.JSON200 == nil {
			return nil, sdktypes.Pagination{}, fmt.Errorf("API returned status %d: %s",
				apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body))
		}
		return apiResp.JSON200.Endpoints, apiResp.JSON200.Pagination, nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Listing serverless endpoints failed", err.Error())
		return
	}

	state := serverlessEndpointsDataSourceModel{Endpoints: []endpointListItemModel{}}
	for _, endpoint := range allEndpoints {
		item, diags := flattenEndpointListItem(ctx, endpoint)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		state.Endpoints = append(state.Endpoints, item)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// flattenEndpointListItem maps one listing entry. The invocation URLs go
// through the same helper the runpod_serverless_endpoint resource uses, so a
// listing entry and a managed endpoint report them identically; which of the
// two shapes the API returned is keyed off the endpoint's routing type.
func flattenEndpointListItem(ctx context.Context, endpoint sdktypes.Endpoint) (endpointListItemModel, diag.Diagnostics) {
	item := endpointListItemModel{
		Id:         types.StringValue(endpoint.Id),
		Name:       types.StringValue(endpoint.Name),
		Image:      types.StringPointerValue(endpoint.Image),
		WorkersMin: types.Int64Value(int64(endpoint.Workers.Min)),
		WorkersMax: types.Int64Value(int64(endpoint.Workers.Max)),
		CreatedAt:  types.StringValue(endpoint.CreatedAt.Format(time.RFC3339)),
	}
	if endpoint.Type != nil {
		item.EndpointType = types.StringValue(string(*endpoint.Type))
	}

	requestUrls, diags := flattenEndpointRequestUrls(ctx, endpoint.RequestUrls, item.EndpointType.ValueString())
	item.RequestUrls = requestUrls
	return item, diags
}

var _ datasource.DataSourceWithConfigure = (*registriesDataSource)(nil)

func NewRegistriesDataSource() datasource.DataSource { return &registriesDataSource{} }

// registryUsernameDeprecation marks the username attribute the 1.x
// runpod_container_registry_auths data source reported. No supported API
// returns a credential's username: REST v2 treats it as write-only, and the
// GraphQL ContainerRegistryAuth type has no username field. The attribute
// stays so a 1.x configuration that references it still plans, and always
// reads null.
const registryUsernameDeprecation = "No API reports a registry username; this attribute is always null and will be removed"

type registriesDataSource struct {
	client *runpod.Client
}

type registryListItemModel struct {
	Id       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Username types.String `tfsdk:"username"`
}

type registriesDataSourceModel struct {
	Registries []registryListItemModel `tfsdk:"registries"`
}

func (d *registriesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_registries"
}

func (d *registriesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "All container registry credentials owned by the authenticated account.",
		Attributes: map[string]schema.Attribute{
			"registries": schema.ListNestedAttribute{
				Computed:    true,
				Description: "One entry per registry credential.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   schema.StringAttribute{Computed: true, Description: "Credential identifier; reference it from a pod or endpoint registry attribute."},
						"name": schema.StringAttribute{Computed: true, Description: "Credential name."},
						"username": schema.StringAttribute{
							Computed: true,
							Description: "Deprecated. Always null: REST v2 treats a credential's username as write-only " +
								"and no other supported API reports it.",
							DeprecationMessage: registryUsernameDeprecation,
						},
					},
				},
			},
		},
	}
}

func (d *registriesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *registriesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	apiResp, err := d.client.Registries.ListRegistriesWithResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Listing registries failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Listing registries failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)))
		return
	}

	state := registriesDataSourceModel{Registries: []registryListItemModel{}}
	for _, registry := range apiResp.JSON200.Registries {
		state.Registries = append(state.Registries, registryListItemModel{
			Id:       types.StringValue(registry.Id),
			Name:     types.StringValue(registry.Name),
			Username: types.StringNull(),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
