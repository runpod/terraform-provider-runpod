package datasource_container_registry_auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewContainerRegistryAuthDataSource() datasource.DataSource {
	return &ContainerRegistryAuthDataSource{}
}

type ContainerRegistryAuthDataSource struct {
	rlClient *client.RunPodClient
}

func (d *ContainerRegistryAuthDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.rlClient = req.ProviderData.(*client.RunPodClient)
	}
}

func (d *ContainerRegistryAuthDataSource) getClient() *client.RunPodClient {
	if d.rlClient != nil {
		return d.rlClient
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	endpoint := os.Getenv("RUNPOD_GRAPHQL_URL")
	if endpoint == "" {
		endpoint = "https://api.runpod.io/graphql"
	}
	baseURL := os.Getenv("RUNPOD_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.runpod.io/v2"
	}
	d.rlClient = client.NewRunPodClient(apiKey, endpoint, baseURL)
	return d.rlClient
}

func (d *ContainerRegistryAuthDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_container_registry_auths"
}

func (d *ContainerRegistryAuthDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = ContainerRegistryAuthDataSourceSchema(ctx)
}

func (d *ContainerRegistryAuthDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	rlClient := d.getClient()

	// Use v2 REST endpoint: GET /v2/registries
	url := rlClient.RestBaseURL + "/registries"

	reqHTTP, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create request: %v", err))
		return
	}

	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rlClient.APIKey))

	httpClient := &http.Client{}
	respHTTP, err := httpClient.Do(reqHTTP)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to make API call: %v", err))
		return
	}
	defer respHTTP.Body.Close()

	// Handle 404 - registries not found
	if respHTTP.StatusCode == 404 {
		resp.Diagnostics.AddError("API Error", "Registries not found")
		return
	}

	// Read and parse response body
	respBody, err := io.ReadAll(respHTTP.Body)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to read response: %v", err))
		return
	}

	// Handle v2 response envelope: {data: {...}, meta: {...}, error: ...}
	var envelope map[string]interface{}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	// Extract data from envelope
	var result map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		result = data
	} else {
		result = envelope
	}

	// Parse registries array from v2 REST response
	var registries []interface{}
	if reg, ok := result["registries"].([]interface{}); ok {
		registries = reg
	} else if reg, ok := result["data"].([]interface{}); ok {
		registries = reg
	} else {
		resp.Diagnostics.AddError("API Error", "Failed to parse registries from v2 REST response")
		return
	}

	models := make([]ContainerRegistryAuthModel, len(registries))
	for i, reg := range registries {
		if regMap, ok := reg.(map[string]interface{}); ok {
			var id, name, username, password, createdAt, updatedAt string

			// Parse fields from v2 REST response
			if v, ok := regMap["id"].(string); ok {
				id = v
			} else {
				resp.Diagnostics.AddError("API Error", "Field 'id' is missing or not a string in registry response")
				return
			}

			if v, ok := regMap["name"].(string); ok {
				name = v
			} else {
				resp.Diagnostics.AddError("API Error", "Field 'name' is missing or not a string in registry response")
				return
			}

			if v, ok := regMap["username"].(string); ok {
				username = v
			} else {
				resp.Diagnostics.AddError("API Error", "Field 'username' is missing or not a string in registry response")
				return
			}

			if v, ok := regMap["password"].(string); ok {
				password = v
			}

			if v, ok := regMap["createdAt"].(string); ok {
				createdAt = v
			}

			if v, ok := regMap["updatedAt"].(string); ok {
				updatedAt = v
			}

			models[i] = ContainerRegistryAuthModel{
				Id:         types.StringValue(id),
				Name:       types.StringValue(name),
				Username:   types.StringValue(username),
				Password:   types.StringValue(password),
				CreatedAt:  types.StringValue(createdAt),
				UpdatedAt:  types.StringValue(updatedAt),
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
}
