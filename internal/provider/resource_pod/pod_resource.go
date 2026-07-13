package resource_pod

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewPodResource() resource.Resource {
	return &PodResource{}
}

type PodResource struct {
	client *client.RunPodClient
}

func (r *PodResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.RunPodClient)
	}
}

func (r *PodResource) getClient() *client.RunPodClient {
	if r.client != nil {
		return r.client
	}
	var apiKey string
	var endpoint string
	
	if r.client != nil {
		apiKey = r.client.APIKey
		endpoint = r.client.RestBaseURL
	} else {
		apiKey = os.Getenv("RUNPOD_API_KEY")
		endpoint = os.Getenv("RUNPOD_BASE_URL")
		if endpoint == "" {
			endpoint = "https://rest.runpod.io/v1"
		}
	}
	graphqlEndpoint := os.Getenv("RUNPOD_GRAPHQL_URL")
	if graphqlEndpoint == "" {
		graphqlEndpoint = "https://api.runpod.io/graphql"
	}
	restBaseURL := os.Getenv("RUNPOD_BASE_URL")
	if restBaseURL == "" {
		restBaseURL = "https://rest.runpod.io/v1"
	}
	r.client = client.NewRunPodClient(apiKey, graphqlEndpoint, restBaseURL)
	return r.client
}

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
	var apiKey string
	var endpoint string
	
	if r.client != nil {
		apiKey = r.client.APIKey
		endpoint = r.client.RestBaseURL
	} else {
		apiKey = os.Getenv("RUNPOD_API_KEY")
		endpoint = os.Getenv("RUNPOD_BASE_URL")
		if endpoint == "" {
			endpoint = "https://rest.runpod.io/v1"
		}
	}
	
	if apiKey == "" {
		resp.Diagnostics.AddError("API Error", "RUNPOD_API_KEY environment variable must be set. Get your API key from https://runpod.io/console/user/settings")
		return
	}

	url := endpoint + "/pods"

  // Build the REST API request body (v2 format with nested objects)
	body := map[string]interface{}{
		"scheduling": map[string]interface{}{
			"gpuCount": int64(config.GpuCount.ValueInt64()),
		},
		"name": config.Name.ValueString(),
	}

	if hasTemplateId {
		body["templateId"] = config.TemplateId.ValueString()
	} else {
		// v2 format: image in container object
		if _, ok := body["container"]; !ok {
			body["container"] = make(map[string]interface{})
		}
		body["container"].(map[string]interface{})["image"] = config.ImageName.ValueString()
	}

	if !config.CloudType.IsNull() && config.CloudType.ValueString() != "" {
		if _, ok := body["scheduling"]; !ok {
			body["scheduling"] = make(map[string]interface{})
		}
		body["scheduling"].(map[string]interface{})["cloudType"] = config.CloudType.ValueString()
	}

	// Add type field (v2 requires explicit ON_DEMAND or SPOT)
	if !config.Interruptible.IsNull() && config.Interruptible.ValueBool() {
		body["type"] = "SPOT"
	} else {
		body["type"] = "ON_DEMAND"
	}

	// Add scheduling fields
	if _, ok := body["scheduling"]; !ok {
		body["scheduling"] = make(map[string]interface{})
	}
	scheduling := body["scheduling"].(map[string]interface{})

	if !config.GpuTypeId.IsNull() && config.GpuTypeId.ValueString() != "" {
		scheduling["gpuTypeId"] = config.GpuTypeId.ValueString()
	}

	if !config.BidPerGpu.IsNull() && config.BidPerGpu.ValueFloat64() > 0 {
		scheduling["bidPerGpu"] = config.BidPerGpu.ValueFloat64()
	}

	if config.VolumeInGb.ValueFloat64() > 0 {
		if _, ok := body["storage"]; !ok {
			body["storage"] = make(map[string]interface{})
		}
		body["storage"].(map[string]interface{})["volumeSizeInGb"] = int64(config.VolumeInGb.ValueFloat64())
	}

	if !config.NetworkVolumeId.IsNull() && config.NetworkVolumeId.ValueString() != "" {
		if _, ok := body["storage"]; !ok {
			body["storage"] = make(map[string]interface{})
		}
		body["storage"].(map[string]interface{})["networkVolumeId"] = config.NetworkVolumeId.ValueString()
	}

	if !config.ContainerDiskInGb.IsNull() {
		if _, ok := body["container"]; !ok {
			body["container"] = make(map[string]interface{})
		}
		body["container"].(map[string]interface{})["diskInGb"] = int64(config.ContainerDiskInGb.ValueInt64())
	}

	// Handle ports - convert from string format to v2 array format
	if !config.Ports.IsNull() && config.Ports.ValueString() != "" {
		portsArray := make([]map[string]interface{}, 0)
		portsStr := config.Ports.ValueString()
		ports := strings.Split(portsStr, ",")
		for _, p := range ports {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				// Parse "8888/http" format
				parts := strings.Split(trimmed, "/")
				if len(parts) == 2 {
					portsArray = append(portsArray, map[string]interface{}{
						"port": parts[0],
						"type": parts[1],
					})
				} else {
					// Try "8888 http" format (space separated)
					parts = strings.Split(trimmed, " ")
					if len(parts) == 2 {
						portsArray = append(portsArray, map[string]interface{}{
							"port": parts[0],
							"type": parts[1],
						})
					}
				}
			}
		}
		if len(portsArray) > 0 {
			body["ports"] = portsArray
		}
	}

	if !config.VolumeMountPath.IsNull() && config.VolumeMountPath.ValueString() != "" {
		if _, ok := body["storage"]; !ok {
			body["storage"] = make(map[string]interface{})
		}
		body["storage"].(map[string]interface{})["volumeMountPath"] = config.VolumeMountPath.ValueString()
	}

	if !config.Env.IsNull() && len(config.Env.Elements()) > 0 {
		// v2 format: array of key/value objects in container.env
		envArray := make([]map[string]interface{}, 0)
		for _, element := range config.Env.Elements() {
			if elementStr, ok := element.(types.String); ok {
				parts := strings.SplitN(elementStr.ValueString(), "=", 2)
				if len(parts) == 2 {
					envArray = append(envArray, map[string]interface{}{
						"key":   parts[0],
						"value": parts[1],
					})
				}
			}
		}
		if len(envArray) > 0 {
			if _, ok := body["container"]; !ok {
				body["container"] = make(map[string]interface{})
			}
			body["container"].(map[string]interface{})["env"] = envArray
		}
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

	// Parse the response (v2 format with envelope)
	var envelope map[string]interface{}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	// Extract pod from data.pod (v2 envelope)
	var result map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if pod, ok := data["pod"].(map[string]interface{}); ok {
			result = pod
		} else if pods, ok := data["pods"].([]interface{}); ok && len(pods) > 0 {
			// For list operations
			result = pods[0].(map[string]interface{})
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to extract pod from response data")
			return
		}
	} else {
		// Fallback for v1 format (backward compatibility)
		result = envelope
	}

	// Extract the pod ID from response
	if podID, ok := result["id"].(string); ok {
		config.Id = types.StringValue(podID)
	} else {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get pod ID from response: %v", result))
		return
	}

	// Extract pod type (v2 required field)
	if podType, ok := result["type"].(string); ok {
		config.Type = types.StringValue(podType)
	}

	// Handle deprecated fields for backward compatibility
	// v1 uses "interruptible" boolean, v2 uses "type" field
	// If v2 type is SPOT, set interruptible to true for backward compat
	if config.Interruptible.IsNull() {
		if config.Type.ValueString() == "SPOT" {
			config.Interruptible = types.BoolValue(true)
		} else {
			config.Interruptible = types.BoolValue(false)
		}
	}

	if config.StartSsh.IsNull() {
		config.StartSsh = types.BoolValue(false)
	}
	if config.StartJupyter.IsNull() {
		config.StartJupyter = types.BoolValue(false)
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

	var apiKey string
	var endpoint string
	
	if r.client != nil {
		apiKey = r.client.APIKey
		endpoint = r.client.RestBaseURL
	} else {
		apiKey = os.Getenv("RUNPOD_API_KEY")
		endpoint = os.Getenv("RUNPOD_BASE_URL")
		if endpoint == "" {
			endpoint = "https://rest.runpod.io/v1"
		}
	}
	
	if apiKey == "" {
		resp.Diagnostics.AddError("API Error", "RUNPOD_API_KEY environment variable must be set")
		return
	}

	url := endpoint + "/pods/" + state.Id.ValueString()

	reqHTTP, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

	if respHTTP.StatusCode == 404 {
		resp.State.RemoveResource(ctx)
		return
	}

	respBody, err := io.ReadAll(respHTTP.Body)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to read response: %v", err))
		return
	}

	// Parse the response (v2 format with envelope)
	var envelope map[string]interface{}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	// Extract pod from data.pod (v2 envelope)
	var result map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if pod, ok := data["pod"].(map[string]interface{}); ok {
			result = pod
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to extract pod from v2 response data")
			return
		}
	} else {
		// Fallback for v1 format (backward compatibility)
		result = envelope
	}

	if result == nil {
		resp.Diagnostics.AddError("API Error", "Empty response from API")
		return
	}

	// Extract pod type (v2 required field)
	if podType, ok := result["type"].(string); ok {
		state.Type = types.StringValue(podType)
	}

	if val, ok := result["desiredStatus"].(string); ok && val != "" {
		state.Status = types.StringValue(val)
	}
	if val, ok := result["createdAt"].(string); ok && val != "" {
		state.CreatedAt = types.StringValue(val)
	}
	if val, ok := result["machineId"].(string); ok && val != "" {
		state.MachineId = types.StringValue(val)
	}
	if val, ok := result["costPerHr"].(float64); ok {
		state.CostPerHr = types.Float64Value(val)
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

	if machine, ok := result["machine"].(map[string]interface{}); ok {
		if val, ok := machine["gpuTypeId"].(string); ok && val != "" {
			state.GpuTypeId = types.StringValue(val)
		}
		if v, ok := machine["secureCloud"].(bool); ok {
			if v {
				state.CloudType = types.StringValue("SECURE")
			} else {
				state.CloudType = types.StringValue("COMMUNITY")
			}
		}
	}

	if result["cloudType"] != nil {
		if val, ok := result["cloudType"].(string); ok && val != "" {
			state.CloudType = types.StringValue(val)
		}
	}

	if result["networkVolume"] != nil {
		if nv, ok := result["networkVolume"].(map[string]interface{}); ok {
			if id, ok := nv["id"].(string); ok && id != "" {
				state.NetworkVolumeId = types.StringValue(id)
			}
		}
	}

	if val, ok := result["dockerEntrypoint"].([]interface{}); ok {
		entrypointList := make([]attr.Value, 0)
		for _, v := range val {
			if vStr, ok := v.(string); ok {
				entrypointList = append(entrypointList, types.StringValue(vStr))
			}
		}
		if len(entrypointList) > 0 {
			state.DockerEntrypoint, diags = types.ListValue(types.StringType, entrypointList)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
		}
	}

	if val, ok := result["dockerStartCmd"].([]interface{}); ok {
		startCmdList := make([]attr.Value, 0)
		for _, v := range val {
			if vStr, ok := v.(string); ok {
				startCmdList = append(startCmdList, types.StringValue(vStr))
			}
		}
		if len(startCmdList) > 0 {
			state.DockerStartCmd, diags = types.ListValue(types.StringType, startCmdList)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
		}
	}

	if val, ok := result["interruptible"].(bool); ok {
		state.Interruptible = types.BoolValue(val)
	}

	if val, ok := result["volumeEncrypted"].(bool); ok {
		state.VolumeEncrypted = types.BoolValue(val)
	}

	diags = resp.State.Set(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *PodResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state PodModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var config PodModel
	diags = req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var apiKey string
	var endpoint string
	
	if r.client != nil {
		apiKey = r.client.APIKey
		endpoint = r.client.RestBaseURL
	} else {
		apiKey = os.Getenv("RUNPOD_API_KEY")
		endpoint = os.Getenv("RUNPOD_BASE_URL")
		if endpoint == "" {
			endpoint = "https://rest.runpod.io/v1"
		}
	}
	if apiKey == "" {
		resp.Diagnostics.AddError("API Error", "RUNPOD_API_KEY environment variable must be set")
		return
	}

	url := endpoint + "/pods/" + state.Id.ValueString()

	body := map[string]interface{}{}

	if !config.Name.IsNull() && config.Name.ValueString() != state.Name.ValueString() {
		body["name"] = config.Name.ValueString()
	}

	if !config.Env.IsNull() {
		envMap := make(map[string]interface{})
		for _, element := range config.Env.Elements() {
			if elementStr, ok := element.(types.String); ok {
				parts := strings.SplitN(elementStr.ValueString(), "=", 2)
				if len(parts) == 2 {
					envMap[parts[0]] = parts[1]
				}
			}
		}
		if len(envMap) > 0 {
			body["env"] = envMap
		}
	}

	if !config.Ports.IsNull() && config.Ports.ValueString() != state.Ports.ValueString() {
		portsStr := config.Ports.ValueString()
		if portsStr != "" {
			portsArray := strings.Split(portsStr, ",")
			portsArrayCleaned := make([]string, 0)
			for _, p := range portsArray {
				trimmed := strings.TrimSpace(p)
				if trimmed != "" {
					portsArrayCleaned = append(portsArrayCleaned, trimmed)
				}
			}
			if len(portsArrayCleaned) > 0 {
				body["ports"] = portsArrayCleaned
			}
		}
	}

	if !config.VolumeInGb.IsNull() && config.VolumeInGb.ValueFloat64() != state.VolumeInGb.ValueFloat64() {
		body["volumeInGb"] = int64(config.VolumeInGb.ValueFloat64())
	}

	if !config.VolumeMountPath.IsNull() && config.VolumeMountPath.ValueString() != state.VolumeMountPath.ValueString() {
		body["volumeMountPath"] = config.VolumeMountPath.ValueString()
	}

	if !config.ContainerDiskInGb.IsNull() && config.ContainerDiskInGb.ValueInt64() != state.ContainerDiskInGb.ValueInt64() {
		body["containerDiskInGb"] = int64(config.ContainerDiskInGb.ValueInt64())
	}

	if len(body) == 0 {
		diags = resp.State.Set(ctx, &config)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
		}
		return
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to marshal request body: %v", err))
		return
	}

	reqHTTP, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewBuffer(jsonBody))
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

	// Parse the response (v2 format with envelope)
	var envelope map[string]interface{}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	// Extract pod from data.pod (v2 envelope)
	var result map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if pod, ok := data["pod"].(map[string]interface{}); ok {
			result = pod
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to extract pod from v2 response data")
			return
		}
	} else {
		// Fallback for v1 format (backward compatibility)
		result = envelope
	}

	if respHTTP.StatusCode != 200 && respHTTP.StatusCode != 201 {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to update pod (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	if id, ok := result["id"].(string); ok {
		config.Id = types.StringValue(id)
	} else {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get pod ID from update response: %v", result))
		return
	}

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *PodResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PodModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var apiKey string
	var endpoint string
	
	if r.client != nil {
		apiKey = r.client.APIKey
		endpoint = r.client.RestBaseURL
	} else {
		apiKey = os.Getenv("RUNPOD_API_KEY")
		endpoint = os.Getenv("RUNPOD_BASE_URL")
		if endpoint == "" {
			endpoint = "https://rest.runpod.io/v1"
		}
	}
	if apiKey == "" {
		resp.Diagnostics.AddError("API Error", "RUNPOD_API_KEY environment variable must be set")
		return
	}

	url := endpoint + "/pods/" + state.Id.ValueString()

	reqHTTP, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
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
