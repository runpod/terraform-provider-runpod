package resource_network_volume

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewNetworkVolumeResource() resource.Resource {
	return &NetworkVolumeResource{}
}

type NetworkVolumeResource struct {
	client *client.RunPodClient
}

func (r *NetworkVolumeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		if clientWrapper, ok := req.ProviderData.(*client.RunPodClientWrapper); ok {
			r.client = &client.RunPodClient{
				APIKey:      clientWrapper.APIKey,
				GraphQLEndpoint: "https://api.runpod.io/graphql",
				RestBaseURL: clientWrapper.RestBaseURL,
				Client: &http.Client{Timeout: 60 * time.Second},
			}
		} else if client, ok := req.ProviderData.(*client.RunPodClient); ok {
			r.client = client
		}
	}
}

func (r *NetworkVolumeResource) getClient() *client.RunPodClient {
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
		baseURL = client.GetRestBaseURL()
	}
	r.client = client.NewRunPodClient(apiKey, endpoint, baseURL)
	return r.client
}

func (r *NetworkVolumeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "runpod_network_volume"
}

func (r *NetworkVolumeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = NetworkVolumeResourceSchema(ctx)
}

func (r *NetworkVolumeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config NetworkVolumeModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	client := r.getClient()

	url := client.RestBaseURL + "/v2/network-volumes"

	body := map[string]interface{}{
		"name":       config.Name.ValueString(),
		"size":       int64(config.Size.ValueInt64()),
		"dataCenter": config.DataCenterId.ValueString(),
	}

	if !config.Type.IsNull() && config.Type.ValueString() != "" {
		body["type"] = config.Type.ValueString()
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

	var envelope map[string]interface{}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	var result map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if networkVolume, ok := data["networkVolume"].(map[string]interface{}); ok {
			result = networkVolume
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to extract network volume from v2 response data")
			return
		}
	} else {
		result = envelope
	}

	if respHTTP.StatusCode != 200 && respHTTP.StatusCode != 201 {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to create network volume (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	if id, ok := result["id"].(string); ok {
		config.Id = types.StringValue(id)
	} else {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to get network volume ID from response: %v", result))
		return
	}

	if val, ok := result["name"].(string); ok {
		config.Name = types.StringValue(val)
	} else {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Missing 'name' in network volume response: %v", result))
		return
	}
	if val, ok := result["size"].(float64); ok {
		config.Size = types.Int64Value(int64(val))
	} else {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Missing 'size' in network volume response: %v", result))
		return
	}
	if val, ok := result["dataCenter"].(string); ok {
		config.DataCenterId = types.StringValue(val)
	} else {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Missing 'dataCenter' in network volume response: %v", result))
		return
	}
	if val, ok := result["type"].(string); ok {
		config.Type = types.StringValue(val)
	}

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *NetworkVolumeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NetworkVolumeModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	client := r.getClient()

	url := client.RestBaseURL + "/v2/network-volumes/" + state.Id.ValueString()

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

	var envelope map[string]interface{}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	var result map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if networkVolume, ok := data["networkVolume"].(map[string]interface{}); ok {
			result = networkVolume
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to extract network volume from v2 response data")
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
	if val, ok := result["size"].(float64); ok {
		state.Size = types.Int64Value(int64(val))
	}
	if val, ok := result["dataCenter"].(string); ok {
		state.DataCenterId = types.StringValue(val)
	}
	if val, ok := result["type"].(string); ok {
		state.Type = types.StringValue(val)
	}

	diags = resp.State.Set(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *NetworkVolumeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config NetworkVolumeModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var state NetworkVolumeModel
	diags = req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	client := r.getClient()

	url := client.RestBaseURL + "/v2/network-volumes/" + state.Id.ValueString()

	body := map[string]interface{}{}

	if !config.Name.IsNull() && config.Name.ValueString() != state.Name.ValueString() {
		body["name"] = config.Name.ValueString()
	}

	if !config.Size.IsNull() && config.Size.ValueInt64() != state.Size.ValueInt64() {
		body["size"] = int64(config.Size.ValueInt64())
	}

	if !config.Type.IsNull() && config.Type.ValueString() != state.Type.ValueString() {
		body["type"] = config.Type.ValueString()
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

	var envelope map[string]interface{}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to parse response (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	var result map[string]interface{}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		if networkVolume, ok := data["networkVolume"].(map[string]interface{}); ok {
			result = networkVolume
		} else {
			resp.Diagnostics.AddError("API Error", "Failed to extract network volume from v2 response data")
			return
		}
	} else {
		result = envelope
	}

	if respHTTP.StatusCode != 200 && respHTTP.StatusCode != 201 {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to update network volume (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}

	if val, ok := result["name"].(string); ok {
		config.Name = types.StringValue(val)
	} else {
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Missing 'name' in network volume update response: %v", result))
		return
	}
	if val, ok := result["size"].(float64); ok {
		config.Size = types.Int64Value(int64(val))
	}
	if val, ok := result["dataCenter"].(string); ok {
		config.DataCenterId = types.StringValue(val)
	}
	if val, ok := result["type"].(string); ok {
		config.Type = types.StringValue(val)
	}

	diags = resp.State.Set(ctx, &config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}

func (r *NetworkVolumeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state NetworkVolumeModel
	diags := req.State.Get(ctx, &state)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	client := r.getClient()

	url := client.RestBaseURL + "/v2/network-volumes/" + state.Id.ValueString()

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
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to delete network volume (status: %d): %s", respHTTP.StatusCode, string(respBody)))
		return
	}
}
