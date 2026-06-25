package resource_pod

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

func NewPodResource() resource.Resource {
	return &PodResource{}
}

type PodResource struct{}

func (r *PodResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "runpod_pod"
}

func (r *PodResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = PodResourceSchema(ctx)
}

func (r *PodResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config PodModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	hasTemplateId := !config.TemplateId.IsNull() && config.TemplateId.ValueString() != ""
	hasImageName := !config.ImageName.IsNull() && config.ImageName.ValueString() != ""

	if hasTemplateId && hasImageName {
		resp.Diagnostics.AddError(
			"Invalid Configuration",
			"Cannot specify both template_id and image_name. Use template_id for templates, or image_name for direct image deployment.",
		)
		return
	}

	if !hasTemplateId && !hasImageName {
		resp.Diagnostics.AddError(
			"Missing Required Field",
			"Must specify either template_id or image_name.",
		)
		return
	}

	// Use REST API endpoint
	apiKey := os.Getenv("RUNPOD_API_KEY")
	if apiKey == "" {
		resp.Diagnostics.AddError("API Error", "RUNPOD_API_KEY environment variable must be set. Get your API key from https://runpod.io/console/user/settings")
		return
	}

	url := client.GetRestBaseURL() + "/pods"

	// Build the REST API request body
	body := map[string]interface{}{
		"gpuCount": int64(config.GpuCount.ValueInt64()),
		"name":     config.Name.ValueString(),
	}

	if hasTemplateId {
		body["templateId"] = config.TemplateId.ValueString()
	} else {
		body["imageName"] = config.ImageName.ValueString()
	}

	if !config.CloudType.IsNull() && config.CloudType.ValueString() != "" {
		body["cloudType"] = config.CloudType.ValueString()
	}

	if config.VolumeInGb.ValueFloat64() > 0 {
		body["volumeInGb"] = int64(config.VolumeInGb.ValueFloat64())
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to marshal request body: %v", err))
		return
	}

	reqHTTP, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create request: %v", err))
		return
	}

	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	client := &http.Client{}
	respHTTP, err := client.Do(reqHTTP)
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

	// Parse the response
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	// Extract the pod ID from response
	if podID, ok := result["id"].(string); ok {
		config.Id = types.StringValue(podID)
	} else {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get pod ID from response: %v", result))
		return
	}

	// Set the state
	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *PodResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PodModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	apiKey := os.Getenv("RUNPOD_API_KEY")
	if apiKey == "" {
		resp.Diagnostics.AddError("API Error", "RUNPOD_API_KEY environment variable must be set")
		return
	}

	url := client.GetRestBaseURL() + "/pods/" + state.Id.ValueString()

	reqHTTP, err := http.NewRequest("GET", url, nil)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create request: %v", err))
		return
	}

	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	client := &http.Client{}
	respHTTP, err := client.Do(reqHTTP)
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

	if respHTTP.StatusCode == 404 {
		resp.Diagnostics.AddWarning("Resource Not Found", "Pod not found - it may have been deleted outside of Terraform")
		return
	}

	if result == nil {
		resp.Diagnostics.AddError("API Error", "Empty response from API")
		return
	}

	if val, ok := result["status"].(string); ok && val != "" {
		state.Status = types.StringValue(val)
	}
	if val, ok := result["gpuTypeId"].(string); ok && val != "" {
		state.GpuTypeId = types.StringValue(val)
	}
	if val, ok := result["machineId"].(string); ok && val != "" {
		state.MachineId = types.StringValue(val)
	}
	if val, ok := result["costPerHr"].(float64); ok {
		state.CostPerHr = types.Float64Value(val)
	}
	if val, ok := result["created_at"].(string); ok && val != "" {
		state.CreatedAt = types.StringValue(val)
	}
	if val, ok := result["memoryInGb"].(float64); ok {
		state.MemoryInGb = types.Float64Value(val)
	}
	if val, ok := result["volumeInGb"].(float64); ok {
		state.VolumeInGb = types.Float64Value(val)
	}
	if val, ok := result["containerDiskInGb"].(float64); ok {
		state.ContainerDiskInGb = types.Int64Value(int64(val))
	}

	if result["templateId"] != nil {
		if val, ok := result["templateId"].(string); ok && val != "" {
			state.TemplateId = types.StringValue(val)
		}
	}
	if result["cloudType"] != nil {
		if val, ok := result["cloudType"].(string); ok && val != "" {
			state.CloudType = types.StringValue(val)
		}
	}

	diags = resp.State.Set(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *PodResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
}

func (r *PodResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PodModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	apiKey := os.Getenv("RUNPOD_API_KEY")
	if apiKey == "" {
		resp.Diagnostics.AddError("API Error", "RUNPOD_API_KEY environment variable must be set")
		return
	}

	url := client.GetRestBaseURL() + "/pods/" + state.Id.ValueString()

	reqHTTP, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create request: %v", err))
		return
	}

	reqHTTP.Header.Set("Content-Type", "application/json")
	reqHTTP.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	client := &http.Client{}
	respHTTP, err := client.Do(reqHTTP)
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
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to delete pod (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}
}
