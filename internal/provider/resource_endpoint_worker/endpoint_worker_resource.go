package resource_endpoint_worker

import (
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

func NewEndpointWorkerResource() resource.Resource {
	return &EndpointWorkerResource{}
}

type EndpointWorkerResource struct {
	client *client.RunPodClient
}

func (r *EndpointWorkerResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.RunPodClient)
	}
}

func (r *EndpointWorkerResource) getClient() *client.RunPodClient {
	if r.client != nil {
		return r.client
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	baseURL := os.Getenv("RUNPOD_BASE_URL")
	if baseURL == "" {
		baseURL = client.GetRestBaseURL()
	}
	r.client = client.NewRunPodClient(apiKey, "", baseURL)
	return r.client
}

func (r *EndpointWorkerResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "runpod_endpoint_worker"
}

func (r *EndpointWorkerResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = EndpointWorkerResourceSchema(ctx)
}

func (r *EndpointWorkerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Workers are created automatically when jobs are submitted
	// This resource is primarily for reading and deleting workers
	resp.Diagnostics.AddError("Create Not Supported", "Endpoint workers are created automatically when jobs are submitted. Use the read operation to get worker information.")
}

func (r *EndpointWorkerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state EndpointWorkerModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	client := r.getClient()

	endpointId := state.EndpointId.ValueString()
	workerId := state.Id.ValueString()
	url := client.RestBaseURL + "/serverless/" + endpointId + "/workers/" + workerId

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

	if result == nil {
		resp.Diagnostics.AddError("API Error", "Empty response from API")
		return
	}

	if id, ok := result["id"].(string); ok {
		state.Id = types.StringValue(id)
	}

	if podId, ok := result["podId"].(string); ok {
		state.PodId = types.StringValue(podId)
	}

	if status, ok := result["status"].(string); ok {
		state.Status = types.StringValue(status)
	}

	if uptimeMs, ok := result["uptimeMs"].(float64); ok {
		state.UptimeMs = types.Int64Value(int64(uptimeMs))
	}

	if startTime, ok := result["startTime"].(string); ok {
		state.StartTime = types.StringValue(startTime)
	}

	if lastBusyMs, ok := result["lastBusyMs"].(float64); ok {
		state.LastBusyMs = types.Int64Value(int64(lastBusyMs))
	}

	if idleSeconds, ok := result["idleSeconds"].(float64); ok {
		state.IdleSeconds = types.Int64Value(int64(idleSeconds))
	}

	if containerId, ok := result["containerId"].(string); ok {
		state.ContainerId = types.StringValue(containerId)
	}

	diags = resp.State.Set(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *EndpointWorkerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Workers cannot be updated
	resp.Diagnostics.AddError("Update Not Supported", "Endpoint workers cannot be updated.")
}

func (r *EndpointWorkerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state EndpointWorkerModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	client := r.getClient()

	endpointId := state.EndpointId.ValueString()
	workerId := state.Id.ValueString()
	url := client.RestBaseURL + "/serverless/" + endpointId + "/workers/" + workerId

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

	if respHTTP.StatusCode != 200 && respHTTP.StatusCode != 204 {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to cancel worker (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}
}
