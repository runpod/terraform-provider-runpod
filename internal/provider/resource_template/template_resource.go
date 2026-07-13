package resource_template

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

func NewTemplateResource() resource.Resource {
	return &TemplateResource{}
}

type TemplateResource struct {
	client *client.RunPodClient
}

func (r *TemplateResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*client.RunPodClient)
	}
}

func (r *TemplateResource) getClient() *client.RunPodClient {
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

	client := r.getClient()

	url := client.RestBaseURL + "/templates"

	body := map[string]interface{}{
		"name":       config.Name.ValueString(),
		"imageName":  config.ImageName.ValueString(),
	}

	if !config.Category.IsNull() && config.Category.ValueString() != "" {
		body["category"] = config.Category.ValueString()
	}

	if !config.ContainerDiskInGb.IsNull() && config.ContainerDiskInGb.ValueInt64() > 0 {
		body["containerDiskInGb"] = int64(config.ContainerDiskInGb.ValueInt64())
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
		body["isPublic"] = config.IsPublic.ValueBool()
	}

	if !config.IsServerless.IsNull() {
		body["isServerless"] = config.IsServerless.ValueBool()
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
		body["volumeInGb"] = int64(config.VolumeInGb.ValueInt64())
	}

	if !config.VolumeMountPath.IsNull() && config.VolumeMountPath.ValueString() != "" {
		body["volumeMountPath"] = config.VolumeMountPath.ValueString()
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
	if val, ok := result["imageName"].(string); ok {
		config.ImageName = types.StringValue(val)
	}
	if val, ok := result["category"].(string); ok {
		config.Category = types.StringValue(val)
	}
	if val, ok := result["containerDiskInGb"].(float64); ok {
		config.ContainerDiskInGb = types.Int64Value(int64(val))
	}
	if val, ok := result["containerRegistryAuthId"].(string); ok {
		config.ContainerRegistryAuthId = types.StringValue(val)
	}
	if val, ok := result["dockerEntrypoint"].([]interface{}); ok {
		entrypoint := make([]attr.Value, 0)
		for _, v := range val {
			if vStr, ok := v.(string); ok {
				entrypoint = append(entrypoint, types.StringValue(vStr))
			}
		}
		if len(entrypoint) > 0 {
			entrypointList, diags := types.ListValue(types.StringType, entrypoint)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			config.DockerEntrypoint = entrypointList
		}
	}
	if val, ok := result["dockerStartCmd"].([]interface{}); ok {
		startCmd := make([]attr.Value, 0)
		for _, v := range val {
			if vStr, ok := v.(string); ok {
				startCmd = append(startCmd, types.StringValue(vStr))
			}
		}
		if len(startCmd) > 0 {
			startCmdList, diags := types.ListValue(types.StringType, startCmd)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			config.DockerStartCmd = startCmdList
		}
	}
	if val, ok := result["env"].(map[string]interface{}); ok {
		envMap := make(map[string]attr.Value)
		for key, v := range val {
			if strVal, ok := v.(string); ok {
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
	if val, ok := result["isPublic"].(bool); ok {
		config.IsPublic = types.BoolValue(val)
	}
	if val, ok := result["isServerless"].(bool); ok {
		config.IsServerless = types.BoolValue(val)
	}
	if val, ok := result["ports"].([]interface{}); ok {
		ports := make([]attr.Value, 0)
		for _, v := range val {
			if vStr, ok := v.(string); ok {
				ports = append(ports, types.StringValue(vStr))
			}
		}
		if len(ports) > 0 {
			portsList, diags := types.ListValue(types.StringType, ports)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			config.Ports = portsList
		}
	}
	if val, ok := result["readme"].(string); ok {
		config.Readme = types.StringValue(val)
	}
	if val, ok := result["volumeInGb"].(float64); ok {
		config.VolumeInGb = types.Int64Value(int64(val))
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

	client := r.getClient()

	url := client.RestBaseURL + "/templates/" + state.Id.ValueString()

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

	if val, ok := result["name"].(string); ok {
		state.Name = types.StringValue(val)
	}
	if val, ok := result["imageName"].(string); ok {
		state.ImageName = types.StringValue(val)
	}
	if val, ok := result["category"].(string); ok {
		state.Category = types.StringValue(val)
	}
	if val, ok := result["containerDiskInGb"].(float64); ok {
		state.ContainerDiskInGb = types.Int64Value(int64(val))
	}
	if val, ok := result["containerRegistryAuthId"].(string); ok {
		state.ContainerRegistryAuthId = types.StringValue(val)
	}
	if val, ok := result["dockerEntrypoint"].([]interface{}); ok {
		entrypoint := make([]attr.Value, 0)
		for _, v := range val {
			if vStr, ok := v.(string); ok {
				entrypoint = append(entrypoint, types.StringValue(vStr))
			}
		}
		if len(entrypoint) > 0 {
			entrypointList, diags := types.ListValue(types.StringType, entrypoint)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			state.DockerEntrypoint = entrypointList
		}
	}
	if val, ok := result["dockerStartCmd"].([]interface{}); ok {
		startCmd := make([]attr.Value, 0)
		for _, v := range val {
			if vStr, ok := v.(string); ok {
				startCmd = append(startCmd, types.StringValue(vStr))
			}
		}
		if len(startCmd) > 0 {
			startCmdList, diags := types.ListValue(types.StringType, startCmd)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			state.DockerStartCmd = startCmdList
		}
	}
	if val, ok := result["env"].(map[string]interface{}); ok {
		envMap := make(map[string]attr.Value)
		for key, v := range val {
			if strVal, ok := v.(string); ok {
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
	if val, ok := result["isPublic"].(bool); ok {
		state.IsPublic = types.BoolValue(val)
	}
	if val, ok := result["isServerless"].(bool); ok {
		state.IsServerless = types.BoolValue(val)
	}
	if val, ok := result["ports"].([]interface{}); ok {
		ports := make([]attr.Value, 0)
		for _, v := range val {
			if vStr, ok := v.(string); ok {
				ports = append(ports, types.StringValue(vStr))
			}
		}
		if len(ports) > 0 {
			portsList, diags := types.ListValue(types.StringType, ports)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			state.Ports = portsList
		}
	}
	if val, ok := result["readme"].(string); ok {
		state.Readme = types.StringValue(val)
	}
	if val, ok := result["volumeInGb"].(float64); ok {
		state.VolumeInGb = types.Int64Value(int64(val))
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
	var config TemplateModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var state TemplateModel
	diags = req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	client := r.getClient()

	url := client.RestBaseURL + "/templates/" + state.Id.ValueString()

	body := map[string]interface{}{}

  if !config.Name.IsNull() && config.Name.ValueString() != "" {
    body["name"] = config.Name.ValueString()
  }

  if !config.ImageName.IsNull() && config.ImageName.ValueString() != "" {
    body["imageName"] = config.ImageName.ValueString()
  }

  // NOTE: 'category' is a computed field in the v1 templates PATCH API
  // and cannot be updated. It is included in Create/Read but excluded from Update.

  if !config.ContainerDiskInGb.IsNull() && config.ContainerDiskInGb.ValueInt64() > 0 {
		body["containerDiskInGb"] = int64(config.ContainerDiskInGb.ValueInt64())
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
		body["isPublic"] = config.IsPublic.ValueBool()
	}

	if !config.IsServerless.IsNull() {
		body["isServerless"] = config.IsServerless.ValueBool()
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
		body["volumeInGb"] = int64(config.VolumeInGb.ValueInt64())
	}

	if !config.VolumeMountPath.IsNull() && config.VolumeMountPath.ValueString() != "" {
		body["volumeMountPath"] = config.VolumeMountPath.ValueString()
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
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to update template (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	if val, ok := result["name"].(string); ok {
		state.Name = types.StringValue(val)
	}
	if val, ok := result["imageName"].(string); ok {
		state.ImageName = types.StringValue(val)
	}
	if val, ok := result["category"].(string); ok {
		state.Category = types.StringValue(val)
	}
	if val, ok := result["containerDiskInGb"].(float64); ok {
		state.ContainerDiskInGb = types.Int64Value(int64(val))
	}
	if val, ok := result["containerRegistryAuthId"].(string); ok {
		state.ContainerRegistryAuthId = types.StringValue(val)
	}
	if val, ok := result["dockerEntrypoint"].([]interface{}); ok {
		entrypoint := make([]attr.Value, 0)
		for _, v := range val {
			if vStr, ok := v.(string); ok {
				entrypoint = append(entrypoint, types.StringValue(vStr))
			}
		}
		if len(entrypoint) > 0 {
			entrypointList, diags := types.ListValue(types.StringType, entrypoint)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			state.DockerEntrypoint = entrypointList
		}
	}
	if val, ok := result["dockerStartCmd"].([]interface{}); ok {
		startCmd := make([]attr.Value, 0)
		for _, v := range val {
			if vStr, ok := v.(string); ok {
				startCmd = append(startCmd, types.StringValue(vStr))
			}
		}
		if len(startCmd) > 0 {
			startCmdList, diags := types.ListValue(types.StringType, startCmd)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			state.DockerStartCmd = startCmdList
		}
	}
	if val, ok := result["env"].(map[string]interface{}); ok {
		envMap := make(map[string]attr.Value)
		for key, v := range val {
			if strVal, ok := v.(string); ok {
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
	if val, ok := result["isPublic"].(bool); ok {
		state.IsPublic = types.BoolValue(val)
	}
	if val, ok := result["isServerless"].(bool); ok {
		state.IsServerless = types.BoolValue(val)
	}
	if val, ok := result["ports"].([]interface{}); ok {
		ports := make([]attr.Value, 0)
		for _, v := range val {
			if vStr, ok := v.(string); ok {
				ports = append(ports, types.StringValue(vStr))
			}
		}
		if len(ports) > 0 {
			portsList, diags := types.ListValue(types.StringType, ports)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
			state.Ports = portsList
		}
	}
	if val, ok := result["readme"].(string); ok {
		state.Readme = types.StringValue(val)
	}
	if val, ok := result["volumeInGb"].(float64); ok {
		state.VolumeInGb = types.Int64Value(int64(val))
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

func (r *TemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state TemplateModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	client := r.getClient()

	url := client.RestBaseURL + "/templates/" + state.Id.ValueString()

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
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to delete template (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}
}
