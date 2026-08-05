package datasource_ecr_delegations

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

func NewEcrDelegationsDataSource() datasource.DataSource {
	return &EcrDelegationsDataSource{}
}

type EcrDelegationsDataSource struct {
	rlClient *client.RunPodClient
}

func (d *EcrDelegationsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.rlClient = req.ProviderData.(*client.RunPodClient)
	}
}

func (d *EcrDelegationsDataSource) getClient() *client.RunPodClient {
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

func (d *EcrDelegationsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_ecr_delegations"
}

func (d *EcrDelegationsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = EcrDelegationsDataSourceSchema(ctx)
}

func (d *EcrDelegationsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	apiKey := d.getClient().APIKey
	if apiKey == "" {
		resp.Diagnostics.AddError("API Error", "RUNPOD_API_KEY environment variable must be set")
		return
	}

	url := d.getClient().BaseURL() + "/registries/delegations"

	reqHTTP, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create request: %v", err))
		return
	}

	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	httpClient := &http.Client{}
	respHTTP, err := httpClient.Do(reqHTTP)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to make API call: %v", err))
		return
	}
	defer respHTTP.Body.Close()

	if respHTTP.StatusCode != 200 {
		respBody, err := io.ReadAll(respHTTP.Body)
		if err != nil {
			resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to read response: %v", err))
			return
		}
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to fetch delegations (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	respBody, err := io.ReadAll(respHTTP.Body)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to read response: %v", err))
		return
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	var delegations []map[string]interface{}
	// v2 returns the list at the top level: {"delegations": [...]}; tolerate a
	// {data: {...}} envelope too
	source := envelope
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		source = data
	}
	if delegationsList, ok := source["delegations"].([]interface{}); ok {
		for _, d := range delegationsList {
			if delegation, ok := d.(map[string]interface{}); ok {
				delegations = append(delegations, delegation)
			}
		}
	} else {
		resp.Diagnostics.AddError("API Error", "Failed to extract delegations from v2 response data")
		return
	}

	models := make([]EcrDelegationModel, len(delegations))
	for i, d := range delegations {
		var id, name, resource, awsUser, repository, tag, awsRegion, dockerRegistryUri, createdAt string

		if v, ok := d["id"].(string); ok {
			id = v
		} else {
			resp.Diagnostics.AddError("API Error", "Field 'id' is missing or not a string in ECR delegation response")
			return
		}

		if v, ok := d["name"].(string); ok {
			name = v
		}

		if v, ok := d["resource"].(string); ok {
			resource = v
		}

		if v, ok := d["awsUser"].(string); ok {
			awsUser = v
		}

		if v, ok := d["repository"].(string); ok {
			repository = v
		}

		if v, ok := d["tag"].(string); ok {
			tag = v
		}

		if v, ok := d["awsRegion"].(string); ok {
			awsRegion = v
		}

		if v, ok := d["dockerRegistryUri"].(string); ok {
			dockerRegistryUri = v
		}

		if v, ok := d["createdAt"].(string); ok {
			createdAt = v
		}

		models[i] = EcrDelegationModel{
			Id:                types.StringValue(id),
			Name:              types.StringValue(name),
			Resource:          types.StringValue(resource),
			AwsUser:           types.StringValue(awsUser),
			Repository:        types.StringValue(repository),
			Tag:               types.StringValue(tag),
			AwsRegion:         types.StringValue(awsRegion),
			DockerRegistryUri: types.StringValue(dockerRegistryUri),
			CreatedAt:         types.StringValue(createdAt),
		}
	}

	parent := EcrDelegationsDataSourceModel{
		EcrDelegations: models,
	}
	diags := resp.State.Set(ctx, &parent)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}
