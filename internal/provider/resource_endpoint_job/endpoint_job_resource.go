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
	rlClient *client.RunPodClient
}

func (r *EndpointJobResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.rlClient = req.ProviderData.(*client.RunPodClient)
	}
}

func (r *EndpointJobResource) getClient() *client.RunPodClient {
	if r.rlClient != nil {
		return r.rlClient
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	baseURL := os.Getenv("RUNPOD_BASE_URL")
	if baseURL == "" {
		baseURL = client.GetRestBaseURL()
	}
	r.rlClient = client.NewRunPodClient(apiKey, "https://api.runpod.io/graphql", baseURL)
	return r.rlClient
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

	rlClient := r.getClient()
	endpointId := config.EndpointId.ValueString()

	// First, fetch the endpoint to get the run URL
	endpointUrl := rlClient.RestBaseURL + "/serverless/" + endpointId
	reqHTTP, err := http.NewRequestWithContext(ctx, "GET", endpointUrl, nil)
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

	if respHTTP.StatusCode == 404 {
		resp.Diagnostics.AddError("API Error", "Endpoint not found")
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

	var endpointResult map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if endpoint, ok := data["endpoint"].(map[string]interface{}); ok {
			endpointResult = endpoint
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to extract endpoint from v2 response data")
			return
		}
	} else {
		endpointResult = envelope
	}

	// Get the run URL from the endpoint
	var runUrl string
	if requestUrls, ok := endpointResult["requestUrls"].(map[string]interface{}); ok {
		if url, ok := requestUrls["run"].(string); ok {
			runUrl = url
		} else if url, ok := requestUrls["runSync"].(string); ok {
			runUrl = url
		} else {
			resp.Diagnostics.AddError("API Error", "Endpoint response missing run or runSync URL")
			return
		}
	} else {
		resp.Diagnostics.AddError("API Error", "Endpoint response missing requestUrls")
		return
	}

	// Now submit the job
	jobBody := map[string]interface{}{}

	if !config.Input.IsNull() && config.Input.ValueString() != "" {
		jobBody["input"] = config.Input.ValueString()
	}

	if !config.TemplateId.IsNull() && config.TemplateId.ValueString() != "" {
		jobBody["templateId"] = config.TemplateId.ValueString()
	}

	if !config.WorkerId.IsNull() && config.WorkerId.ValueString() != "" {
		jobBody["workerId"] = config.WorkerId.ValueString()
	}

	jsonBody, err := json.Marshal(jobBody)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to marshal request body: %v", err))
		return
	}

	reqHTTP, err = http.NewRequestWithContext(ctx, "POST", runUrl, bytes.NewBuffer(jsonBody))
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create request: %v", err))
		return
	}

	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rlClient.APIKey))

	respHTTP, err = httpClient.Do(reqHTTP)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to make API call: %v", err))
		return
	}
	defer respHTTP.Body.Close()

	respBody, err = io.ReadAll(respHTTP.Body)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to read response: %v", err))
		return
	}

	var jobResult map[string]interface{}
	if err := json.Unmarshal(respBody, &jobResult); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	if respHTTP.StatusCode != 200 && respHTTP.StatusCode != 201 {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create endpoint job (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	// Extract job ID
	var jobId string
	if id, ok := jobResult["id"].(string); ok {
		jobId = id
	} else if id, ok := jobResult["jobId"].(string); ok {
		jobId = id
	} else {
		resp.Diagnostics.AddError("API Error", "Job response missing id field")
		return
	}

	// Extract job status
	var jobStatus string
	if status, ok := jobResult["status"].(string); ok {
		jobStatus = status
	} else if status, ok := jobResult["jobStatus"].(string); ok {
		jobStatus = status
	}

	// Extract output if available
	var jobOutput string
	if output, ok := jobResult["output"].(string); ok {
		jobOutput = output
	} else if output, ok := jobResult["output"].(map[string]interface{}); ok {
		outputBytes, _ := json.Marshal(output)
		jobOutput = string(outputBytes)
	}

	config.Id = types.StringValue(jobId)
	config.Status = types.StringValue(jobStatus)
	config.Output = types.StringValue(jobOutput)

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *EndpointJobResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Jobs are ephemeral in serverless - once created, they complete and are not queryable
	// For now, we just return the stored state
	var state EndpointJobModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.State.Set(ctx, &state)
}

func (r *EndpointJobResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update Not Supported", "Endpoint jobs cannot be updated")
}

func (r *EndpointJobResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Jobs cannot be deleted once submitted
	// They complete and are cleaned up automatically by RunPod
}
