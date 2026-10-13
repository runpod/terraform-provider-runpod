package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	runpod "github.com/runpod/go-sdk/v2"
	sdktypes "github.com/runpod/go-sdk/v2/lib/types"
)

// Account-wide ECR delegation listing: runpod_registry_delegations. Carries
// every field GET /v2/registries/delegations returns. The delegated ECR ARN is
// not among them — the API does not echo it back — so it is visible only on the
// runpod_registry_delegation resource that created the delegation.

var _ datasource.DataSourceWithConfigure = (*registryDelegationsDataSource)(nil)

func NewRegistryDelegationsDataSource() datasource.DataSource {
	return &registryDelegationsDataSource{}
}

type registryDelegationsDataSource struct {
	client *runpod.Client
}

type registryDelegationListItemModel struct {
	Id                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	AwsRegion         types.String `tfsdk:"aws_region"`
	AwsUser           types.String `tfsdk:"aws_user"`
	Repository        types.String `tfsdk:"repository"`
	Tag               types.String `tfsdk:"tag"`
	DockerRegistryUri types.String `tfsdk:"docker_registry_uri"`
	DelegatorUserId   types.String `tfsdk:"delegator_user_id"`
	CreatedAt         types.String `tfsdk:"created_at"`
}

type registryDelegationsDataSourceModel struct {
	Delegations []registryDelegationListItemModel `tfsdk:"delegations"`
}

func (d *registryDelegationsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_registry_delegations"
}

func (d *registryDelegationsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "All AWS ECR pull delegations owned by the authenticated account. " +
			"Replaces the 1.x runpod_ecr_delegations data source.",
		Attributes: map[string]schema.Attribute{
			"delegations": schema.ListNestedAttribute{
				Computed:    true,
				Description: "One entry per ECR delegation.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                  schema.StringAttribute{Computed: true, Description: "Delegation identifier."},
						"name":                schema.StringAttribute{Computed: true, Description: "Human-readable delegation name; null when the delegation was created without one."},
						"aws_region":          schema.StringAttribute{Computed: true, Description: "AWS region of the delegated ECR repository."},
						"aws_user":            schema.StringAttribute{Computed: true, Description: "AWS principal Runpod pulls as; grant it read access on the repository."},
						"repository":          schema.StringAttribute{Computed: true, Description: "ECR repository name parsed from the delegated ARN."},
						"tag":                 schema.StringAttribute{Computed: true, Description: "Image tag the delegation covers."},
						"docker_registry_uri": schema.StringAttribute{Computed: true, Description: "Registry URI to pull the image from, for a Docker login."},
						"delegator_user_id":   schema.StringAttribute{Computed: true, Description: "Runpod user that created the delegation."},
						"created_at":          schema.StringAttribute{Computed: true, Description: "Creation timestamp in RFC 3339 format."},
					},
				},
			},
		},
	}
}

func (d *registryDelegationsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req.ProviderData, &resp.Diagnostics)
}

func (d *registryDelegationsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	apiResp, err := d.client.Registries.ListDelegationsWithResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Listing registry delegations failed", err.Error())
		return
	}
	if apiResp.JSON200 == nil {
		resp.Diagnostics.AddError("Listing registry delegations failed",
			fmt.Sprintf("API returned status %d: %s", apiResp.HTTPResponse.StatusCode, boundedBody(apiResp.Body)))
		return
	}

	state := registryDelegationsDataSourceModel{Delegations: []registryDelegationListItemModel{}}
	for _, delegation := range apiResp.JSON200.Delegations {
		state.Delegations = append(state.Delegations, flattenDelegationListItem(delegation))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func flattenDelegationListItem(delegation sdktypes.EcrDelegation) registryDelegationListItemModel {
	return registryDelegationListItemModel{
		Id:                types.StringValue(delegation.Id),
		Name:              types.StringPointerValue(delegation.Name),
		AwsRegion:         types.StringValue(delegation.AwsRegion),
		AwsUser:           types.StringValue(delegation.AwsUser),
		Repository:        types.StringValue(delegation.Repository),
		Tag:               types.StringValue(delegation.Tag),
		DockerRegistryUri: types.StringPointerValue(delegation.DockerRegistryUri),
		DelegatorUserId:   types.StringValue(delegation.DelegatorUserId),
		CreatedAt:         types.StringValue(delegation.CreatedAt.Format(time.RFC3339)),
	}
}
