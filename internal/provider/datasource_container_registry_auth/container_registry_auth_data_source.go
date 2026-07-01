package datasource_container_registry_auth

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewContainerRegistryAuthDataSource() datasource.DataSource {
	return &ContainerRegistryAuthDataSource{}
}

type ContainerRegistryAuthDataSource struct {
	client *client.RunPodClient
}

func (d *ContainerRegistryAuthDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.client = req.ProviderData.(*client.RunPodClient)
	}
}

func (d *ContainerRegistryAuthDataSource) getClient() *client.RunPodClient {
	if d.client != nil {
		return d.client
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	endpoint := os.Getenv("RUNPOD_GRAPHQL_URL")
	if endpoint == "" {
		endpoint = "https://api.runpod.io/graphql"
	}
	d.client = client.NewRunPodClient(apiKey, endpoint)
	return d.client
}

func (d *ContainerRegistryAuthDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_container_registry_auths"
}

func (d *ContainerRegistryAuthDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = ContainerRegistryAuthDataSourceSchema(ctx)
}

func (d *ContainerRegistryAuthDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	query := `
		query GetContainerRegistryAuths {
			containerRegistryAuths {
				id
				name
				username
			}
		}
	`

	variables := map[string]interface{}{}

	result, err := d.getClient().Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	if auths, ok := result["containerRegistryAuths"].([]interface{}); ok {
		models := make([]ContainerRegistryAuthModel, len(auths))
		for i, auth := range auths {
			if authMap, ok := auth.(map[string]interface{}); ok {
				models[i] = ContainerRegistryAuthModel{
					Id:       types.StringValue(authMap["id"].(string)),
					Name:     types.StringValue(authMap["name"].(string)),
					Username: types.StringValue(authMap["username"].(string)),
				}
			}
		}
		parent := ContainerRegistryAuthDataSourceModel{
			ContainerRegistryAuths: models,
		}
		diags := resp.State.Set(ctx, &parent)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	} else {
		resp.Diagnostics.AddError("API Error", "Failed to parse container registry auths from response")
	}
}
