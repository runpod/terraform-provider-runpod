package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
	"github.com/runpod/go-sdk/v2/lib/templates"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

var (
	_ resource.Resource                = (*templateResource)(nil)
	_ resource.ResourceWithConfigure   = (*templateResource)(nil)
	_ resource.ResourceWithImportState = (*templateResource)(nil)
)

func NewTemplateResource() resource.Resource { return &templateResource{} }

type templateResource struct {
	client *runpod.Client
	legacy *legacy.Client
}

type templateResourceModel struct {
	Id                  types.String  `tfsdk:"id"`
	Name                types.String  `tfsdk:"name"`
	Image               types.String  `tfsdk:"image"`
	Category            types.String  `tfsdk:"category"`
	Disk                types.Int64   `tfsdk:"disk"`
	Args                types.String  `tfsdk:"args"`
	Env                 types.Map     `tfsdk:"env"`
	Ports               types.List    `tfsdk:"ports"`
	Public              types.Bool    `tfsdk:"public"`
	Serverless          types.Bool    `tfsdk:"serverless"`
	PersistentVolume    types.Object  `tfsdk:"persistent_volume"`
	Registry            types.String  `tfsdk:"registry"`
	StartSsh            types.Bool    `tfsdk:"start_ssh"`
	StartJupyter        types.Bool    `tfsdk:"start_jupyter"`
	AllowedCudaVersions types.List    `tfsdk:"allowed_cuda_versions"`
	DockerEntrypoint    types.List    `tfsdk:"docker_entrypoint"`
	Readme              types.String  `tfsdk:"readme"`
	Earned              types.Float64 `tfsdk:"earned"`
	IsRunpod            types.Bool    `tfsdk:"is_runpod"`
	RuntimeInMin        types.Int64   `tfsdk:"runtime_in_min"`
}

func (r *templateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_template"
}

func (r *templateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A reusable pod or endpoint configuration template.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Template identifier assigned by the API.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":  schema.StringAttribute{Required: true, Description: "Template name."},
			"image": schema.StringAttribute{Required: true, Description: "Docker image reference."},
			"category": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Console grouping for the template: CPU, NVIDIA, or AMD. Server default is NVIDIA. It does not affect scheduling or billing.",
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(sdktypes.TemplateCategoryCPU),
						string(sdktypes.TemplateCategoryNVIDIA),
						string(sdktypes.TemplateCategoryAMD),
					),
				},
			},
			"disk": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Container disk in GB.",
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
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
				Description: "Environment variables.",
			},
			"ports": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Description: "Exposed ports, e.g. [\"8888/http\", \"22/tcp\"].",
			},
			"public": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether the template is publicly visible.",
			},
			"serverless": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether the template configures Serverless workers (true) or pods (false). Server default is false.",
			},
			"persistent_volume": schema.SingleNestedAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Host-local persistent storage for containers created from this template, replacing the 1.x volume_in_gb and volume_mount_path attributes. Templates carry a single persistent mount and no network volume; CPU pods created from the template ignore it. The API leaves an existing mount in place when the block is dropped from the configuration.",
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
			},
			"registry": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Registry credential ID (runpod_registry) for pulling private images, called container_registry_auth_id in 1.x. Removing it from the configuration leaves the credential attached: the provider cannot clear it yet.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"start_ssh": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Provision SSH access in containers created from this template, by injecting a PUBLIC_KEY variable carrying the deployer's registered SSH keys. Needs an image that honors the convention (the Runpod official images do), and \"22/tcp\" in ports for direct SSH. Server default is true, matching console-created templates.",
			},
			"start_jupyter": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Start JupyterLab in containers created from this template, by injecting a generated JUPYTER_PASSWORD variable. Needs an image that honors the convention (the Runpod official images do) and \"8888/http\" in ports. Server default is true, matching console-created templates.",
			},
			"allowed_cuda_versions": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Description: "Acceptable CUDA versions for containers created from this template, as major.minor, e.g. [\"12.8\", \"12.6\"]. Matching is exact. Expanded into GPU pod and endpoint creates; CPU pods ignore it. Set it to [] to accept any version.",
			},
			"docker_entrypoint": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Description: "Deprecated. Container entrypoint for containers created from this template, e.g. [\"/bin/bash\"], preserving the 1.x docker_entrypoint attribute. Applied through the REST v1 shim on create and update, and read back from it, so the template's own value fills it in when the configuration leaves it out. " +
					legacyShimPartialPatchCaveat + " " + legacyShimDescriptionSuffix,
				DeprecationMessage: legacyShimDeprecation,
				PlanModifiers:      []planmodifier.List{legacyShimUnknownList(types.StringType)},
			},
			"readme": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "Deprecated. Markdown description shown on the template's console page, preserving the 1.x readme attribute. Applied through the REST v1 shim on create and update, and read back from it; set it to \"\" to clear the text. " +
					legacyShimPartialPatchCaveat + " " + legacyShimDescriptionSuffix,
				DeprecationMessage: legacyShimDeprecation,
				PlanModifiers:      []planmodifier.String{legacyShimUnknownString()},
			},
			"earned": schema.Float64Attribute{
				Computed: true,
				Description: "Deprecated. Total earnings in USD credited to this template through the Runpod creator programme, preserving the 1.x earned attribute. Zero for a private template. " +
					legacyShimDescriptionSuffix,
				DeprecationMessage: legacyShimDeprecation,
				PlanModifiers:      []planmodifier.Float64{legacyShimUnknownFloat64()},
			},
			"is_runpod": schema.BoolAttribute{
				Computed: true,
				Description: "Deprecated. Whether Runpod itself publishes the template, as opposed to the account that reads it, preserving the 1.x is_runpod attribute. " +
					legacyShimDescriptionSuffix,
				DeprecationMessage: legacyShimDeprecation,
				PlanModifiers:      []planmodifier.Bool{legacyShimUnknownBool()},
			},
			"runtime_in_min": schema.Int64Attribute{
				Computed: true,
				Description: "Deprecated. Total minutes containers created from this template have run, preserving the 1.x runtime_in_min attribute. " +
					legacyShimDescriptionSuffix,
				DeprecationMessage: legacyShimDeprecation,
				PlanModifiers:      []planmodifier.Int64{legacyShimUnknownInt64()},
			},
		},
	}
}

func (r *templateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, &resp.Diagnostics)
	r.legacy = configuredLegacyClient(req.ProviderData, &resp.Diagnostics)
}

// applyLegacyTemplateFields settles every REST v1 shim attribute in one step.
// A configuration that sets docker_entrypoint or readme turns the write into a
// PATCH whose response already carries the read-backs; anything else just
// reads. A failed write is an error, since the configuration asked for it; a
// failed read warns. Either way the three read-backs go null and the two
// configured inputs keep what the configuration set, so the resource still
// converges on the next plan instead of diffing against a null forever; see
// clearLegacyTemplateFieldsInto.
func (r *templateResource) applyLegacyTemplateFields(ctx context.Context, model *templateResourceModel, patch legacy.TemplatePatchV1) diag.Diagnostics {
	var diags diag.Diagnostics
	if r.legacy == nil {
		clearLegacyTemplateFields(model)
		return diags
	}

	if !patch.IsEmpty() {
		template, err := r.legacy.PatchTemplateV1(ctx, model.Id.ValueString(), patch)
		if err != nil {
			clearLegacyTemplateFields(model)
			appendLegacyWriteFailure(&diags, "template", err)
			return diags
		}
		diags.Append(flattenLegacyTemplate(ctx, template, true, model)...)
		return diags
	}

	template, err := r.legacy.GetTemplateV1(ctx, model.Id.ValueString())
	if err != nil {
		clearLegacyTemplateFields(model)
		appendLegacyReadFailure(&diags, "template", err)
		return diags
	}
	// A GET reports the template as it is, so a field it does not carry has
	// been cleared, on the console say, and the attribute follows it to
	// null instead of holding the old value against every later plan.
	diags.Append(flattenLegacyTemplate(ctx, template, false, model)...)
	return diags
}

func (r *templateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config templateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, diags := expandCreateTemplate(ctx, plan)
	resp.Diagnostics.Append(diags...)
	// The legacy patch carries only what the configuration set; see
	// expandLegacyTemplatePatch.
	patch, patchDiags := expandLegacyTemplatePatch(ctx, config)
	resp.Diagnostics.Append(patchDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.Templates.CreateTemplateWithResponse(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating template failed", err.Error())
		return
	}
	if apiResp.JSON201 == nil {
		resp.Diagnostics.AddError(
			"Creating template failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	resp.Diagnostics.Append(flattenTemplate(ctx, apiResp.JSON201, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The template exists from here on, so its state is written before a
	// legacy failure is reported: returning first would leave it in the
	// account with nothing in state, and the next apply would create a second
	// one.
	legacyDiags := r.applyLegacyTemplateFields(ctx, &plan, patch)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	resp.Diagnostics.Append(legacyDiags...)
}

func (r *templateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state templateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.Templates.GetTemplateWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading template failed", err.Error())
		return
	}
	if apiResp.HTTPResponse.StatusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Reading template failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	resp.Diagnostics.Append(flattenTemplate(ctx, apiResp.JSON200, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// A refresh only reads: whatever the configuration set is re-applied by the
	// next update, not by this call.
	resp.Diagnostics.Append(r.applyLegacyTemplateFields(ctx, &state, legacy.TemplatePatchV1{})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *templateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, config templateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// From the configuration, so an update of a template that sets neither
	// shim input never reaches the legacy host; see expandLegacyTemplatePatch.
	patch, patchDiags := expandLegacyTemplatePatch(ctx, config)
	resp.Diagnostics.Append(patchDiags...)

	body := templates.UpdateTemplateJSONRequestBody{
		Name:  plan.Name.ValueStringPointer(),
		Image: plan.Image.ValueStringPointer(),
	}
	if !plan.Category.IsNull() && !plan.Category.IsUnknown() {
		category := sdktypes.TemplateCategory(plan.Category.ValueString())
		body.Category = &category
	}
	if !plan.Disk.IsNull() && !plan.Disk.IsUnknown() {
		disk := int(plan.Disk.ValueInt64())
		body.Disk = &disk
	}
	if !plan.Args.IsNull() && !plan.Args.IsUnknown() {
		body.Args = plan.Args.ValueStringPointer()
	}
	if !plan.Public.IsNull() && !plan.Public.IsUnknown() {
		body.Public = plan.Public.ValueBoolPointer()
	}
	if !plan.Serverless.IsNull() && !plan.Serverless.IsUnknown() {
		body.Serverless = plan.Serverless.ValueBoolPointer()
	}
	if !plan.Registry.IsNull() && !plan.Registry.IsUnknown() {
		body.Registry = plan.Registry.ValueStringPointer()
	}
	if !plan.StartSsh.IsNull() && !plan.StartSsh.IsUnknown() {
		body.StartSsh = plan.StartSsh.ValueBoolPointer()
	}
	if !plan.StartJupyter.IsNull() && !plan.StartJupyter.IsUnknown() {
		body.StartJupyter = plan.StartJupyter.ValueBoolPointer()
	}
	if !plan.AllowedCudaVersions.IsNull() && !plan.AllowedCudaVersions.IsUnknown() {
		var versions []string
		resp.Diagnostics.Append(plan.AllowedCudaVersions.ElementsAs(ctx, &versions, false)...)
		body.AllowedCudaVersions = &versions
	}
	mounts, mountDiags := expandTemplateMounts(ctx, plan)
	resp.Diagnostics.Append(mountDiags...)
	body.Mounts = mounts
	if !plan.Env.IsNull() && !plan.Env.IsUnknown() {
		env := map[string]string{}
		resp.Diagnostics.Append(plan.Env.ElementsAs(ctx, &env, false)...)
		body.Env = &env
	}
	if !plan.Ports.IsNull() && !plan.Ports.IsUnknown() {
		var ports []string
		resp.Diagnostics.Append(plan.Ports.ElementsAs(ctx, &ports, false)...)
		body.Ports = &ports
	}
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.Templates.UpdateTemplateWithResponse(ctx, state.Id.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Updating template failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Updating template failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	resp.Diagnostics.Append(flattenTemplate(ctx, apiResp.JSON200, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// As on Create: the v2 update has already landed, so the state is written
	// before a legacy failure is reported. Returning first would leave state
	// describing the template as it was before the rename.
	legacyDiags := r.applyLegacyTemplateFields(ctx, &plan, patch)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	resp.Diagnostics.Append(legacyDiags...)
}

func (r *templateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state templateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.Templates.DeleteTemplateWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Deleting template failed", err.Error())
		return
	}
	switch apiResp.HTTPResponse.StatusCode {
	case http.StatusNoContent, http.StatusNotFound:
		// Deleted, or already gone.
	default:
		resp.Diagnostics.AddError(
			"Deleting template failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
	}
}

func (r *templateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
