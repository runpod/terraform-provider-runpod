package datasource_template

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewTemplateDataSource() datasource.DataSource {
	return &TemplateDataSource{}
}

type TemplateDataSource struct {
	rlClient *client.RunPodClient
}

func (d *TemplateDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.rlClient = req.ProviderData.(*client.RunPodClient)
	}
}

func (d *TemplateDataSource) getClient() *client.RunPodClient {
	if d.rlClient != nil {
		return d.rlClient
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	endpoint := os.Getenv("RUNPOD_GRAPHQL_URL")
	if endpoint == "" {
		endpoint = "https://api.runpod.io/graphql"
	}
	baseURL := os.Getenv("RUNPOD_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.runpod.io/v2"
	}
	d.rlClient = client.NewRunPodClient(apiKey, endpoint, baseURL)
	return d.rlClient
}

func (d *TemplateDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_template"
}

func (d *TemplateDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = TemplateDataSourceSchema(ctx)
}

func (d *TemplateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config TemplateModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	rlClient := d.getClient()

	// Use v2 REST endpoint: GET /v2/templates/{templateId}
	url := rlClient.GetTemplateURL(config.Id.ValueString())

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

	// Handle 404 - template not found
	if respHTTP.StatusCode == 404 {
		resp.Diagnostics.AddError("API Error", "Template not found")
		return
	}

	// Read and parse response body
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

	// Handle v2 response envelope: {data: {...}, meta: {...}, error: ...}
	var result map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		// Check for nested template object (some endpoints wrap the response)
		if template, ok := data["template"].(map[string]interface{}); ok {
			result = template
		} else {
			result = data
		}
	} else {
		result = envelope
	}

	if result == nil {
		resp.Diagnostics.AddError("API Error", "Empty response from API")
		return
	}

	// Parse v2 REST response fields
	// Map: templateId->id, name->name, imageName->image, isPublic->public, isServerless->serverless,
	//      containerDiskInGb->disk, volumeInGb->volumeInGb, volumeMountPath->mountPath,
	//      dockerArgs->args, dockerStartCmd->cmd, dockerEntrypoint->entrypoint, env->env,
	//      createdBy->userId, createdAt->createdAt, gpuCount->gpuCount, gpuTypeId->gpuTypeId

	var name, image, category, containerRegistryAuthId, readme, volumeMountPath string
	var containerDiskInGb, volumeInGb, earned, runtimeInMin float64
	var isPublic, isServerless, isRunpod bool

	if v, ok := result["name"].(string); ok {
		name = v
	} else {
		resp.Diagnostics.AddError("API Error", "Field 'name' is missing or not a string in template response")
		return
	}

	if v, ok := result["image"].(string); ok {
		image = v
	} else {
		resp.Diagnostics.AddError("API Error", "Field 'image' is missing or not a string in template response")
		return
	}

	if v, ok := result["category"].(string); ok {
		category = v
	} else {
		resp.Diagnostics.AddError("API Error", "Field 'category' is missing or not a string in template response")
		return
	}

	if v, ok := result["disk"].(float64); ok {
		containerDiskInGb = v
	} else {
		resp.Diagnostics.AddError("API Error", "Field 'disk' is missing or not a float64 in template response")
		return
	}

	if v, ok := result["containerRegistryAuthId"].(string); ok {
		containerRegistryAuthId = v
	} else {
		resp.Diagnostics.AddError("API Error", "Field 'containerRegistryAuthId' is missing or not a string in template response")
		return
	}

	if v, ok := result["public"].(bool); ok {
		isPublic = v
	} else {
		resp.Diagnostics.AddError("API Error", "Field 'public' is missing or not a bool in template response")
		return
	}

	if v, ok := result["serverless"].(bool); ok {
		isServerless = v
	} else {
		resp.Diagnostics.AddError("API Error", "Field 'serverless' is missing or not a bool in template response")
		return
	}

	if v, ok := result["readme"].(string); ok {
		readme = v
	} else {
		resp.Diagnostics.AddError("API Error", "Field 'readme' is missing or not a string in template response")
		return
	}

	if v, ok := result["volumeInGb"].(float64); ok {
		volumeInGb = v
	} else {
		resp.Diagnostics.AddError("API Error", "Field 'volumeInGb' is missing or not a float64 in template response")
		return
	}

	if v, ok := result["mountPath"].(string); ok {
		volumeMountPath = v
	} else {
		resp.Diagnostics.AddError("API Error", "Field 'mountPath' is missing or not a string in template response")
		return
	}

	if v, ok := result["earned"].(float64); ok {
		earned = v
	} else {
		resp.Diagnostics.AddError("API Error", "Field 'earned' is missing or not a float64 in template response")
		return
	}

	if v, ok := result["isRunpod"].(bool); ok {
		isRunpod = v
	} else {
		resp.Diagnostics.AddError("API Error", "Field 'isRunpod' is missing or not a bool in template response")
		return
	}

	if v, ok := result["runtimeInMin"].(float64); ok {
		runtimeInMin = v
	} else {
		resp.Diagnostics.AddError("API Error", "Field 'runtimeInMin' is missing or not a float64 in template response")
		return
	}

	// Parse array fields
	var dockerEntrypoint []attr.Value
	if val, ok := result["entrypoint"].([]interface{}); ok {
		for _, v := range val {
			if vStr, ok := v.(string); ok {
				dockerEntrypoint = append(dockerEntrypoint, types.StringValue(vStr))
			}
		}
	}

	var dockerStartCmd []attr.Value
	if val, ok := result["cmd"].([]interface{}); ok {
		for _, v := range val {
			if vStr, ok := v.(string); ok {
				dockerStartCmd = append(dockerStartCmd, types.StringValue(vStr))
			}
		}
	}

	var envMap map[string]attr.Value
	if val, ok := result["env"].(map[string]interface{}); ok {
		envMap = make(map[string]attr.Value)
		for key, v := range val {
			if strVal, ok := v.(string); ok {
				envMap[key] = types.StringValue(strVal)
			}
		}
	}

	var ports []attr.Value
	if val, ok := result["ports"].([]interface{}); ok {
		for _, v := range val {
			if vStr, ok := v.(string); ok {
				ports = append(ports, types.StringValue(vStr))
			}
		}
	}

	// Build list and map values
	dockerEntrypointList, diags := types.ListValue(types.StringType, dockerEntrypoint)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	dockerStartCmdList, diags := types.ListValue(types.StringType, dockerStartCmd)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var envObj types.Map
	if envMap != nil {
		envObj, diags = types.MapValue(types.StringType, envMap)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	} else {
		envObj, diags = types.MapValue(types.StringType, make(map[string]attr.Value))
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	}

	portsList, diags := types.ListValue(types.StringType, ports)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	model := TemplateModel{
		Id:                      config.Id,
		Name:                    types.StringValue(name),
		ImageName:               types.StringValue(image),
		Category:                types.StringValue(category),
		ContainerDiskInGb:       types.Int64Value(int64(containerDiskInGb)),
		ContainerRegistryAuthId: types.StringValue(containerRegistryAuthId),
		DockerEntrypoint:        dockerEntrypointList,
		DockerStartCmd:          dockerStartCmdList,
		Env:                     envObj,
		IsPublic:                types.BoolValue(isPublic),
		IsServerless:            types.BoolValue(isServerless),
		Ports:                   portsList,
		Readme:                  types.StringValue(readme),
		VolumeInGb:              types.Int64Value(int64(volumeInGb)),
		VolumeMountPath:         types.StringValue(volumeMountPath),
		Earned:                  types.Float64Value(earned),
		IsRunpod:                types.BoolValue(isRunpod),
		RuntimeInMin:            types.Int64Value(int64(runtimeInMin)),
	}
	diags = resp.State.Set(ctx, &model)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}
