package resource_endpoint

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
	"github.com/hashicorp/terraform-plugin-framework/attr"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewEndpointResource() resource.Resource {
	return &EndpointResource{}
}

type EndpointResource struct {
	client *client.RunPodClient
}

func (r *EndpointResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.RunPodClient)
	}
}

func (r *EndpointResource) getClient() *client.RunPodClient {
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

func (r *EndpointResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "runpod_endpoint"
}

func (r *EndpointResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = EndpointResourceSchema(ctx)
}

func (r *EndpointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config EndpointModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	client := r.getClient()

	url := client.RestBaseURL + "/endpoints"

	body := map[string]interface{}{
		"templateId": config.TemplateId.ValueString(),
	}

	if !config.Name.IsNull() && config.Name.ValueString() != "" {
		body["name"] = config.Name.ValueString()
	}

	if !config.NetworkVolumeId.IsNull() && config.NetworkVolumeId.ValueString() != "" {
		body["networkVolumeId"] = config.NetworkVolumeId.ValueString()
	}

	if !config.NetworkVolumeIds.IsNull() && len(config.NetworkVolumeIds.Elements()) > 0 {
		networkVolumeIds := make([]string, 0)
		for _, id := range config.NetworkVolumeIds.Elements() {
			if strVal, ok := id.(types.String); ok {
				networkVolumeIds = append(networkVolumeIds, strVal.ValueString())
			}
		}
		if len(networkVolumeIds) > 0 {
			body["networkVolumeIds"] = networkVolumeIds
		}
	}

	if !config.WorkersMin.IsNull() {
		body["workersMin"] = int64(config.WorkersMin.ValueInt64())
	}

	if !config.WorkersMax.IsNull() {
		body["workersMax"] = int64(config.WorkersMax.ValueInt64())
	}

	if !config.IdleTimeout.IsNull() {
		body["idleTimeout"] = int64(config.IdleTimeout.ValueInt64())
	}

	if !config.ScalerType.IsNull() && config.ScalerType.ValueString() != "" {
		body["scalerType"] = config.ScalerType.ValueString()
	}

	if !config.ScalerValue.IsNull() {
		body["scalerValue"] = int64(config.ScalerValue.ValueInt64())
	}

	if !config.ExecutionTimeoutMs.IsNull() {
		body["executionTimeoutMs"] = int64(config.ExecutionTimeoutMs.ValueInt64())
	}

	if !config.ComputeType.IsNull() && config.ComputeType.ValueString() != "" {
		body["computeType"] = config.ComputeType.ValueString()
	}

	if !config.GpuCount.IsNull() {
		body["gpuCount"] = int64(config.GpuCount.ValueInt64())
	}

	if !config.VcpuCount.IsNull() {
		body["vcpuCount"] = int64(config.VcpuCount.ValueInt64())
	}

	if !config.DataCenterIds.IsNull() && len(config.DataCenterIds.Elements()) > 0 {
		dataCenterIds := make([]string, 0)
		for _, id := range config.DataCenterIds.Elements() {
			if strVal, ok := id.(types.String); ok {
				dataCenterIds = append(dataCenterIds, strVal.ValueString())
			}
		}
		if len(dataCenterIds) > 0 {
			body["dataCenterIds"] = dataCenterIds
		}
	}

	if !config.Env.IsNull() {
		envMap := make(map[string]interface{})
		for key, val := range config.Env.Elements() {
			if strVal, ok := val.(types.String); ok {
				envMap[key] = strVal.ValueString()
			}
		}
		if len(envMap) > 0 {
			body["env"] = envMap
		}
	}

	if !config.AllowedCudaVersions.IsNull() && len(config.AllowedCudaVersions.Elements()) > 0 {
		allowedCudaVersions := make([]interface{}, 0)
		for _, id := range config.AllowedCudaVersions.Elements() {
			if strVal, ok := id.(types.String); ok {
				allowedCudaVersions = append(allowedCudaVersions, strVal.ValueString())
			}
		}
		if len(allowedCudaVersions) > 0 {
			body["allowedCudaVersions"] = allowedCudaVersions
		}
	}

	if !config.MinCudaVersion.IsNull() && config.MinCudaVersion.ValueString() != "" {
		body["minCudaVersion"] = config.MinCudaVersion.ValueString()
	}

	if !config.GpuTypeIds.IsNull() && len(config.GpuTypeIds.Elements()) > 0 {
		gpuTypeIds := make([]interface{}, 0)
		for _, id := range config.GpuTypeIds.Elements() {
			if strVal, ok := id.(types.String); ok {
				gpuTypeIds = append(gpuTypeIds, strVal.ValueString())
			}
		}
		if len(gpuTypeIds) > 0 {
			body["gpuTypeIds"] = gpuTypeIds
		}
	}

	if !config.GpuTypePriority.IsNull() && config.GpuTypePriority.ValueString() != "" {
		body["gpuTypePriority"] = config.GpuTypePriority.ValueString()
	}

	if !config.CpuFlavorIds.IsNull() && len(config.CpuFlavorIds.Elements()) > 0 {
		cpuFlavorIds := make([]interface{}, 0)
		for _, id := range config.CpuFlavorIds.Elements() {
			if strVal, ok := id.(types.String); ok {
				cpuFlavorIds = append(cpuFlavorIds, strVal.ValueString())
			}
		}
		if len(cpuFlavorIds) > 0 {
			body["cpuFlavorIds"] = cpuFlavorIds
		}
	}

	if !config.CpuFlavorPriority.IsNull() && config.CpuFlavorPriority.ValueString() != "" {
		body["cpuFlavorPriority"] = config.CpuFlavorPriority.ValueString()
	}

	if !config.Flashboot.IsNull() {
		body["flashboot"] = config.Flashboot.ValueBool()
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
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create endpoint (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	if id, ok := result["id"].(string); ok {
		config.Id = types.StringValue(id)
	} else {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get endpoint ID from response: %v", result))
		return
	}

	if val, ok := result["templateId"].(string); ok {
		config.TemplateId = types.StringValue(val)
	}
	if val, ok := result["name"].(string); ok {
		config.Name = types.StringValue(val)
	}

	if val, ok := result["userId"].(string); ok {
		users, diags := types.ListValue(types.StringType, []attr.Value{types.StringValue(val)})
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
		config.Users = users
	}

	if val, ok := result["template"].(map[string]interface{}); ok {
		if version, ok := val["version"].(float64); ok {
			config.TemplateVersion = types.Int64Value(int64(version))
		}
	}

	if val, ok := result["version"].(float64); ok {
		config.Version = types.Int64Value(int64(val))
	}

	if val, ok := result["workers"].([]interface{}); ok {
		workers := make([]attr.Value, 0)
		for _, w := range val {
			if workerMap, ok := w.(map[string]interface{}); ok {
				worker := make(map[string]attr.Value)
				if id, ok := workerMap["id"].(string); ok {
					worker["id"] = types.StringValue(id)
				}
				if podId, ok := workerMap["podId"].(string); ok {
					worker["pod_id"] = types.StringValue(podId)
				}
				if status, ok := workerMap["status"].(string); ok {
					worker["status"] = types.StringValue(status)
				}
				if uptimeMs, ok := workerMap["uptimeMs"].(float64); ok {
					worker["uptime_ms"] = types.Int64Value(int64(uptimeMs))
				}
				if startTime, ok := workerMap["startTime"].(string); ok {
					worker["start_time"] = types.StringValue(startTime)
				}
				if lastBusyMs, ok := workerMap["lastBusyMs"].(float64); ok {
					worker["last_busy_ms"] = types.Int64Value(int64(lastBusyMs))
				}
				workerObj, diags := types.ObjectValue(map[string]attr.Type{
					"id":           types.StringType,
					"pod_id":       types.StringType,
					"status":       types.StringType,
					"uptime_ms":    types.Int64Type,
					"start_time":   types.StringType,
					"last_busy_ms": types.Int64Type,
				}, worker)
				if diags.HasError() {
					resp.Diagnostics.Append(diags...)
					return
				}
				workers = append(workers, workerObj)
			}
		}
		if len(workers) > 0 {
			workersList, diags := types.ListValue(types.ObjectType{
				AttrTypes: map[string]attr.Type{
					"id":           types.StringType,
					"pod_id":       types.StringType,
					"status":       types.StringType,
					"uptime_ms":    types.Int64Type,
					"start_time":   types.StringType,
					"last_busy_ms": types.Int64Type,
				},
			}, workers)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			config.Workers = workersList
		}
	}

	if val, ok := result["createdAt"].(string); ok {
		config.CreatedAt = types.StringValue(val)
	}

	if val, ok := result["dataCenterIds"].([]interface{}); ok {
		dataCenterIds := make([]attr.Value, 0)
		for _, id := range val {
			if idStr, ok := id.(string); ok {
				dataCenterIds = append(dataCenterIds, types.StringValue(idStr))
			}
		}
		if len(dataCenterIds) > 0 {
			dataCenterIdsList, diags := types.ListValue(types.StringType, dataCenterIds)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			config.DataCenterIds = dataCenterIdsList
		}
	}

	if val, ok := result["computeType"].(string); ok {
		config.ComputeType = types.StringValue(val)
	}

	if val, ok := result["gpuCount"].(float64); ok {
		config.GpuCount = types.Int64Value(int64(val))
	}

	if val, ok := result["vcpuCount"].(float64); ok {
		config.VcpuCount = types.Int64Value(int64(val))
	}

	if val, ok := result["workersMin"].(float64); ok {
		config.WorkersMin = types.Int64Value(int64(val))
	}

	if val, ok := result["workersMax"].(float64); ok {
		config.WorkersMax = types.Int64Value(int64(val))
	}

	if val, ok := result["idleTimeout"].(float64); ok {
		config.IdleTimeout = types.Int64Value(int64(val))
	}

	if val, ok := result["scalerType"].(string); ok {
		config.ScalerType = types.StringValue(val)
	}

	if val, ok := result["scalerValue"].(float64); ok {
		config.ScalerValue = types.Int64Value(int64(val))
	}

	if val, ok := result["executionTimeoutMs"].(float64); ok {
		config.ExecutionTimeoutMs = types.Int64Value(int64(val))
	}

	if val, ok := result["env"].(map[string]interface{}); ok {
		envMap := make(map[string]attr.Value)
		for key, val := range val {
			if strVal, ok := val.(string); ok {
				envMap[key] = types.StringValue(strVal)
			}
		}
		if len(envMap) > 0 {
			envObj, diags := types.MapValue(types.StringType, envMap)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			config.Env = envObj
		}
	}

	if val, ok := result["networkVolumeId"].(string); ok {
		config.NetworkVolumeId = types.StringValue(val)
	}

	if val, ok := result["networkVolumeIds"].([]interface{}); ok {
		networkVolumeIds := make([]attr.Value, 0)
		for _, id := range val {
			if idStr, ok := id.(string); ok {
				networkVolumeIds = append(networkVolumeIds, types.StringValue(idStr))
			}
		}
		if len(networkVolumeIds) > 0 {
			networkVolumeIdsList, diags := types.ListValue(types.StringType, networkVolumeIds)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			config.NetworkVolumeIds = networkVolumeIdsList
		}
	}

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *EndpointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state EndpointModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	client := r.getClient()

	url := client.RestBaseURL + "/endpoints/" + state.Id.ValueString()

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

	if val, ok := result["templateId"].(string); ok {
		state.TemplateId = types.StringValue(val)
	}
	if val, ok := result["name"].(string); ok {
		state.Name = types.StringValue(val)
	}
	if val, ok := result["userId"].(string); ok {
		users, diags := types.ListValue(types.StringType, []attr.Value{types.StringValue(val)})
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
		state.Users = users
	}
	if val, ok := result["version"].(float64); ok {
		state.Version = types.Int64Value(int64(val))
	}
	if val, ok := result["workers"].([]interface{}); ok {
		workers := make([]attr.Value, 0)
		for _, w := range val {
			if workerMap, ok := w.(map[string]interface{}); ok {
				worker := make(map[string]attr.Value)
				if id, ok := workerMap["id"].(string); ok {
					worker["id"] = types.StringValue(id)
				}
				if podId, ok := workerMap["podId"].(string); ok {
					worker["pod_id"] = types.StringValue(podId)
				}
				if status, ok := workerMap["status"].(string); ok {
					worker["status"] = types.StringValue(status)
				}
				if uptimeMs, ok := workerMap["uptimeMs"].(float64); ok {
					worker["uptime_ms"] = types.Int64Value(int64(uptimeMs))
				}
				if startTime, ok := workerMap["startTime"].(string); ok {
					worker["start_time"] = types.StringValue(startTime)
				}
				if lastBusyMs, ok := workerMap["lastBusyMs"].(float64); ok {
					worker["last_busy_ms"] = types.Int64Value(int64(lastBusyMs))
				}
				workerObj, diags := types.ObjectValue(map[string]attr.Type{
					"id":           types.StringType,
					"pod_id":       types.StringType,
					"status":       types.StringType,
					"uptime_ms":    types.Int64Type,
					"start_time":   types.StringType,
					"last_busy_ms": types.Int64Type,
				}, worker)
				if diags.HasError() {
					resp.Diagnostics.Append(diags...)
					return
				}
				workers = append(workers, workerObj)
			}
		}
		if len(workers) > 0 {
			workersList, diags := types.ListValue(types.ObjectType{
				AttrTypes: map[string]attr.Type{
					"id":           types.StringType,
					"pod_id":       types.StringType,
					"status":       types.StringType,
					"uptime_ms":    types.Int64Type,
					"start_time":   types.StringType,
					"last_busy_ms": types.Int64Type,
				},
			}, workers)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			state.Workers = workersList
		}
	}
	if val, ok := result["createdAt"].(string); ok {
		state.CreatedAt = types.StringValue(val)
	}
	if val, ok := result["dataCenterIds"].([]interface{}); ok {
		dataCenterIds := make([]attr.Value, 0)
		for _, id := range val {
			if idStr, ok := id.(string); ok {
				dataCenterIds = append(dataCenterIds, types.StringValue(idStr))
			}
		}
		if len(dataCenterIds) > 0 {
			dataCenterIdsList, diags := types.ListValue(types.StringType, dataCenterIds)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			state.DataCenterIds = dataCenterIdsList
		}
	}
	if val, ok := result["computeType"].(string); ok {
		state.ComputeType = types.StringValue(val)
	}
	if val, ok := result["gpuCount"].(float64); ok {
		state.GpuCount = types.Int64Value(int64(val))
	}
	if val, ok := result["vcpuCount"].(float64); ok {
		state.VcpuCount = types.Int64Value(int64(val))
	}
	if val, ok := result["workersMin"].(float64); ok {
		state.WorkersMin = types.Int64Value(int64(val))
	}
	if val, ok := result["workersMax"].(float64); ok {
		state.WorkersMax = types.Int64Value(int64(val))
	}
	if val, ok := result["idleTimeout"].(float64); ok {
		state.IdleTimeout = types.Int64Value(int64(val))
	}
	if val, ok := result["scalerType"].(string); ok {
		state.ScalerType = types.StringValue(val)
	}
	if val, ok := result["scalerValue"].(float64); ok {
		state.ScalerValue = types.Int64Value(int64(val))
	}
	if val, ok := result["executionTimeoutMs"].(float64); ok {
		state.ExecutionTimeoutMs = types.Int64Value(int64(val))
	}
	if val, ok := result["env"].(map[string]interface{}); ok {
		envMap := make(map[string]attr.Value)
		for key, val := range val {
			if strVal, ok := val.(string); ok {
				envMap[key] = types.StringValue(strVal)
			}
		}
		if len(envMap) > 0 {
			envObj, diags := types.MapValue(types.StringType, envMap)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			state.Env = envObj
		}
	}
	if val, ok := result["networkVolumeId"].(string); ok {
		state.NetworkVolumeId = types.StringValue(val)
	}
	if val, ok := result["networkVolumeIds"].([]interface{}); ok {
		networkVolumeIds := make([]attr.Value, 0)
		for _, id := range val {
			if idStr, ok := id.(string); ok {
				networkVolumeIds = append(networkVolumeIds, types.StringValue(idStr))
			}
		}
		if len(networkVolumeIds) > 0 {
			networkVolumeIdsList, diags := types.ListValue(types.StringType, networkVolumeIds)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			state.NetworkVolumeIds = networkVolumeIdsList
		}
	}

	diags = resp.State.Set(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *EndpointResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config EndpointModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var state EndpointModel
	diags = req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	client := r.getClient()

	url := client.RestBaseURL + "/endpoints/" + state.Id.ValueString()

	body := map[string]interface{}{}

	if !config.Name.IsNull() && config.Name.ValueString() != "" {
		body["name"] = config.Name.ValueString()
	}

	if !config.NetworkVolumeId.IsNull() && config.NetworkVolumeId.ValueString() != "" {
		body["networkVolumeId"] = config.NetworkVolumeId.ValueString()
	}

	if !config.NetworkVolumeIds.IsNull() && len(config.NetworkVolumeIds.Elements()) > 0 {
		networkVolumeIds := make([]string, 0)
		for _, id := range config.NetworkVolumeIds.Elements() {
			if strVal, ok := id.(types.String); ok {
				networkVolumeIds = append(networkVolumeIds, strVal.ValueString())
			}
		}
		if len(networkVolumeIds) > 0 {
			body["networkVolumeIds"] = networkVolumeIds
		}
	}

	if !config.WorkersMin.IsNull() {
		body["workersMin"] = int64(config.WorkersMin.ValueInt64())
	}

	if !config.WorkersMax.IsNull() {
		body["workersMax"] = int64(config.WorkersMax.ValueInt64())
	}

	if !config.IdleTimeout.IsNull() {
		body["idleTimeout"] = int64(config.IdleTimeout.ValueInt64())
	}

	if !config.ScalerType.IsNull() && config.ScalerType.ValueString() != "" {
		body["scalerType"] = config.ScalerType.ValueString()
	}

	if !config.ScalerValue.IsNull() {
		body["scalerValue"] = int64(config.ScalerValue.ValueInt64())
	}

	if !config.ExecutionTimeoutMs.IsNull() {
		body["executionTimeoutMs"] = int64(config.ExecutionTimeoutMs.ValueInt64())
	}

	if !config.ComputeType.IsNull() && config.ComputeType.ValueString() != "" {
		body["computeType"] = config.ComputeType.ValueString()
	}

	if !config.GpuCount.IsNull() {
		body["gpuCount"] = int64(config.GpuCount.ValueInt64())
	}

	if !config.VcpuCount.IsNull() {
		body["vcpuCount"] = int64(config.VcpuCount.ValueInt64())
	}

	if !config.DataCenterIds.IsNull() && len(config.DataCenterIds.Elements()) > 0 {
		dataCenterIds := make([]string, 0)
		for _, id := range config.DataCenterIds.Elements() {
			if strVal, ok := id.(types.String); ok {
				dataCenterIds = append(dataCenterIds, strVal.ValueString())
			}
		}
		if len(dataCenterIds) > 0 {
			body["dataCenterIds"] = dataCenterIds
		}
	}

	if !config.Env.IsNull() {
		envMap := make(map[string]interface{})
		for key, val := range config.Env.Elements() {
			if strVal, ok := val.(types.String); ok {
				envMap[key] = strVal.ValueString()
			}
		}
		if len(envMap) > 0 {
			body["env"] = envMap
		}
	}

	if !config.AllowedCudaVersions.IsNull() && len(config.AllowedCudaVersions.Elements()) > 0 {
		allowedCudaVersions := make([]interface{}, 0)
		for _, id := range config.AllowedCudaVersions.Elements() {
			if strVal, ok := id.(types.String); ok {
				allowedCudaVersions = append(allowedCudaVersions, strVal.ValueString())
			}
		}
		if len(allowedCudaVersions) > 0 {
			body["allowedCudaVersions"] = allowedCudaVersions
		}
	}

	if !config.MinCudaVersion.IsNull() && config.MinCudaVersion.ValueString() != "" {
		body["minCudaVersion"] = config.MinCudaVersion.ValueString()
	}

	if !config.GpuTypeIds.IsNull() && len(config.GpuTypeIds.Elements()) > 0 {
		gpuTypeIds := make([]interface{}, 0)
		for _, id := range config.GpuTypeIds.Elements() {
			if strVal, ok := id.(types.String); ok {
				gpuTypeIds = append(gpuTypeIds, strVal.ValueString())
			}
		}
		if len(gpuTypeIds) > 0 {
			body["gpuTypeIds"] = gpuTypeIds
		}
	}

	if !config.GpuTypePriority.IsNull() && config.GpuTypePriority.ValueString() != "" {
		body["gpuTypePriority"] = config.GpuTypePriority.ValueString()
	}

	if !config.CpuFlavorIds.IsNull() && len(config.CpuFlavorIds.Elements()) > 0 {
		cpuFlavorIds := make([]interface{}, 0)
		for _, id := range config.CpuFlavorIds.Elements() {
			if strVal, ok := id.(types.String); ok {
				cpuFlavorIds = append(cpuFlavorIds, strVal.ValueString())
			}
		}
		if len(cpuFlavorIds) > 0 {
			body["cpuFlavorIds"] = cpuFlavorIds
		}
	}

	if !config.CpuFlavorPriority.IsNull() && config.CpuFlavorPriority.ValueString() != "" {
		body["cpuFlavorPriority"] = config.CpuFlavorPriority.ValueString()
	}

	if !config.Flashboot.IsNull() {
		body["flashboot"] = config.Flashboot.ValueBool()
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
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to update endpoint (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	if val, ok := result["name"].(string); ok {
		state.Name = types.StringValue(val)
	}

	if val, ok := result["workers"].([]interface{}); ok {
		workers := make([]attr.Value, 0)
		for _, w := range val {
			if workerMap, ok := w.(map[string]interface{}); ok {
				worker := make(map[string]attr.Value)
				if id, ok := workerMap["id"].(string); ok {
					worker["id"] = types.StringValue(id)
				}
				if podId, ok := workerMap["podId"].(string); ok {
					worker["pod_id"] = types.StringValue(podId)
				}
				if status, ok := workerMap["status"].(string); ok {
					worker["status"] = types.StringValue(status)
				}
				if uptimeMs, ok := workerMap["uptimeMs"].(float64); ok {
					worker["uptime_ms"] = types.Int64Value(int64(uptimeMs))
				}
				if startTime, ok := workerMap["startTime"].(string); ok {
					worker["start_time"] = types.StringValue(startTime)
				}
				if lastBusyMs, ok := workerMap["lastBusyMs"].(float64); ok {
					worker["last_busy_ms"] = types.Int64Value(int64(lastBusyMs))
				}
				workerObj, diags := types.ObjectValue(map[string]attr.Type{
					"id":           types.StringType,
					"pod_id":       types.StringType,
					"status":       types.StringType,
					"uptime_ms":    types.Int64Type,
					"start_time":   types.StringType,
					"last_busy_ms": types.Int64Type,
				}, worker)
				if diags.HasError() {
					resp.Diagnostics.Append(diags...)
					return
				}
				workers = append(workers, workerObj)
			}
		}
		if len(workers) > 0 {
			workersList, diags := types.ListValue(types.ObjectType{
				AttrTypes: map[string]attr.Type{
					"id":           types.StringType,
					"pod_id":       types.StringType,
					"status":       types.StringType,
					"uptime_ms":    types.Int64Type,
					"start_time":   types.StringType,
					"last_busy_ms": types.Int64Type,
				},
			}, workers)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			state.Workers = workersList
		}
	}

	if val, ok := result["dataCenterIds"].([]interface{}); ok {
		dataCenterIds := make([]attr.Value, 0)
		for _, id := range val {
			if idStr, ok := id.(string); ok {
				dataCenterIds = append(dataCenterIds, types.StringValue(idStr))
			}
		}
		if len(dataCenterIds) > 0 {
			dataCenterIdsList, diags := types.ListValue(types.StringType, dataCenterIds)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			state.DataCenterIds = dataCenterIdsList
		}
	}

	if val, ok := result["computeType"].(string); ok {
		state.ComputeType = types.StringValue(val)
	}

	if val, ok := result["gpuCount"].(float64); ok {
		state.GpuCount = types.Int64Value(int64(val))
	}

	if val, ok := result["vcpuCount"].(float64); ok {
		state.VcpuCount = types.Int64Value(int64(val))
	}

	if val, ok := result["workersMin"].(float64); ok {
		state.WorkersMin = types.Int64Value(int64(val))
	}

	if val, ok := result["workersMax"].(float64); ok {
		state.WorkersMax = types.Int64Value(int64(val))
	}

	if val, ok := result["idleTimeout"].(float64); ok {
		state.IdleTimeout = types.Int64Value(int64(val))
	}

	if val, ok := result["scalerType"].(string); ok {
		state.ScalerType = types.StringValue(val)
	}

	if val, ok := result["scalerValue"].(float64); ok {
		state.ScalerValue = types.Int64Value(int64(val))
	}

	if val, ok := result["executionTimeoutMs"].(float64); ok {
		state.ExecutionTimeoutMs = types.Int64Value(int64(val))
	}

	if val, ok := result["env"].(map[string]interface{}); ok {
		envMap := make(map[string]attr.Value)
		for key, val := range val {
			if strVal, ok := val.(string); ok {
				envMap[key] = types.StringValue(strVal)
			}
		}
		if len(envMap) > 0 {
			envObj, diags := types.MapValue(types.StringType, envMap)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			state.Env = envObj
		}
	}

	if val, ok := result["networkVolumeId"].(string); ok {
		state.NetworkVolumeId = types.StringValue(val)
	}

	if val, ok := result["networkVolumeIds"].([]interface{}); ok {
		networkVolumeIds := make([]attr.Value, 0)
		for _, id := range val {
			if idStr, ok := id.(string); ok {
				networkVolumeIds = append(networkVolumeIds, types.StringValue(idStr))
			}
		}
		if len(networkVolumeIds) > 0 {
			networkVolumeIdsList, diags := types.ListValue(types.StringType, networkVolumeIds)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			state.NetworkVolumeIds = networkVolumeIdsList
		}
	}

	diags = resp.State.Set(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *EndpointResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state EndpointModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	client := r.getClient()

	url := client.RestBaseURL + "/endpoints/" + state.Id.ValueString()

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
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to delete endpoint (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}
}
