package provider

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/lib/globalvolumes"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

var (
	_ resource.Resource                = (*globalVolumeResource)(nil)
	_ resource.ResourceWithConfigure   = (*globalVolumeResource)(nil)
	_ resource.ResourceWithImportState = (*globalVolumeResource)(nil)
)

// globalVolumeNameMaxLength is the API's limit on a global volume name, in
// characters (CreateGlobalVolumeRequest.name maxLength).
const globalVolumeNameMaxLength = 191

// globalVolumeTrimmedName matches a name with no leading or trailing
// whitespace. The API trims both ends before storing the name, so a configured
// name that carries either would read back different from the configuration
// and fail the apply with an inconsistent result; rejecting it at plan time
// says why instead. The character class is the whitespace the API's own name
// pattern excludes (CreateGlobalVolumeRequest.name): the Unicode separators,
// tab through carriage return, and NEL.
var globalVolumeTrimmedName = regexp.MustCompile(`^[^\p{Z}\t-\r\x{85}](?s:.*[^\p{Z}\t-\r\x{85}])?$`)

func NewGlobalVolumeResource() resource.Resource { return &globalVolumeResource{} }

type globalVolumeResource struct {
	client *runpod.Client
}

// globalVolumeModel is the state of both the runpod_global_volume resource and
// data source: the API reports the same four fields for each.
type globalVolumeModel struct {
	Id              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	CreatedAt       types.String `tfsdk:"created_at"`
	CreatedByUserId types.String `tfsdk:"created_by_user_id"`
}

func flattenGlobalVolume(volume *sdktypes.GlobalVolume, model *globalVolumeModel) {
	model.Id = types.StringValue(volume.Id)
	model.Name = types.StringValue(volume.Name)
	model.CreatedAt = types.StringValue(volume.CreatedAt.Format(time.RFC3339))
	model.CreatedByUserId = types.StringPointerValue(volume.CreatedByUserId)
}

func (r *globalVolumeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_global_volume"
}

func (r *globalVolumeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A global volume: usage-billed storage that is not tied to a data center. Only a name is " +
			"configured; the API provisions the storage, and an account can hold a limited number of global " +
			"volumes. Deleting one deletes its data, and fails while any pod (stopped pods included) or " +
			"serverless endpoint still mounts it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Global volume identifier assigned by the API.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required: true,
				Description: fmt.Sprintf("Human-readable name, 1 to %d characters, with no leading or trailing "+
					"whitespace. Need not be unique. Changing it renames the volume in place.", globalVolumeNameMaxLength),
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, globalVolumeNameMaxLength),
					stringvalidator.RegexMatches(globalVolumeTrimmedName,
						"must not be blank or start or end with whitespace, because the API trims it"),
				},
			},
			"created_at": schema.StringAttribute{
				Computed:      true,
				Description:   "When the volume was created, in RFC 3339 format.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"created_by_user_id": schema.StringAttribute{
				Computed:      true,
				Description:   "Identifier of the user who created the volume; null when the API has no creator recorded.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *globalVolumeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (r *globalVolumeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan globalVolumeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := globalvolumes.CreateGlobalVolumeJSONRequestBody{Name: plan.Name.ValueString()}
	apiResp, err := r.client.GlobalVolumes.CreateGlobalVolumeWithResponse(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating global volume failed", err.Error())
		return
	}
	if apiResp.JSON201 == nil {
		resp.Diagnostics.AddError(
			"Creating global volume failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	flattenGlobalVolume(apiResp.JSON201, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *globalVolumeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state globalVolumeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.GlobalVolumes.GetGlobalVolumeWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading global volume failed", err.Error())
		return
	}
	if apiResp.HTTPResponse.StatusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Reading global volume failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	flattenGlobalVolume(apiResp.JSON200, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *globalVolumeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state globalVolumeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// name is the only configurable attribute, so an update is always a rename.
	body := globalvolumes.UpdateGlobalVolumeJSONRequestBody{Name: plan.Name.ValueStringPointer()}
	apiResp, err := r.client.GlobalVolumes.UpdateGlobalVolumeWithResponse(ctx, state.Id.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Updating global volume failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Updating global volume failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	flattenGlobalVolume(apiResp.JSON200, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *globalVolumeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state globalVolumeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.GlobalVolumes.DeleteGlobalVolumeWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Deleting global volume failed", err.Error())
		return
	}
	switch apiResp.HTTPResponse.StatusCode {
	case http.StatusNoContent, http.StatusNotFound:
		// Deleted, or already gone.
	case http.StatusConflict:
		// The API detaches nothing on its own, so the fix is in the
		// configuration: say which kind of workload still holds the volume.
		resp.Diagnostics.AddError(
			"Deleting global volume failed",
			fmt.Sprintf("The global volume is still mounted. Terminate every pod that mounts it (stopped pods "+
				"count) and detach it from every serverless endpoint, then destroy it again. API returned "+
				"status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
	default:
		resp.Diagnostics.AddError(
			"Deleting global volume failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
	}
}

func (r *globalVolumeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
