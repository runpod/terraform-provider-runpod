package resource_pod_action

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

func NewPodActionResource() resource.Resource {
	return &PodActionResource{}
}

type PodActionResource struct {
	apiKey   string
	baseURL  string
	httpClient *http.Client
}

func (r *PodActionResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		// Provider stores the client wrapper for pod_action (REST only)
		if clientWrapper, ok := req.ProviderData.(*client.RunPodClientWrapper); ok {
			r.apiKey = clientWrapper.APIKey
			r.baseURL = clientWrapper.RestBaseURL
		}
	}
	
	// Fallback to environment variables
	if r.apiKey == "" {
		r.apiKey = os.Getenv("RUNPOD_API_KEY")
	}
	if r.baseURL == "" {
		r.baseURL = os.Getenv("RUNPOD_BASE_URL")
	}
	if r.baseURL == "" {
		r.baseURL = client.GetRestBaseURL()
	}
	
	// Initialize httpClient if not already set
	if r.httpClient == nil {
		r.httpClient = &http.Client{}
	}
}

func (r *PodActionResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "runpod_pod_action"
}

func (r *PodActionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = PodActionResourceSchema(ctx)
}

func (r *PodActionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config PodActionModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	action := config.Action.ValueString()
	podID := config.PodId.ValueString()

	// Validate action (POST /v2/pods/{id}/action accepts exactly these)
	validActions := map[string]bool{
		"start": true, "stop": true, "restart": true, "terminate": true,
	}
	if !validActions[action] {
		resp.Diagnostics.AddError("Invalid Action", "Action must be one of: start, stop, restart, terminate")
		return
	}

	// Build REST API URL for pod actions
	url := client.NormalizeRestBaseURL(r.baseURL) + "/pods/" + podID + "/action"

	// Build request body with action
	body := map[string]interface{}{
		"action": action,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to marshal request body: %v", err))
		return
	}

	// Create HTTP request
	reqHTTP, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create request: %v", err))
		return
	}

	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("Authorization", fmt.Sprintf("Bearer %s", r.apiKey))

	// Initialize httpClient if not already set (for tests that don't call Configure)
	if r.httpClient == nil {
		r.httpClient = &http.Client{}
	}

	// Execute request
	respHTTP, err := r.httpClient.Do(reqHTTP)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to make API call: %v", err))
		return
	}
	defer respHTTP.Body.Close()

	// Read response
	respBody, err := io.ReadAll(respHTTP.Body)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to read response: %v", err))
		return
	}

	// Parse response
	var envelope map[string]interface{}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	// Extract status from response
	var status string
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if result, ok := data["result"].(map[string]interface{}); ok {
			if podStatus, ok := result["status"].(string); ok {
				status = podStatus
			}
		}
	}

	// If status not found in result, try direct status field
	if status == "" {
		if statusField, ok := envelope["status"].(string); ok {
			status = statusField
		}
	}

	// If still not found, try data field
	if status == "" {
		if data, ok := envelope["data"].(map[string]interface{}); ok {
			if podStatus, ok := data["status"].(string); ok {
				status = podStatus
			}
		}
	}

	// Try to extract from v2 response format
	if result, ok := envelope["result"].(map[string]interface{}); ok {
		if podStatus, ok := result["status"].(string); ok {
			status = podStatus
		}
	}

	// If status is still empty, set it to empty string (not an error)
	// This allows the response to succeed but with empty status
	config.Status = types.StringValue(status)

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *PodActionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// pod_action is create-only, no Read needed
}

func (r *PodActionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// pod_action is create-only, no Update needed
}

func (r *PodActionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// pod_action is create-only, no Delete needed
}
