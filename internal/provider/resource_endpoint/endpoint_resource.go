package resource_endpoint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewEndpointResource() resource.Resource {
	return &EndpointResource{}
}

type EndpointResource struct {
	rlClient *client.RunPodClient
}

func (r *EndpointResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		if clientWrapper, ok := req.ProviderData.(*client.RunPodClientWrapper); ok {
			r.rlClient = &client.RunPodClient{
				APIKey:      clientWrapper.APIKey,
				GraphQLEndpoint: "https://api.runpod.io/graphql",
				RestBaseURL: clientWrapper.RestBaseURL,
				Client: &http.Client{Timeout: 60 * time.Second},
			}
		} else if client, ok := req.ProviderData.(*client.RunPodClient); ok {
			r.rlClient = client
		}
	}
}

func (r *EndpointResource) getClient() *client.RunPodClient {
	if r.rlClient != nil {
		return r.rlClient
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	endpoint := os.Getenv("RUNPOD_GRAPHQL_URL")
	if endpoint == "" {
		endpoint = "https://api.runpod.io/graphql"
	}
	baseURL := os.Getenv("RUNPOD_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.runpod.io"
	}
	r.rlClient = client.NewRunPodClient(apiKey, endpoint, baseURL)
	return r.rlClient
}

func (r *EndpointResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "runpod_endpoint"
}

// resolveGpuPool maps a GPU type id (e.g. "NVIDIA A100-SXM-80GB") to the
// serverless GPU pool id the v2 endpoint API expects (e.g. "AMPERE_80"),
// via GET /v2/catalog/gpus. If the input is already a pool id it passes through.
func resolveGpuPool(ctx context.Context, c *client.RunPodClient, gpuTypeId string) (string, error) {
	url := c.BaseURL() + "/catalog/gpus"
	reqHTTP, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create catalog request: %v", err)
	}
	reqHTTP.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))
	respHTTP, err := client.DefaultHTTPClient.Do(reqHTTP)
	if err != nil {
		return "", fmt.Errorf("failed to fetch GPU catalog: %v", err)
	}
	defer respHTTP.Body.Close()
	if respHTTP.StatusCode != 200 {
		return "", fmt.Errorf("GPU catalog fetch failed (status %d)", respHTTP.StatusCode)
	}
	var envelope struct {
		Gpus []struct {
			Id   string  `json:"id"`
			Pool *string `json:"pool"`
		} `json:"gpus"`
	}
	if err := json.NewDecoder(respHTTP.Body).Decode(&envelope); err != nil {
		return "", fmt.Errorf("failed to parse GPU catalog: %v", err)
	}
	for _, g := range envelope.Gpus {
		if g.Id == gpuTypeId {
			if g.Pool != nil && *g.Pool != "" {
				return *g.Pool, nil
			}
			return "", fmt.Errorf("GPU type %q has no serverless pool; it cannot back an endpoint", gpuTypeId)
		}
	}
	// Not a GPU type id: assume caller passed a pool id directly
	return gpuTypeId, nil
}

// flattenEndpointResponse rewrites the nested v2 endpoint object into the flat
// keys the state-mapping code below was written against (gpuCount, workersMin,
// scalerType, ...). No-op keys already flat.
func flattenEndpointResponse(result map[string]interface{}) {
	if gpu, ok := result["gpu"].(map[string]interface{}); ok {
		if c, ok := gpu["count"].(float64); ok {
			result["gpuCount"] = c
		}
	}
	if workers, ok := result["workers"].(map[string]interface{}); ok {
		if v, ok := workers["min"].(float64); ok {
			result["workersMin"] = v
		}
		if v, ok := workers["max"].(float64); ok {
			result["workersMax"] = v
		}
	}
	if scaling, ok := result["scaling"].(map[string]interface{}); ok {
		if t, ok := scaling["type"].(string); ok {
			result["scalerType"] = t
		}
		if v, ok := scaling["queueDelay"].(float64); ok {
			result["scalerValue"] = v
		} else if v, ok := scaling["requestCount"].(float64); ok {
			result["scalerValue"] = v
		}
	}
	if d, ok := result["disk"].(float64); ok {
		result["containerDiskInGb"] = d
	}
	if t, ok := result["timeout"].(float64); ok {
		result["executionTimeoutMs"] = t
	}
	if nvs, ok := result["networkVolumes"].([]interface{}); ok {
		if _, exists := result["networkVolumeIds"]; !exists && len(nvs) > 0 {
			result["networkVolumeIds"] = nvs
		}
		if _, exists := result["networkVolumeId"]; !exists && len(nvs) > 0 {
			if s, ok := nvs[0].(string); ok {
				result["networkVolumeId"] = s
			}
		}
	}
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

	rlClient := r.getClient()

	// If template_id is provided, fetch the template to get the image
	var image string
	if !config.TemplateId.IsNull() && config.TemplateId.ValueString() != "" {
		templateId := config.TemplateId.ValueString()
		templateUrl := rlClient.GetTemplateURL(templateId)

		reqHTTP, err := http.NewRequestWithContext(ctx, "GET", templateUrl, nil)
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

		respBody, err := io.ReadAll(respHTTP.Body)
		if err != nil {
			resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to read response: %v", err))
			return
		}

		if respHTTP.StatusCode != 200 {
			resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to fetch template (status: %d): %s", respHTTP.StatusCode, string(respBody)))
			return
		}

		var templateResult map[string]interface{}
		if err := json.Unmarshal(respBody, &templateResult); err != nil {
			resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse template response: %v", err))
			return
		}

		if img, ok := templateResult["image"].(string); ok {
			image = img
		} else {
			resp.Diagnostics.AddError("API Error", "Template response missing image field")
			return
		}
	} else {
		image = config.ImageName.ValueString()
	}

	url := rlClient.BaseURL() + "/serverless"

	// v2 requires the endpoint discriminator and a GPU pool (not a GPU type id).
	gpuPools := make([]string, 0)
	if !config.GpuTypeId.IsNull() && config.GpuTypeId.ValueString() != "" {
		pool, err := resolveGpuPool(ctx, rlClient, config.GpuTypeId.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("API Error", err.Error())
			return
		}
		gpuPools = append(gpuPools, pool)
	}

	body := map[string]interface{}{
		"name":  config.Name.ValueString(),
		"image": image,
		"type":  "QUEUE",
		"gpu": map[string]interface{}{
			"pools": gpuPools,
			"count": config.GpuCount.ValueInt64(),
		},
	}

	// workers configuration
	workers := make(map[string]interface{})
	if !config.WorkersMin.IsNull() {
		workers["min"] = int64(config.WorkersMin.ValueInt64())
	}
	if !config.WorkersMax.IsNull() {
		workers["max"] = int64(config.WorkersMax.ValueInt64())
	}
	if len(workers) > 0 {
		body["workers"] = workers
	}

	// scaling is required in v2 and is a discriminated union on type:
	// QUEUE_DELAY wants queueDelay, REQUEST_COUNT wants requestCount.
	// idleTimeout is not accepted at create time.
	scalerType := "QUEUE_DELAY"
	if !config.ScalerType.IsNull() && config.ScalerType.ValueString() != "" {
		scalerType = config.ScalerType.ValueString()
	}
	scalerValue := float64(5)
	if !config.ScalerValue.IsNull() {
		scalerValue = float64(config.ScalerValue.ValueInt64())
	}
	scaling := map[string]interface{}{"type": scalerType}
	switch scalerType {
	case "QUEUE_DELAY":
		scaling["queueDelay"] = scalerValue
	case "REQUEST_COUNT":
		scaling["requestCount"] = scalerValue
	default:
		resp.Diagnostics.AddError("Invalid Configuration", fmt.Sprintf("scaler_type must be QUEUE_DELAY or REQUEST_COUNT, got %q", scalerType))
		return
	}
	body["scaling"] = scaling

	// v2 takes a single networkVolumes array
	networkVolumes := make([]string, 0)
	if !config.NetworkVolumeId.IsNull() && config.NetworkVolumeId.ValueString() != "" {
		networkVolumes = append(networkVolumes, config.NetworkVolumeId.ValueString())
	}
	if !config.NetworkVolumeIds.IsNull() {
		for _, id := range config.NetworkVolumeIds.Elements() {
			if strVal, ok := id.(types.String); ok && strVal.ValueString() != "" {
				networkVolumes = append(networkVolumes, strVal.ValueString())
			}
		}
	}
	if len(networkVolumes) > 0 {
		body["networkVolumes"] = networkVolumes
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
		resp.Diagnostics.AddError("Unsupported in v2", "allowed_cuda_versions is not accepted by the v2 endpoint create API")
		return
	}

	if !config.MinCudaVersion.IsNull() && config.MinCudaVersion.ValueString() != "" {
		resp.Diagnostics.AddError("Unsupported in v2", "min_cuda_version is not accepted by the v2 endpoint create API")
		return
	}

	if !config.ExecutionTimeoutMs.IsNull() {
		body["timeout"] = int64(config.ExecutionTimeoutMs.ValueInt64())
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
	reqHTTP.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rlClient.APIKey))

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
	flattenEndpointResponse(result)

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

	if val, ok := result["gpuTypePriority"].(string); ok {
		config.GpuTypePriority = types.StringValue(val)
	}

	if val, ok := result["cpuFlavorIds"].([]interface{}); ok {
		cpuFlavorIds := make([]attr.Value, 0)
		for _, id := range val {
			if idStr, ok := id.(string); ok {
				cpuFlavorIds = append(cpuFlavorIds, types.StringValue(idStr))
			}
		}
		if len(cpuFlavorIds) > 0 {
			cpuFlavorIdsList, diags := types.ListValue(types.StringType, cpuFlavorIds)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			config.CpuFlavorIds = cpuFlavorIdsList
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

	rlClient := r.getClient()

	url := rlClient.BaseURL() + "/serverless/" + state.Id.ValueString()

	reqHTTP, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
	flattenEndpointResponse(result)

	if result == nil {
		resp.Diagnostics.AddError("API Error", "Empty response from API")
		return
	}

	if val, ok := result["image"].(string); ok {
		state.ImageName = types.StringValue(val)
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

	if val, ok := result["flashboot"].(bool); ok {
		state.Flashboot = types.BoolValue(val)
	}
	if val, ok := result["cloud"].(string); ok {
		state.CloudType = types.StringValue(val)
	}
	if val, ok := result["disk"].(float64); ok {
		state.ContainerDiskInGb = types.Int64Value(int64(val))
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

	rlClient := r.getClient()

	url := rlClient.BaseURL() + "/serverless/" + state.Id.ValueString()

	body := map[string]interface{}{}

	if !config.Name.IsNull() && config.Name.ValueString() != "" {
		body["name"] = config.Name.ValueString()
	}

	if !config.ImageName.IsNull() && config.ImageName.ValueString() != "" {
		body["image"] = config.ImageName.ValueString()
	}

	// v2 takes a GPU pool (not a GPU type id)
	if !config.GpuTypeId.IsNull() && config.GpuTypeId.ValueString() != "" {
		pool, err := resolveGpuPool(ctx, rlClient, config.GpuTypeId.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("API Error", err.Error())
			return
		}
		body["gpu"] = map[string]interface{}{
			"pools": []string{pool},
			"count": config.GpuCount.ValueInt64(),
		}
	}

	// v2 takes a single networkVolumes array
	networkVolumes := make([]string, 0)
	if !config.NetworkVolumeId.IsNull() && config.NetworkVolumeId.ValueString() != "" {
		networkVolumes = append(networkVolumes, config.NetworkVolumeId.ValueString())
	}
	if !config.NetworkVolumeIds.IsNull() {
		for _, id := range config.NetworkVolumeIds.Elements() {
			if strVal, ok := id.(types.String); ok && strVal.ValueString() != "" {
				networkVolumes = append(networkVolumes, strVal.ValueString())
			}
		}
	}
	if len(networkVolumes) > 0 {
		body["networkVolumes"] = networkVolumes
	}

	// workers configuration
	if !config.WorkersMin.IsNull() || !config.WorkersMax.IsNull() {
		workers := make(map[string]interface{})
		if !config.WorkersMin.IsNull() {
			workers["min"] = int64(config.WorkersMin.ValueInt64())
		}
		if !config.WorkersMax.IsNull() {
			workers["max"] = int64(config.WorkersMax.ValueInt64())
		}
		body["workers"] = workers
	}

	// scaling is a discriminated union on type in v2 (queueDelay/requestCount);
	// idleTimeout is not accepted
	if !config.ScalerType.IsNull() || !config.ScalerValue.IsNull() {
		scalerType := "QUEUE_DELAY"
		if !config.ScalerType.IsNull() && config.ScalerType.ValueString() != "" {
			scalerType = config.ScalerType.ValueString()
		}
		scaling := map[string]interface{}{"type": scalerType}
		if !config.ScalerValue.IsNull() {
			switch scalerType {
			case "QUEUE_DELAY":
				scaling["queueDelay"] = config.ScalerValue.ValueInt64()
			case "REQUEST_COUNT":
				scaling["requestCount"] = config.ScalerValue.ValueInt64()
			default:
				resp.Diagnostics.AddError("Invalid Configuration", fmt.Sprintf("scaler_type must be QUEUE_DELAY or REQUEST_COUNT, got %q", scalerType))
				return
			}
		}
		body["scaling"] = scaling
	}

	if !config.ExecutionTimeoutMs.IsNull() {
		body["timeout"] = int64(config.ExecutionTimeoutMs.ValueInt64())
	}

	if !config.ComputeType.IsNull() && config.ComputeType.ValueString() != "" {
		body["computeType"] = config.ComputeType.ValueString()
	}

	if !config.CloudType.IsNull() && config.CloudType.ValueString() != "" {
		body["cloudType"] = config.CloudType.ValueString()
	}

	if !config.ContainerDiskInGb.IsNull() {
		body["containerDiskInGb"] = int64(config.ContainerDiskInGb.ValueInt64())
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
		resp.Diagnostics.AddError("Unsupported in v2", "allowed_cuda_versions is not accepted by the v2 endpoint update API")
		return
	}

	if !config.MinCudaVersion.IsNull() && config.MinCudaVersion.ValueString() != "" {
		resp.Diagnostics.AddError("Unsupported in v2", "min_cuda_version is not accepted by the v2 endpoint update API")
		return
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
	reqHTTP.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rlClient.APIKey))

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
	flattenEndpointResponse(result)

	if respHTTP.StatusCode != 200 && respHTTP.StatusCode != 201 {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to update endpoint (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	// Seed identity from prior state; v2 PATCH responses may omit fields
	config.Id = state.Id

	if val, ok := result["name"].(string); ok {
		config.Name = types.StringValue(val)
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

	if val, ok := result["flashboot"].(bool); ok {
		config.Flashboot = types.BoolValue(val)
	}

	if val, ok := result["gpuTypePriority"].(string); ok {
		config.GpuTypePriority = types.StringValue(val)
	}

	if val, ok := result["cpuFlavorIds"].([]interface{}); ok {
		cpuFlavorIds := make([]attr.Value, 0)
		for _, id := range val {
			if idStr, ok := id.(string); ok {
				cpuFlavorIds = append(cpuFlavorIds, types.StringValue(idStr))
			}
		}
		if len(cpuFlavorIds) > 0 {
			cpuFlavorIdsList, diags := types.ListValue(types.StringType, cpuFlavorIds)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			config.CpuFlavorIds = cpuFlavorIdsList
		}
	}

	diags = resp.State.Set(ctx, &config)
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

	rlClient := r.getClient()

	url := rlClient.BaseURL() + "/serverless/" + state.Id.ValueString()

	reqHTTP, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
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
