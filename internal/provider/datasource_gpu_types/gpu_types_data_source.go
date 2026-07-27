package datasource_gpu_types

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	client "github.com/runpod/terraform-provider-runpod/internal/provider/client"
)

func NewGpuTypesDataSource() datasource.DataSource {
	return &GpuTypesDataSource{}
}

type GpuTypesDataSource struct {
	rlClient *client.RunPodClient
}

func (d *GpuTypesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData != nil {
		d.rlClient = req.ProviderData.(*client.RunPodClient)
	}
}

func (d *GpuTypesDataSource) getClient() *client.RunPodClient {
	if d.rlClient != nil {
		return d.rlClient
	}
	apiKey := os.Getenv("RUNPOD_API_KEY")
	graphqlEndpoint := os.Getenv("RUNPOD_GRAPHQL_URL")
	if graphqlEndpoint == "" {
		graphqlEndpoint = "https://api.runpod.io/graphql"
	}
	restBaseURL := os.Getenv("RUNPOD_BASE_URL")
	if restBaseURL == "" {
		restBaseURL = "https://api.runpod.io/v2"
	}
	d.rlClient = client.NewRunPodClient(apiKey, graphqlEndpoint, restBaseURL)
	return d.rlClient
}

func (d *GpuTypesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "runpod_gpu_types"
}

func (d *GpuTypesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = GpuTypesDataSourceSchema(ctx)
}

func (d *GpuTypesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	rlClient := d.getClient()

	// Use v2 REST endpoint: GET /v2/catalog/gpus
	url := rlClient.RestBaseURL + "/catalog/gpus"

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

	// Handle non-200 responses
	if respHTTP.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respHTTP.Body)
		resp.Diagnostics.AddError("API Error", fmt.Sprintf("Failed to fetch GPU types (status %d): %s", respHTTP.StatusCode, string(body)))
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
		result = data
	} else {
		result = envelope
	}

	// Parse GPU types list
	gpus, ok := result["gpus"].([]interface{})
	if !ok {
		resp.Diagnostics.AddError("API Error", "Failed to parse GPU types from response - 'gpus' field missing or not an array")
		return
	}

	models := make([]GpuTypesModel, len(gpus))
	for i, gpu := range gpus {
		if gpuMap, ok := gpu.(map[string]interface{}); ok {
			var id, name, displayName, manufacturer string
			var memoryInGb, cudaCores float64
			var communityPrice, securePrice float64
			var secureCloud bool

			if v, ok := gpuMap["id"].(string); ok {
				id = v
			} else {
				resp.Diagnostics.AddError("API Error", "Field 'id' is missing or not a string in GPU type response")
				return
			}

			// v2 uses 'name' field
			if v, ok := gpuMap["name"].(string); ok {
				name = v
			}
			
			// For backwards compatibility, use displayName if available
			if v, ok := gpuMap["displayName"].(string); ok {
				displayName = v
			} else if name != "" {
				displayName = name
			} else {
				resp.Diagnostics.AddError("API Error", "Field 'displayName' or 'name' is missing or not a string in GPU type response")
				return
			}

			if v, ok := gpuMap["manufacturer"].(string); ok {
				manufacturer = v
			} else {
				resp.Diagnostics.AddError("API Error", "Field 'manufacturer' is missing or not a string in GPU type response")
				return
			}

			// v2 uses 'memory' instead of 'memory_in_gb'
			if v, ok := gpuMap["memory"].(float64); ok {
				memoryInGb = v
			} else if v, ok := gpuMap["memory_in_gb"].(float64); ok {
				memoryInGb = v
			} else {
				resp.Diagnostics.AddError("API Error", "Field 'memory' or 'memory_in_gb' is missing or not a float64 in GPU type response")
				return
			}

			// v2 uses nested 'price' object
			if price, ok := gpuMap["price"].(map[string]interface{}); ok {
				if v, ok := price["secure"].(float64); ok {
					securePrice = v
				}
				if v, ok := price["community"].(float64); ok {
					communityPrice = v
				}
			} else {
				// v1 uses flat fields
				if v, ok := gpuMap["community_price"].(float64); ok {
					communityPrice = v
				}
				if v, ok := gpuMap["secure_price"].(float64); ok {
					securePrice = v
				}
			}
			// Default to 0 if not provided
			if communityPrice == 0 {
				communityPrice = 0
			}
			if securePrice == 0 {
				securePrice = 0
			}

			// v2 uses 'secure' and 'community' boolean fields
			if v, ok := gpuMap["secure"].(bool); ok {
				secureCloud = v
			} else if v, ok := gpuMap["secure_cloud"].(bool); ok {
				secureCloud = v
			} else {
				resp.Diagnostics.AddError("API Error", "Field 'secure', 'secure_cloud', or 'secure_cloud' is missing or not a bool in GPU type response")
				return
			}

			models[i] = GpuTypesModel{
				Id:             types.StringValue(id),
				DisplayName:    types.StringValue(displayName),
				Manufacturer:   types.StringValue(manufacturer),
				CudaCores:      types.Int64Value(int64(cudaCores)),
				MemoryInGb:     types.Int64Value(int64(memoryInGb)),
				CommunityPrice: types.Float64Value(communityPrice),
				SecurePrice:    types.Float64Value(securePrice),
				SecureCloud:    types.BoolValue(secureCloud),
			}
		}
	}

	parent := GpuTypesDataSourceModel{
		GpuTypes: models,
	}
	diags := resp.State.Set(ctx, &parent)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}
}
