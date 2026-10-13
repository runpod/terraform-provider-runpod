package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/lib/serverless"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// Serverless endpoint detail: runpod_endpoint_workers, runpod_endpoint_builds,
// runpod_endpoint_build, runpod_endpoint_releases. All four are per-endpoint
// operational views; the endpoint's own configuration lives on the
// runpod_serverless_endpoint resource and the runpod_serverless_endpoints
// listing.

// configuredEndpointId reads the lookup key a per-endpoint listing takes, and
// only that key. Decoding the whole state model out of configuration does not
// work for these: everything besides endpoint_id is computed, so Terraform
// sends it null, and a computed nested object such as the worker summary or
// the rollout has no null to put in a plain struct field. Asking for the one
// attribute a practitioner writes avoids the question.
func configuredEndpointId(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) (string, bool) {
	var endpointId types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("endpoint_id"), &endpointId)...)
	if resp.Diagnostics.HasError() {
		return "", false
	}
	return endpointId.ValueString(), true
}

var (
	_ datasource.DataSource              = (*endpointWorkersDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*endpointWorkersDataSource)(nil)
)

func NewEndpointWorkersDataSource() datasource.DataSource { return &endpointWorkersDataSource{} }

type endpointWorkersDataSource struct {
	client *runpod.Client
}

type endpointWorkerModel struct {
	Id            types.String `tfsdk:"id"`
	Status        types.String `tfsdk:"status"`
	Image         types.String `tfsdk:"image"`
	GpuTypeId     types.String `tfsdk:"gpu_type_id"`
	GpuCount      types.Int64  `tfsdk:"gpu_count"`
	DataCenterId  types.String `tfsdk:"data_center_id"`
	IsStale       types.Bool   `tfsdk:"is_stale"`
	StartedAt     types.String `tfsdk:"started_at"`
	UptimeSeconds types.Int64  `tfsdk:"uptime_seconds"`
	Version       types.Int64  `tfsdk:"version"`
}

type endpointWorkerSummaryModel struct {
	Total        types.Int64 `tfsdk:"total"`
	Idle         types.Int64 `tfsdk:"idle"`
	Initializing types.Int64 `tfsdk:"initializing"`
	Running      types.Int64 `tfsdk:"running"`
	Throttled    types.Int64 `tfsdk:"throttled"`
	Unhealthy    types.Int64 `tfsdk:"unhealthy"`
}

type endpointWorkersDataSourceModel struct {
	EndpointId      types.String               `tfsdk:"endpoint_id"`
	EndpointVersion types.Int64                `tfsdk:"endpoint_version"`
	Summary         endpointWorkerSummaryModel `tfsdk:"summary"`
	Workers         []endpointWorkerModel      `tfsdk:"workers"`
}

func (d *endpointWorkersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint_workers"
}

func (d *endpointWorkersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Workers currently allocated to a Serverless endpoint, with the per-status counts the console shows. Worker allocation changes with load, so every plan shows this data source as needing a refresh.",
		Attributes: map[string]schema.Attribute{
			"endpoint_id":      schema.StringAttribute{Required: true, Description: "Serverless endpoint identifier."},
			"endpoint_version": schema.Int64Attribute{Computed: true, Description: "The endpoint's current configuration version; compare against a worker's version to spot workers on an older release."},
			"summary": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "Worker counts by status.",
				Attributes: map[string]schema.Attribute{
					"total":        schema.Int64Attribute{Computed: true, Description: "All workers allocated to the endpoint."},
					"idle":         schema.Int64Attribute{Computed: true, Description: "Workers ready but not handling a request."},
					"initializing": schema.Int64Attribute{Computed: true, Description: "Workers still starting up."},
					"running":      schema.Int64Attribute{Computed: true, Description: "Workers handling a request."},
					"throttled":    schema.Int64Attribute{Computed: true, Description: "Workers held back by capacity limits."},
					"unhealthy":    schema.Int64Attribute{Computed: true, Description: "Workers the platform considers unhealthy."},
				},
			},
			"workers": schema.ListNestedAttribute{
				Computed:    true,
				Description: "One entry per worker.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":             schema.StringAttribute{Computed: true, Description: "Worker identifier; pass it to runpod_endpoint_worker_logs."},
						"status":         schema.StringAttribute{Computed: true, Description: "Worker status: IDLE, INITIALIZING, RUNNING, THROTTLED, or UNHEALTHY."},
						"image":          schema.StringAttribute{Computed: true, Description: "Docker image the worker is running."},
						"gpu_type_id":    schema.StringAttribute{Computed: true, Description: "GPU type the worker was placed on."},
						"gpu_count":      schema.Int64Attribute{Computed: true, Description: "Number of GPUs attached to the worker."},
						"data_center_id": schema.StringAttribute{Computed: true, Description: "Data center the worker runs in."},
						"is_stale":       schema.BoolAttribute{Computed: true, Description: "Whether the worker runs an older configuration version than the endpoint's current one."},
						"started_at":     schema.StringAttribute{Computed: true, Description: "Worker start timestamp in RFC 3339 format; null while it is still initializing."},
						"uptime_seconds": schema.Int64Attribute{Computed: true, Description: "Seconds since the worker started."},
						"version":        schema.Int64Attribute{Computed: true, Description: "Endpoint configuration version the worker is running."},
					},
				},
			},
		},
	}
}

func (d *endpointWorkersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *endpointWorkersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	endpointId, ok := configuredEndpointId(ctx, req, resp)
	if !ok {
		return
	}

	apiResp, err := d.client.Serverless.ListEndpointWorkersWithResponse(ctx, endpointId)
	if err != nil {
		resp.Diagnostics.AddError("Listing endpoint workers failed", err.Error())
		return
	}
	if apiResp.HTTPResponse.StatusCode == http.StatusNotFound || apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Listing endpoint workers failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)))
		return
	}

	state := flattenEndpointWorkers(endpointId, *apiResp.JSON200)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenEndpointWorkers(endpointId string, response sdktypes.ListEndpointWorkersResponse) endpointWorkersDataSourceModel {
	state := endpointWorkersDataSourceModel{
		EndpointId:      types.StringValue(endpointId),
		EndpointVersion: types.Int64Null(),
		Summary: endpointWorkerSummaryModel{
			Total:        types.Int64Value(int64(response.Summary.Total)),
			Idle:         types.Int64Value(int64(response.Summary.Idle)),
			Initializing: types.Int64Value(int64(response.Summary.Initializing)),
			Running:      types.Int64Value(int64(response.Summary.Running)),
			Throttled:    types.Int64Value(int64(response.Summary.Throttled)),
			Unhealthy:    types.Int64Value(int64(response.Summary.Unhealthy)),
		},
		Workers: []endpointWorkerModel{},
	}
	if response.EndpointVersion != nil {
		state.EndpointVersion = types.Int64Value(int64(*response.EndpointVersion))
	}

	for _, worker := range response.Workers {
		item := endpointWorkerModel{
			Id:            types.StringValue(worker.Id),
			Status:        types.StringValue(string(worker.Status)),
			Image:         types.StringPointerValue(worker.Image),
			GpuTypeId:     types.StringPointerValue(worker.GpuTypeId),
			GpuCount:      types.Int64Value(int64(worker.GpuCount)),
			DataCenterId:  types.StringPointerValue(worker.DataCenterId),
			IsStale:       types.BoolValue(worker.IsStale),
			StartedAt:     types.StringNull(),
			UptimeSeconds: types.Int64Null(),
			Version:       types.Int64Null(),
		}
		if worker.StartedAt != nil {
			item.StartedAt = types.StringValue(worker.StartedAt.Format(time.RFC3339))
		}
		if worker.UptimeSeconds != nil {
			item.UptimeSeconds = types.Int64Value(int64(*worker.UptimeSeconds))
		}
		if worker.Version != nil {
			item.Version = types.Int64Value(int64(*worker.Version))
		}
		state.Workers = append(state.Workers, item)
	}
	return state
}

var (
	_ datasource.DataSource              = (*endpointBuildsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*endpointBuildsDataSource)(nil)
)

func NewEndpointBuildsDataSource() datasource.DataSource { return &endpointBuildsDataSource{} }

type endpointBuildsDataSource struct {
	client *runpod.Client
}

type endpointBuildModel struct {
	Id            types.String `tfsdk:"id"`
	Status        types.String `tfsdk:"status"`
	ImageName     types.String `tfsdk:"image_name"`
	Branch        types.String `tfsdk:"branch"`
	CommitHash    types.String `tfsdk:"commit_hash"`
	CommitMessage types.String `tfsdk:"commit_message"`
	CommitDate    types.String `tfsdk:"commit_date"`
	StartedAt     types.String `tfsdk:"started_at"`
	CompletedAt   types.String `tfsdk:"completed_at"`
	Error         types.String `tfsdk:"error"`
}

type endpointBuildsDataSourceModel struct {
	EndpointId types.String         `tfsdk:"endpoint_id"`
	Builds     []endpointBuildModel `tfsdk:"builds"`
}

// endpointBuildAttributes describes a build; the listing nests it and the
// singular data source uses it at the top level alongside its lookup keys.
func endpointBuildAttributes(idRequired bool) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Required:    idRequired,
			Computed:    !idRequired,
			Description: "Build identifier.",
		},
		"status":         schema.StringAttribute{Computed: true, Description: "Build state: PENDING, BUILDING, UPLOADING, TESTING, COMPLETED, FAILED, TEST_FAILED, or CANCELLED."},
		"image_name":     schema.StringAttribute{Computed: true, Description: "Image the build produced; null until it completes."},
		"branch":         schema.StringAttribute{Computed: true, Description: "Git branch the build came from."},
		"commit_hash":    schema.StringAttribute{Computed: true, Description: "Git commit the build came from."},
		"commit_message": schema.StringAttribute{Computed: true, Description: "Commit message of the built commit."},
		"commit_date":    schema.StringAttribute{Computed: true, Description: "Commit timestamp in RFC 3339 format."},
		"started_at":     schema.StringAttribute{Computed: true, Description: "Build start timestamp in RFC 3339 format."},
		"completed_at":   schema.StringAttribute{Computed: true, Description: "Build finish timestamp in RFC 3339 format; null while it is still running."},
		"error":          schema.StringAttribute{Computed: true, Description: "Failure detail; null unless the build failed."},
	}
}

func (d *endpointBuildsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint_builds"
}

func (d *endpointBuildsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Container builds of a Serverless endpoint built from a Git repository. Empty for an endpoint that deploys a prebuilt image.",
		Attributes: map[string]schema.Attribute{
			"endpoint_id": schema.StringAttribute{Required: true, Description: "Serverless endpoint identifier."},
			"builds": schema.ListNestedAttribute{
				Computed:     true,
				Description:  "One entry per build.",
				NestedObject: schema.NestedAttributeObject{Attributes: endpointBuildAttributes(false)},
			},
		},
	}
}

func (d *endpointBuildsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *endpointBuildsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	endpointId, ok := configuredEndpointId(ctx, req, resp)
	if !ok {
		return
	}

	allBuilds, err := listAllPages(ctx, func(ctx context.Context, cursor *string) ([]sdktypes.Build, sdktypes.Pagination, error) {
		apiResp, err := d.client.Serverless.ListEndpointBuildsWithResponse(ctx, endpointId, &serverless.ListEndpointBuildsParams{Cursor: cursor})
		if err != nil {
			return nil, sdktypes.Pagination{}, err
		}
		if apiResp.HTTPResponse.StatusCode == http.StatusNotFound || apiResp.JSON200 == nil {
			return nil, sdktypes.Pagination{}, fmt.Errorf("API returned status %d: %s",
				apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body))
		}
		return apiResp.JSON200.Builds, apiResp.JSON200.Pagination, nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Listing endpoint builds failed", err.Error())
		return
	}

	state := endpointBuildsDataSourceModel{
		EndpointId: types.StringValue(endpointId),
		Builds:     []endpointBuildModel{},
	}
	for _, build := range allBuilds {
		state.Builds = append(state.Builds, flattenEndpointBuild(build))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenEndpointBuild(build sdktypes.Build) endpointBuildModel {
	model := endpointBuildModel{
		Id:            types.StringValue(build.Id),
		Status:        types.StringValue(string(build.Status)),
		ImageName:     types.StringPointerValue(build.ImageName),
		Branch:        types.StringPointerValue(build.Branch),
		CommitHash:    types.StringPointerValue(build.CommitHash),
		CommitMessage: types.StringPointerValue(build.CommitMessage),
		CommitDate:    types.StringNull(),
		StartedAt:     types.StringNull(),
		CompletedAt:   types.StringNull(),
		Error:         types.StringPointerValue(build.Error),
	}
	if build.CommitDate != nil {
		model.CommitDate = types.StringValue(build.CommitDate.Format(time.RFC3339))
	}
	if build.StartedAt != nil {
		model.StartedAt = types.StringValue(build.StartedAt.Format(time.RFC3339))
	}
	if build.CompletedAt != nil {
		model.CompletedAt = types.StringValue(build.CompletedAt.Format(time.RFC3339))
	}
	return model
}

var (
	_ datasource.DataSource              = (*endpointBuildDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*endpointBuildDataSource)(nil)
)

func NewEndpointBuildDataSource() datasource.DataSource { return &endpointBuildDataSource{} }

type endpointBuildDataSource struct {
	client *runpod.Client
}

// endpointBuildDetailModel is a build as the singular data source reports it:
// the listed build fields plus the endpoint it belongs to, which is an input
// here rather than a field of the API object.
type endpointBuildDetailModel struct {
	EndpointId types.String `tfsdk:"endpoint_id"`
	endpointBuildModel
}

func (d *endpointBuildDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint_build"
}

func (d *endpointBuildDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := endpointBuildAttributes(true)
	attributes["endpoint_id"] = schema.StringAttribute{Required: true, Description: "Serverless endpoint identifier the build belongs to."}
	resp.Schema = schema.Schema{
		Description: "A single container build of a Serverless endpoint, looked up by endpoint and build identifier.",
		Attributes:  attributes,
	}
}

func (d *endpointBuildDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *endpointBuildDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config endpointBuildDetailModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpointId := config.EndpointId.ValueString()
	apiResp, err := d.client.Serverless.GetEndpointBuildWithResponse(ctx, endpointId, config.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading endpoint build failed", err.Error())
		return
	}
	if apiResp.HTTPResponse.StatusCode == http.StatusNotFound || apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Reading endpoint build failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)))
		return
	}

	state := flattenEndpointBuildDetail(endpointId, *apiResp.JSON200)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenEndpointBuildDetail(endpointId string, build sdktypes.Build) endpointBuildDetailModel {
	return endpointBuildDetailModel{
		EndpointId:         types.StringValue(endpointId),
		endpointBuildModel: flattenEndpointBuild(build),
	}
}

var (
	_ datasource.DataSource              = (*endpointReleasesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*endpointReleasesDataSource)(nil)
)

func NewEndpointReleasesDataSource() datasource.DataSource { return &endpointReleasesDataSource{} }

type endpointReleasesDataSource struct {
	client *runpod.Client
}

type endpointReleaseDiffModel struct {
	Field types.String `tfsdk:"field"`
	Old   types.String `tfsdk:"old"`
	New   types.String `tfsdk:"new"`
}

type endpointReleaseModel struct {
	Id              types.String               `tfsdk:"id"`
	Version         types.Int64                `tfsdk:"version"`
	Source          types.String               `tfsdk:"source"`
	BuildId         types.String               `tfsdk:"build_id"`
	CreatedAt       types.String               `tfsdk:"created_at"`
	CreatedByUserId types.String               `tfsdk:"created_by_user_id"`
	WorkerCount     types.Int64                `tfsdk:"worker_count"`
	Diff            []endpointReleaseDiffModel `tfsdk:"diff"`
}

type endpointRolloutModel struct {
	InProgress      types.Bool  `tfsdk:"in_progress"`
	PercentOnLatest types.Int64 `tfsdk:"percent_on_latest"`
	WorkersOnLatest types.Int64 `tfsdk:"workers_on_latest"`
	WorkersTotal    types.Int64 `tfsdk:"workers_total"`
}

type endpointReleasesDataSourceModel struct {
	EndpointId      types.String           `tfsdk:"endpoint_id"`
	EndpointVersion types.Int64            `tfsdk:"endpoint_version"`
	Rollout         endpointRolloutModel   `tfsdk:"rollout"`
	Releases        []endpointReleaseModel `tfsdk:"releases"`
}

func (d *endpointReleasesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint_releases"
}

func (d *endpointReleasesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Configuration releases of a Serverless endpoint and the state of the current rollout. Each release records what changed from the one before it.",
		Attributes: map[string]schema.Attribute{
			"endpoint_id":      schema.StringAttribute{Required: true, Description: "Serverless endpoint identifier."},
			"endpoint_version": schema.Int64Attribute{Computed: true, Description: "The endpoint's current configuration version."},
			"rollout": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "How far the current version has reached the endpoint's workers.",
				Attributes: map[string]schema.Attribute{
					"in_progress":       schema.BoolAttribute{Computed: true, Description: "Whether any worker still runs an older version."},
					"percent_on_latest": schema.Int64Attribute{Computed: true, Description: "Percentage of workers on the current version; 0 when the endpoint has no workers."},
					"workers_on_latest": schema.Int64Attribute{Computed: true, Description: "Workers running the current version."},
					"workers_total":     schema.Int64Attribute{Computed: true, Description: "All workers allocated to the endpoint."},
				},
			},
			"releases": schema.ListNestedAttribute{
				Computed:    true,
				Description: "One entry per release.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                 schema.StringAttribute{Computed: true, Description: "Release identifier."},
						"version":            schema.Int64Attribute{Computed: true, Description: "Configuration version this release produced."},
						"source":             schema.StringAttribute{Computed: true, Description: "What created the release: GIT_BUILD or MANUAL."},
						"build_id":           schema.StringAttribute{Computed: true, Description: "Build the release deployed; null for a MANUAL release."},
						"created_at":         schema.StringAttribute{Computed: true, Description: "Release timestamp in RFC 3339 format."},
						"created_by_user_id": schema.StringAttribute{Computed: true, Description: "Account user who created the release."},
						"worker_count":       schema.Int64Attribute{Computed: true, Description: "Workers currently running this release."},
						"diff": schema.ListNestedAttribute{
							Computed:    true,
							Description: "Configuration fields this release changed.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"field": schema.StringAttribute{Computed: true, Description: "The changed field, top-level (e.g. gpuCount) or template-scoped (e.g. template.imageName)."},
									"old":   schema.StringAttribute{Computed: true, Description: "Previous value as JSON text; null when the field was added."},
									"new":   schema.StringAttribute{Computed: true, Description: "New value as JSON text; null when the field was removed."},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (d *endpointReleasesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *endpointReleasesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	endpointId, ok := configuredEndpointId(ctx, req, resp)
	if !ok {
		return
	}

	// The rollout summary and the endpoint version repeat on every page and
	// describe the endpoint, not the page; the last page read carries the
	// freshest view of a rollout that is still moving.
	var listing sdktypes.ListEndpointReleasesResponse
	allReleases, err := listAllPages(ctx, func(ctx context.Context, cursor *string) ([]sdktypes.Release, sdktypes.Pagination, error) {
		apiResp, err := d.client.Serverless.ListEndpointReleasesWithResponse(ctx, endpointId, &serverless.ListEndpointReleasesParams{Cursor: cursor})
		if err != nil {
			return nil, sdktypes.Pagination{}, err
		}
		if apiResp.HTTPResponse.StatusCode == http.StatusNotFound || apiResp.JSON200 == nil {
			return nil, sdktypes.Pagination{}, fmt.Errorf("API returned status %d: %s",
				apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body))
		}
		listing = *apiResp.JSON200
		return apiResp.JSON200.Releases, apiResp.JSON200.Pagination, nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Listing endpoint releases failed", err.Error())
		return
	}
	listing.Releases = allReleases

	state := flattenEndpointReleases(endpointId, listing)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenEndpointReleases(endpointId string, response sdktypes.ListEndpointReleasesResponse) endpointReleasesDataSourceModel {
	state := endpointReleasesDataSourceModel{
		EndpointId:      types.StringValue(endpointId),
		EndpointVersion: types.Int64Null(),
		Rollout: endpointRolloutModel{
			InProgress:      types.BoolValue(response.Rollout.InProgress),
			PercentOnLatest: types.Int64Value(int64(response.Rollout.PercentOnLatest)),
			WorkersOnLatest: types.Int64Value(int64(response.Rollout.WorkersOnLatest)),
			WorkersTotal:    types.Int64Value(int64(response.Rollout.WorkersTotal)),
		},
		Releases: []endpointReleaseModel{},
	}
	if response.EndpointVersion != nil {
		state.EndpointVersion = types.Int64Value(int64(*response.EndpointVersion))
	}

	for _, release := range response.Releases {
		item := endpointReleaseModel{
			Id:              types.StringValue(release.Id),
			Version:         types.Int64Null(),
			Source:          types.StringValue(string(release.Source)),
			BuildId:         types.StringPointerValue(release.BuildId),
			CreatedAt:       types.StringValue(release.CreatedAt.Format(time.RFC3339)),
			CreatedByUserId: types.StringPointerValue(release.CreatedByUserId),
			WorkerCount:     types.Int64Value(int64(release.WorkerCount)),
			Diff:            []endpointReleaseDiffModel{},
		}
		if release.Version != nil {
			item.Version = types.Int64Value(int64(*release.Version))
		}
		for _, entry := range release.Diff {
			item.Diff = append(item.Diff, endpointReleaseDiffModel{
				Field: types.StringValue(entry.Field),
				Old:   releaseDiffValue(entry.Old),
				New:   releaseDiffValue(entry.New),
			})
		}
		state.Releases = append(state.Releases, item)
	}
	return state
}

// releaseDiffValue renders a diff side as JSON text: the API types these as
// arbitrary JSON (a string, number, object, or array depending on the field),
// which Terraform cannot hold in one typed attribute.
func releaseDiffValue(value any) types.String {
	if value == nil {
		return types.StringNull()
	}
	if text, ok := value.(string); ok {
		return types.StringValue(text)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return types.StringValue(fmt.Sprintf("%v", value))
	}
	return types.StringValue(string(encoded))
}
