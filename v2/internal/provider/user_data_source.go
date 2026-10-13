package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/go-sdk/v2/legacy" //nolint:staticcheck // deprecated on purpose: the shim client goes when REST v2 carries these fields
)

// runpod_user, ported from the 1.x provider's datasource_user. 1.x read it
// from REST v1 GET /user; that route is gone from the v1 host (it answers 400
// "path does not exist in the specification" since at least 2026-10-08), so
// the read now goes through the GraphQL `myself` query, the only account
// identity read the platform still serves. The 1.x schema is kept: the
// GraphQL User type carries only the id and the email of it, so every other
// attribute reads null and says so in its description.

var (
	_ datasource.DataSource              = (*userDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*userDataSource)(nil)
)

// userQuery is the whole of what the GraphQL User type carries of the 1.x
// user fields (node/graphql/schema/user.ts: no name, pubKey, verified,
// cloudType or quota fields).
const userQuery = `
	query Myself {
		myself {
			id
			email
		}
	}
`

const noGraphQLSource = "Always null: the GraphQL account read has no such field."

func NewUserDataSource() datasource.DataSource { return &userDataSource{} }

type userDataSource struct {
	client *legacy.Client
}

type userDataSourceModel struct {
	Id           types.String  `tfsdk:"id"`
	Name         types.String  `tfsdk:"name"`
	Email        types.String  `tfsdk:"email"`
	PubKey       types.String  `tfsdk:"pub_key"`
	Verified     types.Bool    `tfsdk:"verified"`
	CloudType    types.String  `tfsdk:"cloud_type"`
	GpuLimit     types.Float64 `tfsdk:"gpu_limit"`
	GpuUsage     types.Float64 `tfsdk:"gpu_usage"`
	StorageLimit types.Float64 `tfsdk:"storage_limit"`
	StorageUsage types.Float64 `tfsdk:"storage_usage"`
}

func (d *userDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *userDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:        "The account the API key belongs to. Deprecated: " + userDeprecation,
		DeprecationMessage: userDeprecation,
		Attributes: map[string]schema.Attribute{
			"id":            schema.StringAttribute{Computed: true, Description: "Account identifier, from the GraphQL myself query."},
			"name":          schema.StringAttribute{Computed: true, Description: "Account holder name. " + noGraphQLSource},
			"email":         schema.StringAttribute{Computed: true, Description: "Account email address, from the GraphQL myself query."},
			"pub_key":       schema.StringAttribute{Computed: true, Description: "Public SSH key registered on the account. " + noGraphQLSource + " The v2 SSH keys route serves the registered keys."},
			"verified":      schema.BoolAttribute{Computed: true, Description: "Whether the account email is verified. " + noGraphQLSource},
			"cloud_type":    schema.StringAttribute{Computed: true, Description: "Default cloud tier for the account. " + noGraphQLSource},
			"gpu_limit":     schema.Float64Attribute{Computed: true, Description: "Maximum number of GPUs the account may run. " + noGraphQLSource},
			"gpu_usage":     schema.Float64Attribute{Computed: true, Description: "GPUs the account is currently running. " + noGraphQLSource},
			"storage_limit": schema.Float64Attribute{Computed: true, Description: "Maximum storage the account may allocate, in GB. " + noGraphQLSource},
			"storage_usage": schema.Float64Attribute{Computed: true, Description: "Storage the account currently allocates, in GB. " + noGraphQLSource},
		},
	}
}

func (d *userDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredLegacyClient(req.ProviderData, &resp.Diagnostics)
}

func (d *userDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	state, err := readUser(ctx, d.client)
	if err != nil {
		resp.Diagnostics.AddError("Reading the account failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// readUser runs the myself query and maps its account onto the 1.x shape.
func readUser(ctx context.Context, client *legacy.Client) (userDataSourceModel, error) {
	data, err := client.Query(ctx, userQuery, nil)
	if err != nil {
		return userDataSourceModel{}, err
	}
	myself, ok := data["myself"].(map[string]any)
	if !ok {
		return userDataSourceModel{}, errNoAccount
	}
	state := flattenUser(myself)
	if state.Id.IsNull() {
		return userDataSourceModel{}, errNoAccount
	}
	return state, nil
}

var errNoAccount = errors.New("the myself query returned no account")

// flattenUser maps the GraphQL User fields onto the 1.x attributes. Only id
// and email exist on the GraphQL side; the rest read null.
func flattenUser(user map[string]any) userDataSourceModel {
	return userDataSourceModel{
		Id:           jsonString(user, "id"),
		Name:         types.StringNull(),
		Email:        jsonString(user, "email"),
		PubKey:       types.StringNull(),
		Verified:     types.BoolNull(),
		CloudType:    types.StringNull(),
		GpuLimit:     types.Float64Null(),
		GpuUsage:     types.Float64Null(),
		StorageLimit: types.Float64Null(),
		StorageUsage: types.Float64Null(),
	}
}
