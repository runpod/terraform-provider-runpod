package datasource_container_registry_auth

import (
	"os"
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewContainerRegistryAuthDataSource() datasource.DataSource {
	return &ContainerRegistryAuthDataSource{}
}

type ContainerRegistryAuthDataSource struct{}

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

	apiKey := os.Getenv("RUNPOD_API_KEY")
	runpodClient := client.NewRunPodClient(apiKey, client.GetGraphQLEndpoint())
	result, err := runpodClient.Query(ctx, query, variables)
	if err != nil {
		resp.Diagnostics.AddError("API Error", err.Error())
		return
	}

	if data, ok := result["data"].(map[string]interface{}); ok {
		if auths, ok := data["containerRegistryAuths"].([]interface{}); ok {
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
			diags := resp.State.Set(ctx, &models)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to parse container registry auths from response")
		}
	} else {
		resp.Diagnostics.AddError("API Error", "Failed to get data from response")
	}
}
