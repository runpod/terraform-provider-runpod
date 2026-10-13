package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/lib/networkvolumes"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

var (
	_ resource.Resource                = (*networkVolumeResource)(nil)
	_ resource.ResourceWithConfigure   = (*networkVolumeResource)(nil)
	_ resource.ResourceWithImportState = (*networkVolumeResource)(nil)
)

func NewNetworkVolumeResource() resource.Resource { return &networkVolumeResource{} }

type networkVolumeResource struct {
	client *runpod.Client
}

type networkVolumeResourceModel struct {
	Id         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Size       types.Int64  `tfsdk:"size"`
	DataCenter types.String `tfsdk:"data_center"`
	Type       types.String `tfsdk:"type"`
}

func (r *networkVolumeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_volume"
}

func (r *networkVolumeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A persistent network storage volume.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Volume identifier assigned by the API.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Human-readable volume name.",
			},
			"size": schema.Int64Attribute{
				Required:    true,
				Description: "Storage to allocate in GB, between 10 and 4096. Can grow, never shrink.",
				Validators: []validator.Int64{
					int64validator.Between(10, 4096),
				},
			},
			"data_center": schema.StringAttribute{
				Required:    true,
				Description: "Data center to create the volume in. Changing it replaces the volume.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Storage tier: STANDARD or HIGH_PERFORMANCE. Defaults to the data center's primary tier. Immutable.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIfConfigured(),
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(sdktypes.VolumeTypeStandard),
						string(sdktypes.VolumeTypeHighPerformance),
					),
				},
			},
		},
	}
}

func (r *networkVolumeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (r *networkVolumeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan networkVolumeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.NetworkVolumes.CreateNetworkVolumeWithResponse(ctx, expandCreateNetworkVolume(plan))
	if err != nil {
		resp.Diagnostics.AddError("Creating network volume failed", err.Error())
		return
	}
	if apiResp.JSON201 == nil {
		resp.Diagnostics.AddError(
			"Creating network volume failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	flattenNetworkVolume(apiResp.JSON201, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *networkVolumeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state networkVolumeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.NetworkVolumes.GetNetworkVolumeWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading network volume failed", err.Error())
		return
	}
	if apiResp.HTTPResponse.StatusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Reading network volume failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	flattenNetworkVolume(apiResp.JSON200, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *networkVolumeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state networkVolumeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	size := int(plan.Size.ValueInt64())
	body := networkvolumes.UpdateNetworkVolumeJSONRequestBody{
		Name: plan.Name.ValueStringPointer(),
		Size: &size,
	}

	apiResp, err := r.client.NetworkVolumes.UpdateNetworkVolumeWithResponse(ctx, state.Id.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Updating network volume failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Updating network volume failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	flattenNetworkVolume(apiResp.JSON200, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *networkVolumeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state networkVolumeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.NetworkVolumes.DeleteNetworkVolumeWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Deleting network volume failed", err.Error())
		return
	}
	switch apiResp.HTTPResponse.StatusCode {
	case http.StatusNoContent, http.StatusNotFound:
		// Deleted, or already gone.
	default:
		resp.Diagnostics.AddError(
			"Deleting network volume failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
	}
}

func (r *networkVolumeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
