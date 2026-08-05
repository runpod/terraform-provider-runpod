package resource_template

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

func NewTemplateResource() resource.Resource {
	return &TemplateResource{}
}

type TemplateResource struct {
	rlClient *client.RunPodClient
}

func (r *TemplateResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TemplateResource) getClient() *client.RunPodClient {
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

func (r *TemplateResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "runpod_template"
}

func (r *TemplateResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = TemplateResourceSchema(ctx)
}

func (r *TemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config TemplateModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	rlClient := r.getClient()

	url := rlClient.BaseURL() + "/templates"

	body := map[string]interface{}{
		"name":   config.Name.ValueString(),
		"image":  config.ImageName.ValueString(),
	}

	if !config.Category.IsNull() && config.Category.ValueString() != "" {
		body["category"] = config.Category.ValueString()
	}

	if !config.ContainerDiskInGb.IsNull() && config.ContainerDiskInGb.ValueInt64() > 0 {
		body["disk"] = int64(config.ContainerDiskInGb.ValueInt64())
	}

	if !config.ContainerRegistryAuthId.IsNull() && config.ContainerRegistryAuthId.ValueString() != "" {
		body["containerRegistryAuthId"] = config.ContainerRegistryAuthId.ValueString()
	}

	if !config.DockerEntrypoint.IsNull() && len(config.DockerEntrypoint.Elements()) > 0 {
		entrypoint := make([]string, 0)
		for _, val := range config.DockerEntrypoint.Elements() {
			if strVal, ok := val.(types.String); ok {
				entrypoint = append(entrypoint, strVal.ValueString())
			}
		}
		if len(entrypoint) > 0 {
			body["dockerEntrypoint"] = entrypoint
		}
	}

	if !config.DockerStartCmd.IsNull() && len(config.DockerStartCmd.Elements()) > 0 {
		startCmd := make([]string, 0)
		for _, val := range config.DockerStartCmd.Elements() {
			if strVal, ok := val.(types.String); ok {
				startCmd = append(startCmd, strVal.ValueString())
			}
		}
		if len(startCmd) > 0 {
			body["dockerStartCmd"] = startCmd
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

	if !config.IsPublic.IsNull() {
		body["public"] = config.IsPublic.ValueBool()
	}

	if !config.IsServerless.IsNull() {
		body["serverless"] = config.IsServerless.ValueBool()
	}

	if !config.Ports.IsNull() && len(config.Ports.Elements()) > 0 {
		ports := make([]string, 0)
		for _, val := range config.Ports.Elements() {
			if strVal, ok := val.(types.String); ok {
				ports = append(ports, strVal.ValueString())
			}
		}
		if len(ports) > 0 {
			body["ports"] = ports
		}
	}

	if !config.Readme.IsNull() && config.Readme.ValueString() != "" {
		body["readme"] = config.Readme.ValueString()
	}

	if !config.VolumeInGb.IsNull() && config.VolumeInGb.ValueInt64() > 0 {
		mounts := make([]map[string]interface{}, 0)
		mount := map[string]interface{}{
			"volumeInGb": int64(config.VolumeInGb.ValueInt64()),
		}
		if !config.VolumeMountPath.IsNull() && config.VolumeMountPath.ValueString() != "" {
			mount["volumeMountPath"] = config.VolumeMountPath.ValueString()
		}
		mounts = append(mounts, mount)
		body["mounts"] = mounts
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

	var envelope map[string]interface{}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	var result map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if template, ok := data["template"].(map[string]interface{}); ok {
			result = template
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to extract template from v2 response data")
			return
		}
	} else {
		result = envelope
	}

	if respHTTP.StatusCode != 200 && respHTTP.StatusCode != 201 {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create template (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	if id, ok := result["id"].(string); ok {
		config.Id = types.StringValue(id)
	} else {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get template ID from response: %v", result))
		return
	}

	if val, ok := result["name"].(string); ok {
		config.Name = types.StringValue(val)
	}
	if val, ok := result["image"].(string); ok {
		config.ImageName = types.StringValue(val)
	}
	if val, ok := result["category"].(string); ok {
		config.Category = types.StringValue(val)
	}
	if val, ok := result["disk"].(float64); ok {
		config.ContainerDiskInGb = types.Int64Value(int64(val))
	}
	if val, ok := result["containerRegistryAuthId"].(string); ok {
		config.ContainerRegistryAuthId = types.StringValue(val)
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

	if val, ok := result["public"].(bool); ok {
		config.IsPublic = types.BoolValue(val)
	}

	if val, ok := result["serverless"].(bool); ok {
		config.IsServerless = types.BoolValue(val)
	}

	if val, ok := result["readme"].(string); ok {
		config.Readme = types.StringValue(val)
	}

	if result["volumeInGb"] != nil {
		if val, ok := result["volumeInGb"].(float64); ok {
			config.VolumeInGb = types.Int64Value(int64(val))
		}
	}

	if val, ok := result["volumeMountPath"].(string); ok {
		config.VolumeMountPath = types.StringValue(val)
	}

	if val, ok := result["earned"].(float64); ok {
		config.Earned = types.Float64Value(val)
	}

	if val, ok := result["isRunpod"].(bool); ok {
		config.IsRunpod = types.BoolValue(val)
	}

	if val, ok := result["runtimeInMin"].(float64); ok {
		config.RuntimeInMin = types.Int64Value(int64(val))
	}

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *TemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state TemplateModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	rlClient := r.getClient()

	url := rlClient.GetTemplateURL(state.Id.ValueString())

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

	var envelope map[string]interface{}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	var result map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if template, ok := data["template"].(map[string]interface{}); ok {
			result = template
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to extract template from v2 response data")
			return
		}
	} else {
		result = envelope
	}

	if result == nil {
		resp.Diagnostics.AddError("API Error", "Empty response from API")
		return
	}

	if val, ok := result["name"].(string); ok {
		state.Name = types.StringValue(val)
	}
	if val, ok := result["image"].(string); ok {
		state.ImageName = types.StringValue(val)
	}
	if val, ok := result["category"].(string); ok {
		state.Category = types.StringValue(val)
	}
	if val, ok := result["disk"].(float64); ok {
		state.ContainerDiskInGb = types.Int64Value(int64(val))
	}
	if val, ok := result["containerRegistryAuthId"].(string); ok {
		state.ContainerRegistryAuthId = types.StringValue(val)
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

	if val, ok := result["public"].(bool); ok {
		state.IsPublic = types.BoolValue(val)
	}

	if val, ok := result["serverless"].(bool); ok {
		state.IsServerless = types.BoolValue(val)
	}

	if val, ok := result["readme"].(string); ok {
		state.Readme = types.StringValue(val)
	}

	if result["volumeInGb"] != nil {
		if val, ok := result["volumeInGb"].(float64); ok {
			state.VolumeInGb = types.Int64Value(int64(val))
		}
	}

	if val, ok := result["volumeMountPath"].(string); ok {
		state.VolumeMountPath = types.StringValue(val)
	}

	if val, ok := result["earned"].(float64); ok {
		state.Earned = types.Float64Value(val)
	}

	if val, ok := result["isRunpod"].(bool); ok {
		state.IsRunpod = types.BoolValue(val)
	}

	if val, ok := result["runtimeInMin"].(float64); ok {
		state.RuntimeInMin = types.Int64Value(int64(val))
	}

	diags = resp.State.Set(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *TemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state TemplateModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var config TemplateModel
	diags = req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	rlClient := r.getClient()

	url := rlClient.GetTemplateURL(state.Id.ValueString())

	body := map[string]interface{}{}

	if !config.Name.IsNull() && config.Name.ValueString() != state.Name.ValueString() {
		body["name"] = config.Name.ValueString()
	}

	if !config.ImageName.IsNull() && config.ImageName.ValueString() != state.ImageName.ValueString() {
		body["image"] = config.ImageName.ValueString()
	}

	if !config.Category.IsNull() && config.Category.ValueString() != state.Category.ValueString() {
		body["category"] = config.Category.ValueString()
	}

	if !config.ContainerDiskInGb.IsNull() && config.ContainerDiskInGb.ValueInt64() != state.ContainerDiskInGb.ValueInt64() {
		body["disk"] = int64(config.ContainerDiskInGb.ValueInt64())
	}

	if !config.ContainerRegistryAuthId.IsNull() && config.ContainerRegistryAuthId.ValueString() != state.ContainerRegistryAuthId.ValueString() {
		body["containerRegistryAuthId"] = config.ContainerRegistryAuthId.ValueString()
	}

	if !config.IsPublic.IsNull() && config.IsPublic.ValueBool() != state.IsPublic.ValueBool() {
		body["public"] = config.IsPublic.ValueBool()
	}

	if !config.IsServerless.IsNull() && config.IsServerless.ValueBool() != state.IsServerless.ValueBool() {
		body["serverless"] = config.IsServerless.ValueBool()
	}

	if !config.Readme.IsNull() && config.Readme.ValueString() != state.Readme.ValueString() {
		body["readme"] = config.Readme.ValueString()
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

	if !config.DockerEntrypoint.IsNull() && len(config.DockerEntrypoint.Elements()) > 0 {
		entrypoint := make([]string, 0)
		for _, val := range config.DockerEntrypoint.Elements() {
			if strVal, ok := val.(types.String); ok {
				entrypoint = append(entrypoint, strVal.ValueString())
			}
		}
		if len(entrypoint) > 0 {
			body["dockerEntrypoint"] = entrypoint
		}
	}

	if !config.DockerStartCmd.IsNull() && len(config.DockerStartCmd.Elements()) > 0 {
		startCmd := make([]string, 0)
		for _, val := range config.DockerStartCmd.Elements() {
			if strVal, ok := val.(types.String); ok {
				startCmd = append(startCmd, strVal.ValueString())
			}
		}
		if len(startCmd) > 0 {
			body["dockerStartCmd"] = startCmd
		}
	}

	if !config.Ports.IsNull() && len(config.Ports.Elements()) > 0 {
		ports := make([]string, 0)
		for _, val := range config.Ports.Elements() {
			if strVal, ok := val.(types.String); ok {
				ports = append(ports, strVal.ValueString())
			}
		}
		if len(ports) > 0 {
			body["ports"] = ports
		}
	}

	if !config.VolumeInGb.IsNull() && config.VolumeInGb.ValueInt64() != state.VolumeInGb.ValueInt64() {
		mounts := make([]map[string]interface{}, 0)
		mount := map[string]interface{}{
			"volumeInGb": int64(config.VolumeInGb.ValueInt64()),
		}
		if !config.VolumeMountPath.IsNull() && config.VolumeMountPath.ValueString() != "" {
			mount["volumeMountPath"] = config.VolumeMountPath.ValueString()
		}
		mounts = append(mounts, mount)
		body["mounts"] = mounts
	}

	if len(body) == 0 {
		// No fields to update, just read the full response to get all fields including computed ones
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
			if template, ok := data["template"].(map[string]interface{}); ok {
				result = template
			} else {
				resp.Diagnostics.AddError("API Error", "Failed to extract template from v2 response data")
				return
			}
		} else {
			result = envelope
		}

		if result == nil {
			resp.Diagnostics.AddError("API Error", "Empty response from API")
			return
		}

		config.Id = state.Id
		config.Name = state.Name
		config.ImageName = state.ImageName
		config.Category = state.Category
		config.ContainerDiskInGb = state.ContainerDiskInGb
		config.ContainerRegistryAuthId = state.ContainerRegistryAuthId
		config.DockerEntrypoint = state.DockerEntrypoint
		config.DockerStartCmd = state.DockerStartCmd
		config.Env = state.Env
		config.IsPublic = state.IsPublic
		config.IsServerless = state.IsServerless
		config.Ports = state.Ports
		config.Readme = state.Readme
		config.VolumeInGb = state.VolumeInGb
		config.VolumeMountPath = state.VolumeMountPath
		config.Earned = state.Earned
		config.IsRunpod = state.IsRunpod
		config.RuntimeInMin = state.RuntimeInMin

		// Parse computed fields from response
		if val, ok := result["earned"].(float64); ok {
			config.Earned = types.Float64Value(val)
		}

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

	var envelope map[string]interface{}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	var result map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if template, ok := data["template"].(map[string]interface{}); ok {
			result = template
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to extract template from v2 response data")
			return
		}
	} else {
		result = envelope
	}

	if respHTTP.StatusCode != 200 && respHTTP.StatusCode != 201 {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to update template (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	if id, ok := result["id"].(string); ok {
		config.Id = types.StringValue(id)
	} else {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get template ID from update response: %v", result))
		return
	}

	// Parse computed fields from response
	if val, ok := result["earned"].(float64); ok {
		config.Earned = types.Float64Value(val)
	}
	if val, ok := result["isRunpod"].(bool); ok {
		config.IsRunpod = types.BoolValue(val)
	}
	if val, ok := result["runtimeInMin"].(float64); ok {
		config.RuntimeInMin = types.Int64Value(int64(val))
	}

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *TemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state TemplateModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	rlClient := r.getClient()

	url := rlClient.GetTemplateURL(state.Id.ValueString())

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
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to delete template (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}
}
