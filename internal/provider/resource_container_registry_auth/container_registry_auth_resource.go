package resource_container_registry_auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewContainerRegistryAuthResource() resource.Resource {
	return &ContainerRegistryAuthResource{}
}

type ContainerRegistryAuthResource struct {
	client *client.RunPodClient
}

func (r *ContainerRegistryAuthResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		if clientWrapper, ok := req.ProviderData.(*client.RunPodClientWrapper); ok {
			r.client = &client.RunPodClient{
				APIKey:      clientWrapper.APIKey,
				GraphQLEndpoint: "https://api.runpod.io/graphql",
				RestBaseURL: clientWrapper.RestBaseURL,
				Client: &http.Client{Timeout: 60 * time.Second},
			}
		} else if client, ok := req.ProviderData.(*client.RunPodClient); ok {
			r.client = client
		}
	}
}

func (r *ContainerRegistryAuthResource) getClient() *client.RunPodClient {
	if r.client != nil {
		return r.client
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	graphqlEndpoint := os.Getenv("RUNPOD_GRAPHQL_URL")
	if graphqlEndpoint == "" {
		graphqlEndpoint = "https://api.runpod.io/graphql"
	}
	restBaseURL := os.Getenv("RUNPOD_BASE_URL")
	if restBaseURL == "" {
		restBaseURL = client.GetRestBaseURL()
	}
	r.client = client.NewRunPodClient(apiKey, graphqlEndpoint, restBaseURL)
	return r.client
}

func (r *ContainerRegistryAuthResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "runpod_container_registry_auth"
}

func (r *ContainerRegistryAuthResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = ContainerRegistryAuthResourceSchema(ctx)
}

func (r *ContainerRegistryAuthResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config ContainerRegistryAuthModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	apiKey := r.getClient().APIKey
	if apiKey == "" {
		resp.Diagnostics.AddError("API Error", "RUNPOD_API_KEY environment variable must be set")
		return
	}

	url := client.GetRestBaseURL() + "/v2/registries"

	body := map[string]interface{}{
		"name":     config.Name.ValueString(),
		"username": config.Username.ValueString(),
		"password": config.Password.ValueString(),
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to marshal request body: %v", err))
		return
	}

	reqHTTP, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
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

	var result map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if registry, ok := data["registry"].(map[string]interface{}); ok {
			result = registry
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to extract registry from v2 response data")
			return
		}
	} else {
		result = envelope
	}

	if respHTTP.StatusCode != 200 && respHTTP.StatusCode != 201 {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create container registry auth (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	if id, ok := result["id"].(string); ok {
		config.Id = types.StringValue(id)
	} else {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get container registry auth ID from response: %v", result))
		return
	}

	if val, ok := result["name"].(string); ok {
		config.Name = types.StringValue(val)
	} else {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get name from response: %v", result))
		return
	}
	if val, ok := result["username"].(string); ok {
		config.Username = types.StringValue(val)
	} else {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get username from response: %v", result))
		return
	}

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *ContainerRegistryAuthResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ContainerRegistryAuthModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	apiKey := r.getClient().APIKey
	if apiKey == "" {
		resp.Diagnostics.AddError("API Error", "RUNPOD_API_KEY environment variable must be set")
		return
	}

	url := client.GetRestBaseURL() + "/v2/registries/" + state.Id.ValueString()

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

	if respHTTP.StatusCode == 404 {
		resp.State.RemoveResource(ctx)
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

	var result map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if registry, ok := data["registry"].(map[string]interface{}); ok {
			result = registry
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to extract registry from v2 response data")
			return
		}
	} else {
		result = envelope
	}

	if result == nil {
		resp.Diagnostics.AddError("API Error", "Empty response from API")
		return
	}

	if val, ok := result["name"].(string); ok {
		state.Name = types.StringValue(val)
	}
	if val, ok := result["username"].(string); ok {
		state.Username = types.StringValue(val)
	}

	diags = resp.State.Set(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *ContainerRegistryAuthResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ContainerRegistryAuthModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	apiKey := r.getClient().APIKey
	if apiKey == "" {
		resp.Diagnostics.AddError("API Error", "RUNPOD_API_KEY environment variable must be set")
		return
	}

	url := client.GetRestBaseURL() + "/v2/registries/" + state.Id.ValueString()

	reqHTTP, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
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

	respBody, err := io.ReadAll(respHTTP.Body)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to read response: %v", err))
		return
	}

	if respHTTP.StatusCode != 204 {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to delete container registry auth (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}
}

func (r *ContainerRegistryAuthResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update Not Supported", "Container registry auth does not support updates")
}
