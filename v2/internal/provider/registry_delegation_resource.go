package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
	"github.com/runpod/go-sdk/v2/lib/registries"
)

var (
	_ resource.Resource              = (*registryDelegationResource)(nil)
	_ resource.ResourceWithConfigure = (*registryDelegationResource)(nil)
)

func NewRegistryDelegationResource() resource.Resource { return &registryDelegationResource{} }

type registryDelegationResource struct {
	client *runpod.Client
}

type registryDelegationResourceModel struct {
	Id                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Resource          types.String `tfsdk:"resource"`
	AwsRegion         types.String `tfsdk:"aws_region"`
	AwsUser           types.String `tfsdk:"aws_user"`
	Repository        types.String `tfsdk:"repository"`
	Tag               types.String `tfsdk:"tag"`
	DockerRegistryUri types.String `tfsdk:"docker_registry_uri"`
	CreatedAt         types.String `tfsdk:"created_at"`
}

func (r *registryDelegationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_registry_delegation"
}

func (r *registryDelegationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An AWS ECR pull delegation. The API has no update operation, so " +
			"any change replaces the delegation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Delegation identifier assigned by the API.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Optional:      true,
				Description:   "Human-readable delegation name.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"resource": schema.StringAttribute{
				Required:      true,
				Description:   "ECR resource ARN to delegate pulls for. Not echoed by the API; tracked from configuration.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"aws_region": schema.StringAttribute{
				Computed:    true,
				Description: "AWS region of the delegated ECR repository.",
			},
			"aws_user": schema.StringAttribute{
				Computed:    true,
				Description: "AWS principal Runpod pulls as; grant it read access on the repository.",
			},
			"repository": schema.StringAttribute{
				Computed:    true,
				Description: "ECR repository name parsed from the delegated ARN.",
			},
			"tag": schema.StringAttribute{
				Computed:    true,
				Description: "Image tag the delegation covers.",
			},
			"docker_registry_uri": schema.StringAttribute{
				Computed:    true,
				Description: "Registry URI to pull the image from.",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "Creation timestamp in RFC 3339 format.",
			},
		},
	}
}

func (r *registryDelegationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (r *registryDelegationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan registryDelegationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := registries.CreateDelegationJSONRequestBody{
		Resource: plan.Resource.ValueString(),
	}
	if !plan.Name.IsNull() {
		body.Name = plan.Name.ValueStringPointer()
	}
	apiResp, err := r.client.Registries.CreateDelegationWithResponse(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating registry delegation failed", err.Error())
		return
	}
	if apiResp.JSON201 == nil {
		resp.Diagnostics.AddError(
			"Creating registry delegation failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	flattenDelegation(apiResp.JSON201, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read lists delegations and matches by id, since the API has no
// per-delegation GET.
func (r *registryDelegationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state registryDelegationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.Registries.ListDelegationsWithResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading registry delegation failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Reading registry delegation failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
		return
	}

	for _, delegation := range apiResp.JSON200.Delegations {
		if delegation.Id == state.Id.ValueString() {
			flattenDelegation(&delegation, &state)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *registryDelegationResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All configurable attributes carry RequiresReplace; update is unreachable.
	resp.Diagnostics.AddError(
		"Registry delegations cannot be updated",
		"The API has no update operation; changes must replace the delegation.",
	)
}

func (r *registryDelegationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state registryDelegationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.Registries.RevokeDelegationWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Revoking registry delegation failed", err.Error())
		return
	}
	switch apiResp.HTTPResponse.StatusCode {
	case http.StatusNoContent, http.StatusNotFound:
		// Revoked, or already gone.
	default:
		resp.Diagnostics.AddError(
			"Revoking registry delegation failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)),
		)
	}
}
