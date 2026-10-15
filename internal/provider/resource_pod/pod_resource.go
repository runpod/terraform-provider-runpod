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
	// The provider passes a *client.RunPodClientWrapper as ResourceData (see
	// main.go), not a *client.RunPodClient. Use the comma-ok form so a type
	// mismatch leaves r.client nil (Create/Read/Update/Delete then fall back to
	// the RUNPOD_API_KEY / RUNPOD_BASE_URL env path) instead of panicking.
	if req.ProviderData != nil {
		if c, ok := req.ProviderData.(*client.RunPodClient); ok {
			r.client = c
		}
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

	url := endpoint + "/pods"

	body := map[string]interface{}{}

	if !config.Name.IsNull() && config.Name.ValueString() != "" {
		body["name"] = config.Name.ValueString()
	}

	if hasTemplateId {
		body["templateId"] = config.TemplateId.ValueString()
	} else if hasImageName {
		// REST v1 expects a FLAT body: imageName (not a nested "image").
		body["imageName"] = config.ImageName.ValueString()
	}

	// gpuCount is a top-level integer (not gpu.count).
	if !config.GpuCount.IsNull() && config.GpuCount.ValueInt64() > 0 {
		body["gpuCount"] = int64(config.GpuCount.ValueInt64())
	}

	// gpuTypeIds is a top-level array of strings (not gpu.id).
	if !config.GpuTypeId.IsNull() && config.GpuTypeId.ValueString() != "" {
		body["gpuTypeIds"] = []string{config.GpuTypeId.ValueString()}
	}

	// cloudType (not "cloud").
	if !config.CloudType.IsNull() && config.CloudType.ValueString() != "" {
		body["cloudType"] = config.CloudType.ValueString()
	}

	if !config.BidPerGpu.IsNull() && config.BidPerGpu.ValueFloat64() > 0 {
		body["bidPerGpu"] = config.BidPerGpu.ValueFloat64()
	}

	// Volume config uses FLAT top-level keys, NOT a nested "mounts" array
	// (the API rejects "mounts" with "Extra input keys provided").
	// Only send each field when actually set.
	if config.VolumeInGb.ValueFloat64() > 0 {
		body["volumeInGb"] = int64(config.VolumeInGb.ValueFloat64())
	}
	if !config.VolumeMountPath.IsNull() && config.VolumeMountPath.ValueString() != "" {
		body["volumeMountPath"] = config.VolumeMountPath.ValueString()
	}
	if !config.NetworkVolumeId.IsNull() && config.NetworkVolumeId.ValueString() != "" {
		body["networkVolumeId"] = config.NetworkVolumeId.ValueString()
	}
	// NOTE: volumeEncrypted and networkVolumeIds (plural) are intentionally NOT
	// sent — they are not top-level keys in the create-pod body schema and would
	// re-trigger "Extra input keys". Confirm before adding either.

	// containerDiskInGb (not "disk").
	if !config.ContainerDiskInGb.IsNull() && config.ContainerDiskInGb.ValueInt64() > 0 {
		body["containerDiskInGb"] = int64(config.ContainerDiskInGb.ValueInt64())
	}

	if !config.Ports.IsNull() && config.Ports.ValueString() != "" {
		portsArray := make([]string, 0)
		portsStr := config.Ports.ValueString()
		ports := strings.Split(portsStr, ",")
		for _, p := range ports {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				portsArray = append(portsArray, trimmed)
			}
		}
		if len(portsArray) > 0 {
			body["ports"] = portsArray
		}
	}

	if !config.Env.IsNull() && len(config.Env.Elements()) > 0 {
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

	if !config.StartSsh.IsNull() {
		body["startSsh"] = config.StartSsh.ValueBool()
	}

	if !config.StartJupyter.IsNull() {
		body["startJupyter"] = config.StartJupyter.ValueBool()
	}

	// Container start command (e.g. ["bash","-c","..."]). The field already
	// existed in the schema/model and was parsed by Read(), but Create() never
	// sent it — this wires it into the request body as dockerStartCmd.
	if !config.DockerStartCmd.IsNull() && len(config.DockerStartCmd.Elements()) > 0 {
		startCmd := make([]string, 0)
		for _, e := range config.DockerStartCmd.Elements() {
			if s, ok := e.(types.String); ok {
				startCmd = append(startCmd, s.ValueString())
			}
		}
		if len(startCmd) > 0 {
			body["dockerStartCmd"] = startCmd
		}
	}

	if !config.DockerEntrypoint.IsNull() && len(config.DockerEntrypoint.Elements()) > 0 {
		entrypoint := make([]string, 0)
		for _, e := range config.DockerEntrypoint.Elements() {
			if s, ok := e.(types.String); ok {
				entrypoint = append(entrypoint, s.ValueString())
			}
		}
		if len(entrypoint) > 0 {
			body["dockerEntrypoint"] = entrypoint
		}
	}

	// Target data center(s). Sent as dataCenterIds (array), matching the
	// convention used by the endpoint resource.
	if !config.DataCenterIds.IsNull() && len(config.DataCenterIds.Elements()) > 0 {
		dcIds := make([]string, 0)
		for _, e := range config.DataCenterIds.Elements() {
			if s, ok := e.(types.String); ok {
				dcIds = append(dcIds, s.ValueString())
			}
		}
		if len(dcIds) > 0 {
			body["dataCenterIds"] = dcIds
		}
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to marshal request body: %v", err))
		return
	}

	fmt.Fprintf(os.Stderr, "[runpod_pod] POST %s body=%s\n", url, string(jsonBody))

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

	var envelope map[string]interface{}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	var result map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if pod, ok := data["pod"].(map[string]interface{}); ok {
			result = pod
		} else if pods, ok := data["pods"].([]interface{}); ok && len(pods) > 0 {
			result = pods[0].(map[string]interface{})
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to extract pod from response data")
			return
		}
	} else {
		result = envelope
	}

	if podID, ok := result["id"].(string); ok {
		config.Id = types.StringValue(podID)
	} else {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get pod ID from response: %v", result))
		return
	}

	if podType, ok := result["type"].(string); ok {
		config.Type = types.StringValue(podType)
	} else {
		config.Type = types.StringValue("")
	}

	if val, ok := result["desiredStatus"].(string); ok && val != "" {
		config.Status = types.StringValue(val)
	}

	if val, ok := result["createdAt"].(string); ok && val != "" {
		config.CreatedAt = types.StringValue(val)
	}

	if val, ok := result["machineId"].(string); ok && val != "" {
		config.MachineId = types.StringValue(val)
	}

	if val, ok := result["costPerHr"].(float64); ok {
		config.CostPerHr = types.Float64Value(val)
	}

	if val, ok := result["memoryInGb"].(float64); ok {
		config.MemoryInGb = types.Float64Value(val)
	}

	if val, ok := result["volumeInGb"].(float64); ok {
		config.VolumeInGb = types.Float64Value(val)
	}

	if val, ok := result["containerDiskInGb"].(float64); ok {
		config.ContainerDiskInGb = types.Int64Value(int64(val))
	}

	if result["templateId"] != nil {
		if val, ok := result["templateId"].(string); ok && val != "" {
			config.TemplateId = types.StringValue(val)
		}
	}

	if machine, ok := result["machine"].(map[string]interface{}); ok {
		if val, ok := machine["gpuTypeId"].(string); ok && val != "" {
			config.GpuTypeId = types.StringValue(val)
		}
		if v, ok := machine["secureCloud"].(bool); ok {
			if v {
				config.CloudType = types.StringValue("SECURE")
			} else {
				config.CloudType = types.StringValue("COMMUNITY")
			}
		}
	}

	if result["cloudType"] != nil {
		if val, ok := result["cloudType"].(string); ok && val != "" {
			config.CloudType = types.StringValue(val)
		}
	}

	if result["networkVolume"] != nil {
		if nv, ok := result["networkVolume"].(map[string]interface{}); ok {
			if id, ok := nv["id"].(string); ok && id != "" {
				config.NetworkVolumeId = types.StringValue(id)
			}
		}
	}

	if result["networkVolumeIds"] != nil {
		if nvIds, ok := result["networkVolumeIds"].([]interface{}); ok {
			nvIdList := make([]attr.Value, 0)
			for _, nv := range nvIds {
				if nvMap, ok := nv.(map[string]interface{}); ok {
					if id, ok := nvMap["id"].(string); ok && id != "" {
						nvIdList = append(nvIdList, types.StringValue(id))
					}
				} else if idStr, ok := nv.(string); ok && idStr != "" {
					nvIdList = append(nvIdList, types.StringValue(idStr))
				}
			}
			if len(nvIdList) > 0 {
				config.NetworkVolumeIds, diags = types.ListValue(types.StringType, nvIdList)
				if diags.HasError() {
					resp.Diagnostics.Append(diags...)
					return
				}
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
			config.DockerEntrypoint, diags = types.ListValue(types.StringType, entrypointList)
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
			config.DockerStartCmd, diags = types.ListValue(types.StringType, startCmdList)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
		}
	}

	if val, ok := result["interruptible"].(bool); ok {
		config.Interruptible = types.BoolValue(val)
	}

	if val, ok := result["volumeEncrypted"].(bool); ok {
		config.VolumeEncrypted = types.BoolValue(val)
	}

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

	var envelope map[string]interface{}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	var result map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if pod, ok := data["pod"].(map[string]interface{}); ok {
			result = pod
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to extract pod from v2 response data")
			return
		}
	} else {
		result = envelope
	}

	if result == nil {
		resp.Diagnostics.AddError("API Error", "Empty response from API")
		return
	}

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

	if result["networkVolumeIds"] != nil {
		if nvIds, ok := result["networkVolumeIds"].([]interface{}); ok {
			nvIdList := make([]attr.Value, 0)
			for _, nv := range nvIds {
				if nvMap, ok := nv.(map[string]interface{}); ok {
					if id, ok := nvMap["id"].(string); ok && id != "" {
						nvIdList = append(nvIdList, types.StringValue(id))
					}
				} else if idStr, ok := nv.(string); ok && idStr != "" {
					nvIdList = append(nvIdList, types.StringValue(idStr))
				}
			}
			if len(nvIdList) > 0 {
				state.NetworkVolumeIds, diags = types.ListValue(types.StringType, nvIdList)
				if diags.HasError() {
					resp.Diagnostics.Append(diags...)
					return
				}
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

	var envelope map[string]interface{}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	var result map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if pod, ok := data["pod"].(map[string]interface{}); ok {
			result = pod
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to extract pod from v2 response data")
			return
		}
	} else {
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
