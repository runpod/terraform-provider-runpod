package datasource_user

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewUserDataSource() datasource.DataSource {
	return &UserDataSource{}
}

type UserDataSource struct {
	rlClient *client.RunPodClient
}

func (d *UserDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.rlClient = req.ProviderData.(*client.RunPodClient)
	}
}

func (d *UserDataSource) getClient() *client.RunPodClient {
	if d.rlClient != nil {
		return d.rlClient
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	endpoint := os.Getenv("RUNPOD_GRAPHQL_URL")
	if endpoint == "" {
		endpoint = "https://api.runpod.io/graphql"
	}
	d.rlClient = client.NewRunPodClient(apiKey, endpoint, "")
	return d.rlClient
}

func (d *UserDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_user"
}

func (d *UserDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = UserDataSourceSchema(ctx)
}

func (d *UserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	// There is no user/identity endpoint in the v2 REST API; account info is
	// only available via the GraphQL `myself` query, which exposes id and
	// pubKey for the current user (matching the schema in the provider spec).
	query := `
		query GetUser {
			myself {
				id
				pubKey
			}
		}
	`

	result, err := d.getClient().Query(ctx, query, map[string]interface{}{})
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	user, ok := result["myself"].(map[string]interface{})
	if !ok {
		resp.Diagnostics.AddError("API Error", "User data not found in response")
		return
	}

	id, ok := user["id"].(string)
	if !ok {
		resp.Diagnostics.AddError("API Error", "Field 'id' is missing or not a string in user response")
		return
	}

	model := UserModel{
		Id:     types.StringValue(id),
		PubKey: types.StringNull(),
	}
	if pubKey, ok := user["pubKey"].(string); ok {
		model.PubKey = types.StringValue(pubKey)
	}

	diags := resp.State.Set(ctx, &model)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}
