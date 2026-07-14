package resource_ecr_delegation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewEcrDelegationResource() resource.Resource {
	return &EcrDelegationResource{}
}

type EcrDelegationResource struct {
	client *client.RunPodClient
}

func (r *EcrDelegationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.RunPodClient)
	}
}

func (r *EcrDelegationResource) getClient() *client.RunPodClient {
	if r.client != nil {
		return r.client
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
	r.client = client.NewRunPodClient(apiKey, endpoint, baseURL)
	return r.client
}

func (r *EcrDelegationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "runpod_ecr_delegation"
}

func (r *EcrDelegationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = EcrDelegationResourceSchema(ctx)
}

func (r *EcrDelegationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config EcrDelegationModel
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

	url := r.getClient().RestBaseURL + "/registries/delegations"

	body := map[string]interface{}{
		"resource": config.Resource.ValueString(),
	}

	if !config.Name.IsNull() && config.Name.ValueString() != "" {
		body["name"] = config.Name.ValueString()
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

	if respHTTP.StatusCode != 201 && respHTTP.StatusCode != 200 {
		respBody, err := io.ReadAll(respHTTP.Body)
		if err != nil {
			resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to read response: %v", err))
			return
		}
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create ECR delegation (status: %d): %s", respHTTP.StatusCode, string(respBody)))
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
		if delegation, ok := data["delegation"].(map[string]interface{}); ok {
			result = delegation
		} else if delegations, ok := data["delegations"].([]interface{}); ok && len(delegations) > 0 {
			if delegation, ok := delegations[0].(map[string]interface{}); ok {
				result = delegation
			} else {
				resp.Diagnostics.AddError("API Error", "Expected delegation object but got different type in delegations array")
				return
			}
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to extract delegation from v2 response data")
			return
		}
	} else {
		result = envelope
	}

	if id, ok := result["id"].(string); ok {
		config.Id = types.StringValue(id)
	} else {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get delegation ID from response: %v", result))
		return
	}

	if val, ok := result["name"].(string); ok {
		config.Name = types.StringValue(val)
	}
	if val, ok := result["awsUser"].(string); ok {
		config.AwsUser = types.StringValue(val)
	}
	if val, ok := result["repository"].(string); ok {
		config.Repository = types.StringValue(val)
	}
	if val, ok := result["tag"].(string); ok {
		config.Tag = types.StringValue(val)
	}
	if val, ok := result["awsRegion"].(string); ok {
		config.AwsRegion = types.StringValue(val)
	}
	if val, ok := result["dockerRegistryUri"].(string); ok {
		config.DockerRegistryUri = types.StringValue(val)
	}
	if val, ok := result["createdAt"].(string); ok {
		config.CreatedAt = types.StringValue(val)
	}

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *EcrDelegationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state EcrDelegationModel
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

	url := r.getClient().RestBaseURL + "/registries/delegations"

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

	var delegations []map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if delegationsList, ok := data["delegations"].([]interface{}); ok {
			for _, d := range delegationsList {
				if delegation, ok := d.(map[string]interface{}); ok {
					delegations = append(delegations, delegation)
				}
			}
		}
	} else {
		resp.Diagnostics.AddError("API Error", "Failed to extract delegations from v2 response data")
		return
	}

	if len(delegations) == 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	var found bool
	for _, d := range delegations {
		if delegationId, ok := d["id"].(string); ok && delegationId == state.Id.ValueString() {
			found = true
			if val, ok := d["name"].(string); ok {
				state.Name = types.StringValue(val)
			}
			if val, ok := d["awsUser"].(string); ok {
				state.AwsUser = types.StringValue(val)
			}
			if val, ok := d["repository"].(string); ok {
				state.Repository = types.StringValue(val)
			}
			if val, ok := d["tag"].(string); ok {
				state.Tag = types.StringValue(val)
			}
			if val, ok := d["awsRegion"].(string); ok {
				state.AwsRegion = types.StringValue(val)
			}
			if val, ok := d["dockerRegistryUri"].(string); ok {
				state.DockerRegistryUri = types.StringValue(val)
			}
			if val, ok := d["createdAt"].(string); ok {
				state.CreatedAt = types.StringValue(val)
			}
			break
		}
	}

	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	diags = resp.State.Set(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *EcrDelegationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update Not Supported", "ECR delegations do not support updates")
}

func (r *EcrDelegationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state EcrDelegationModel
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

	url := r.getClient().RestBaseURL + "/registries/delegations/" + state.Id.ValueString()

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

	if respHTTP.StatusCode != 204 && respHTTP.StatusCode != 200 {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to delete ECR delegation (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}
}
