package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// runpod_template: one template by id. The attributes the runpod_template
// resource also carries keep the resource's names; the read-only fields the
// resource does not manage (registry, start_jupyter, start_ssh,
// allowed_cuda_versions, mounts) are added here. The same attribute set backs
// the entries of runpod_catalog_templates.
//
// The 1.x data source also served docker_entrypoint, read here from the v2
// template's entrypoint, and four attributes that have no REST v2 field
// (readme, earned, is_runpod and runtime_in_min), which are read from the
// GraphQL template after the v2 read and marked deprecated. None of the five
// is part of the shared attribute set: runpod_catalog_templates keeps the set
// it has, and lists public catalog entries from REST v2 alone with no GraphQL
// read.

var (
	_ datasource.DataSource              = (*templateDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*templateDataSource)(nil)
)

func NewTemplateDataSource() datasource.DataSource { return &templateDataSource{} }

type templateDataSource struct {
	client *runpod.Client
	legacy *legacy.Client
}

type templatePersistentMountModel struct {
	Path types.String `tfsdk:"path"`
	Size types.Int64  `tfsdk:"size"`
}

type templateMountsModel struct {
	Persistent *templatePersistentMountModel `tfsdk:"persistent"`
}

type templateDetailModel struct {
	Id                  types.String         `tfsdk:"id"`
	Name                types.String         `tfsdk:"name"`
	Image               types.String         `tfsdk:"image"`
	Category            types.String         `tfsdk:"category"`
	Disk                types.Int64          `tfsdk:"disk"`
	Args                types.String         `tfsdk:"args"`
	Env                 types.Map            `tfsdk:"env"`
	Ports               types.List           `tfsdk:"ports"`
	Public              types.Bool           `tfsdk:"public"`
	Serverless          types.Bool           `tfsdk:"serverless"`
	Registry            types.String         `tfsdk:"registry"`
	StartJupyter        types.Bool           `tfsdk:"start_jupyter"`
	StartSsh            types.Bool           `tfsdk:"start_ssh"`
	AllowedCudaVersions types.List           `tfsdk:"allowed_cuda_versions"`
	Mounts              *templateMountsModel `tfsdk:"mounts"`
}

// templateDataSourceModel is templateDetailModel plus docker_entrypoint and
// the four deprecated GraphQL read-backs. The two models are separate because
// the shared one also types the entries of runpod_catalog_templates, whose
// schema declares none of them.
type templateDataSourceModel struct {
	Id                  types.String         `tfsdk:"id"`
	Name                types.String         `tfsdk:"name"`
	Image               types.String         `tfsdk:"image"`
	Category            types.String         `tfsdk:"category"`
	Disk                types.Int64          `tfsdk:"disk"`
	Args                types.String         `tfsdk:"args"`
	Env                 types.Map            `tfsdk:"env"`
	Ports               types.List           `tfsdk:"ports"`
	Public              types.Bool           `tfsdk:"public"`
	Serverless          types.Bool           `tfsdk:"serverless"`
	Registry            types.String         `tfsdk:"registry"`
	StartJupyter        types.Bool           `tfsdk:"start_jupyter"`
	StartSsh            types.Bool           `tfsdk:"start_ssh"`
	AllowedCudaVersions types.List           `tfsdk:"allowed_cuda_versions"`
	Mounts              *templateMountsModel `tfsdk:"mounts"`
	DockerEntrypoint    types.List           `tfsdk:"docker_entrypoint"`
	Readme              types.String         `tfsdk:"readme"`
	Earned              types.Float64        `tfsdk:"earned"`
	IsRunpod            types.Bool           `tfsdk:"is_runpod"`
	RuntimeInMin        types.Int64          `tfsdk:"runtime_in_min"`
}

func (d *templateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_template"
}

// templateDetailAttributes is the read-only attribute set of a template, used
// both as the runpod_template schema and as the nested object of
// runpod_catalog_templates. The id carries `required` only in the singular
// data source, which is where the lookup key comes from.
func templateDetailAttributes(idRequired bool) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Required:    idRequired,
			Computed:    !idRequired,
			Description: "Template identifier, e.g. \"tpl_abc\".",
		},
		"name":       schema.StringAttribute{Computed: true, Description: "Template name."},
		"image":      schema.StringAttribute{Computed: true, Description: "Docker image reference."},
		"category":   schema.StringAttribute{Computed: true, Description: "Console grouping: CPU, NVIDIA, or AMD. It does not affect scheduling or billing."},
		"disk":       schema.Int64Attribute{Computed: true, Description: "Container disk in GB."},
		"args":       schema.StringAttribute{Computed: true, Description: "Arguments passed to the container entrypoint."},
		"public":     schema.BoolAttribute{Computed: true, Description: "Whether the template is visible to other Runpod users."},
		"serverless": schema.BoolAttribute{Computed: true, Description: "Whether the template configures Serverless workers (true) or pods (false)."},
		"registry":   schema.StringAttribute{Computed: true, Description: "Registry credential ID used to pull the image; null for public images."},
		"env": schema.MapAttribute{
			ElementType: types.StringType,
			Computed:    true,
			Description: "Environment variables set in containers created from the template.",
		},
		"ports": schema.ListAttribute{
			ElementType: types.StringType,
			Computed:    true,
			Description: "Exposed ports, formatted as port/protocol.",
		},
		"start_jupyter": schema.BoolAttribute{Computed: true, Description: "Whether containers created from the template start JupyterLab at startup."},
		"start_ssh":     schema.BoolAttribute{Computed: true, Description: "Whether containers created from the template get SSH access provisioned at startup."},
		"allowed_cuda_versions": schema.ListAttribute{
			ElementType: types.StringType,
			Computed:    true,
			Description: "Acceptable CUDA versions as major.minor; empty means any version.",
		},
		"mounts": schema.SingleNestedAttribute{
			Computed:    true,
			Description: "Storage mounted into containers created from the template. Templates support a single persistent mount.",
			Attributes: map[string]schema.Attribute{
				"persistent": schema.SingleNestedAttribute{
					Computed:    true,
					Description: "Host-local persistent storage; null when the template declares no mount.",
					Attributes: map[string]schema.Attribute{
						"path": schema.StringAttribute{Computed: true, Description: "Mount path inside the container."},
						"size": schema.Int64Attribute{Computed: true, Description: "Allocated storage in GB."},
					},
				},
			},
		},
	}
}

// templateLegacyAttributes are four of the read-backs the 1.x data source got
// from GraphQL, which is still where they come from.
func templateLegacyAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"readme": schema.StringAttribute{
			Computed:           true,
			Description:        "Deprecated. Template README in Markdown, read from the GraphQL template. " + legacyShimDescriptionSuffix,
			DeprecationMessage: legacyShimDeprecation,
		},
		"earned": schema.Float64Attribute{
			Computed:           true,
			Description:        "Deprecated. Earnings the template has generated in USD, read from the GraphQL template; 0 for a template the account does not own. " + legacyShimDescriptionSuffix,
			DeprecationMessage: legacyShimDeprecation,
		},
		"is_runpod": schema.BoolAttribute{
			Computed:           true,
			Description:        "Deprecated. Whether Runpod publishes the template, read from the GraphQL template. " + legacyShimDescriptionSuffix,
			DeprecationMessage: legacyShimDeprecation,
		},
		"runtime_in_min": schema.Int64Attribute{
			Computed:           true,
			Description:        "Deprecated. Minutes containers created from the template have run, read from the GraphQL template; 0 for a template the account does not own. " + legacyShimDescriptionSuffix,
			DeprecationMessage: legacyShimDeprecation,
		},
	}
}

func (d *templateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A single template looked up by identifier. Covers the account's own templates and any public template the account can see.",
		Attributes: withAttributes(templateDetailAttributes(true), withAttributes(templateLegacyAttributes(), map[string]schema.Attribute{
			"docker_entrypoint": schema.ListAttribute{
				ElementType: types.StringType,
				Computed:    true,
				Description: "Container entrypoint for containers created from the template, in exec form, e.g. [\"/bin/bash\", \"-c\"], as the API decodes it from args.",
			},
		})),
	}
}

func (d *templateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
	d.legacy = configuredLegacyClient(req.ProviderData, &resp.Diagnostics)
}

func (d *templateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config templateDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := d.client.Templates.GetTemplateWithResponse(ctx, config.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading template failed", err.Error())
		return
	}
	if apiResp.HTTPResponse.StatusCode == http.StatusNotFound || apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Reading template failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	detail, diags := flattenTemplateDetail(ctx, *apiResp.JSON200)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state := templateDataSourceState(detail)
	// An omitted entrypoint is null rather than an empty command.
	entrypoint, entrypointDiags := types.ListValueFrom(ctx, types.StringType, apiResp.JSON200.Entrypoint)
	resp.Diagnostics.Append(entrypointDiags...)
	state.DockerEntrypoint = entrypoint
	resp.Diagnostics.Append(d.readLegacyTemplateFields(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// readLegacyTemplateFields fills the four deprecated read-backs from the
// GraphQL template. Nothing here is configurable, so a failure only warns and
// the four attributes come back null; a template the GraphQL API does not
// report to the caller leaves them null silently.
func (d *templateDataSource) readLegacyTemplateFields(ctx context.Context, state *templateDataSourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if d.legacy == nil {
		clearLegacyTemplateDetailFields(state)
		return diags
	}

	template, err := fetchLegacyTemplateFields(ctx, d.legacy, state.Id.ValueString())
	if err != nil {
		clearLegacyTemplateDetailFields(state)
		appendLegacyReadFailure(&diags, "template", err)
		return diags
	}
	if template == nil {
		clearLegacyTemplateDetailFields(state)
		return diags
	}

	// state.Readme starts null (templateDataSourceState), so the answer always
	// lands as it is.
	flattenLegacyTemplateInto(template, &state.Readme, &state.Earned, &state.IsRunpod, &state.RuntimeInMin)
	return diags
}

// clearLegacyTemplateDetailFields nulls all four, where the resource's
// clearLegacyTemplateFields keeps readme at what the configuration set. A data
// source has no configuration to keep and rebuilds its state from the v2 read
// on every call (templateDataSourceState starts here), so every one of the
// four is a read-back and a failed read means null.
func clearLegacyTemplateDetailFields(state *templateDataSourceModel) {
	state.Readme = types.StringNull()
	state.Earned = types.Float64Null()
	state.IsRunpod = types.BoolNull()
	state.RuntimeInMin = types.Int64Null()
}

// templateDataSourceState widens the shared template model into the one the
// data source writes; docker_entrypoint and the four shim attributes start
// null, filled by Read from the v2 template and the GraphQL read respectively.
func templateDataSourceState(detail templateDetailModel) templateDataSourceModel {
	state := templateDataSourceModel{
		Id:                  detail.Id,
		Name:                detail.Name,
		Image:               detail.Image,
		Category:            detail.Category,
		Disk:                detail.Disk,
		Args:                detail.Args,
		Env:                 detail.Env,
		Ports:               detail.Ports,
		Public:              detail.Public,
		Serverless:          detail.Serverless,
		Registry:            detail.Registry,
		StartJupyter:        detail.StartJupyter,
		StartSsh:            detail.StartSsh,
		AllowedCudaVersions: detail.AllowedCudaVersions,
		Mounts:              detail.Mounts,
		DockerEntrypoint:    types.ListNull(types.StringType),
	}
	clearLegacyTemplateDetailFields(&state)
	return state
}

func flattenTemplateDetail(ctx context.Context, template sdktypes.Template) (templateDetailModel, diag.Diagnostics) {
	model := templateDetailModel{
		Id:           types.StringValue(template.Id),
		Name:         types.StringValue(template.Name),
		Image:        types.StringValue(template.Image),
		Category:     types.StringValue(string(template.Category)),
		Disk:         types.Int64Value(int64(template.Disk)),
		Args:         types.StringValue(template.Args),
		Public:       types.BoolValue(template.Public),
		Serverless:   types.BoolValue(template.Serverless),
		Registry:     types.StringPointerValue(template.Registry),
		StartJupyter: types.BoolValue(template.StartJupyter),
		StartSsh:     types.BoolValue(template.StartSsh),
		Mounts:       &templateMountsModel{},
	}
	if template.Mounts.Persistent != nil {
		model.Mounts.Persistent = &templatePersistentMountModel{
			Path: types.StringValue(template.Mounts.Persistent.Path),
			Size: types.Int64Value(int64(template.Mounts.Persistent.Size)),
		}
	}

	diags := templateCollections(ctx, template.Env, template.Ports, template.AllowedCudaVersions,
		&model.Env, &model.Ports, &model.AllowedCudaVersions)
	return model, diags
}

// templateCollections converts the three collection fields shared by the
// account-template and catalog-template shapes.
func templateCollections(
	ctx context.Context,
	env map[string]string,
	ports, cudaVersions []string,
	envOut *types.Map,
	portsOut, cudaOut *types.List,
) diag.Diagnostics {
	var diags diag.Diagnostics

	envValue, envDiags := types.MapValueFrom(ctx, types.StringType, env)
	diags.Append(envDiags...)
	*envOut = envValue

	portsValue, portDiags := types.ListValueFrom(ctx, types.StringType, ports)
	diags.Append(portDiags...)
	*portsOut = portsValue

	cudaValue, cudaDiags := types.ListValueFrom(ctx, types.StringType, cudaVersions)
	diags.Append(cudaDiags...)
	*cudaOut = cudaValue

	return diags
}
