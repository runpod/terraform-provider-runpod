package resource_endpoint_job

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

func NewEndpointJobResource() resource.Resource {
	return &EndpointJobResource{}
}

type EndpointJobResource struct {
	client *client.RunPodClient
}

func (r *EndpointJobResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.RunPodClient)
	}
}

func (r *EndpointJobResource) getClient() *client.RunPodClient {
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
		baseURL = "https://rest.runpod.io/v1"
	}
	r.client = client.NewRunPodClient(apiKey, endpoint, baseURL)
	return r.client
}

func (r *EndpointJobResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "runpod_endpoint_job"
}

func (r *EndpointJobResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = EndpointJobResourceSchema(ctx)
}

func (r *EndpointJobResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config EndpointJobModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	client := r.getClient()

	url := client.RestBaseURL + "/serverless/" + config.EndpointId.ValueString() + "/jobs"

	body := map[string]interface{}{}

	if !config.Input.IsNull() {
		body["input"] = config.Input.ValueString()
	}

	if !config.TemplateId.IsNull() && config.TemplateId.ValueString() != "" {
		body["templateId"] = config.TemplateId.ValueString()
	}

	if !config.WorkerId.IsNull() && config.WorkerId.ValueString() != "" {
		body["workerId"] = config.WorkerId.ValueString()
	}

	if !config.HttpCallbackUrl.IsNull() && config.HttpCallbackUrl.ValueString() != "" {
		body["httpCallbackUrl"] = config.HttpCallbackUrl.ValueString()
	}

	if !config.HttpCallbackMethod.IsNull() && config.HttpCallbackMethod.ValueString() != "" {
		body["httpCallbackMethod"] = config.HttpCallbackMethod.ValueString()
	}

	if !config.PauseLogs.IsNull() {
		body["pauseLogs"] = config.PauseLogs.ValueBool()
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
	reqHTTP.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.APIKey))

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

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	if respHTTP.StatusCode != 200 && respHTTP.StatusCode != 201 {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create job (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	jobData, ok := result["job"].(map[string]interface{})
	if !ok {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get job data from response: %v", result))
		return
	}

	if id, ok := jobData["id"].(string); ok {
		config.Id = types.StringValue(id)
	} else {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get job ID from response: %v", jobData))
		return
	}

	if status, ok := jobData["status"].(string); ok {
		config.Status = types.StringValue(status)
	}

	if createdAt, ok := jobData["createdAt"].(string); ok {
		config.CreatedAt = types.StringValue(createdAt)
	}

	if durationMs, ok := jobData["durationMs"].(float64); ok {
		config.DurationMs = types.Int64Value(int64(durationMs))
	}

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *EndpointJobResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state EndpointJobModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	client := r.getClient()

	url := client.RestBaseURL + "/serverless/" + state.EndpointId.ValueString() + "/jobs/" + state.Id.ValueString()

	reqHTTP, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create request: %v", err))
		return
	}

	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.APIKey))

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

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	jobData, ok := result["job"].(map[string]interface{})
	if !ok {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get job data from response: %v", result))
		return
	}

	if status, ok := jobData["status"].(string); ok {
		state.Status = types.StringValue(status)
	}

	if createdAt, ok := jobData["createdAt"].(string); ok {
		state.CreatedAt = types.StringValue(createdAt)
	}

	if durationMs, ok := jobData["durationMs"].(float64); ok {
		state.DurationMs = types.Int64Value(int64(durationMs))
	}

	if completedAt, ok := jobData["completedAt"].(string); ok {
		state.CompletedAt = types.StringValue(completedAt)
	}

	if output, ok := jobData["output"].(string); ok {
		state.Output = types.StringValue(output)
	}

	diags = resp.State.Set(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *EndpointJobResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update Not Supported", "Endpoint jobs cannot be updated")
}

func (r *EndpointJobResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state EndpointJobModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	client := r.getClient()

	url := client.RestBaseURL + "/serverless/" + state.EndpointId.ValueString() + "/jobs/" + state.Id.ValueString()

	reqHTTP, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create request: %v", err))
		return
	}

	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.APIKey))

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
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to delete job (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}
}
